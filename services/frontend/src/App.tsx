import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Sidebar } from './components/Sidebar';
import { MapView } from './components/MapView';
import { TimelineScrubber } from './components/TimelineScrubber';
import { LayerToggles } from './components/LayerToggles';
import { VPPInspector } from './components/VPPInspector';
import { BoatPreset, Point, RouteResult, WeatherGridResponse } from './types';
import { fetchPresets, calculateRoute, fetchWeatherGrid, ROUTE_PRESETS } from './services/api';
import { Map as MapIcon, Gauge } from 'lucide-react';
import './styles/App.css';

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'routing' | 'vpp'>('routing');
  const [presets, setPresets] = useState<BoatPreset[]>([]);
  const [selectedPresetId, setSelectedPresetId] = useState<string>('36ft-ketch');

  const [startPoint, setStartPoint] = useState<Point>(ROUTE_PRESETS[0].start);
  const [destPoint, setDestPoint] = useState<Point>(ROUTE_PRESETS[0].dest);
  const [departureTime, setDepartureTime] = useState<string>(
    new Date().toISOString().slice(0, 16)
  );
  const [timeStepHours, setTimeStepHours] = useState<number>(5 / 60);
  const [tackPenaltyMinutes, setTackPenaltyMinutes] = useState<number>(5.0);
  const [gybePenaltyMinutes, setGybePenaltyMinutes] = useState<number>(8.0);

  const [loading, setLoading] = useState<boolean>(false);
  const [routeResult, setRouteResult] = useState<RouteResult | null>(null);
  const [currentWaypointIndex, setCurrentWaypointIndex] = useState<number>(0);

  const [weatherGrid, setWeatherGrid] = useState<WeatherGridResponse | null>(null);
  const [showIsochrones, setShowIsochrones] = useState<boolean>(true);
  const [showWindGrid, setShowWindGrid] = useState<boolean>(true);

  // Weather grid cache by timestamp
  const weatherCacheRef = useRef<Map<string, WeatherGridResponse>>(new Map());

  // 1. Initial presets load
  useEffect(() => {
    fetchPresets().then((data) => {
      setPresets(data);
      if (data.length > 0) {
        setSelectedPresetId(data[0].id);
      }
    });
  }, []);

  // Determine active forecast timestamp
  const activeTime =
    routeResult && routeResult.waypoints[currentWaypointIndex]
      ? routeResult.waypoints[currentWaypointIndex].time
      : departureTime;

  // 2. Load weather grid dynamically for the active timestamp
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

  // 3. Handle Route Calculation
  const handleCalculateRoute = async () => {
    setLoading(true);
    // Clear weather cache when calculating a new route
    weatherCacheRef.current.clear();
    try {
      const result = await calculateRoute({
        start: startPoint,
        dest: destPoint,
        startTime: departureTime,
        boatPreset: selectedPresetId,
        timeStepHours,
        tackPenaltyMinutes,
        gybePenaltyMinutes,
      });
      setRouteResult(result);
      setCurrentWaypointIndex(0);
      setActiveTab('routing');
    } catch (err: any) {
      alert(`Route Calculation Failed: ${err.message || err}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="app-container">
      {/* Top Main Navigation Tabs */}
      <div className="app-tabs-bar">
        <button
          className={`tab-nav-btn ${activeTab === 'routing' ? 'active' : ''}`}
          onClick={() => setActiveTab('routing')}
        >
          <MapIcon size={16} />
          <span>Weather Routing & Passage</span>
        </button>
        <button
          className={`tab-nav-btn ${activeTab === 'vpp' ? 'active' : ''}`}
          onClick={() => setActiveTab('vpp')}
        >
          <Gauge size={16} />
          <span>VPP Sailboat Performance & Polars</span>
        </button>
      </div>

      <Sidebar
        presets={presets}
        selectedPresetId={selectedPresetId}
        onSelectPreset={setSelectedPresetId}
        startPoint={startPoint}
        destPoint={destPoint}
        onStartChange={(p) => {
          setStartPoint(p);
          weatherCacheRef.current.clear();
        }}
        onDestChange={(p) => {
          setDestPoint(p);
          weatherCacheRef.current.clear();
        }}
        departureTime={departureTime}
        onDepartureTimeChange={(t) => {
          setDepartureTime(t);
          weatherCacheRef.current.clear();
        }}
        timeStepHours={timeStepHours}
        onTimeStepChange={setTimeStepHours}
        tackPenaltyMinutes={tackPenaltyMinutes}
        onTackPenaltyChange={setTackPenaltyMinutes}
        gybePenaltyMinutes={gybePenaltyMinutes}
        onGybePenaltyChange={setGybePenaltyMinutes}
        onCalculateRoute={handleCalculateRoute}
        loading={loading}
        routeResult={routeResult}
      />

      <div className="main-view">
        {activeTab === 'routing' ? (
          <>
            <MapView
              startPoint={startPoint}
              destPoint={destPoint}
              routeResult={routeResult}
              currentWaypointIndex={currentWaypointIndex}
              weatherGrid={weatherGrid}
              showIsochrones={showIsochrones}
              showWindGrid={showWindGrid}
            />

            <LayerToggles
              showIsochrones={showIsochrones}
              onToggleIsochrones={() => setShowIsochrones(!showIsochrones)}
              showWindGrid={showWindGrid}
              onToggleWindGrid={() => setShowWindGrid(!showWindGrid)}
              activeTime={activeTime}
            />

            {routeResult && (
              <TimelineScrubber
                routeResult={routeResult}
                currentIndex={currentWaypointIndex}
                onIndexChange={setCurrentWaypointIndex}
              />
            )}
          </>
        ) : (
          <VPPInspector
            presets={presets}
            selectedPresetId={selectedPresetId}
            onSelectPreset={setSelectedPresetId}
          />
        )}
      </div>
    </div>
  );
};

export default App;
