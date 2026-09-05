package polar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// VPPClient fetches boat polars from the VPP service, which is the only source of them.
//
// Preset polars are cached in memory after their first successful fetch: they are identical
// for every request and cost the VPP service a second or more to solve, so re-fetching one
// per route calculation would put that on the critical path. The cache also means a preset
// already in use keeps working if the VPP service later becomes unreachable.
type VPPClient struct {
	BaseURL    string
	HTTPClient *http.Client

	cacheMu sync.RWMutex
	cache   map[string]*PolarTable
}

func NewVPPClient(baseURL string) *VPPClient {
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	return &VPPClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: make(map[string]*PolarTable),
	}
}

type SolveMatrixReq struct {
	PresetName string      `json:"preset_name,omitempty"`
	Boat       interface{} `json:"boat,omitempty"`
}

type SolveMatrixResp struct {
	BoatName    string      `json:"boat_name"`
	TWSList     []float64   `json:"tws_list"`
	TWAList     []float64   `json:"twa_list"`
	SpeedMatrix [][]float64 `json:"speed_matrix"`
}

// FetchPolar returns the polar for a preset or a custom boat.
//
// Presets are served from cache when available. Custom boats are always solved fresh, since
// their geometry is particular to one request and caching them would grow without bound.
// An error is returned rather than a substitute table: routing against the wrong boat
// silently is worse than failing loudly.
func (c *VPPClient) FetchPolar(presetID string, customBoat interface{}) (*PolarTable, error) {
	isPreset := customBoat == nil && presetID != ""

	if isPreset {
		c.cacheMu.RLock()
		cached, ok := c.cache[presetID]
		c.cacheMu.RUnlock()
		if ok {
			return cached, nil
		}
	}

	table, err := c.solveMatrix(presetID, customBoat)
	if err != nil {
		return nil, err
	}

	if isPreset {
		c.cacheMu.Lock()
		c.cache[presetID] = table
		c.cacheMu.Unlock()
	}

	return table, nil
}

// solveMatrix performs the actual request to the VPP service.
func (c *VPPClient) solveMatrix(presetID string, customBoat interface{}) (*PolarTable, error) {
	body, err := json.Marshal(SolveMatrixReq{PresetName: presetID, Boat: customBoat})
	if err != nil {
		return nil, fmt.Errorf("failed to encode polar request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/solve/matrix", c.BaseURL)
	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("VPP service unreachable at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("VPP service returned %s solving polar for %q", resp.Status, presetID)
	}

	var data SolveMatrixResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode polar response: %w", err)
	}

	if len(data.TWSList) == 0 || len(data.TWAList) == 0 || len(data.SpeedMatrix) == 0 {
		return nil, fmt.Errorf("VPP service returned an empty polar for %q", presetID)
	}

	return &PolarTable{
		BoatName: data.BoatName,
		TWSList:  data.TWSList,
		TWAList:  data.TWAList,
		Speeds:   data.SpeedMatrix,
	}, nil
}

// CachedPresets lists the presets currently held in memory. Exposed for diagnostics.
func (c *VPPClient) CachedPresets() []string {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	ids := make([]string, 0, len(c.cache))
	for id := range c.cache {
		ids = append(ids, id)
	}
	return ids
}
