package isochrone

import (
	"testing"
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/landmask"
	"github.com/jaclar/routing-service/polar"
	"github.com/jaclar/routing-service/weather"
)

var (
	benchLandMask *landmask.LandMask
	benchWeather  weather.WeatherProvider
	benchStartTime = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
)

func init() {
	benchLandMask = landmask.NewGSHHGLandMask()
	benchWeather = weather.NewRealisticGFSEngine(benchStartTime)
}

type BenchScenario struct {
	Name      string
	Start     geo.Point
	Dest      geo.Point
	TimeStep  time.Duration
	BoatPolar *polar.PolarTable
}

func getBenchScenarios() []BenchScenario {
	ketch := polar.Get36ftKetchPolar()
	sloop := polar.GetPresetPolar("36ft-sloop")
	cruiser := polar.GetPresetPolar("40ft-cruiser")

	return []BenchScenario{
		{
			Name:      "Grenada_to_Trinidad_5m",
			Start:     geo.Point{Lat: 11.975, Lon: -61.765},
			Dest:      geo.Point{Lat: 10.675, Lon: -61.645},
			TimeStep:  5 * time.Minute,
			BoatPolar: ketch,
		},
		{
			Name:      "Cowes_to_Fastnet_30m",
			Start:     geo.Point{Lat: 50.76, Lon: -1.20},
			Dest:      geo.Point{Lat: 51.39, Lon: -9.60},
			TimeStep:  30 * time.Minute,
			BoatPolar: sloop,
		},
		{
			Name:      "Newport_to_Bermuda_1h",
			Start:     geo.Point{Lat: 41.40, Lon: -71.35},
			Dest:      geo.Point{Lat: 32.40, Lon: -64.55},
			TimeStep:  1 * time.Hour,
			BoatPolar: cruiser,
		},
		{
			Name:      "Lisbon_to_Madeira_1h",
			Start:     geo.Point{Lat: 38.67, Lon: -9.42},
			Dest:      geo.Point{Lat: 32.64, Lon: -16.90},
			TimeStep:  1 * time.Hour,
			BoatPolar: ketch,
		},
		{
			Name:      "SF_to_Hawaii_4h",
			Start:     geo.Point{Lat: 37.75, Lon: -122.60},
			Dest:      geo.Point{Lat: 21.25, Lon: -157.60},
			TimeStep:  4 * time.Hour,
			BoatPolar: ketch,
		},
	}
}

func BenchmarkPruningStrategies(b *testing.B) {
	scenarios := getBenchScenarios()
	strategies := []struct {
		name     string
		strategy PruningStrategy
	}{
		{"RadialSector", PruningRadialSector},
		{"SpatialGrid", PruningSpatialGrid},
		{"AStarBeam", PruningAStarBeam},
		{"ParetoEnvelope", PruningParetoEnvelope},
		{"StateSpaceGrid", PruningStateSpaceGrid},
	}

	for _, strat := range strategies {
		b.Run(strat.name, func(b *testing.B) {
			for _, sc := range scenarios {
				b.Run(sc.Name, func(b *testing.B) {
					cfg := DefaultRouterConfig()
					cfg.TimeStep = sc.TimeStep
					cfg.PruningStrategy = strat.strategy

					b.ResetTimer()
					b.ReportAllocs()

					for i := 0; i < b.N; i++ {
						_, err := CalculateOptimalRoute(
							sc.Start,
							sc.Dest,
							benchStartTime,
							sc.BoatPolar,
							benchWeather,
							benchLandMask,
							cfg,
						)
						if err != nil {
							b.Fatalf("Routing failed: %v", err)
						}
					}
				})
			}
		})
	}
}
