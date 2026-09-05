package driver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sailboat/meteo/internal/grib2"
	"sailboat/meteo/internal/model"
)

// ECMWFENSDriver implements ModelDriver for ECMWF Open Data IFS 0.25° 50-member ensemble forecasts.
type ECMWFENSDriver struct {
	httpClient *http.Client
	baseURL    string
	indexCache onceCache[map[string]ecmwfByteRange]
}

// NewECMWFENSDriver creates a new ECMWF IFS-ENS 0.25° driver.
func NewECMWFENSDriver(client *http.Client) *ECMWFENSDriver {
	if client == nil {
		client = DefaultHTTPClient()
	}
	return &ECMWFENSDriver{
		httpClient: client,
		baseURL:    ECMWFBaseS3URL,
	}
}

func (e *ECMWFENSDriver) ModelID() string {
	return model.ModelIFSEns025
}

func defaultECMWFENSMembers() []int {
	members := make([]int, 50)
	for i := 1; i <= 50; i++ {
		members[i-1] = i
	}
	return members
}

func defaultECMWFENSSteps() []int {
	steps := make([]int, 0, 60)
	for h := 0; h <= 144; h += 3 {
		steps = append(steps, h)
	}
	for h := 150; h <= 240; h += 6 {
		steps = append(steps, h)
	}
	return steps
}

// CheckLatestCycle checks S3 availability to find the most recent completed ECMWF ensemble cycle (00z, 12z with ~7h publication lag).
func (e *ECMWFENSDriver) CheckLatestCycle(ctx context.Context) (*model.ModelCycle, error) {
	now := time.Now().UTC()
	candidate := now.Add(-7 * time.Hour)

	cycleHour := (candidate.Hour() / 12) * 12
	refTime := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), cycleHour, 0, 0, 0, time.UTC)

	steps := defaultECMWFENSSteps()
	members := defaultECMWFENSMembers()
	testStep := 48
	if testStep > steps[len(steps)-1] {
		testStep = steps[len(steps)-1]
	}

	// Probe recent cycles to ensure full ensemble data is available
	for attempt := 0; attempt < 4; attempt++ {
		testCycle := refTime.Add(-time.Duration(attempt*12) * time.Hour)
		dateStr := fmt.Sprintf("%04d%02d%02d", testCycle.Year(), testCycle.Month(), testCycle.Day())
		hourStr := fmt.Sprintf("%02d", testCycle.Hour())

		testURL := fmt.Sprintf("%s/%s/%sz/ifs/0p25/enfo/%s%s0000-%dh-enfo-ef.index",
			e.baseURL, dateStr, hourStr, dateStr, hourStr, testStep)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, testURL, nil)
		if err == nil {
			resp, err := e.httpClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return &model.ModelCycle{
						ModelName:     e.ModelID(),
						ReferenceTime: testCycle,
						ResolutionDeg: 0.25,
						ForecastSteps: steps,
						Members:       members,
						IsEnsemble:    true,
					}, nil
				}
			}
		}
	}

	return &model.ModelCycle{
		ModelName:     e.ModelID(),
		ReferenceTime: refTime,
		ResolutionDeg: 0.25,
		ForecastSteps: steps,
		Members:       members,
		IsEnsemble:    true,
	}, nil
}

// DiscoverSlices produces fetch tasks for ECMWF ensemble variables and members.
func (e *ECMWFENSDriver) DiscoverSlices(cycle *model.ModelCycle, variables []string) ([]model.FetchTask, error) {
	dateStr := fmt.Sprintf("%04d%02d%02d", cycle.ReferenceTime.Year(), cycle.ReferenceTime.Month(), cycle.ReferenceTime.Day())
	hourStr := fmt.Sprintf("%02d", cycle.ReferenceTime.Hour())

	members := cycle.Members
	if len(members) == 0 {
		members = defaultECMWFENSMembers()
	}

	var tasks []model.FetchTask

	for _, step := range cycle.ForecastSteps {
		gribURL := fmt.Sprintf("%s/%s/%sz/ifs/0p25/enfo/%s%s0000-%dh-enfo-ef.grib2",
			e.baseURL, dateStr, hourStr, dateStr, hourStr, step)
		idxURL := fmt.Sprintf("%s/%s/%sz/ifs/0p25/enfo/%s%s0000-%dh-enfo-ef.index",
			e.baseURL, dateStr, hourStr, dateStr, hourStr, step)

		for _, v := range variables {
			paramName := ecmwfParamName(v)
			if paramName == "" {
				continue
			}

			for _, m := range members {
				tasks = append(tasks, model.FetchTask{
					ModelName: cycle.ModelName,
					Cycle:     cycle.ReferenceTime,
					StepHours: step,
					Member:    m,
					Variable:  v,
					SourceURL: gribURL,
					ExtraParams: map[string]string{
						"param":   paramName,
						"member":  strconv.Itoa(m),
						"idx_url": idxURL,
					},
				})
			}
		}
	}

	return tasks, nil
}

// IngestSlice resolves the byte range from the ECMWF JSON-lines index file for a specific member and downloads it.
func (e *ECMWFENSDriver) IngestSlice(ctx context.Context, task model.FetchTask) (*model.RawGridSlice, error) {
	nlats := 721
	nlons := 1440

	if task.Variable == model.VarPrecipAccum && task.StepHours == 0 {
		return &model.RawGridSlice{
			Variable:  task.Variable,
			ValidTime: task.Cycle,
			StepHours: 0,
			Member:    task.Member,
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
	memberStr := task.ExtraParams["member"]
	if memberStr == "" {
		memberStr = strconv.Itoa(task.Member)
	}

	// 1. Fetch .index file to find exact byte offset and length for this member
	offset, length, err := e.lookupECMWFMemberIndex(ctx, idxURL, param, memberStr)
	if err != nil {
		return nil, fmt.Errorf("ECMWF-ENS index lookup failed for %s member %s (param %s): %w", task.Variable, memberStr, param, err)
	}

	// 2. Fetch exact byte range with exponential retry backoff
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
			fetchErr = fmt.Errorf("ECMWF-ENS upstream status %d for byte range %d-%d: %s", resp.StatusCode, offset, offset+length-1, string(body))
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
		return nil, fmt.Errorf("failed to fetch ECMWF-ENS byte range after %d retries: %w", maxS3Attempts, fetchErr)
	}

	// 3. Parse GRIB2 message
	msg, err := grib2.Parse(gribBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ECMWF-ENS GRIB2: %w", err)
	}

	slice := msg.ToRawGridSlice(task.Variable)
	slice.StepHours = task.StepHours
	slice.Member = task.Member

	// Normalize antimeridian coordinates to Greenwich-centered [0, 359.75]
	if msg.Lo1 == 180.0 || msg.Lo1 == -180.0 {
		slice.Data = normalizeECMWFGrid(msg.Values, msg.Nj, msg.Ni)
		slice.LonStart = 0.0
		slice.LonEnd = 359.75
		slice.LonStep = 0.25
	}

	return slice, nil
}

// fetchAndParseIndex returns the parsed index for idxURL. Concurrent workers needing the
// same index share one fetch; workers needing different indexes never block each other.
func (e *ECMWFENSDriver) fetchAndParseIndex(ctx context.Context, idxURL string) (map[string]ecmwfByteRange, error) {
	return e.indexCache.get(idxURL, func() (map[string]ecmwfByteRange, error) {
		return e.fetchIndex(ctx, idxURL)
	})
}

// fetchIndex downloads and parses a single GRIB index file.
func (e *ECMWFENSDriver) fetchIndex(ctx context.Context, idxURL string) (map[string]ecmwfByteRange, error) {
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
		Number  string `json:"number"`
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
			key := fmt.Sprintf("%s:%s", strings.ToLower(rec.Param), rec.Number)
			res[key] = ecmwfByteRange{
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

func (e *ECMWFENSDriver) lookupECMWFMemberIndex(ctx context.Context, idxURL, targetParam, member string) (int64, int64, error) {
	ranges, err := e.fetchAndParseIndex(ctx, idxURL)
	if err != nil {
		return 0, 0, err
	}

	lower := strings.ToLower(targetParam)
	key := fmt.Sprintf("%s:%s", lower, member)
	if r, ok := ranges[key]; ok {
		return r.Offset, r.Length, nil
	}

	if lower == "10fg" {
		if r, ok := ranges[fmt.Sprintf("10fg3:%s", member)]; ok {
			return r.Offset, r.Length, nil
		}
		if r, ok := ranges[fmt.Sprintf("10fg6:%s", member)]; ok {
			return r.Offset, r.Length, nil
		}
	}

	return 0, 0, fmt.Errorf("param %q member %s not found in ECMWF-ENS index %s", targetParam, member, idxURL)
}
