package polar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type VPPClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewVPPClient(baseURL string) *VPPClient {
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	return &VPPClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type SolveMatrixReq struct {
	PresetName string      `json:"preset_name,omitempty"`
	Boat       interface{} `json:"boat,omitempty"`
}

type SolveMatrixResp struct {
	BoatName     string      `json:"boat_name"`
	TWSList      []float64   `json:"tws_list"`
	TWAList      []float64   `json:"twa_list"`
	SpeedMatrix  [][]float64 `json:"speed_matrix"`
}

// FetchPolar queries the VPP service for a boat polar matrix, with fallback to built-in presets.
func (c *VPPClient) FetchPolar(presetID string, customBoat interface{}) (*PolarTable, error) {
	if customBoat == nil && presetID != "" {
		// Fast path if local preset exists
		return GetPresetPolar(presetID), nil
	}

	payload := SolveMatrixReq{
		PresetName: presetID,
		Boat:       customBoat,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return GetPresetPolar(presetID), err
	}

	url := fmt.Sprintf("%s/api/v1/solve/matrix", c.BaseURL)
	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		// Fallback to built-in preset
		return GetPresetPolar(presetID), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GetPresetPolar(presetID), nil
	}

	var data SolveMatrixResp
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return GetPresetPolar(presetID), nil
	}

	return &PolarTable{
		BoatName: data.BoatName,
		TWSList:  data.TWSList,
		TWAList:  data.TWAList,
		Speeds:   data.SpeedMatrix,
	}, nil
}
