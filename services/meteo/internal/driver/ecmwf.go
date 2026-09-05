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
	"time"

	"sailboat/meteo/internal/grib2"
	"sailboat/meteo/internal/model"
)

const (
	ECMWFBaseS3URL = "https://data.ecmwf.int/forecasts"
)

type ecmwfByteRange struct {
	Offset int64
	Length int64
}

// ECMWFDriver implements ModelDriver for ECMWF Open Data (IFS 0.25° and AIFS 0.25°) via S3 index byte ranges.
type ECMWFDriver struct {
	httpClient *http.Client
	modelID    string
	baseURL    string
	indexCache onceCache[map[string]ecmwfByteRange]
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

	// ECMWF Open Data uses an antimeridian-centered longitudinal grid:
	// Lo1 = 180.0° / -180.0° to Lo2 = 179.75°.
	// Standardize to Greenwich-centered [0.0°, 359.75°] (consistent with GFS/ICON and Zarr store layout).
	if msg.Lo1 == 180.0 || msg.Lo1 == -180.0 {
		slice.Data = normalizeECMWFGrid(msg.Values, msg.Nj, msg.Ni)
		slice.LonStart = 0.0
		slice.LonEnd = 359.75
		slice.LonStep = 0.25
	}

	return slice, nil
}

func normalizeECMWFGrid(data []float32, nlats, nlons int) []float32 {
	if nlons <= 0 || nlats <= 0 || len(data) != nlats*nlons {
		return data
	}
	normalized := make([]float32, len(data))
	halfLon := nlons / 2 // 720
	for r := 0; r < nlats; r++ {
		rowOffset := r * nlons
		// First half of output (0..179.75°) comes from second half of ECMWF input (cols 720..1439)
		copy(normalized[rowOffset:rowOffset+halfLon], data[rowOffset+halfLon:rowOffset+nlons])
		// Second half of output (180..359.75°) comes from first half of ECMWF input (cols 0..719)
		copy(normalized[rowOffset+halfLon:rowOffset+nlons], data[rowOffset:rowOffset+halfLon])
	}
	return normalized
}

// fetchAndParseIndex downloads and parses an ECMWF JSON-Lines .index file once, caching the results in memory.
// fetchAndParseIndex returns the parsed index for idxURL. Concurrent workers needing the
// same index share one fetch; workers needing different indexes never block each other.
func (e *ECMWFDriver) fetchAndParseIndex(ctx context.Context, idxURL string) (map[string]ecmwfByteRange, error) {
	return e.indexCache.get(idxURL, func() (map[string]ecmwfByteRange, error) {
		return e.fetchIndex(ctx, idxURL)
	})
}

// fetchIndex downloads and parses a single GRIB index file.
func (e *ECMWFDriver) fetchIndex(ctx context.Context, idxURL string) (map[string]ecmwfByteRange, error) {
	var rawBytes []byte
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
				return nil, ctx.Err()
			case <-time.After(sleepDuration):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, idxURL, nil)
		if err != nil {
			return nil, err
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
			return nil, fetchErr
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
		return nil, fmt.Errorf("failed to fetch ECMWF index after %d attempts: %w", maxIdxAttempts, fetchErr)
	}

	type ecmwfIdxRecord struct {
		Param   string `json:"param"`
		LevType string `json:"levtype"`
		Offset  int64  `json:"_offset"`
		Length  int64  `json:"_length"`
	}

	res := make(map[string]ecmwfByteRange)
	scanner := bufio.NewScanner(bytes.NewReader(rawBytes))
	for scanner.Scan() {
		line := scanner.Bytes()
		var rec ecmwfIdxRecord
		if err := json.Unmarshal(line, &rec); err == nil {
			res[strings.ToLower(rec.Param)] = ecmwfByteRange{
				Offset: rec.Offset,
				Length: rec.Length,
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

// lookupECMWFIndex scans the JSON-Lines .index file from ECMWF S3 with thread-safe caching and exponential retry backoff.
func (e *ECMWFDriver) lookupECMWFIndex(ctx context.Context, idxURL, targetParam string) (int64, int64, error) {
	ranges, err := e.fetchAndParseIndex(ctx, idxURL)
	if err != nil {
		return 0, 0, err
	}

	lower := strings.ToLower(targetParam)
	if r, ok := ranges[lower]; ok {
		return r.Offset, r.Length, nil
	}
	if lower == "10fg" {
		if r, ok := ranges["10fg3"]; ok {
			return r.Offset, r.Length, nil
		}
		if r, ok := ranges["10fg6"]; ok {
			return r.Offset, r.Length, nil
		}
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
