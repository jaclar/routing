package weather

import (
	"testing"
	"time"
)

func TestGFSWeatherEngine(t *testing.T) {
	now := time.Now().UTC()
	engine := NewRealisticGFSEngine(now)

	// Sample in Atlantic Trade Winds (20N, -40W)
	wTrades := engine.GetWind(20.0, -40.0, now)
	if wTrades.TWS < 5.0 || wTrades.TWS > 35.0 {
		t.Fatalf("Expected realistic trade wind speed ~5-35 kts, got %.2f kts", wTrades.TWS)
	}

	// Sample in North Atlantic (40N, -60W)
	wNorth := engine.GetWind(40.0, -60.0, now.Add(24*time.Hour))
	if wNorth.TWS < 5.0 || wNorth.TWS > 50.0 {
		t.Fatalf("Expected realistic wind speed ~5-50 kts, got %.2f kts", wNorth.TWS)
	}

	// Test Grid extraction
	grid, err := engine.GetGrid(30.0, 45.0, -75.0, -60.0, 1.0, 1.0, now)
	if err != nil || len(grid) != 16 || len(grid[0]) != 16 {
		t.Fatalf("Expected 16x16 grid, got %dx%d (err: %v)", len(grid), len(grid[0]), err)
	}
}

func TestLiveNOAAGFSEngine(t *testing.T) {
	now := time.Now().UTC()
	liveEngine := NewLiveNOAAGFSEngine(now)

	// Sample with live provider
	grid, err := liveEngine.GetGrid(20.0, 25.0, -160.0, -155.0, 2.5, 2.5, now)
	if err != nil || len(grid) == 0 || len(grid[0]) == 0 {
		t.Fatalf("Expected non-empty grid from LiveNOAAGFSEngine: %v", err)
	}

	w := liveEngine.GetWind(21.3, -157.8, now)
	if w.TWS < 1.0 || w.TWS > 70.0 {
		t.Fatalf("Expected valid wind speed, got %.2f kts", w.TWS)
	}
}
