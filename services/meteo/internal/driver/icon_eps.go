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
	DWDEPSS3URL = "https://opendata.dwd.de/weather/nwp/icon-eps/grib"
)

// ICONEPSDriver implements ModelDriver for DWD ICON-EPS 40-member global ensemble forecasts.
type ICONEPSDriver struct {
	httpClient *http.Client
	baseURL    string
	regridLUT  []int32
	lutMu      sync.Mutex
}

// NewICONEPSDriver creates a DWD ICON-EPS driver.
func NewICONEPSDriver(client *http.Client) *ICONEPSDriver {
	if client == nil {
		client = DefaultHTTPClient()
	}
	return &ICONEPSDriver{
		httpClient: client,
		baseURL:    DWDEPSS3URL,
	}
}

func (i *ICONEPSDriver) ModelID() string {
	return model.ModelICONEPS025
}

func defaultICONEPSMembers() []int {
	members := make([]int, 40)
	for idx := 0; idx < 40; idx++ {
		members[idx] = idx + 1
	}
	return members
}

func defaultICONEPSSteps() []int {
	steps := make([]int, 0, 25)
	for h := 0; h <= 120; h += 6 {
		steps = append(steps, h)
	}
	return steps
}

// CheckLatestCycle probes DWD OpenData to discover the latest fully uploaded ensemble run (00z, 06z, 12z, 18z with ~3.5h lag).
func (i *ICONEPSDriver) CheckLatestCycle(ctx context.Context) (*model.ModelCycle, error) {
	now := time.Now().UTC()
	candidate := now.Add(-3 * time.Hour)

	cycleHour := (candidate.Hour() / 6) * 6
	refTime := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), cycleHour, 0, 0, 0, time.UTC)

	steps := defaultICONEPSSteps()
	members := defaultICONEPSMembers()

	for attempt := 0; attempt < 4; attempt++ {
		testCycle := refTime.Add(-time.Duration(attempt*6) * time.Hour)
		dateStr := fmt.Sprintf("%04d%02d%02d", testCycle.Year(), testCycle.Month(), testCycle.Day())
		hourStr := fmt.Sprintf("%02d", testCycle.Hour())

		testURLA := fmt.Sprintf("%s/%s/u_10m/icon-eps_global_icosahedral_single-level_%s%s_048_u_10m.grib2.bz2",
			i.baseURL, hourStr, dateStr, hourStr)
		testURLB := fmt.Sprintf("%s/%s/tot_prec/icon-eps_global_icosahedral_single-level_%s%s_048_tot_prec.grib2.bz2",
			i.baseURL, hourStr, dateStr, hourStr)

		reqA, errA := http.NewRequestWithContext(ctx, http.MethodHead, testURLA, nil)
		reqB, errB := http.NewRequestWithContext(ctx, http.MethodHead, testURLB, nil)
		if errA == nil && errB == nil {
			respA, errA := i.httpClient.Do(reqA)
			respB, errB := i.httpClient.Do(reqB)
			if errA == nil && errB == nil {
				respA.Body.Close()
				respB.Body.Close()
				if respA.StatusCode == http.StatusOK && respB.StatusCode == http.StatusOK {
					return &model.ModelCycle{
						ModelName:     i.ModelID(),
						ReferenceTime: testCycle,
						ResolutionDeg: 0.25,
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
		ModelName:     i.ModelID(),
		ReferenceTime: refTime,
		ResolutionDeg: 0.25,
		ForecastSteps: steps,
		Members:       members,
		IsEnsemble:    true,
	}, nil
}

// DiscoverSlices produces fetch tasks for DWD ensemble files (each bz2 file contains all 40 members).
func (i *ICONEPSDriver) DiscoverSlices(cycle *model.ModelCycle, variables []string) ([]model.FetchTask, error) {
	dateStr := fmt.Sprintf("%04d%02d%02d", cycle.ReferenceTime.Year(), cycle.ReferenceTime.Month(), cycle.ReferenceTime.Day())
	hourStr := fmt.Sprintf("%02d", cycle.ReferenceTime.Hour())

	members := cycle.Members
	if len(members) == 0 {
		members = defaultICONEPSMembers()
	}

	var tasks []model.FetchTask

	for _, step := range cycle.ForecastSteps {
		for _, v := range variables {
			if v == model.VarWindGust10m && step == 0 {
				continue
			}

			dwdFolder, dwdFile := iconEPSDWDVarParts(v)
			if dwdFolder == "" {
				continue
			}

			fileName := fmt.Sprintf("icon-eps_global_icosahedral_single-level_%s%s_%03d_%s.grib2.bz2",
				dateStr, hourStr, step, dwdFile)
			url := fmt.Sprintf("%s/%s/%s/%s", i.baseURL, hourStr, dwdFolder, fileName)

			tasks = append(tasks, model.FetchTask{
				ModelName: cycle.ModelName,
				Cycle:     cycle.ReferenceTime,
				StepHours: step,
				Variable:  v,
				Member:    -1, // Bundled members
				Members:   members,
				SourceURL: url,
			})
		}
	}

	return tasks, nil
}

// IngestSlice streams the bz2 compressed multi-message GRIB2 file from DWD, decompresses it on the fly, and decodes all 40 member messages.
func (i *ICONEPSDriver) IngestSlice(ctx context.Context, task model.FetchTask) (*model.RawGridSlice, error) {
	nlats := 721
	nlons := 1440
	nMembers := len(task.Members)
	if nMembers == 0 {
		nMembers = 40
	}

	if (task.Variable == model.VarPrecipAccum || task.Variable == model.VarWindGust10m) && task.StepHours == 0 {
		membersData := make([][]float32, nMembers)
		for m := range membersData {
			membersData[m] = make([]float32, nlats*nlons)
		}
		return &model.RawGridSlice{
			Variable:    task.Variable,
			ValidTime:   task.Cycle,
			StepHours:   0,
			NumMembers:  nMembers,
			NLats:       nlats,
			NLons:       nlons,
			LatStart:    90.0,
			LatEnd:      -90.0,
			LatStep:     0.25,
			LonStart:    0.0,
			LonEnd:      359.75,
			LonStep:     0.25,
			MembersData: membersData,
		}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, task.SourceURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ICON-EPS slice: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch ICON-EPS slice: HTTP status %d for %s", resp.StatusCode, task.Variable)
	}

	// Decompress bz2 stream on the fly
	bzReader := bzip2.NewReader(resp.Body)
	gribBytes, err := io.ReadAll(bzReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress ICON-EPS bz2 stream: %w", err)
	}

	// Parse all concatenated GRIB2 messages
	messages, err := grib2.ParseAll(gribBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ICON-EPS GRIB2 messages: %w", err)
	}

	lut, err := i.ensureRegridLUT(ctx, task.Cycle)
	if err != nil {
		return nil, fmt.Errorf("failed to build/retrieve ICON-EPS regrid LUT: %w", err)
	}

	membersData := make([][]float32, len(messages))
	for mIdx, msg := range messages {
		regData := make([]float32, nlats*nlons)
		if len(msg.Values) == 2949120 || msg.GridTemplate == 100 || msg.GridTemplate == 101 {
			for idx := range regData {
				srcIdx := lut[idx]
				if int(srcIdx) < len(msg.Values) {
					regData[idx] = msg.Values[srcIdx]
				}
			}
		} else if len(msg.Values) == nlats*nlons {
			copy(regData, msg.Values)
		}
		membersData[mIdx] = regData
	}

	return &model.RawGridSlice{
		Variable:    task.Variable,
		ValidTime:   task.Cycle.Add(time.Duration(task.StepHours) * time.Hour),
		StepHours:   task.StepHours,
		NumMembers:  len(messages),
		NLats:       nlats,
		NLons:       nlons,
		LatStart:    90.0,
		LatEnd:      -90.0,
		LatStep:     0.25,
		LonStart:    0.0,
		LonEnd:      359.75,
		LonStep:     0.25,
		MembersData: membersData,
	}, nil
}

func (i *ICONEPSDriver) ensureRegridLUT(ctx context.Context, cycleTime time.Time) ([]int32, error) {
	i.lutMu.Lock()
	defer i.lutMu.Unlock()

	if i.regridLUT != nil && len(i.regridLUT) == 721*1440 {
		return i.regridLUT, nil
	}

	dateStr := fmt.Sprintf("%04d%02d%02d", cycleTime.Year(), cycleTime.Month(), cycleTime.Day())
	hourStr := fmt.Sprintf("%02d", cycleTime.Hour())

	fetchCoord := func(varName string) ([]float32, error) {
		url := fmt.Sprintf("%s/%s/%s/icon-eps_global_icosahedral_time-invariant_%s%s_%s.grib2.bz2",
			i.baseURL, hourStr, strings.ToLower(varName), dateStr, hourStr, strings.ToLower(varName))
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

	clat, err := fetchCoord("clat")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ICON-EPS CLAT: %w", err)
	}
	clon, err := fetchCoord("clon")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ICON-EPS CLON: %w", err)
	}

	if len(clat) != len(clon) || len(clat) == 0 {
		return nil, fmt.Errorf("invalid coordinate dimensions: CLAT=%d, CLON=%d", len(clat), len(clon))
	}

	// Build 2D spatial binning grid
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

func iconEPSDWDVarParts(canonicalVar string) (folder, fileLower string) {
	switch canonicalVar {
	case model.VarWindU10m:
		return "u_10m", "u_10m"
	case model.VarWindV10m:
		return "v_10m", "v_10m"
	case model.VarWindGust10m:
		return "vmax_10m", "vmax_10m"
	case model.VarMSLP:
		return "ps", "ps"
	case model.VarTemp2m:
		return "t_2m", "t_2m"
	case model.VarPrecipAccum:
		return "tot_prec", "tot_prec"
	default:
		return "", ""
	}
}
