// Package polartest provides a fixed polar table for tests.
//
// The routing service deliberately holds no built-in polars: the VPP service is the only
// source of those, and a second copy here would silently drift from it. Tests still need a
// deterministic table to route against, so this package carries one, clearly separated from
// production code and never used by it.
//
// The numbers began life as a solved 36ft cruising ketch. They are a fixture, not a model
// output: nothing here needs regenerating when the VPP model changes.
package polartest

import "github.com/jaclar/routing-service/polar"

// Table returns a fresh cruising-yacht polar for use in tests.
//
// A new value is returned on every call so a test that mutates it cannot affect another.
func Table() *polar.PolarTable {
	return &polar.PolarTable{
		BoatName: "Test Cruising Yacht",
		TWSList:  []float64{6.0, 8.0, 10.0, 12.0, 14.0, 16.0, 20.0, 25.0},
		TWAList:  []float64{0.0, 20.0, 25.0, 30.0, 35.0, 40.0, 45.0, 52.0, 60.0, 70.0, 80.0, 90.0, 110.0, 120.0, 135.0, 150.0, 165.0, 180.0},
		Speeds: [][]float64{
			{0.00, 0.00, 0.00, 2.89, 3.32, 3.75, 4.00, 4.22, 4.38, 4.49, 4.54, 4.53, 4.42, 4.36, 4.06, 3.50, 2.99, 2.70}, // 6 kt
			{0.00, 0.00, 0.00, 3.83, 4.18, 4.41, 4.58, 4.76, 4.89, 5.00, 5.05, 5.03, 5.04, 5.00, 4.63, 4.38, 3.90, 3.61}, // 8 kt
			{0.00, 0.00, 0.00, 4.35, 4.62, 4.83, 4.98, 5.16, 5.30, 5.41, 5.45, 5.46, 5.55, 5.44, 5.11, 4.94, 4.34, 4.30}, // 10 kt
			{0.00, 0.00, 0.00, 4.71, 4.96, 5.16, 5.32, 5.49, 5.79, 5.92, 5.96, 6.01, 6.09, 5.98, 5.53, 4.81, 4.70, 4.77}, // 12 kt
			{0.00, 0.00, 0.00, 5.00, 5.25, 5.44, 5.74, 5.94, 6.06, 6.15, 6.19, 6.26, 6.32, 6.19, 6.03, 5.17, 5.08, 5.17}, // 14 kt
			{0.00, 0.00, 0.00, 5.25, 5.50, 5.85, 6.00, 6.14, 6.25, 6.33, 6.37, 6.46, 6.50, 6.40, 6.27, 5.48, 5.42, 5.52}, // 16 kt
			{0.00, 0.00, 0.00, 5.81, 6.04, 6.19, 6.30, 6.42, 6.51, 6.60, 6.68, 6.80, 6.79, 6.75, 6.54, 6.01, 6.08, 6.20}, // 20 kt
			{0.00, 0.00, 0.00, 6.10, 6.26, 6.40, 6.49, 6.68, 6.79, 6.88, 7.02, 7.17, 7.18, 7.15, 6.80, 6.43, 6.50, 6.62}, // 25 kt
		},
	}
}
