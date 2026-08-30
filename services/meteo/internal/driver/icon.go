package driver

import (
	"compress/bzip2"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"sailboat/meteo/internal/grib2"
	"sailboat/meteo/internal/model"
)

const (
	DWDBaseURL = "https://opendata.dwd.de/weather/nwp/icon/grib"
)

// ICONDriver implements ModelDriver for DWD ICON Global forecasts.
type ICONDriver struct {
	httpClient *http.Client
	baseURL    string
}

// NewICONDriver creates a DWD ICON Global driver.
func NewICONDriver(client *http.Client) *ICONDriver {
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	return &ICONDriver{
		httpClient: client,
		baseURL:    DWDBaseURL,
	}
}

func (i *ICONDriver) ModelID() string {
	return model.ModelICON025
}

// CheckLatestCycle probes DWD OpenData to discover the latest fully uploaded cycle run (00z, 06z, 12z, 18z with ~3.5h lag).
func (i *ICONDriver) CheckLatestCycle(ctx context.Context) (*model.ModelCycle, error) {
	now := time.Now().UTC()
	candidate := now.Add(-3 * time.Hour)

	cycleHour := (candidate.Hour() / 6) * 6
	refTime := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), cycleHour, 0, 0, 0, time.UTC)

	steps := defaultICONSteps()

	// Probe upstream HTTP endpoint to verify the run exists
	for attempt := 0; attempt < 4; attempt++ {
		testCycle := refTime.Add(-time.Duration(attempt*6) * time.Hour)
		dateStr := fmt.Sprintf("%04d%02d%02d", testCycle.Year(), testCycle.Month(), testCycle.Day())
		hourStr := fmt.Sprintf("%02d", testCycle.Hour())

		// Test existence of mature forecast step (078) to ensure DWD has finished uploading the cycle
		testURL := fmt.Sprintf("%s/%s/u_10m/icon_global_icosahedral_single-level_%s%s_078_U_10M.grib2.bz2", i.baseURL, hourStr, dateStr, hourStr)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, testURL, nil)
		if err == nil {
			resp, err := i.httpClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return &model.ModelCycle{
						ModelName:     i.ModelID(),
						ReferenceTime: testCycle,
						ResolutionDeg: 0.25,
						ForecastSteps: steps,
					}, nil
				}
			}
		}
	}

	return &model.ModelCycle{
		ModelName:     i.ModelID(),
		ReferenceTime: refTime,
		ResolutionDeg: 0.25,
		ForecastSteps: steps,
	}, nil
}

func defaultICONSteps() []int {
	steps := make([]int, 0, 70)
	for h := 0; h <= 78; h += 3 {
		steps = append(steps, h)
	}
	for h := 84; h <= 120; h += 3 {
		steps = append(steps, h)
	}
	return steps
}

// DiscoverSlices produces fetch tasks for DWD single-variable bz2 compressed files.
func (i *ICONDriver) DiscoverSlices(cycle *model.ModelCycle, variables []string) ([]model.FetchTask, error) {
	dateStr := fmt.Sprintf("%04d%02d%02d", cycle.ReferenceTime.Year(), cycle.ReferenceTime.Month(), cycle.ReferenceTime.Day())
	hourStr := fmt.Sprintf("%02d", cycle.ReferenceTime.Hour())

	var tasks []model.FetchTask

	for _, step := range cycle.ForecastSteps {
		for _, v := range variables {
			dwdFolder, dwdFileUpper := iconDWDVarParts(v)
			if dwdFolder == "" {
				continue
			}

			// DWD ICON OpenData URL pattern
			// e.g. https://opendata.dwd.de/weather/nwp/icon/grib/06/u_10m/icon_global_icosahedral_single-level_2026083006_012_U_10M.grib2.bz2
			fileName := fmt.Sprintf("icon_global_icosahedral_single-level_%s%s_%03d_%s.grib2.bz2", dateStr, hourStr, step, dwdFileUpper)
			url := fmt.Sprintf("%s/%s/%s/%s", i.baseURL, hourStr, dwdFolder, fileName)

			tasks = append(tasks, model.FetchTask{
				ModelName: cycle.ModelName,
				Cycle:     cycle.ReferenceTime,
				StepHours: step,
				Variable:  v,
				SourceURL: url,
			})
		}
	}

	return tasks, nil
}

// IngestSlice streams the bz2 compressed GRIB2 file from DWD, decompresses it on the fly, and decodes the message.
func (i *ICONDriver) IngestSlice(ctx context.Context, task model.FetchTask) (*model.RawGridSlice, error) {
	nlats := 721
	nlons := 1440

	if task.Variable == model.VarPrecipAccum && task.StepHours == 0 {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, task.SourceURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ICON slice: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Return zero/NaN grid slice if step is missing or not yet uploaded
		return &model.RawGridSlice{
			Variable:  task.Variable,
			ValidTime: task.Cycle,
			StepHours: task.StepHours,
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

	// Decompress bz2 stream on the fly
	bzReader := bzip2.NewReader(resp.Body)
	gribBytes, err := io.ReadAll(bzReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress bz2 stream: %w", err)
	}

	msg, err := grib2.Parse(gribBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ICON GRIB2: %w", err)
	}

	slice := msg.ToRawGridSlice(task.Variable)
	slice.StepHours = task.StepHours
	return slice, nil
}

func iconDWDVarParts(canonicalVar string) (folder, fileUpper string) {
	switch canonicalVar {
	case model.VarWindU10m:
		return "u_10m", "U_10M"
	case model.VarWindV10m:
		return "v_10m", "V_10M"
	case model.VarWindGust10m:
		return "vmax_10m", "VMAX_10M"
	case model.VarMSLP:
		return "pmsl", "PMSL"
	case model.VarTemp2m:
		return "t_2m", "T_2M"
	case model.VarPrecipAccum:
		return "tot_prec", "TOT_PREC"
	default:
		return "", ""
	}
}
