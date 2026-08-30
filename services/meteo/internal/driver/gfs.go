package driver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"sailboat/meteo/internal/grib2"
	"sailboat/meteo/internal/model"
)

const (
	GFSBaseS3URL = "https://noaa-gfs-bdp-pds.s3.amazonaws.com"
)

type gfsIdxRecord struct {
	offset  int64
	content string
}

// GFSDriver implements ModelDriver for NOAA GFS 0.25° global forecasts.
type GFSDriver struct {
	httpClient       *http.Client
	baseURL          string
	parsedIndexCache map[string][]gfsIdxRecord
	idxMu            sync.Mutex
}

// NewGFSDriver creates a new NOAA GFS 0.25° driver.
func NewGFSDriver(client *http.Client) *GFSDriver {
	if client == nil {
		client = DefaultHTTPClient()
	}
	return &GFSDriver{
		httpClient:       client,
		baseURL:          GFSBaseS3URL,
		parsedIndexCache: make(map[string][]gfsIdxRecord),
	}
}

func (g *GFSDriver) ModelID() string {
	return model.ModelGFS025
}

// CheckLatestCycle calculates the most recent candidate GFS cycle (lag ~3.5 hours) and verifies upstream index presence.
func (g *GFSDriver) CheckLatestCycle(ctx context.Context) (*model.ModelCycle, error) {
	// GFS publishes at 00, 06, 12, 18 UTC.
	// Standard operational lag is ~3.5 to 4 hours.
	now := time.Now().UTC()
	candidate := now.Add(-3*time.Hour - 30*time.Minute)

	// Round down to nearest 6h cycle
	cycleHour := (candidate.Hour() / 6) * 6
	refTime := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), cycleHour, 0, 0, 0, time.UTC)

	// Try verifying latest cycle, or step back 6h if still in progress
	steps := defaultGFSSteps()
	lastStep := steps[len(steps)-1]

	for attempt := 0; attempt < 4; attempt++ {
		testCycle := refTime.Add(-time.Duration(attempt*6) * time.Hour)
		dateStr := fmt.Sprintf("%04d%02d%02d", testCycle.Year(), testCycle.Month(), testCycle.Day())
		hourStr := fmt.Sprintf("%02d", testCycle.Hour())

		// Test existence of the final forecast step index file to ensure NOAA has finished uploading the full run
		idxURL := fmt.Sprintf("%s/gfs.%s/%s/atmos/gfs.t%sz.pgrb2.0p25.f%03d.idx", g.baseURL, dateStr, hourStr, hourStr, lastStep)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, idxURL, nil)
		if err == nil {
			resp, err := g.httpClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					// Found fully uploaded cycle!
					return &model.ModelCycle{
						ModelName:     g.ModelID(),
						ReferenceTime: testCycle,
						ResolutionDeg: 0.25,
						ForecastSteps: steps,
					}, nil
				}
			}
		}
	}

	// Fallback to computed candidate if offline
	return &model.ModelCycle{
		ModelName:     g.ModelID(),
		ReferenceTime: refTime,
		ResolutionDeg: 0.25,
		ForecastSteps: steps,
	}, nil
}

func defaultGFSSteps() []int {
	// Standard 10-day ocean routing horizon:
	// Hourly out to 72h, then 3-hourly out to 240h
	steps := make([]int, 0, 80)
	for h := 0; h <= 72; h += 3 {
		steps = append(steps, h)
	}
	for h := 75; h <= 240; h += 3 {
		steps = append(steps, h)
	}
	return steps
}

// DiscoverSlices fetches the S3 `.idx` file for each step and extracts byte ranges for target canonical variables.
func (g *GFSDriver) DiscoverSlices(cycle *model.ModelCycle, variables []string) ([]model.FetchTask, error) {
	dateStr := fmt.Sprintf("%04d%02d%02d", cycle.ReferenceTime.Year(), cycle.ReferenceTime.Month(), cycle.ReferenceTime.Day())
	hourStr := fmt.Sprintf("%02d", cycle.ReferenceTime.Hour())

	var tasks []model.FetchTask

	for _, step := range cycle.ForecastSteps {
		stepStr := fmt.Sprintf("%03d", step)
		gribURL := fmt.Sprintf("%s/gfs.%s/%s/atmos/gfs.t%sz.pgrb2.0p25.f%s", g.baseURL, dateStr, hourStr, hourStr, stepStr)
		idxURL := fmt.Sprintf("%s.idx", gribURL)

		// Create a task descriptor for each variable
		for _, v := range variables {
			tasks = append(tasks, model.FetchTask{
				ModelName: cycle.ModelName,
				Cycle:     cycle.ReferenceTime,
				StepHours: step,
				Variable:  v,
				SourceURL: gribURL,
				ExtraParams: map[string]string{
					"idx_url": idxURL,
				},
			})
		}
	}

	return tasks, nil
}

// IngestSlice resolves the byte range from the index file and downloads the exact slice via HTTP Range request.
func (g *GFSDriver) IngestSlice(ctx context.Context, task model.FetchTask) (*model.RawGridSlice, error) {
	idxURL := task.ExtraParams["idx_url"]
	if idxURL == "" {
		idxURL = fmt.Sprintf("%s.idx", task.SourceURL)
	}

	// At step 0 (initial analysis t=0), precipitation accumulation is identically 0.0 mm
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
			Data:      make([]float32, nlats*nlons), // All 0.0 mm
		}, nil
	}

	// 1. Fetch .idx file to find byte offset range
	pattern := gfsVariableIndexPattern(task.Variable)
	if pattern == "" {
		return nil, fmt.Errorf("unsupported GFS variable %s", task.Variable)
	}

	startByte, endByte, err := g.lookupByteRange(ctx, idxURL, pattern)
	if err != nil {
		// If gust or optional field is missing at step 0, return NaNs instead of failing the entire cycle
		if task.StepHours == 0 {
			nlats := 721
			nlons := 1440
			data := make([]float32, nlats*nlons)
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
				Data:      data,
			}, nil
		}
		return nil, fmt.Errorf("failed to lookup byte range for %s (%s): %w", task.Variable, pattern, err)
	}

	// 2. Fetch byte range from GRIB2 file with exponential retry backoff
	var gribBytes []byte
	var fetchErr error
	const maxRangeAttempts = 8
	for attempt := 0; attempt < maxRangeAttempts; attempt++ {
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

		if endByte > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", startByte, endByte))
		} else {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
		}

		resp, err := g.httpClient.Do(req)
		if err != nil {
			fetchErr = err
			continue
		}

		if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			fetchErr = fmt.Errorf("unexpected HTTP status %d for range request: %s", resp.StatusCode, string(body))
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
		return nil, fmt.Errorf("failed to fetch GRIB slice range after %d retries: %w", maxRangeAttempts, fetchErr)
	}

	// 3. Parse GRIB2 message
	msg, err := grib2.Parse(gribBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GRIB2 slice for %s: %w", task.Variable, err)
	}

	slice := msg.ToRawGridSlice(task.Variable)
	slice.StepHours = task.StepHours
	return slice, nil
}

// fetchAndParseIndex downloads and parses a NOAA GFS .idx file once, caching the results in memory.
func (g *GFSDriver) fetchAndParseIndex(ctx context.Context, idxURL string) ([]gfsIdxRecord, error) {
	g.idxMu.Lock()
	defer g.idxMu.Unlock()

	if g.parsedIndexCache == nil {
		g.parsedIndexCache = make(map[string][]gfsIdxRecord)
	}
	if records, exists := g.parsedIndexCache[idxURL]; exists {
		return records, nil
	}

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

		resp, err := g.httpClient.Do(req)
		if err != nil {
			fetchErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			fetchErr = fmt.Errorf("HTTP status %d fetching GFS index %s: %s", resp.StatusCode, idxURL, string(body))
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
		return nil, fmt.Errorf("failed to fetch GFS index after %d attempts: %w", maxIdxAttempts, fetchErr)
	}

	var records []gfsIdxRecord
	scanner := bufio.NewScanner(bytes.NewReader(rawBytes))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 3 {
			offset, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil {
				records = append(records, gfsIdxRecord{
					offset:  offset,
					content: line,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	g.parsedIndexCache[idxURL] = records
	return records, nil
}

// lookupByteRange reads the .idx file and finds the start and end byte offsets for the target pattern.
func (g *GFSDriver) lookupByteRange(ctx context.Context, idxURL, targetPattern string) (int64, int64, error) {
	records, err := g.fetchAndParseIndex(ctx, idxURL)
	if err != nil {
		return 0, 0, err
	}

	for i, rec := range records {
		if strings.Contains(rec.content, targetPattern) {
			start := rec.offset
			var end int64 = 0
			if i+1 < len(records) {
				end = records[i+1].offset - 1
			}
			return start, end, nil
		}
	}

	return 0, 0, fmt.Errorf("pattern %q not found in index %s", targetPattern, idxURL)
}

// ParseIndexByteRange parses index records from an io.Reader and returns byte start/end offsets.
func ParseIndexByteRange(r io.Reader, targetPattern string) (int64, int64, error) {
	type idxRecord struct {
		offset  int64
		content string
	}

	var records []idxRecord
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 3 {
			offset, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil {
				records = append(records, idxRecord{
					offset:  offset,
					content: line,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}

	for i, rec := range records {
		if strings.Contains(rec.content, targetPattern) {
			start := rec.offset
			var end int64 = 0
			if i+1 < len(records) {
				end = records[i+1].offset - 1
			}
			return start, end, nil
		}
	}

	return 0, 0, fmt.Errorf("pattern %q not found in index", targetPattern)
}

// gfsVariableIndexPattern maps canonical variables to GFS .idx search substrings.
func gfsVariableIndexPattern(variable string) string {
	switch variable {
	case model.VarWindU10m:
		return ":UGRD:10 m above ground:"
	case model.VarWindV10m:
		return ":VGRD:10 m above ground:"
	case model.VarWindGust10m:
		return ":GUST:surface:"
	case model.VarMSLP:
		return ":PRMSL:mean sea level:"
	case model.VarTemp2m:
		return ":TMP:2 m above ground:"
	case model.VarPrecipAccum:
		return ":APCP:surface:"
	case model.VarWaveHeightSig:
		return ":HTSGW:surface:"
	default:
		return ""
	}
}
