package driver

import (
	"context"
	"net/http"
	"time"

	"sailboat/meteo/internal/model"
)

// DefaultHTTPClient creates an optimized HTTP client with generous timeouts and connection pooling.
func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 180 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 15 * time.Second,
		},
	}
}

// ModelDriver defines the pluggable contract for fetching and decoding any NWP weather model.
type ModelDriver interface {
	// ModelID returns the canonical slug: "gfs_0p25", "ifs_0p25", "icon_global", "gefs_0p50", "ifs_ens_0p25", "icon_eps_global"
	ModelID() string

	// CheckLatestCycle checks upstream availability to find the most recent available run
	CheckLatestCycle(ctx context.Context) (*model.ModelCycle, error)

	// DiscoverSlices produces fetch tasks for the requested forecast steps and canonical variables
	DiscoverSlices(cycle *model.ModelCycle, variables []string) ([]model.FetchTask, error)

	// IngestSlice downloads the byte range / message for a single slice, decodes it, and returns the normalized tensor
	IngestSlice(ctx context.Context, task model.FetchTask) (*model.RawGridSlice, error)
}

// NewDeterministicDrivers returns drivers for deterministic runs (GFS 0.25, IFS 0.25, ICON Global).
func NewDeterministicDrivers(client *http.Client) []ModelDriver {
	return []ModelDriver{
		NewGFSDriver(client),
		NewECMWFDriver(model.ModelIFS025, client),
		NewICONDriver(client),
	}
}

// NewEnsembleDrivers returns drivers for ensemble runs (GEFS 0.50, IFS-ENS 0.25, ICON-EPS Global).
func NewEnsembleDrivers(client *http.Client) []ModelDriver {
	return []ModelDriver{
		NewGEFSDriver(client),
		NewECMWFENSDriver(client),
		NewICONEPSDriver(client),
	}
}

// NewAllDrivers returns all registered deterministic and ensemble model drivers.
func NewAllDrivers(client *http.Client) []ModelDriver {
	var all []ModelDriver
	all = append(all, NewDeterministicDrivers(client)...)
	all = append(all, NewEnsembleDrivers(client)...)
	return all
}

