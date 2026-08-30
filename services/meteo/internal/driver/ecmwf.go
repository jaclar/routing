package driver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"sailboat/meteo/internal/grib2"
	"sailboat/meteo/internal/model"
)

const (
	ECMWFBaseS3URL = "https://ecmwf-forecasts.s3.amazonaws.com"
)

// ECMWFDriver implements ModelDriver for ECMWF Open Data (IFS 0.25° and AIFS 0.25°) via S3 index byte ranges.
type ECMWFDriver struct {
	httpClient *http.Client
	modelID    string
	baseURL    string
	idxCache   map[string][]byte
	idxMu      sync.RWMutex
}

// NewECMWFDriver creates an ECMWF driver for IFS or AIFS.
func NewECMWFDriver(modelID string, client *http.Client) *ECMWFDriver {
	if client == nil {
		client = DefaultHTTPClient()
	}
	if modelID == "" {
		modelID = model.ModelIFS025
	}
	return &ECMWFDriver{
		httpClient: client,
		modelID:    modelID,
		baseURL:    ECMWFBaseS3URL,
		idxCache:   make(map[string][]byte),
	}
}

func (e *ECMWFDriver) ModelID() string {
	return e.modelID
}

// CheckLatestCycle checks S3 availability to find the most recent completed ECMWF cycle (00z, 12z with ~7h publication lag).
func (e *ECMWFDriver) CheckLatestCycle(ctx context.Context) (*model.ModelCycle, error) {
	now := time.Now().UTC()
	candidate := now.Add(-7 * time.Hour)

	cycleHour := (candidate.Hour() / 12) * 12
	refTime := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), cycleHour, 0, 0, 0, time.UTC)

	steps := defaultECMWFSteps()
	modelPath := "ifs/0p25/oper"
	if e.modelID == model.ModelAIFS025 {
		modelPath = "aifs/0p25/oper"
	}

	// Probe recent cycles to ensure full S3 data is available
	lastStep := steps[len(steps)-1]
	for attempt := 0; attempt < 4; attempt++ {
		testCycle := refTime.Add(-time.Duration(attempt*12) * time.Hour)
		dateStr := fmt.Sprintf("%04d%02d%02d", testCycle.Year(), testCycle.Month(), testCycle.Day())
		hourStr := fmt.Sprintf("%02d", testCycle.Hour())

		testURL := fmt.Sprintf("%s/%s/%sz/%s/%s%s0000-%dh-oper-fc.index", e.baseURL, dateStr, hourStr, modelPath, dateStr, hourStr, lastStep)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, testURL, nil)
		if err == nil {
			resp, err := e.httpClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return &model.ModelCycle{
						ModelName:     e.modelID,
						ReferenceTime: testCycle,
						ResolutionDeg: 0.25,
						ForecastSteps: steps,
					}, nil
				}
			}
		}
	}

	return &model.ModelCycle{
		ModelName:     e.modelID,
		ReferenceTime: refTime,
		ResolutionDeg: 0.25,
		ForecastSteps: steps,
	}, nil
}

func defaultECMWFSteps() []int {
	steps := make([]int, 0, 81)
	for h := 0; h <= 144; h += 3 {
		steps = append(steps, h)
	}
	for h := 150; h <= 240; h += 6 {
		steps = append(steps, h)
	}
	return steps
}

// DiscoverSlices produces fetch tasks for ECMWF variables using S3 index files.
func (e *ECMWFDriver) DiscoverSlices(cycle *model.ModelCycle, variables []string) ([]model.FetchTask, error) {
	dateStr := fmt.Sprintf("%04d%02d%02d", cycle.ReferenceTime.Year(), cycle.ReferenceTime.Month(), cycle.ReferenceTime.Day())
	hourStr := fmt.Sprintf("%02d", cycle.ReferenceTime.Hour())

	modelPath := "ifs/0p25/oper"
	if e.modelID == model.ModelAIFS025 {
		modelPath = "aifs/0p25/oper"
	}

	var tasks []model.FetchTask

	for _, step := range cycle.ForecastSteps {
		gribURL := fmt.Sprintf("%s/%s/%sz/%s/%s%s0000-%dh-oper-fc.grib2", e.baseURL, dateStr, hourStr, modelPath, dateStr, hourStr, step)
		idxURL := fmt.Sprintf("%s/%s/%sz/%s/%s%s0000-%dh-oper-fc.index", e.baseURL, dateStr, hourStr, modelPath, dateStr, hourStr, step)

		for _, v := range variables {
			paramName := ecmwfParamName(v)
			if paramName == "" {
				continue
			}

			tasks = append(tasks, model.FetchTask{
				ModelName: cycle.ModelName,
				Cycle:     cycle.ReferenceTime,
				StepHours: step,
				Variable:  v,
				SourceURL: gribURL,
				ExtraParams: map[string]string{
					"param":   paramName,
					"idx_url": idxURL,
				},
			})
		}
	}

	return tasks, nil
}

// IngestSlice resolves the byte range from the ECMWF JSON-lines index file and downloads only the required variable slice.
func (e *ECMWFDriver) IngestSlice(ctx context.Context, task model.FetchTask) (*model.RawGridSlice, error) {
	if task.Variable == model.VarPrecipAccum && task.StepHours == 0 {
		nlats := 721
		nlons := 1440
		return &model.RawGridSlice{
			Variable:  task.Variable,
			ValidTime: task.Cycle,
			StepHours: 0,
			NLats:     nlats,
			NLons:     nlons,
			LatStart:  90.0,
			LatEnd:    -90.0,
			LatStep:   0.25,
			LonStart:  0.0,
			LonEnd:    359.75,
			LonStep:   0.25,
			Data:      make([]float32, nlats*nlons),
		}, nil
	}

	idxURL := task.ExtraParams["idx_url"]
	param := task.ExtraParams["param"]

	// 1. Fetch .index file to find exact byte offset and length
	offset, length, err := e.lookupECMWFIndex(ctx, idxURL, param)
	if err != nil {
		return nil, fmt.Errorf("ECMWF index lookup failed for %s (param %s): %w", task.Variable, param, err)
	}

	// 2. Fetch exact byte range with exponential retry backoff on 503/429/network errors
	var gribBytes []byte
	var fetchErr error
	const maxS3Attempts = 8
	for attempt := 0; attempt < maxS3Attempts; attempt++ {
		if attempt > 0 {
			backoffMs := 500 * (1 << (attempt - 1))
			if backoffMs > 30000 {
				backoffMs = 30000
			}
			jitterMs := int(time.Now().UnixNano() % 500)
			sleepDuration := time.Duration(backoffMs+jitterMs) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleepDuration):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, task.SourceURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))

		resp, err := e.httpClient.Do(req)
		if err != nil {
			fetchErr = err
			continue
		}

		if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			fetchErr = fmt.Errorf("ECMWF upstream status %d for byte range %d-%d: %s", resp.StatusCode, offset, offset+length-1, string(body))
			if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				continue
			}
			return nil, fetchErr
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fetchErr = err
			continue
		}

		gribBytes = data
		fetchErr = nil
		break
	}

	if fetchErr != nil {
		return nil, fmt.Errorf("failed to fetch ECMWF byte range after %d retries: %w", maxS3Attempts, fetchErr)
	}

	// 3. Parse GRIB2 message
	msg, err := grib2.Parse(gribBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ECMWF GRIB2: %w", err)
	}

	slice := msg.ToRawGridSlice(task.Variable)
	slice.StepHours = task.StepHours
	return slice, nil
}

// lookupECMWFIndex scans the JSON-Lines .index file from ECMWF S3 with caching and exponential retry backoff.
func (e *ECMWFDriver) lookupECMWFIndex(ctx context.Context, idxURL, targetParam string) (int64, int64, error) {
	e.idxMu.RLock()
	cachedData, exists := e.idxCache[idxURL]
	e.idxMu.RUnlock()

	var rawBytes []byte
	if exists {
		rawBytes = cachedData
	} else {
		// Fetch with retries and exponential backoff
		var fetchErr error
		const maxIdxAttempts = 8
		for attempt := 0; attempt < maxIdxAttempts; attempt++ {
			if attempt > 0 {
				backoffMs := 500 * (1 << (attempt - 1))
				if backoffMs > 30000 {
					backoffMs = 30000
				}
				jitterMs := int(time.Now().UnixNano() % 500)
				sleepDuration := time.Duration(backoffMs+jitterMs) * time.Millisecond
				select {
				case <-ctx.Done():
					return 0, 0, ctx.Err()
				case <-time.After(sleepDuration):
				}
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, idxURL, nil)
			if err != nil {
				return 0, 0, err
			}

			resp, err := e.httpClient.Do(req)
			if err != nil {
				fetchErr = err
				continue
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				fetchErr = fmt.Errorf("HTTP status %d fetching ECMWF index %s: %s", resp.StatusCode, idxURL, string(body))
				if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
					continue
				}
				return 0, 0, fetchErr
			}

			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				fetchErr = err
				continue
			}

			rawBytes = data
			fetchErr = nil
			break
		}

		if fetchErr != nil {
			return 0, 0, fmt.Errorf("failed to fetch ECMWF index after %d attempts: %w", maxIdxAttempts, fetchErr)
		}

		e.idxMu.Lock()
		e.idxCache[idxURL] = rawBytes
		e.idxMu.Unlock()
	}

	type ecmwfIdxRecord struct {
		Param   string `json:"param"`
		LevType string `json:"levtype"`
		Offset  int64  `json:"_offset"`
		Length  int64  `json:"_length"`
	}

	scanner := bufio.NewScanner(bytes.NewReader(rawBytes))
	for scanner.Scan() {
		line := scanner.Bytes()
		var rec ecmwfIdxRecord
		if err := json.Unmarshal(line, &rec); err == nil {
			if strings.EqualFold(rec.Param, targetParam) ||
				(targetParam == "10fg" && (rec.Param == "10fg3" || rec.Param == "10fg6")) {
				return rec.Offset, rec.Length, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}

	return 0, 0, fmt.Errorf("param %q not found in ECMWF index %s", targetParam, idxURL)
}

func ecmwfParamName(canonicalVar string) string {
	switch canonicalVar {
	case model.VarWindU10m:
		return "10u"
	case model.VarWindV10m:
		return "10v"
	case model.VarWindGust10m:
		return "10fg" // 10m wind gust
	case model.VarMSLP:
		return "msl"
	case model.VarTemp2m:
		return "2t"
	case model.VarPrecipAccum:
		return "tp"
	case model.VarWaveHeightSig:
		return "swh"
	default:
		return ""
	}
}
