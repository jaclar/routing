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
	"time"

	"sailboat/meteo/internal/grib2"
	"sailboat/meteo/internal/model"
)

const (
	GEFSBaseS3URL = "https://noaa-gefs-pds.s3.amazonaws.com"
)

type gefsIdxRecord struct {
	offset  int64
	content string
}

// GEFSDriver implements ModelDriver for NOAA GEFS 0.50° 31-member global ensemble forecasts.
type GEFSDriver struct {
	httpClient *http.Client
	baseURL    string
	indexCache onceCache[[]gefsIdxRecord]
}

// NewGEFSDriver creates a new NOAA GEFS 0.50° ensemble driver.
func NewGEFSDriver(client *http.Client) *GEFSDriver {
	if client == nil {
		client = DefaultHTTPClient()
	}
	return &GEFSDriver{
		httpClient: client,
		baseURL:    GEFSBaseS3URL,
	}
}

func (g *GEFSDriver) ModelID() string {
	return model.ModelGEFS050
}

// defaultGEFSMembers returns the list of 31 ensemble members: 0 (control gec00) and 1..30 (perturbed gep01..gep30).
func defaultGEFSMembers() []int {
	members := make([]int, 31)
	for i := 0; i <= 30; i++ {
		members[i] = i
	}
	return members
}

func defaultGEFSSteps() []int {
	// Standard ensemble horizon: 3-hourly out to 72h, then 6-hourly out to 240h
	steps := make([]int, 0, 55)
	for h := 0; h <= 72; h += 3 {
		steps = append(steps, h)
	}
	for h := 78; h <= 240; h += 6 {
		steps = append(steps, h)
	}
	return steps
}

// CheckLatestCycle calculates candidate GEFS cycles (00, 06, 12, 18 UTC with ~3.5h lag) and verifies upstream index availability.
func (g *GEFSDriver) CheckLatestCycle(ctx context.Context) (*model.ModelCycle, error) {
	now := time.Now().UTC()
	candidate := now.Add(-3*time.Hour - 30*time.Minute)

	cycleHour := (candidate.Hour() / 6) * 6
	refTime := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), cycleHour, 0, 0, 0, time.UTC)

	steps := defaultGEFSSteps()
	members := defaultGEFSMembers()
	testStep := 48 // Verify a mature forecast step is uploaded
	if testStep > steps[len(steps)-1] {
		testStep = steps[len(steps)-1]
	}

	for attempt := 0; attempt < 4; attempt++ {
		testCycle := refTime.Add(-time.Duration(attempt*6) * time.Hour)
		dateStr := fmt.Sprintf("%04d%02d%02d", testCycle.Year(), testCycle.Month(), testCycle.Day())
		hourStr := fmt.Sprintf("%02d", testCycle.Hour())

		// Test existence of perturbed member 30 index file in both pgrb2a and pgrb2b to ensure NOAA has finished uploading all members
		idxURLA := fmt.Sprintf("%s/gefs.%s/%s/atmos/pgrb2ap5/gep30.t%sz.pgrb2a.0p50.f%03d.idx", g.baseURL, dateStr, hourStr, hourStr, testStep)
		idxURLB := fmt.Sprintf("%s/gefs.%s/%s/atmos/pgrb2bp5/gep30.t%sz.pgrb2b.0p50.f%03d.idx", g.baseURL, dateStr, hourStr, hourStr, testStep)

		reqA, errA := http.NewRequestWithContext(ctx, http.MethodHead, idxURLA, nil)
		reqB, errB := http.NewRequestWithContext(ctx, http.MethodHead, idxURLB, nil)
		if errA == nil && errB == nil {
			respA, errA := g.httpClient.Do(reqA)
			respB, errB := g.httpClient.Do(reqB)
			if errA == nil && errB == nil {
				respA.Body.Close()
				respB.Body.Close()
				if respA.StatusCode == http.StatusOK && respB.StatusCode == http.StatusOK {
					return &model.ModelCycle{
						ModelName:     g.ModelID(),
						ReferenceTime: testCycle,
						ResolutionDeg: 0.50,
						ForecastSteps: steps,
						Members:       members,
						IsEnsemble:    true,
					}, nil
				}
			} else {
				if respA != nil {
					respA.Body.Close()
				}
				if respB != nil {
					respB.Body.Close()
				}
			}
		}
	}

	return &model.ModelCycle{
		ModelName:     g.ModelID(),
		ReferenceTime: refTime,
		ResolutionDeg: 0.50,
		ForecastSteps: steps,
		Members:       members,
		IsEnsemble:    true,
	}, nil
}

// DiscoverSlices produces fetch tasks for all forecast steps, variables, and ensemble members.
func (g *GEFSDriver) DiscoverSlices(cycle *model.ModelCycle, variables []string) ([]model.FetchTask, error) {
	dateStr := fmt.Sprintf("%04d%02d%02d", cycle.ReferenceTime.Year(), cycle.ReferenceTime.Month(), cycle.ReferenceTime.Day())
	hourStr := fmt.Sprintf("%02d", cycle.ReferenceTime.Hour())

	members := cycle.Members
	if len(members) == 0 {
		members = defaultGEFSMembers()
	}

	var tasks []model.FetchTask

	for _, step := range cycle.ForecastSteps {
		stepStr := fmt.Sprintf("%03d", step)

		for _, m := range members {
			var memberPrefix string
			if m == 0 {
				memberPrefix = "gec00"
			} else {
				memberPrefix = fmt.Sprintf("gep%02d", m)
			}

			for _, v := range variables {
				subDir := "pgrb2ap5"
				fileType := "pgrb2a"
				if v == model.VarWindGust10m {
					subDir = "pgrb2bp5"
					fileType = "pgrb2b"
				}

				gribURL := fmt.Sprintf("%s/gefs.%s/%s/atmos/%s/%s.t%sz.%s.0p50.f%s",
					g.baseURL, dateStr, hourStr, subDir, memberPrefix, hourStr, fileType, stepStr)
				idxURL := fmt.Sprintf("%s.idx", gribURL)

				tasks = append(tasks, model.FetchTask{
					ModelName: cycle.ModelName,
					Cycle:     cycle.ReferenceTime,
					StepHours: step,
					Member:    m,
					Variable:  v,
					SourceURL: gribURL,
					ExtraParams: map[string]string{
						"idx_url": idxURL,
					},
				})
			}
		}
	}

	return tasks, nil
}

// IngestSlice resolves the byte range from the index file and downloads the exact slice via HTTP Range request.
func (g *GEFSDriver) IngestSlice(ctx context.Context, task model.FetchTask) (*model.RawGridSlice, error) {
	idxURL := task.ExtraParams["idx_url"]
	if idxURL == "" {
		idxURL = fmt.Sprintf("%s.idx", task.SourceURL)
	}

	nlats := 361
	nlons := 720

	// At step 0, precipitation accumulation is 0.0 mm
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
			LatStep:   0.50,
			LonStart:  0.0,
			LonEnd:    359.50,
			LonStep:   0.50,
			Data:      make([]float32, nlats*nlons),
		}, nil
	}

	// 1. Fetch .idx file to find byte offset range
	pattern := gefsVariableIndexPattern(task.Variable)
	if pattern == "" {
		return nil, fmt.Errorf("unsupported GEFS variable %s", task.Variable)
	}

	startByte, endByte, err := g.lookupByteRange(ctx, idxURL, pattern)
	if err != nil {
		if task.StepHours == 0 {
			data := make([]float32, nlats*nlons)
			return &model.RawGridSlice{
				Variable:  task.Variable,
				ValidTime: task.Cycle,
				StepHours: 0,
				Member:    task.Member,
				NLats:     nlats,
				NLons:     nlons,
				LatStart:  90.0,
				LatEnd:    -90.0,
				LatStep:   0.50,
				LonStart:  0.0,
				LonEnd:    359.50,
				LonStep:   0.50,
				Data:      data,
			}, nil
		}
		return nil, fmt.Errorf("failed to lookup byte range for GEFS %s (%s): %w", task.Variable, pattern, err)
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
			fetchErr = fmt.Errorf("unexpected HTTP status %d for GEFS range request: %s", resp.StatusCode, string(body))
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
		return nil, fmt.Errorf("failed to fetch GEFS GRIB slice after %d retries: %w", maxRangeAttempts, fetchErr)
	}

	// 3. Parse GRIB2 message
	msg, err := grib2.Parse(gribBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GEFS GRIB2 slice for %s: %w", task.Variable, err)
	}

	slice := msg.ToRawGridSlice(task.Variable)
	slice.StepHours = task.StepHours
	slice.Member = task.Member
	return slice, nil
}

// fetchAndParseIndex returns the parsed index for idxURL. Concurrent workers needing the
// same index share one fetch; workers needing different indexes never block each other.
func (g *GEFSDriver) fetchAndParseIndex(ctx context.Context, idxURL string) ([]gefsIdxRecord, error) {
	return g.indexCache.get(idxURL, func() ([]gefsIdxRecord, error) {
		return g.fetchIndex(ctx, idxURL)
	})
}

// fetchIndex downloads and parses a single GRIB index file.
func (g *GEFSDriver) fetchIndex(ctx context.Context, idxURL string) ([]gefsIdxRecord, error) {
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
			fetchErr = fmt.Errorf("HTTP status %d fetching GEFS index %s: %s", resp.StatusCode, idxURL, string(body))
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
		return nil, fmt.Errorf("failed to fetch GEFS index after %d attempts: %w", maxIdxAttempts, fetchErr)
	}

	var records []gefsIdxRecord
	scanner := bufio.NewScanner(bytes.NewReader(rawBytes))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) >= 3 {
			offset, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil {
				records = append(records, gefsIdxRecord{
					offset:  offset,
					content: line,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return records, nil
}

func (g *GEFSDriver) lookupByteRange(ctx context.Context, idxURL, targetPattern string) (int64, int64, error) {
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

	return 0, 0, fmt.Errorf("pattern %q not found in GEFS index %s", targetPattern, idxURL)
}

func gefsVariableIndexPattern(variable string) string {
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
