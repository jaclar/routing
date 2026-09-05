import React, { useState, useEffect, useCallback, useRef } from 'react';
import { MapView } from './components/MapView';
import { TimelineScrubber } from './components/TimelineScrubber';
import { LayerToggles } from './components/LayerToggles';
import { WaypointControls } from './components/WaypointControls';
import { SettingsView } from './components/SettingsView';
import { VPPInspector } from './components/VPPInspector';
import { PassageStatistics } from './components/PassageStatistics';
import { WeatherWindowFinder } from './components/WeatherWindowFinder';
import { ConfirmDialog } from './components/ConfirmDialog';
import {
  BoatPreset,
  DEFAULT_WEATHER_MODEL,
  LandmaskPolygon,
  MultiRouteResult,
  Point,
  RouteResult,
  WaypointChangeSource,
  WEATHER_MODELS,
  WeatherGridResponse,
  WeatherModelId,
} from './types';
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
  usePersistedState,
  dropIsochrones,
  dropIsochronesFromAll,
  nowDepartureValue,
  reviveDepartureTime,
} from './services/persistence';
import {
  Compass,
  Menu,
  X,
  Sliders,
  BarChart3,
  Clock,
  CalendarRange,
} from 'lucide-react';
import './styles/App.css';

export const App: React.FC = () => {
  // Navigation. Restored so a reload reopens the view the user was last on.
  const [activeView, setActiveView] = usePersistedState<'routing' | 'settings' | 'vpp' | 'window-finder'>(
    'activeView',
    'routing'
  );

  // Routing sub-tab: 'map' (Map View) or 'stats' (Passage Statistics) - ONLY shown in routing view
  const [routingSubTab, setRoutingSubTab] = usePersistedState<'map' | 'stats'>('routingSubTab', 'map');

  // Deliberately not persisted: a menu that reopens by itself on load is a nuisance.
  const [isMenuOpen, setIsMenuOpen] = useState<boolean>(false);

  const [placementMode, setPlacementMode] = usePersistedState<'start' | 'dest'>('placementMode', 'start');
  // Not persisted: refetched from the API, then merged with the separately stored custom boats.
  const [presets, setPresets] = useState<BoatPreset[]>([]);
  const [selectedPresetId, setSelectedPresetId] = usePersistedState<string>('selectedPresetId', '36ft-ketch');

  const [startPoint, setStartPoint] = usePersistedState<Point>('startPoint', ROUTE_PRESETS[0].start);
  const [destPoint, setDestPoint] = usePersistedState<Point>('destPoint', ROUTE_PRESETS[0].dest);
  const [departureTime, setDepartureTime] = usePersistedState<string>(
    'departureTime',
    nowDepartureValue,
    { revive: reviveDepartureTime }
  );
  const [timeStepHours, setTimeStepHours] = usePersistedState<number>('timeStepHours', () => {
    const d = calcDirectDistanceNM(ROUTE_PRESETS[0].start, ROUTE_PRESETS[0].dest);
    return getSaneDefaultTimeStepHours(d);
  });
  const [tackPenaltyMinutes, setTackPenaltyMinutes] = usePersistedState<number>('tackPenaltyMinutes', 5.0);
  const [gybePenaltyMinutes, setGybePenaltyMinutes] = usePersistedState<number>('gybePenaltyMinutes', 8.0);

  const [loading, setLoading] = useState<boolean>(false);
  const [activeModel, setActiveModel] = usePersistedState<WeatherModelId>('activeModel', DEFAULT_WEATHER_MODEL);

  // Computed routes are restored too, so the map comes back with the passage already on it.
  // Isochrone geometry is shed first if storage runs out; see services/persistence.
  const [routeResult, setRouteResult] = usePersistedState<RouteResult | null>('routeResult', null, {
    shrink: [dropIsochrones],
  });
  const [multiRouteResult, setMultiRouteResult] = usePersistedState<MultiRouteResult | null>(
    'multiRouteResult',
    null,
    { shrink: [dropIsochronesFromAll] }
  );
  const [currentWaypointIndex, setCurrentWaypointIndex] = usePersistedState<number>('currentWaypointIndex', 0);

  // Not persisted: both are refetched for the restored waypoints and time on mount.
  const [weatherGrid, setWeatherGrid] = useState<WeatherGridResponse | null>(null);
  const [landmaskPolygons, setLandmaskPolygons] = useState<LandmaskPolygon[]>([]);

  // Layer Defaults: Active Wind & Barbs and Landmass Polygons enabled by default
  const [showWindGrid, setShowWindGrid] = usePersistedState<boolean>('showWindGrid', true);
  const [showIsochrones, setShowIsochrones] = usePersistedState<boolean>('showIsochrones', false);
  const [showLandmask, setShowLandmask] = usePersistedState<boolean>('showLandmask', true);

  // Weather grid cache by model and timestamp
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

  // Load weather grid dynamically for the active model and timestamp
  const loadWeatherForTime = useCallback(
    async (timeStr: string, modelId: string = activeModel) => {
      if (!showWindGrid) return;

      const cacheKey = `${modelId}_${startPoint.lat.toFixed(1)}_${startPoint.lon.toFixed(1)}_${destPoint.lat.toFixed(1)}_${destPoint.lon.toFixed(1)}_${timeStr}`;
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
          model: modelId,
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
        setWeatherGrid(null);
      }
    },
    [startPoint, destPoint, showWindGrid, activeModel]
  );

  // Trigger weather reload whenever activeTime, activeModel, or start/dest points change
  useEffect(() => {
    loadWeatherForTime(activeTime, activeModel);
  }, [activeTime, activeModel, loadWeatherForTime]);

  // Snapshot of parameters used when current routes were calculated. Persisted alongside the
  // routes so the "needs recalculating" indicator stays truthful across a reload — in particular
  // when a stale departure time was snapped forward on restore.
  const [solvedParams, setSolvedParams] = usePersistedState<{
    start: Point;
    dest: Point;
    departureTime: string;
    boatPresetId: string;
    tackPenaltyMinutes: number;
    gybePenaltyMinutes: number;
    timeStepHours: number;
  } | null>('solvedParams', null);

  // A restored waypoint index can outrun the route it belongs to, if the route was shed under
  // storage pressure or replaced by a shorter one. Keep it addressable.
  useEffect(() => {
    const waypointCount = routeResult?.waypoints.length ?? 0;
    if (waypointCount === 0) {
      if (currentWaypointIndex !== 0) setCurrentWaypointIndex(0);
    } else if (currentWaypointIndex > waypointCount - 1) {
      setCurrentWaypointIndex(waypointCount - 1);
    }
  }, [routeResult, currentWaypointIndex, setCurrentWaypointIndex]);

  const normalizeTimeStr = (t?: string) => {
    if (!t) return '';
    try {
      return new Date(t).toISOString().slice(0, 16);
    } catch {
      return t.slice(0, 16);
    }
  };

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

  // Handle Multi-Model Route Calculation
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
        model: 'all',
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

      if (result.routes && Object.keys(result.routes).length > 0) {
        setMultiRouteResult(result.routes);
        const resolvedActive = result.routes[activeModel] ? activeModel : result.active_model || Object.keys(result.routes)[0];
        setActiveModel(resolvedActive);
        setRouteResult(result.routes[resolvedActive]);
      } else {
        setMultiRouteResult({ [activeModel]: result });
        setRouteResult(result);
      }

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

  // Switch Active Model Handler
  const handleActiveModelChange = useCallback((newModelId: WeatherModelId) => {
    setActiveModel(newModelId);
    if (multiRouteResult && multiRouteResult[newModelId]) {
      setRouteResult(multiRouteResult[newModelId]);
      setCurrentWaypointIndex(0);
    }
  }, [multiRouteResult]);

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
  }, [destPoint, setStartPoint, setTimeStepHours]);

  const handleDestPointChange = useCallback((newDest: Point) => {
    setDestPoint(newDest);
    weatherCacheRef.current.clear();
    const d = calcDirectDistanceNM(startPoint, newDest);
    setTimeStepHours(getSaneDefaultTimeStepHours(d));
  }, [startPoint, setDestPoint, setTimeStepHours]);

  // Waypoint moves made on the map. A solved route belongs to the waypoints it was solved for, so
  // moving one discards it — confirm first, then hand back an uncalculated route.
  const [pendingWaypointMove, setPendingWaypointMove] = useState<{
    which: 'start' | 'dest';
    point: Point;
    source: WaypointChangeSource;
  } | null>(null);

  // Bumped when a move is declined. MapView redraws its markers from props, but declining changes
  // no props, so without this nudge the dragged marker would stay at the rejected position.
  const [waypointRevertNonce, setWaypointRevertNonce] = useState<number>(0);

  const hasCalculatedRoute = Boolean(routeResult || multiRouteResult);

  /** Clears everything derived from a solved route, leaving the inputs untouched. */
  const resetRouteState = useCallback(() => {
    setRouteResult(null);
    setMultiRouteResult(null);
    setSolvedParams(null);
    setCurrentWaypointIndex(0);
    weatherCacheRef.current.clear();
  }, [setRouteResult, setMultiRouteResult, setSolvedParams, setCurrentWaypointIndex]);

  const applyWaypointMove = useCallback(
    (which: 'start' | 'dest', point: Point, source: WaypointChangeSource) => {
      if (which === 'start') handleStartPointChange(point);
      else handleDestPointChange(point);
      // Click-to-place advances to the other waypoint, but only now that the move has stuck.
      if (source === 'click') setPlacementMode(which === 'start' ? 'dest' : 'start');
    },
    [handleStartPointChange, handleDestPointChange, setPlacementMode]
  );

  // With nothing calculated there is nothing to lose, so the move applies straight away.
  const requestMapStartChange = useCallback(
    (point: Point, source: WaypointChangeSource) => {
      if (!hasCalculatedRoute) return applyWaypointMove('start', point, source);
      setPendingWaypointMove({ which: 'start', point, source });
    },
    [hasCalculatedRoute, applyWaypointMove]
  );

  const requestMapDestChange = useCallback(
    (point: Point, source: WaypointChangeSource) => {
      if (!hasCalculatedRoute) return applyWaypointMove('dest', point, source);
      setPendingWaypointMove({ which: 'dest', point, source });
    },
    [hasCalculatedRoute, applyWaypointMove]
  );

  const confirmWaypointMove = useCallback(() => {
    if (!pendingWaypointMove) return;
    const { which, point, source } = pendingWaypointMove;
    applyWaypointMove(which, point, source);
    resetRouteState();
    setPendingWaypointMove(null);
  }, [pendingWaypointMove, applyWaypointMove, resetRouteState]);

  const cancelWaypointMove = useCallback(() => {
    setPendingWaypointMove(null);
    setWaypointRevertNonce((n) => n + 1);
  }, []);

  return (
    <div className="app-container">
      {/* Top Floating Navigation Cluster */}
      <div className="top-navigation-cluster">
        
        <div className="top-nav-primary-row">
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
                    className={`hamburger-item ${activeView === 'window-finder' ? 'active' : ''}`}
                    onClick={() => {
                      setActiveView('window-finder');
                      setIsMenuOpen(false);
                    }}
                  >
                    <div className="hamburger-item-icon text-amber">
                      <CalendarRange size={17} />
                    </div>
                    <div className="hamburger-item-text">
                      <span className="hamburger-item-name">Weather Window Finder</span>
                      <span className="hamburger-item-sub">Ranked departures &amp; passage comfort</span>
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

          {/* 2. Top Tabs (Map View vs Passage Statistics vs Return to Map) */}
          {activeView === 'routing' ? (
            <div className="app-tabs-bar">
              <button
                type="button"
                className={`tab-nav-btn ${routingSubTab === 'map' ? 'active' : ''}`}
                onClick={() => setRoutingSubTab('map')}
              >
                <Compass size={15} />
                <span className="tab-label-full">Map View</span>
                <span className="tab-label-short">Map</span>
              </button>
              <button
                type="button"
                className={`tab-nav-btn ${routingSubTab === 'stats' ? 'active' : ''}`}
                onClick={() => setRoutingSubTab('stats')}
              >
                <BarChart3 size={15} />
                <span className="tab-label-full">Passage Statistics</span>
                <span className="tab-label-short">Stats</span>
              </button>
            </div>
          ) : activeView === 'window-finder' ? (
            <div className="app-tabs-bar">
              <button
                type="button"
                className="tab-nav-btn"
                onClick={() => {
                  setActiveView('routing');
                  setRoutingSubTab('map');
                }}
              >
                <Compass size={15} />
                <span className="tab-label-full">Return to Map</span>
                <span className="tab-label-short">Map</span>
              </button>
            </div>
          ) : null}
        </div>

        {/* 3. Current Simulation Time Chip with Active Model indicator */}
        {activeView === 'routing' && activeTime && (
          <div className="simulation-time-chip" title="Current Simulation Time (UTC)">
            <Clock size={13} className="sim-clock-icon" />
            <span className="sim-time-text">
              {(() => {
                try {
                  const d = new Date(activeTime);
                  const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
                  const day = d.getUTCDate();
                  const month = months[d.getUTCMonth()];
                  const hours = String(d.getUTCHours()).padStart(2, '0');
                  const mins = String(d.getUTCMinutes()).padStart(2, '0');
                  return `${day} ${month} • ${hours}:${mins} UTC`;
                } catch {
                  return activeTime;
                }
              })()}
            </span>
            {routeResult && (
              <span
                className="sim-model-tag"
                style={{
                  backgroundColor: WEATHER_MODELS[activeModel]?.badgeBg || 'rgba(56, 189, 248, 0.15)',
                  color: WEATHER_MODELS[activeModel]?.lightColor || '#38bdf8',
                  border: `1px solid ${WEATHER_MODELS[activeModel]?.badgeBorder || 'rgba(56, 189, 248, 0.3)'}`,
                }}
              >
                {WEATHER_MODELS[activeModel]?.shortName || activeModel}
              </span>
            )}
            {routeResult && routeResult.waypoints[currentWaypointIndex] && (
              <span className="sim-elapsed-badge">
                +{Math.round(
                  Math.max(
                    0,
                    (new Date(routeResult.waypoints[currentWaypointIndex].time).getTime() -
                      new Date(routeResult.start_time).getTime()) /
                      (3600 * 1000)
                  )
                )}h
              </span>
            )}
          </div>
        )}

      </div>

      {/* Main View Area */}
      <main className="main-view full-screen-view">
        
        {/* 1. Weather Routing View (Map View & Passage Statistics) */}
        {activeView === 'routing' && (
          <>
            {routingSubTab === 'map' ? (
              <>
                <MapView
                  startPoint={startPoint}
                  destPoint={destPoint}
                  onStartChange={requestMapStartChange}
                  onDestChange={requestMapDestChange}
                  waypointRevertNonce={waypointRevertNonce}
                  placementMode={placementMode}
                  routeResult={routeResult}
                  multiRouteResult={multiRouteResult}
                  activeModel={activeModel}
                  onSelectModel={handleActiveModelChange}
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
                    activeModel={activeModel}
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
                  multiRouteResult={multiRouteResult}
                  activeModel={activeModel}
                  onActiveModelChange={handleActiveModelChange}
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
                  <PassageStatistics
                    routeResult={routeResult}
                    multiRouteResult={multiRouteResult}
                    activeModel={activeModel}
                    onSelectModel={handleActiveModelChange}
                  />
                ) : (
                  <div className="stats-empty-state">
                    <BarChart3 size={48} color="#38bdf8" />
                    <h2>No Route Computed Yet</h2>
                    <p>Calculate an optimal route on the Map View to inspect multi-model performance plots and passage telemetry.</p>
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

        {/* 2. Settings View */}
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

        {/* 3. VPP Dedicated Page */}
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

        {/* 4. Weather Window Finder Dedicated View */}
        {activeView === 'window-finder' && (
          <WeatherWindowFinder
            startPoint={startPoint}
            destPoint={destPoint}
            onStartChange={handleStartPointChange}
            onDestChange={handleDestPointChange}
            presets={presets}
            selectedPresetId={selectedPresetId}
            onSelectPreset={setSelectedPresetId}
            onSelectWindowRoute={(route, focusTime) => {
              setRouteResult(route);
              const modelKey = route.model_id || activeModel;
              setMultiRouteResult({ [modelKey]: route });
              if (route.model_id) {
                setActiveModel(route.model_id);
              }
              if (focusTime && route.waypoints && route.waypoints.length > 0) {
                const targetMs = new Date(focusTime).getTime();
                let closestIdx = 0;
                let minDiff = Infinity;
                route.waypoints.forEach((wp, idx) => {
                  const diff = Math.abs(new Date(wp.time).getTime() - targetMs);
                  if (diff < minDiff) {
                    minDiff = diff;
                    closestIdx = idx;
                  }
                });
                setCurrentWaypointIndex(closestIdx);
              } else {
                setCurrentWaypointIndex(0);
              }
              setDepartureTime(new Date(route.start_time).toISOString().slice(0, 16));
              setActiveView('routing');
              setRoutingSubTab('map');
            }}
            onOpenMapPlacement={() => {
              setActiveView('routing');
              setRoutingSubTab('map');
            }}
          />
        )}

      </main>

      {pendingWaypointMove && (
        <ConfirmDialog
          title={pendingWaypointMove.which === 'start' ? 'Move start point?' : 'Move finish point?'}
          message={
            <>
              The calculated route was solved for the current waypoints and will be discarded.
              You will need to calculate a new one.
              <span className="confirm-dialog-coords">
                New position: {pendingWaypointMove.point.lat.toFixed(4)}°,{' '}
                {pendingWaypointMove.point.lon.toFixed(4)}°
              </span>
            </>
          }
          confirmLabel="Move and reset route"
          onConfirm={confirmWaypointMove}
          onCancel={cancelWaypointMove}
        />
      )}
    </div>
  );
};

export default App;
