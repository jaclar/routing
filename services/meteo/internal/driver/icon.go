package driver

import (
	"compress/bzip2"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
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
	regridLUT  []int32
	lutMu      sync.Mutex
}

// NewICONDriver creates a DWD ICON Global driver.
func NewICONDriver(client *http.Client) *ICONDriver {
	if client == nil {
		client = DefaultHTTPClient()
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
			// DWD does not publish 000_VMAX_10M (gusts are diagnostic maximums starting at step > 0)
			if v == model.VarWindGust10m && step == 0 {
				continue
			}

			dwdFolder, dwdFileUpper := iconDWDVarParts(v)
			if dwdFolder == "" {
				continue
			}

			// DWD ICON OpenData URL pattern
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

	if (task.Variable == model.VarPrecipAccum || task.Variable == model.VarWindGust10m) && task.StepHours == 0 {
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
		return nil, fmt.Errorf("failed to fetch ICON slice: HTTP status %d for %s", resp.StatusCode, task.Variable)
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

	// Regrid unstructured icosahedral grid (2,949,120 points) to regular 721x1440 grid
	if msg.GridTemplate == 100 || msg.GridTemplate == 101 || len(msg.Values) == 2949120 {
		lut, err := i.ensureRegridLUT(ctx, task.Cycle)
		if err != nil {
			return nil, fmt.Errorf("failed to build/retrieve ICON regrid LUT: %w", err)
		}

		regData := make([]float32, nlats*nlons)
		for idx := range regData {
			srcIdx := lut[idx]
			if int(srcIdx) < len(msg.Values) {
				regData[idx] = msg.Values[srcIdx]
			}
		}

		return &model.RawGridSlice{
			Variable:  task.Variable,
			ValidTime: task.Cycle.Add(time.Duration(task.StepHours) * time.Hour),
			StepHours: task.StepHours,
			NLats:     nlats,
			NLons:     nlons,
			LatStart:  90.0,
			LatEnd:    -90.0,
			LatStep:   0.25,
			LonStart:  0.0,
			LonEnd:    359.75,
			LonStep:   0.25,
			Data:      regData,
		}, nil
	}

	slice := msg.ToRawGridSlice(task.Variable)
	slice.StepHours = task.StepHours
	return slice, nil
}

// ensureRegridLUT retrieves or computes the nearest-neighbor lookup table from ICON icosahedral grid to 721x1440 regular lat/lon grid.
func (i *ICONDriver) ensureRegridLUT(ctx context.Context, cycleTime time.Time) ([]int32, error) {
	i.lutMu.Lock()
	defer i.lutMu.Unlock()

	if i.regridLUT != nil && len(i.regridLUT) == 721*1440 {
		return i.regridLUT, nil
	}

	dateStr := fmt.Sprintf("%04d%02d%02d", cycleTime.Year(), cycleTime.Month(), cycleTime.Day())
	hourStr := fmt.Sprintf("%02d", cycleTime.Hour())

	fetchCoord := func(varName string) ([]float32, error) {
		url := fmt.Sprintf("%s/%s/%s/icon_global_icosahedral_time-invariant_%s%s_%s.grib2.bz2",
			i.baseURL, hourStr, strings.ToLower(varName), dateStr, hourStr, strings.ToUpper(varName))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := i.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d for invariant %s", resp.StatusCode, varName)
		}
		bz := bzip2.NewReader(resp.Body)
		data, err := io.ReadAll(bz)
		if err != nil {
			return nil, err
		}
		msg, err := grib2.Parse(data)
		if err != nil {
			return nil, err
		}
		return msg.Values, nil
	}

	clat, err := fetchCoord("CLAT")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch CLAT: %w", err)
	}
	clon, err := fetchCoord("CLON")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch CLON: %w", err)
	}

	if len(clat) != len(clon) || len(clat) == 0 {
		return nil, fmt.Errorf("invalid coordinate dimensions: CLAT=%d, CLON=%d", len(clat), len(clon))
	}

	// Build 2D binning grid for rapid nearest neighbor search
	type spatialPoint struct {
		idx     int32
		x, y, z float32
	}
	bins := make([][][]spatialPoint, 181)
	for b := range bins {
		bins[b] = make([][]spatialPoint, 360)
	}

	deg2rad := math.Pi / 180.0
	for k := 0; k < len(clat); k++ {
		lat := float64(clat[k])
		lon := float64(clon[k])
		if lon < 0 {
			lon += 360.0
		}
		latBin := int(lat + 90.0)
		if latBin < 0 {
			latBin = 0
		}
		if latBin > 180 {
			latBin = 180
		}
		lonBin := int(lon)
		if lonBin < 0 {
			lonBin = 0
		}
		if lonBin >= 360 {
			lonBin = 359
		}

		latRad := lat * deg2rad
		lonRad := lon * deg2rad
		cosLat := math.Cos(latRad)
		x := float32(cosLat * math.Cos(lonRad))
		y := float32(cosLat * math.Sin(lonRad))
		z := float32(math.Sin(latRad))

		bins[latBin][lonBin] = append(bins[latBin][lonBin], spatialPoint{
			idx: int32(k),
			x:   x,
			y:   y,
			z:   z,
		})
	}

	nlats, nlons := 721, 1440
	lut := make([]int32, nlats*nlons)

	for latIdx := 0; latIdx < nlats; latIdx++ {
		latTarget := 90.0 - float64(latIdx)*0.25
		latBin := int(latTarget + 90.0)
		if latBin < 0 {
			latBin = 0
		}
		if latBin > 180 {
			latBin = 180
		}

		latRad := latTarget * deg2rad
		cosLat := math.Cos(latRad)
		sinLat := math.Sin(latRad)

		for lonIdx := 0; lonIdx < nlons; lonIdx++ {
			lonTarget := float64(lonIdx) * 0.25
			lonBin := int(lonTarget)
			if lonBin < 0 {
				lonBin = 0
			}
			if lonBin >= 360 {
				lonBin = 359
			}

			lonRad := lonTarget * deg2rad
			tx := float32(cosLat * math.Cos(lonRad))
			ty := float32(cosLat * math.Sin(lonRad))
			tz := float32(sinLat)

			bestDistSq := float32(1e9)
			bestIdx := int32(0)

			for dLat := -1; dLat <= 1; dLat++ {
				bLat := latBin + dLat
				if bLat < 0 || bLat > 180 {
					continue
				}
				for dLon := -1; dLon <= 1; dLon++ {
					bLon := (lonBin + dLon + 360) % 360
					for _, pt := range bins[bLat][bLon] {
						dx := pt.x - tx
						dy := pt.y - ty
						dz := pt.z - tz
						distSq := dx*dx + dy*dy + dz*dz
						if distSq < bestDistSq {
							bestDistSq = distSq
							bestIdx = pt.idx
						}
					}
				}
			}
			lut[latIdx*nlons+lonIdx] = bestIdx
		}
	}

	i.regridLUT = lut
	return i.regridLUT, nil
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
