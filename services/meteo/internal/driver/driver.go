package driver

import (
	"context"

	"sailboat/meteo/internal/model"
)

// ModelDriver defines the pluggable contract for fetching and decoding any NWP weather model.
type ModelDriver interface {
	// ModelID returns the canonical slug: "gfs_0p25", "ifs_0p25", "icon_global", "aifs_0p25"
	ModelID() string

	// CheckLatestCycle checks upstream availability to find the most recent available run
	CheckLatestCycle(ctx context.Context) (*model.ModelCycle, error)

	// DiscoverSlices produces fetch tasks for the requested forecast steps and canonical variables
	DiscoverSlices(cycle *model.ModelCycle, variables []string) ([]model.FetchTask, error)

	// IngestSlice downloads the byte range / message for a single slice, decodes it, and returns the normalized tensor
	IngestSlice(ctx context.Context, task model.FetchTask) (*model.RawGridSlice, error)
}
