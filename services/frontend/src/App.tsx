import React, { useState, useEffect, useCallback, useRef } from 'react';
import { MapView } from './components/MapView';
import { TimelineScrubber } from './components/TimelineScrubber';
import { LayerToggles } from './components/LayerToggles';
import { WaypointControls } from './components/WaypointControls';
import { SettingsView } from './components/SettingsView';
import { VPPInspector } from './components/VPPInspector';
import { PassageStatistics } from './components/PassageStatistics';
import { BoatPreset, LandmaskPolygon, Point, RouteResult, WeatherGridResponse } from './types';
import {
  fetchPresets,
  calculateRoute,
  fetchWeatherGrid,
  fetchLandmaskPolygons,
  ROUTE_PRESETS,
  calcDirectDistanceNM,
  getSaneDefaultTimeStepHours,
  loadCustomBoatsFromStorage,
  saveCustomBoatToStorage,
  deleteCustomBoatFromStorage,
} from './services/api';
import {
  Compass,
  Menu,
  X,
  Sliders,
  BarChart3,
} from 'lucide-react';
import './styles/App.css';

export const App: React.FC = () => {
  // Primary view mode: 'routing' (Weather Routing) or 'settings' (Settings / VPP)
  const [activeView, setActiveView] = useState<'routing' | 'settings' | 'vpp'>('routing');
  
  // Routing sub-tab: 'map' (Map View) or 'stats' (Passage Statistics) - ONLY shown in routing view
  const [routingSubTab, setRoutingSubTab] = useState<'map' | 'stats'>('map');
  
  const [isMenuOpen, setIsMenuOpen] = useState<boolean>(false);

  const [placementMode, setPlacementMode] = useState<'start' | 'dest'>('start');
  const [presets, setPresets] = useState<BoatPreset[]>([]);
  const [selectedPresetId, setSelectedPresetId] = useState<string>('36ft-ketch');

  const [startPoint, setStartPoint] = useState<Point>(ROUTE_PRESETS[0].start);
  const [destPoint, setDestPoint] = useState<Point>(ROUTE_PRESETS[0].dest);
  const [departureTime, setDepartureTime] = useState<string>(
    new Date().toISOString().slice(0, 16)
  );
  const [timeStepHours, setTimeStepHours] = useState<number>(() => {
    const d = calcDirectDistanceNM(ROUTE_PRESETS[0].start, ROUTE_PRESETS[0].dest);
    return getSaneDefaultTimeStepHours(d);
  });
  const [tackPenaltyMinutes, setTackPenaltyMinutes] = useState<number>(5.0);
  const [gybePenaltyMinutes, setGybePenaltyMinutes] = useState<number>(8.0);

  const [loading, setLoading] = useState<boolean>(false);
  const [routeResult, setRouteResult] = useState<RouteResult | null>(null);
  const [currentWaypointIndex, setCurrentWaypointIndex] = useState<number>(0);

  const [weatherGrid, setWeatherGrid] = useState<WeatherGridResponse | null>(null);
  const [landmaskPolygons, setLandmaskPolygons] = useState<LandmaskPolygon[]>([]);

  // Layer Defaults: Only GFS Wind & Barbs selected; Isochrones and Landmass unselected
  const [showWindGrid, setShowWindGrid] = useState<boolean>(true);
  const [showIsochrones, setShowIsochrones] = useState<boolean>(false);
  const [showLandmask, setShowLandmask] = useState<boolean>(false);

  // Weather grid cache by timestamp
  const weatherCacheRef = useRef<Map<string, WeatherGridResponse>>(new Map());

  // Initial presets load + merge stored custom boats
  useEffect(() => {
    fetchPresets().then((builtin) => {
      const savedCustom = loadCustomBoatsFromStorage();
      const merged = [...builtin, ...savedCustom];
      setPresets(merged);
      if (merged.length > 0) {
        setSelectedPresetId((prev) => (merged.some((p) => p.id === prev) ? prev : merged[0].id));
      }
    });
  }, []);

  // Load landmask polygons dynamically when enabled
  useEffect(() => {
    if (!showLandmask) return;
    const minLat = Math.min(startPoint.lat, destPoint.lat) - 3.0;
    const maxLat = Math.max(startPoint.lat, destPoint.lat) + 3.0;
    const minLon = Math.min(startPoint.lon, destPoint.lon) - 3.0;
    const maxLon = Math.max(startPoint.lon, destPoint.lon) + 3.0;

    fetchLandmaskPolygons({ minLat, maxLat, minLon, maxLon }).then((polys) => {
      setLandmaskPolygons(polys);
    });
  }, [startPoint, destPoint, showLandmask]);

  // Determine active forecast timestamp
  const activeTime =
    routeResult && routeResult.waypoints[currentWaypointIndex]
      ? routeResult.waypoints[currentWaypointIndex].time
      : departureTime;

  // Load weather grid dynamically for the active timestamp
  const loadWeatherForTime = useCallback(
    async (timeStr: string) => {
      if (!showWindGrid) return;

      const cacheKey = `${startPoint.lat.toFixed(1)}_${startPoint.lon.toFixed(1)}_${destPoint.lat.toFixed(1)}_${destPoint.lon.toFixed(1)}_${timeStr}`;
      if (weatherCacheRef.current.has(cacheKey)) {
        setWeatherGrid(weatherCacheRef.current.get(cacheKey)!);
        return;
      }

      try {
        const latSpan = Math.abs(destPoint.lat - startPoint.lat);
        const lonSpan = Math.abs(destPoint.lon - startPoint.lon);
        const latStep = latSpan > 15 ? 2.0 : 1.5;
        const lonStep = lonSpan > 20 ? 2.5 : 1.5;

        const minLat = Math.min(startPoint.lat, destPoint.lat) - 5;
        const maxLat = Math.max(startPoint.lat, destPoint.lat) + 5;
        const minLon = Math.min(startPoint.lon, destPoint.lon) - 5;
        const maxLon = Math.max(startPoint.lon, destPoint.lon) + 5;

        const grid = await fetchWeatherGrid({
          minLat,
          maxLat,
          minLon,
          maxLon,
          latStep,
          lonStep,
          time: timeStr,
        });

        weatherCacheRef.current.set(cacheKey, grid);
        setWeatherGrid(grid);
      } catch (err) {
        console.warn('Weather grid load error:', err);
      }
    },
    [startPoint, destPoint, showWindGrid]
  );

  // Trigger weather reload whenever activeTime or start/dest points change
  useEffect(() => {
    loadWeatherForTime(activeTime);
  }, [activeTime, loadWeatherForTime]);

  // Snapshot of parameters used when the current routeResult was calculated
  const [solvedParams, setSolvedParams] = useState<{
    start: Point;
    dest: Point;
    departureTime: string;
    boatPresetId: string;
    tackPenaltyMinutes: number;
    gybePenaltyMinutes: number;
    timeStepHours: number;
  } | null>(null);

  const normalizeTimeStr = (t?: string) => {
    if (!t) return '';
    try {
      return new Date(t).toISOString().slice(0, 16);
    } catch {
      return t.slice(0, 16);
    }
  };

  // Route is considered outdated if departure time, yacht model, tack/gybe penalties, or coordinates changed
  const isRouteOutdated = Boolean(
    routeResult &&
    solvedParams &&
    (
      normalizeTimeStr(departureTime) !== normalizeTimeStr(solvedParams.departureTime) ||
      selectedPresetId !== solvedParams.boatPresetId ||
      Math.abs(tackPenaltyMinutes - solvedParams.tackPenaltyMinutes) > 0.01 ||
      Math.abs(gybePenaltyMinutes - solvedParams.gybePenaltyMinutes) > 0.01 ||
      Math.abs(startPoint.lat - solvedParams.start.lat) > 0.0001 ||
      Math.abs(startPoint.lon - solvedParams.start.lon) > 0.0001 ||
      Math.abs(destPoint.lat - solvedParams.dest.lat) > 0.0001 ||
      Math.abs(destPoint.lon - solvedParams.dest.lon) > 0.0001 ||
      Math.abs(timeStepHours - solvedParams.timeStepHours) > 0.0001
    )
  );

  // Handle Route Calculation
  const handleCalculateRoute = async () => {
    setLoading(true);
    weatherCacheRef.current.clear();
    try {
      const selectedBoat = presets.find((p) => p.id === selectedPresetId);
      const result = await calculateRoute({
        start: startPoint,
        dest: destPoint,
        startTime: departureTime,
        boatPreset: selectedPresetId,
        timeStepHours,
        tackPenaltyMinutes,
        gybePenaltyMinutes,
        customBoat: selectedBoat?.customBoat,
        customPolar: selectedBoat?.polarData
          ? {
              boat_name: selectedBoat.name,
              tws_list: selectedBoat.polarData.tws_list,
              twa_list: selectedBoat.polarData.twa_list,
              speed_matrix: selectedBoat.polarData.speed_matrix,
            }
          : undefined,
      });
      setRouteResult(result);
      setSolvedParams({
        start: { ...startPoint },
        dest: { ...destPoint },
        departureTime,
        boatPresetId: selectedPresetId,
        tackPenaltyMinutes,
        gybePenaltyMinutes,
        timeStepHours,
      });
      setCurrentWaypointIndex(0);
      setActiveView('routing');
      setRoutingSubTab('map');
    } catch (err: any) {
      alert(`Route Calculation Failed: ${err.message || err}`);
    } finally {
      setLoading(false);
    }
  };

  const handleAddCustomBoat = useCallback((newPreset: BoatPreset) => {
    saveCustomBoatToStorage(newPreset);
    setPresets((prev) => {
      const filtered = prev.filter((p) => p.id !== newPreset.id);
      return [...filtered, newPreset];
    });
    setSelectedPresetId(newPreset.id);
  }, []);

  const handleDeleteCustomBoat = useCallback((presetId: string) => {
    deleteCustomBoatFromStorage(presetId);
    setPresets((prev) => prev.filter((p) => p.id !== presetId));
    setSelectedPresetId('36ft-ketch');
  }, []);

  const handleStartPointChange = useCallback((newStart: Point) => {
    setStartPoint(newStart);
    weatherCacheRef.current.clear();
    const d = calcDirectDistanceNM(newStart, destPoint);
    setTimeStepHours(getSaneDefaultTimeStepHours(d));
  }, [destPoint]);

  const handleDestPointChange = useCallback((newDest: Point) => {
    setDestPoint(newDest);
    weatherCacheRef.current.clear();
    const d = calcDirectDistanceNM(startPoint, newDest);
    setTimeStepHours(getSaneDefaultTimeStepHours(d));
  }, [startPoint]);

  return (
    <div className="app-container">
      {/* Top Floating Navigation Cluster */}
      <div className="top-navigation-cluster">
        
        {/* 1. Standalone Hamburger Button & 2-Item Dropdown Menu */}
        <div className="hamburger-container">
          <button
            type="button"
            className={`standalone-hamburger-btn ${isMenuOpen ? 'active' : ''}`}
            onClick={() => setIsMenuOpen(!isMenuOpen)}
            title="Application Menu"
          >
            {isMenuOpen ? <X size={20} /> : <Menu size={20} />}
          </button>

          {isMenuOpen && (
            <div className="hamburger-dropdown">
              <div className="hamburger-dropdown-header">
                <span className="hamburger-dropdown-title">Menu</span>
              </div>

              {/* Exactly 2 Menu Items as requested: Weather Routing & Settings */}
              <div className="hamburger-menu-list">
                <button
                  type="button"
                  className={`hamburger-item ${activeView === 'routing' ? 'active' : ''}`}
                  onClick={() => {
                    setActiveView('routing');
                    setIsMenuOpen(false);
                  }}
                >
                  <div className="hamburger-item-icon text-sky">
                    <Compass size={17} />
                  </div>
                  <div className="hamburger-item-text">
                    <span className="hamburger-item-name">Weather Routing</span>
                    <span className="hamburger-item-sub">Interactive map &amp; passage statistics</span>
                  </div>
                </button>

                <button
                  type="button"
                  className={`hamburger-item ${activeView === 'settings' || activeView === 'vpp' ? 'active' : ''}`}
                  onClick={() => {
                    setActiveView('settings');
                    setIsMenuOpen(false);
                  }}
                >
                  <div className="hamburger-item-icon text-emerald">
                    <Sliders size={17} />
                  </div>
                  <div className="hamburger-item-text">
                    <span className="hamburger-item-name">Settings</span>
                    <span className="hamburger-item-sub">Boat configuration, penalties &amp; VPP</span>
                  </div>
                </button>
              </div>
            </div>
          )}
        </div>

        {/* 2. Top Tabs (Map View vs Passage Statistics) - ONLY rendered in Routing View */}
        {activeView === 'routing' && (
          <div className="app-tabs-bar">
            <button
              type="button"
              className={`tab-nav-btn ${routingSubTab === 'map' ? 'active' : ''}`}
              onClick={() => setRoutingSubTab('map')}
            >
              <Compass size={16} />
              <span>Map View</span>
            </button>
            <button
              type="button"
              className={`tab-nav-btn ${routingSubTab === 'stats' ? 'active' : ''}`}
              onClick={() => setRoutingSubTab('stats')}
            >
              <BarChart3 size={16} />
              <span>Passage Statistics</span>
            </button>
          </div>
        )}

      </div>

      {/* Main View Area */}
      <main className="main-view full-screen-view">
        
        {/* =========================================================================
            1. Weather Routing View (Map View & Passage Statistics)
            ========================================================================= */}
        {activeView === 'routing' && (
          <>
            {routingSubTab === 'map' ? (
              <>
                <MapView
                  startPoint={startPoint}
                  destPoint={destPoint}
                  onStartChange={handleStartPointChange}
                  onDestChange={handleDestPointChange}
                  placementMode={placementMode}
                  onPlacementModeChange={setPlacementMode}
                  routeResult={routeResult}
                  currentWaypointIndex={currentWaypointIndex}
                  weatherGrid={weatherGrid}
                  landmaskPolygons={landmaskPolygons}
                  showIsochrones={showIsochrones}
                  showWindGrid={showWindGrid}
                  showLandmask={showLandmask}
                />

                {/* Top Right Floating Controls Stack */}
                <div className="map-top-right-overlay">
                  <LayerToggles
                    showIsochrones={showIsochrones}
                    onToggleIsochrones={() => setShowIsochrones(!showIsochrones)}
                    showWindGrid={showWindGrid}
                    onToggleWindGrid={() => setShowWindGrid(!showWindGrid)}
                    showLandmask={showLandmask}
                    onToggleLandmask={() => setShowLandmask(!showLandmask)}
                    activeTime={activeTime}
                  />

                  <WaypointControls
                    startPoint={startPoint}
                    destPoint={destPoint}
                    onStartChange={handleStartPointChange}
                    onDestChange={handleDestPointChange}
                    placementMode={placementMode}
                    onSelectPlacementMode={setPlacementMode}
                    onTimeStepChange={setTimeStepHours}
                  />
                </div>

                {/* Bottom Unified Scrubber & Departure / Calculate Dock */}
                <TimelineScrubber
                  routeResult={routeResult}
                  currentIndex={currentWaypointIndex}
                  onIndexChange={setCurrentWaypointIndex}
                  departureTime={departureTime}
                  onDepartureTimeChange={(t) => {
                    setDepartureTime(t);
                    weatherCacheRef.current.clear();
                  }}
                  onCalculateRoute={handleCalculateRoute}
                  loading={loading}
                  isRecalculateActive={isRouteOutdated}
                />
              </>
            ) : (
              <div className="passage-stats-page-container">
                {routeResult ? (
                  <PassageStatistics routeResult={routeResult} />
                ) : (
                  <div className="stats-empty-state">
                    <BarChart3 size={48} color="#38bdf8" />
                    <h2>No Route Computed Yet</h2>
                    <p>Calculate an optimal route on the Map View to inspect performance plots and passage telemetry.</p>
                    <button
                      type="button"
                      className="btn-primary"
                      onClick={() => setRoutingSubTab('map')}
                    >
                      <Compass size={16} />
                      <span>Go to Map View</span>
                    </button>
                  </div>
                )}
              </div>
            )}
          </>
        )}

        {/* =========================================================================
            2. Settings View (Boat Configuration, Maneuver Penalties & VPP Launcher)
            ========================================================================= */}
        {activeView === 'settings' && (
          <SettingsView
            presets={presets}
            selectedPresetId={selectedPresetId}
            onSelectPreset={setSelectedPresetId}
            onAddCustomBoat={handleAddCustomBoat}
            onDeleteCustomBoat={handleDeleteCustomBoat}
            tackPenaltyMinutes={tackPenaltyMinutes}
            onTackPenaltyChange={setTackPenaltyMinutes}
            gybePenaltyMinutes={gybePenaltyMinutes}
            onGybePenaltyChange={setGybePenaltyMinutes}
            onOpenVPP={() => setActiveView('vpp')}
            onBackToRouting={() => {
              setActiveView('routing');
              setRoutingSubTab('map');
            }}
            routeResult={routeResult}
          />
        )}

        {/* =========================================================================
            3. VPP Dedicated Page (Reached via Settings)
            ========================================================================= */}
        {activeView === 'vpp' && (
          <div className="vpp-page-wrapper">
            <div className="vpp-page-header">
              <div className="vpp-back-buttons-row">
                <button
                  type="button"
                  className="btn-nav-back"
                  onClick={() => setActiveView('settings')}
                >
                  ← Back to Settings
                </button>
                <button
                  type="button"
                  className="btn-nav-map"
                  onClick={() => {
                    setActiveView('routing');
                    setRoutingSubTab('map');
                  }}
                >
                  <Compass size={14} />
                  <span>Map View</span>
                </button>
              </div>
            </div>

            <VPPInspector
              presets={presets}
              selectedPresetId={selectedPresetId}
              onSelectPreset={setSelectedPresetId}
              onAddCustomBoat={handleAddCustomBoat}
              onDeleteCustomBoat={handleDeleteCustomBoat}
            />
          </div>
        )}

      </main>
    </div>
  );
};

export default App;
