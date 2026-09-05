import React, { useState, useMemo, useEffect, useRef } from 'react';
import {
  BoatPreset,
  Point,
  RouteResult,
  WeatherModelId,
  WEATHER_MODELS,
  WeatherWindowResponse,
} from '../types';
import {
  findWeatherWindows,
  ROUTE_PRESETS,
  calcDirectDistanceNM,
} from '../services/api';
import { usePersistedState, reviveDepartureTime } from '../services/persistence';
import {
  POINT_OF_SAIL_METAS,
  getPointOfSail,
} from '../config/pointOfSail';
import { WindRose } from './WindRose';
import {
  Calendar,
  CalendarRange,
  Compass,
  Wind,
  Waves,
  AlertTriangle,
  ShieldCheck,
  CheckCircle2,
  Clock,
  ArrowRight,
  MapPin,
  Trophy,
  Sparkles,
  Maximize2,
  SlidersHorizontal,
  ChevronDown,
  ChevronUp,
  Layers,
  Moon,
} from 'lucide-react';
import '../styles/WeatherWindowFinder.css';

interface WeatherWindowFinderProps {
  startPoint: Point;
  destPoint: Point;
  onStartChange: (p: Point) => void;
  onDestChange: (p: Point) => void;
  presets: BoatPreset[];
  selectedPresetId: string;
  onSelectPreset: (id: string) => void;
  onSelectWindowRoute: (route: RouteResult, focusTime?: string) => void;
  onOpenMapPlacement?: () => void;
}

type SortOption = 'departure' | 'comfort' | 'duration' | 'wind';
type FilterOption = 'all' | 'no-gale' | 'high-comfort' | 'high-confidence';

/** Start of the default search window: the current hour. */
function defaultEarliestDeparture(): string {
  const d = new Date();
  d.setMinutes(0, 0, 0);
  return d.toISOString().slice(0, 16);
}

/** End of the default search window: five days out. */
function defaultLatestDeparture(): string {
  const d = new Date();
  d.setMinutes(0, 0, 0);
  d.setDate(d.getDate() + 5);
  return d.toISOString().slice(0, 16);
}

function isFutureDeparture(value: string): boolean {
  const ms = Date.parse(`${value}:00Z`);
  return Number.isFinite(ms) && ms > Date.now();
}

export const WeatherWindowFinder: React.FC<WeatherWindowFinderProps> = ({
  startPoint,
  destPoint,
  onStartChange,
  onDestChange,
  presets,
  selectedPresetId,
  onSelectPreset,
  onSelectWindowRoute,
  onOpenMapPlacement,
}) => {
  // Dates. Both bounds are restored, but a bound that has already passed snaps forward: a search
  // window ending in the past would return nothing and look broken.
  const [earliestDeparture, setEarliestDeparture] = usePersistedState<string>(
    'windowFinder.earliestDeparture',
    defaultEarliestDeparture,
    { revive: reviveDepartureTime }
  );

  const [hasEndDate, setHasEndDate] = usePersistedState<boolean>('windowFinder.hasEndDate', true);
  const [latestDeparture, setLatestDeparture] = usePersistedState<string>(
    'windowFinder.latestDeparture',
    defaultLatestDeparture,
    { revive: (saved) => (isFutureDeparture(saved) ? saved : defaultLatestDeparture()) }
  );

  const [selectedModel, setSelectedModel] = usePersistedState<WeatherModelId>(
    'windowFinder.selectedModel',
    'gfs_0p25'
  );
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  // Response data. Restored so returning to this view still shows the ranked departures the user
  // was reading, rather than an empty table.
  const [windowResponse, setWindowResponse] = usePersistedState<WeatherWindowResponse | null>(
    'windowFinder.response',
    null
  );

  // View state: strictly table view, ordered chronologically by departure time
  const [sortOption, setSortOption] = usePersistedState<SortOption>('windowFinder.sort', 'departure');
  const [filterOption, setFilterOption] = usePersistedState<FilterOption>('windowFinder.filter', 'all');
  const [selectedWindowIndex, setSelectedWindowIndex] = usePersistedState<number>(
    'windowFinder.selectedIndex',
    0
  );
  const [expandedRowIndex, setExpandedRowIndex] = usePersistedState<number | null>(
    'windowFinder.expandedRow',
    null
  );

  // Dynamic row positions for continuous single-path SVG comfort plot
  const tableRef = useRef<HTMLTableElement>(null);
  const [rowCenters, setRowCenters] = useState<number[]>([]);
  const [colLeft, setColLeft] = useState<number>(0);
  const [tableHeight, setTableHeight] = useState<number>(0);

  const directDistNM = useMemo(
    () => Math.round(calcDirectDistanceNM(startPoint, destPoint) * 10) / 10,
    [startPoint, destPoint]
  );

  // Approximate passage duration and coarseness summary
  const coarsenessSummary = useMemo(() => {
    const estHours = Math.round(directDistNM / 6.0);
    let winHours = 168; // Default 7d
    if (hasEndDate && latestDeparture && earliestDeparture) {
      const diffMs = new Date(latestDeparture).getTime() - new Date(earliestDeparture).getTime();
      winHours = Math.max(6, Math.round(diffMs / (3600 * 1000)));
    }

    let depStepH = 6;
    let isoStepH = '45 min';
    if (directDistNM <= 100) {
      depStepH = winHours <= 48 ? 3 : 6;
      isoStepH = '20 min';
    } else if (directDistNM <= 350) {
      depStepH = winHours <= 96 ? 6 : 12;
      isoStepH = '45 min';
    } else if (directDistNM <= 800) {
      depStepH = winHours <= 168 ? 12 : 24;
      isoStepH = '90 min';
    } else {
      depStepH = winHours <= 168 ? 12 : 24;
      isoStepH = '2 hours';
    }

    const estWindows = Math.min(24, Math.max(2, Math.round(winHours / depStepH) + 1));
    return {
      estHours,
      winHours,
      depStepH,
      isoStepH,
      estWindows,
    };
  }, [directDistNM, hasEndDate, earliestDeparture, latestDeparture]);

  // Execute weather window calculation
  const handleSearchWindows = async () => {
    setLoading(true);
    setError(null);
    try {
      const selectedBoat = presets.find((p) => p.id === selectedPresetId);
      const resp = await findWeatherWindows({
        start: startPoint,
        dest: destPoint,
        earliest_departure: earliestDeparture,
        latest_departure: hasEndDate ? latestDeparture : undefined,
        boat_preset: selectedPresetId,
        model: selectedModel,
        custom_boat: selectedBoat?.customBoat,
        custom_polar: selectedBoat?.polarData
          ? {
              boat_name: selectedBoat.name,
              tws_list: selectedBoat.polarData.tws_list,
              twa_list: selectedBoat.polarData.twa_list,
              speed_matrix: selectedBoat.polarData.speed_matrix,
            }
          : undefined,
      });

      setWindowResponse(resp);
      setSelectedWindowIndex(0);
      setExpandedRowIndex(null);
    } catch (err: any) {
      setError(err.message || 'Failed to search weather windows');
    } finally {
      setLoading(false);
    }
  };

  // Filtered and sorted windows
  const displayWindows = useMemo(() => {
    if (!windowResponse || !windowResponse.windows) return [];
    let list = [...windowResponse.windows];

    // Filter
    if (filterOption === 'no-gale') {
      list = list.filter((w) => !w.gale_warning);
    } else if (filterOption === 'high-comfort') {
      list = list.filter((w) => w.comfort_score >= 75.0);
    } else if (filterOption === 'high-confidence') {
      list = list.filter((w) => w.confidence_score >= 75.0);
    }

    // Sort
    list.sort((a, b) => {
      switch (sortOption) {
        case 'departure':
          return new Date(a.departure_time).getTime() - new Date(b.departure_time).getTime();
        case 'comfort':
          return b.comfort_score - a.comfort_score;
        case 'duration':
          return a.duration_hours - b.duration_hours;
        case 'wind':
          return a.max_wind_kts - b.max_wind_kts;
        default:
          return new Date(a.departure_time).getTime() - new Date(b.departure_time).getTime();
      }
    });

    return list;
  }, [windowResponse, sortOption, filterOption]);

  // Dynamic Comfort Scale (Min score at left, Max score at right)
  const { minComfort, maxComfort } = useMemo(() => {
    if (!displayWindows || displayWindows.length === 0) return { minComfort: 0, maxComfort: 100 };
    let min = Infinity;
    let max = -Infinity;
    for (const w of displayWindows) {
      if (w.comfort_score < min) min = w.comfort_score;
      if (w.comfort_score > max) max = w.comfort_score;
    }
    if (max - min < 4) {
      min = Math.max(0, min - 2);
      max = Math.min(100, max + 2);
    }
    return { minComfort: min, maxComfort: max };
  }, [displayWindows]);

  // Measure exact vertical center of each table row and column position
  useEffect(() => {
    const measure = () => {
      if (!tableRef.current) return;
      const table = tableRef.current;
      const trs = table.querySelectorAll<HTMLTableRowElement>('tbody tr.ww-tr');
      if (trs.length === 0) return;

      const tableRect = table.getBoundingClientRect();
      const centers: number[] = [];

      trs.forEach((tr) => {
        const rect = tr.getBoundingClientRect();
        centers.push(rect.top - tableRect.top + rect.height / 2);
      });

      const firstComfortTd = trs[0].querySelector<HTMLTableCellElement>('td.ww-td-comfort');
      if (firstComfortTd) {
        const tdRect = firstComfortTd.getBoundingClientRect();
        setColLeft(tdRect.left - tableRect.left);
      }

      setTableHeight(table.offsetHeight);
      setRowCenters(centers);
    };

    measure();

    const t = setTimeout(measure, 60);

    let ro: ResizeObserver | null = null;
    if (typeof ResizeObserver !== 'undefined' && tableRef.current) {
      ro = new ResizeObserver(() => {
        measure();
      });
      ro.observe(tableRef.current);
    }

    return () => {
      clearTimeout(t);
      if (ro) ro.disconnect();
    };
  }, [displayWindows, expandedRowIndex]);

  const handleApplyPreset = (presetName: string) => {
    const found = ROUTE_PRESETS.find((p) => p.name === presetName);
    if (found) {
      onStartChange(found.start);
      onDestChange(found.dest);
    }
  };

  // Continuous SVG path connecting all comfort points smoothly from top to bottom
  const svgPlotWidth = 150;
  const svgPlotPad = 16;
  const trackInnerWidth = svgPlotWidth - svgPlotPad * 2;

  const points = useMemo(() => {
    if (rowCenters.length !== displayWindows.length || displayWindows.length === 0) {
      return [];
    }
    const span = Math.max(0.1, maxComfort - minComfort);
    return displayWindows.map((candidate, i) => {
      const norm = (candidate.comfort_score - minComfort) / span;
      const x = svgPlotPad + Math.max(0, Math.min(1, norm)) * trackInnerWidth;
      const y = rowCenters[i];
      return { x, y, score: candidate.comfort_score };
    });
  }, [rowCenters, displayWindows, minComfort, maxComfort]);

  // Compute smooth bezier segments connecting consecutive points
  const pathSegments = useMemo(() => {
    if (points.length < 2) return [];
    const segs: {
      d: string;
      id: string;
      y1: number;
      y2: number;
      startColor: string;
      endColor: string;
    }[] = [];

    for (let i = 0; i < points.length - 1; i++) {
      const p0 = points[i];
      const p1 = points[i + 1];
      const dy = p1.y - p0.y;
      const cp1x = p0.x;
      const cp1y = p0.y + dy * 0.45;
      const cp2x = p1.x;
      const cp2y = p1.y - dy * 0.45;
      const d = `M ${p0.x.toFixed(1)} ${p0.y.toFixed(1)} C ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${p1.x.toFixed(1)} ${p1.y.toFixed(1)}`;
      segs.push({
        d,
        id: `comfort-seg-${i}`,
        y1: p0.y,
        y2: p1.y,
        startColor: getScoreColor(p0.score),
        endColor: getScoreColor(p1.score),
      });
    }
    return segs;
  }, [points]);

  return (
    <div className="weather-window-page-container">
      {/* Top Controls Bar */}
      <div className="ww-controls-panel">
        <div className="ww-controls-header">
          <div className="ww-title-cluster">
            <div className="ww-header-icon">
              <CalendarRange size={22} color="#38bdf8" />
            </div>
            <div>
              <h1 className="ww-main-title">Weather Window Finder</h1>
              <p className="ww-subtitle">
                Chronological departure analysis • Continuous comfort plot, wave steepness, warnings &amp; representative weather
              </p>
            </div>
          </div>

          {/* Quick Route Preset Selector */}
          <div className="ww-preset-select-wrapper">
            <label className="ww-field-label">Predefined Passages</label>
            <select
              className="ww-select-input"
              value={ROUTE_PRESETS.find(
                (p) =>
                  Math.abs(p.start.lat - startPoint.lat) < 0.05 &&
                  Math.abs(p.dest.lat - destPoint.lat) < 0.05
              )?.name || ''}
              onChange={(e) => handleApplyPreset(e.target.value)}
            >
              <option value="" disabled>Select route preset...</option>
              {ROUTE_PRESETS.map((p) => (
                <option key={p.name} value={p.name}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
        </div>

        {/* Input Parameters Grid */}
        <div className="ww-parameters-grid">
          {/* 1. Start / Destination Coordinates */}
          <div className="ww-card-field">
            <div className="ww-field-row-header">
              <span className="ww-field-label">Passage Waypoints</span>
              <span className="ww-distance-badge">{directDistNM} NM</span>
            </div>
            <div className="ww-waypoints-inputs">
              <div className="ww-waypoint-pill start">
                <MapPin size={13} className="text-emerald" />
                <span className="ww-wp-name">Start:</span>
                <span className="ww-wp-coords">
                  {startPoint.lat.toFixed(2)}°, {startPoint.lon.toFixed(2)}°
                </span>
              </div>
              <ArrowRight size={14} className="ww-wp-arrow" />
              <div className="ww-waypoint-pill dest">
                <MapPin size={13} className="text-rose" />
                <span className="ww-wp-name">Dest:</span>
                <span className="ww-wp-coords">
                  {destPoint.lat.toFixed(2)}°, {destPoint.lon.toFixed(2)}°
                </span>
              </div>
            </div>
            {onOpenMapPlacement && (
              <button
                type="button"
                className="ww-btn-link"
                onClick={onOpenMapPlacement}
              >
                <Compass size={13} />
                <span>Adjust on Interactive Map</span>
              </button>
            )}
          </div>

          {/* 2. Earliest Departure */}
          <div className="ww-card-field">
            <span className="ww-field-label">Earliest Departure (UTC)</span>
            <div className="ww-input-with-icon">
              <Calendar size={15} className="ww-input-icon text-sky" />
              <input
                type="datetime-local"
                className="ww-datetime-input"
                value={earliestDeparture}
                onChange={(e) => setEarliestDeparture(e.target.value)}
              />
            </div>
            <span className="ww-field-hint">Search start boundary</span>
          </div>

          {/* 3. Latest Departure (Optional) */}
          <div className="ww-card-field">
            <div className="ww-field-row-header">
              <span className="ww-field-label">Latest Departure (Optional)</span>
              <button
                type="button"
                className="ww-btn-text-toggle"
                onClick={() => setHasEndDate(!hasEndDate)}
              >
                {hasEndDate ? 'Clear (Auto 7d)' : 'Set End Date'}
              </button>
            </div>
            {hasEndDate ? (
              <div className="ww-input-with-icon">
                <Clock size={15} className="ww-input-icon text-purple" />
                <input
                  type="datetime-local"
                  className="ww-datetime-input"
                  value={latestDeparture}
                  onChange={(e) => setLatestDeparture(e.target.value)}
                />
              </div>
            ) : (
              <div className="ww-disabled-end-box">
                <span>Open-ended window (evaluates 7–10 days ahead)</span>
              </div>
            )}
            <span className="ww-field-hint">
              {hasEndDate ? `${coarsenessSummary.winHours}h search span` : 'Default 7-day forecast horizon'}
            </span>
          </div>

          {/* 4. Yacht & Model Selection */}
          <div className="ww-card-field">
            <span className="ww-field-label">Boat &amp; Weather Model</span>
            <div className="ww-dual-selects">
              <select
                className="ww-select-input-sm"
                value={selectedPresetId}
                onChange={(e) => onSelectPreset(e.target.value)}
              >
                {presets.map((b) => (
                  <option key={b.id} value={b.id}>
                    {b.name}
                  </option>
                ))}
              </select>

              <select
                className="ww-select-input-sm"
                value={selectedModel}
                onChange={(e) => setSelectedModel(e.target.value)}
              >
                {Object.values(WEATHER_MODELS).map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.shortName}
                  </option>
                ))}
              </select>
            </div>
            <div className="ww-coarseness-chip" title="Adaptive calculation resolution">
              <SlidersHorizontal size={12} />
              <span>Step: {coarsenessSummary.depStepH}h • Iso: {coarsenessSummary.isoStepH}</span>
            </div>
          </div>
        </div>

        {/* Primary Action Button & Error message */}
        <div className="ww-action-row">
          {error && (
            <div className="ww-error-banner">
              <AlertTriangle size={16} />
              <span>{error}</span>
            </div>
          )}

          <div className="ww-action-right">
            <button
              type="button"
              className="ww-btn-search"
              onClick={handleSearchWindows}
              disabled={loading}
            >
              {loading ? (
                <>
                  <div className="ww-spinner" />
                  <span>Evaluating {coarsenessSummary.estWindows} Departure Windows...</span>
                </>
              ) : (
                <>
                  <Sparkles size={16} />
                  <span>Find Best Weather Windows</span>
                </>
              )}
            </button>
          </div>
        </div>
      </div>

      {/* Main Results Section */}
      {windowResponse && windowResponse.windows && windowResponse.windows.length > 0 && (
        <div className="ww-results-section">
          {/* Sort & Filter Controls Bar */}
          <div className="ww-results-toolbar">
            <div className="ww-results-counts">
              <span className="ww-results-title">Found {windowResponse.windows.length} Weather Windows</span>
              <span className="ww-results-subtitle">
                Departures from {formatDateDisplay(windowResponse.earliest_departure)} to {formatDateDisplay(windowResponse.latest_departure)}
              </span>
            </div>

            <div className="ww-toolbar-actions">
              {/* Filter Pills */}
              <div className="ww-filter-group">
                <button
                  type="button"
                  className={`ww-filter-pill ${filterOption === 'all' ? 'active' : ''}`}
                  onClick={() => setFilterOption('all')}
                >
                  All ({windowResponse.windows.length})
                </button>
                <button
                  type="button"
                  className={`ww-filter-pill ${filterOption === 'no-gale' ? 'active' : ''}`}
                  onClick={() => setFilterOption('no-gale')}
                >
                  <ShieldCheck size={13} />
                  <span>No Gales</span>
                </button>
                <button
                  type="button"
                  className={`ww-filter-pill ${filterOption === 'high-comfort' ? 'active' : ''}`}
                  onClick={() => setFilterOption('high-comfort')}
                >
                  <Sparkles size={13} />
                  <span>Comfort &ge;75</span>
                </button>
                <button
                  type="button"
                  className={`ww-filter-pill ${filterOption === 'high-confidence' ? 'active' : ''}`}
                  onClick={() => setFilterOption('high-confidence')}
                >
                  <CheckCircle2 size={13} />
                  <span>High Conf</span>
                </button>
              </div>

              {/* Sort Dropdown */}
              <div className="ww-sort-group">
                <span className="ww-sort-label">Order:</span>
                <select
                  className="ww-sort-select"
                  value={sortOption}
                  onChange={(e) => setSortOption(e.target.value as SortOption)}
                >
                  <option value="departure">Chronological (Earliest first)</option>
                  <option value="comfort">Ranked by Comfort (Best first)</option>
                  <option value="duration">Fastest Passage (Duration)</option>
                  <option value="wind">Lowest Peak Wind</option>
                </select>
              </div>
            </div>
          </div>

          {/* TABLE VIEW */}
          <div className="ww-table-container">
            <div className="ww-table-scroll-wrapper">
              <table className="ww-table" ref={tableRef}>
                <thead>
                  <tr>
                    <th className="ww-th ww-th-dep">Departure Time</th>
                    <th className="ww-th ww-th-comfort">
                      <span>Comfort Rating</span>
                    </th>
                    <th className="ww-th ww-th-duration">Passage Duration</th>
                    <th className="ww-th ww-th-pos">Point of Sail</th>
                    <th className="ww-th ww-th-waves">Waves &amp; Sea State</th>
                    <th className="ww-th ww-th-wind">Wind &amp; Heel</th>
                    <th className="ww-th ww-th-warn">Warnings</th>
                    <th className="ww-th ww-th-action">Representative Map</th>
                  </tr>
                </thead>
                <tbody>
                  {displayWindows.map((candidate, idx) => {
                    const isExpanded = expandedRowIndex === idx;
                    const isSelected = selectedWindowIndex === idx;
                    const scoreColor = getScoreColor(candidate.comfort_score);
                    const confStyle = getConfidenceStyle(candidate.confidence_score);

                    // Compute centrally defined Point of Sail distribution
                    const posStats = computePointOfSailDistribution(candidate.route);

                    return (
                      <React.Fragment key={`${candidate.departure_time}_${idx}`}>
                        <tr
                          className={`ww-tr ${isSelected ? 'selected' : ''} ${isExpanded ? 'expanded' : ''}`}
                          onClick={() => {
                            setSelectedWindowIndex(idx);
                            setExpandedRowIndex(isExpanded ? null : idx);
                          }}
                        >
                          {/* Column 1: Departure Time */}
                          <td className="ww-td ww-td-dep">
                            <div className="ww-td-dep-content">
                              <span className="ww-td-dep-date">
                                {formatDateDisplay(candidate.departure_time)}
                              </span>
                              <span className="ww-td-dep-utc">
                                {formatTimeUTC(candidate.departure_time)}
                              </span>
                              <div className="ww-td-rank-chip">
                                {candidate.comfort_rank === 1 ? (
                                  <span className="ww-rank-pill gold">
                                    <Trophy size={11} /> #1 Best
                                  </span>
                                ) : candidate.comfort_rank === 2 ? (
                                  <span className="ww-rank-pill silver">
                                    <Sparkles size={11} /> #2
                                  </span>
                                ) : (
                                  <span className="ww-rank-pill normal">
                                    #{candidate.comfort_rank}
                                  </span>
                                )}
                              </div>
                            </div>
                          </td>

                          {/* Column 2: Comfort Column (Track space on left, score badge on right) */}
                          <td className="ww-td ww-td-comfort">
                            <div className="ww-comfort-cell">
                              <div className="ww-comfort-track-spacer" style={{ width: svgPlotWidth }} />
                              <div
                                className="ww-comfort-score-badge"
                                style={{ borderColor: scoreColor, color: scoreColor }}
                              >
                                {Math.round(candidate.comfort_score)}
                              </div>
                            </div>
                          </td>

                          {/* Column 3: Passage Duration */}
                          <td className="ww-td ww-td-duration">
                            <div className="ww-td-duration-content">
                              <div className="ww-duration-top-row">
                                <span className="ww-duration-badge">
                                  {formatDurationHours(candidate.duration_hours)}
                                </span>
                                <span
                                  className="ww-conf-chip"
                                  style={{
                                    color: confStyle.color,
                                    backgroundColor: confStyle.backgroundColor,
                                    borderColor: confStyle.borderColor,
                                  }}
                                  title={`Forecast confidence: ${Math.round(candidate.confidence_score)}% (based on ensemble spread and forecast lead time)`}
                                >
                                  {Math.round(candidate.confidence_score)}% Conf
                                </span>
                              </div>
                              <span className="ww-arr-text">
                                Arr: {formatDateDisplay(candidate.arrival_time)} {formatTimeUTC(candidate.arrival_time)}
                              </span>
                              <span className="ww-dist-speed-text">
                                {candidate.distance_nm} NM &bull; {(candidate.distance_nm / candidate.duration_hours).toFixed(1)} kts avg
                              </span>
                            </div>
                          </td>

                          {/* Column 4: Point of Sail (Centrally Defined Thresholds & Colors) */}
                          <td className="ww-td ww-td-pos">
                            <div className="ww-td-pos-content">
                              <div className="ww-pos-label-row">
                                <span style={{ color: posStats.topColor, fontWeight: 700 }}>
                                  {posStats.topPct.toFixed(0)}% {posStats.topLabel}
                                </span>
                                {posStats.topPos !== 'beam_reach' && posStats.pctBeamReach > 0 && (
                                  <span style={{ color: POINT_OF_SAIL_METAS.beam_reach.color, fontSize: '0.68rem' }}>
                                    {posStats.pctBeamReach.toFixed(0)}% Beam
                                  </span>
                                )}
                                {posStats.pctCloseHauled > 0 && posStats.topPos !== 'close_hauled' && (
                                  <span style={{ color: POINT_OF_SAIL_METAS.close_hauled.color, fontSize: '0.68rem' }}>
                                    {posStats.pctCloseHauled.toFixed(0)}% Upwind
                                  </span>
                                )}
                              </div>

                              <div
                                className="ww-pos-mini-bar"
                                title={`Close Hauled (<60°): ${posStats.pctCloseHauled.toFixed(1)}% • Close Reach (60-75°): ${posStats.pctCloseReach.toFixed(1)}% • Beam Reach (75-105°): ${posStats.pctBeamReach.toFixed(1)}% • Broad Reach (105-150°): ${posStats.pctBroadReach.toFixed(1)}% • Downwind (150-180°): ${posStats.pctDeadDownwind.toFixed(1)}%`}
                              >
                                {posStats.pctCloseHauled > 0 && (
                                  <div
                                    className="ww-pos-seg"
                                    style={{
                                      width: `${posStats.pctCloseHauled}%`,
                                      backgroundColor: POINT_OF_SAIL_METAS.close_hauled.color,
                                    }}
                                  />
                                )}
                                {posStats.pctCloseReach > 0 && (
                                  <div
                                    className="ww-pos-seg"
                                    style={{
                                      width: `${posStats.pctCloseReach}%`,
                                      backgroundColor: POINT_OF_SAIL_METAS.close_reach.color,
                                    }}
                                  />
                                )}
                                {posStats.pctBeamReach > 0 && (
                                  <div
                                    className="ww-pos-seg"
                                    style={{
                                      width: `${posStats.pctBeamReach}%`,
                                      backgroundColor: POINT_OF_SAIL_METAS.beam_reach.color,
                                    }}
                                  />
                                )}
                                {posStats.pctBroadReach > 0 && (
                                  <div
                                    className="ww-pos-seg"
                                    style={{
                                      width: `${posStats.pctBroadReach}%`,
                                      backgroundColor: POINT_OF_SAIL_METAS.broad_reach.color,
                                    }}
                                  />
                                )}
                                {posStats.pctDeadDownwind > 0 && (
                                  <div
                                    className="ww-pos-seg"
                                    style={{
                                      width: `${posStats.pctDeadDownwind}%`,
                                      backgroundColor: POINT_OF_SAIL_METAS.dead_downwind.color,
                                    }}
                                  />
                                )}
                              </div>
                            </div>
                          </td>

                          {/* Column 5: Waves & Sea State */}
                          <td className="ww-td ww-td-waves">
                            <div className="ww-td-waves-content">
                              <span className="ww-wave-main">
                                {candidate.avg_wave_height_m}m <span className="ww-wave-sub">(max {candidate.max_wave_height_m}m)</span>
                              </span>
                              <span className="ww-wave-period">
                                {candidate.avg_wave_period_s}s period
                              </span>
                              <span className={`ww-sea-state-tag ${candidate.max_wave_height_m > 2.5 ? 'rough' : 'gentle'}`}>
                                {candidate.max_wave_height_m > 2.5 ? 'Rough Seas' : 'Gentle Swell'}
                              </span>
                            </div>
                          </td>

                          {/* Column 6: Wind & Heel */}
                          <td className="ww-td ww-td-wind">
                            <div className="ww-td-wind-content">
                              <span className="ww-wind-main">
                                {candidate.avg_wind_kts} <span className="ww-wind-sub">/ {candidate.max_wind_kts} kts</span>
                              </span>
                              <span className="ww-heel-text">
                                {candidate.max_heel_deg}° heel &bull; {candidate.total_tacks} tacks
                              </span>
                            </div>
                          </td>

                          {/* Column 7: Safety Warnings */}
                          <td className="ww-td ww-td-warn">
                            <div className="ww-td-warn-content">
                              {candidate.gale_warning && (
                                <span className="ww-warn-badge gale" title={candidate.gale_warning_detail}>
                                  <AlertTriangle size={12} /> Gale Warn
                                </span>
                              )}
                              {candidate.night_arrival_warning && (
                                <span className="ww-warn-badge night" title={candidate.night_arrival_warning_detail}>
                                  <Moon size={12} /> Night Arr
                                </span>
                              )}
                              {candidate.low_wind_warning && (
                                <span className="ww-warn-badge low-wind" title={candidate.low_wind_warning_detail}>
                                  <Wind size={12} /> Low Wind
                                </span>
                              )}
                            </div>
                          </td>

                          {/* Column 8: Representative Weather Map & Action */}
                          <td className="ww-td ww-td-action">
                            <div className="ww-td-action-content">
                              <div
                                className="ww-td-map-thumb"
                                title={`Representative: ${candidate.representative_event?.description}`}
                              >
                                <RepresentativeWeatherMiniMap
                                  route={candidate.route}
                                  event={candidate.representative_event}
                                  width={110}
                                  height={55}
                                />
                              </div>

                              <button
                                type="button"
                                className="ww-btn-map-inspect"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  onSelectWindowRoute(
                                    candidate.route,
                                    candidate.representative_event?.time || candidate.departure_time
                                  );
                                }}
                                title="Open full route and weather on the interactive map"
                              >
                                <Maximize2 size={13} />
                                <span>Inspect</span>
                              </button>

                              <button
                                type="button"
                                className="ww-btn-expand-row"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setExpandedRowIndex(isExpanded ? null : idx);
                                }}
                                title="Toggle details"
                              >
                                {isExpanded ? <ChevronUp size={15} /> : <ChevronDown size={15} />}
                              </button>
                            </div>
                          </td>
                        </tr>

                        {/* Expandable Inline Details Drawer */}
                        {isExpanded && (
                          <tr className="ww-expanded-row">
                            <td colSpan={8} className="ww-expanded-cell">
                              <div className="ww-expanded-drawer">
                                <div className="ww-expanded-left">
                                  <div className="ww-expanded-header">
                                    <span className="ww-drawer-title">
                                      Representative Weather Map: {candidate.representative_event?.description}
                                    </span>
                                    <span className="ww-drawer-time">
                                      {formatDateDisplay(candidate.representative_event?.time)} • {formatTimeUTC(candidate.representative_event?.time)}
                                    </span>
                                  </div>
                                  <div className="ww-expanded-map-wrap">
                                    <RepresentativeWeatherMiniMap
                                      route={candidate.route}
                                      event={candidate.representative_event}
                                      width={480}
                                      height={180}
                                    />
                                  </div>
                                </div>

                                <div className="ww-expanded-right">
                                  <h4 className="ww-expanded-h4">Passage Breakdown &amp; Analysis</h4>
                                  
                                  <div className="ww-expanded-pos-card">
                                    <div className="ww-expanded-pos-header">
                                      <span className="ww-pos-card-title">Point of Sail Distribution (% Time)</span>
                                      <div className="ww-expanded-pos-legend">
                                        <span className="pos-legend-item">
                                          <span className="pos-dot" style={{ backgroundColor: POINT_OF_SAIL_METAS.close_hauled.color }} />
                                          Close Hauled (&lt;60°) — {posStats.pctCloseHauled.toFixed(1)}%
                                        </span>
                                        <span className="pos-legend-item">
                                          <span className="pos-dot" style={{ backgroundColor: POINT_OF_SAIL_METAS.close_reach.color }} />
                                          Close Reach (60-75°) — {posStats.pctCloseReach.toFixed(1)}%
                                        </span>
                                        <span className="pos-legend-item">
                                          <span className="pos-dot" style={{ backgroundColor: POINT_OF_SAIL_METAS.beam_reach.color }} />
                                          Beam Reach (75-105°) — {posStats.pctBeamReach.toFixed(1)}%
                                        </span>
                                        <span className="pos-legend-item">
                                          <span className="pos-dot" style={{ backgroundColor: POINT_OF_SAIL_METAS.broad_reach.color }} />
                                          Broad Reach (105-150°) — {posStats.pctBroadReach.toFixed(1)}%
                                        </span>
                                        <span className="pos-legend-item">
                                          <span className="pos-dot" style={{ backgroundColor: POINT_OF_SAIL_METAS.dead_downwind.color }} />
                                          Downwind (150-180°) — {posStats.pctDeadDownwind.toFixed(1)}%
                                        </span>
                                      </div>
                                    </div>
                                    <div className="ww-expanded-pos-bar">
                                      {posStats.pctCloseHauled > 0 && (
                                        <div style={{ width: `${posStats.pctCloseHauled}%`, backgroundColor: POINT_OF_SAIL_METAS.close_hauled.color }} />
                                      )}
                                      {posStats.pctCloseReach > 0 && (
                                        <div style={{ width: `${posStats.pctCloseReach}%`, backgroundColor: POINT_OF_SAIL_METAS.close_reach.color }} />
                                      )}
                                      {posStats.pctBeamReach > 0 && (
                                        <div style={{ width: `${posStats.pctBeamReach}%`, backgroundColor: POINT_OF_SAIL_METAS.beam_reach.color }} />
                                      )}
                                      {posStats.pctBroadReach > 0 && (
                                        <div style={{ width: `${posStats.pctBroadReach}%`, backgroundColor: POINT_OF_SAIL_METAS.broad_reach.color }} />
                                      )}
                                      {posStats.pctDeadDownwind > 0 && (
                                        <div style={{ width: `${posStats.pctDeadDownwind}%`, backgroundColor: POINT_OF_SAIL_METAS.dead_downwind.color }} />
                                      )}
                                    </div>
                                  </div>

                                  {/* Embedded Compact Wind Rose Polar & Breakdown */}
                                  <div className="ww-expanded-wind-rose-wrap">
                                    <WindRose routeResult={candidate.route} compact={true} />
                                  </div>

                                  <div className="ww-expanded-stats-grid">
                                    <div className="ww-stat-box">
                                      <span className="stat-label">Departure</span>
                                      <span className="stat-value">{formatDateDisplay(candidate.departure_time)} {formatTimeUTC(candidate.departure_time)}</span>
                                    </div>
                                    <div className="ww-stat-box">
                                      <span className="stat-label">Arrival</span>
                                      <span className="stat-value">{formatDateDisplay(candidate.arrival_time)} {formatTimeUTC(candidate.arrival_time)}</span>
                                    </div>
                                    <div className="ww-stat-box">
                                      <span className="stat-label">Comfort Score</span>
                                      <span className="stat-value">
                                        <span style={{ color: getScoreColor(candidate.comfort_score) }}>
                                          {candidate.comfort_score}/100
                                        </span>
                                        {' ('}
                                        <span style={{ color: confStyle.color, fontWeight: 700 }}>
                                          {candidate.confidence_score}% Conf
                                        </span>
                                        {')'}
                                      </span>
                                    </div>
                                    <div className="ww-stat-box">
                                      <span className="stat-label">Max Heel &amp; Tacks</span>
                                      <span className="stat-value">
                                        {candidate.max_heel_deg}° heel &bull; {candidate.total_tacks} tacks
                                      </span>
                                    </div>
                                  </div>

                                  {candidate.gale_warning && (
                                    <div className="ww-drawer-alert gale">
                                      <AlertTriangle size={15} />
                                      <span>{candidate.gale_warning_detail}</span>
                                    </div>
                                  )}

                                  {candidate.night_arrival_warning && (
                                    <div className="ww-drawer-alert night">
                                      <Moon size={15} />
                                      <span>{candidate.night_arrival_warning_detail}</span>
                                    </div>
                                  )}

                                  {candidate.low_wind_warning && (
                                    <div className="ww-drawer-alert low-wind">
                                      <Wind size={15} />
                                      <span>{candidate.low_wind_warning_detail}</span>
                                    </div>
                                  )}

                                  <div className="ww-drawer-actions">
                                    <button
                                      type="button"
                                      className="ww-btn-primary-inspect"
                                      onClick={() =>
                                        onSelectWindowRoute(
                                          candidate.route,
                                          candidate.representative_event?.time || candidate.departure_time
                                        )
                                      }
                                    >
                                      <Maximize2 size={14} />
                                      <span>Load Full Route &amp; Wind Field on Interactive Map</span>
                                    </button>
                                  </div>
                                </div>
                              </div>
                            </td>
                          </tr>
                        )}
                      </React.Fragment>
                    );
                  })}
                </tbody>
              </table>

              {/* SINGLE UNBROKEN CONTINUOUS SVG OVERLAY ACROSS ALL ROWS */}
              {points.length === displayWindows.length && points.length > 0 && tableHeight > 0 && (
                <svg
                  className={`ww-comfort-overlay-svg ${expandedRowIndex !== null ? 'dimmed' : ''}`}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: colLeft,
                    width: svgPlotWidth,
                    height: tableHeight,
                    pointerEvents: 'none',
                  }}
                >
                  <defs>
                    <filter id="comfortHalo" x="-40%" y="-40%" width="180%" height="180%">
                      <feGaussianBlur stdDeviation="2.5" result="blur" />
                      <feComposite in="SourceGraphic" in2="blur" operator="over" />
                    </filter>
                    {/* Linear gradient per segment for smooth color transition between points */}
                    {pathSegments.map((seg) => (
                      <linearGradient
                        key={seg.id}
                        id={seg.id}
                        gradientUnits="userSpaceOnUse"
                        x1="0"
                        y1={seg.y1}
                        x2="0"
                        y2={seg.y2}
                      >
                        <stop offset="0%" stopColor={seg.startColor} />
                        <stop offset="100%" stopColor={seg.endColor} />
                      </linearGradient>
                    ))}
                  </defs>

                  {/* Vertical Guidelines: Min (left), Mid (center), Max (right) */}
                  <line
                    x1={svgPlotPad}
                    y1={points[0].y}
                    x2={svgPlotPad}
                    y2={points[points.length - 1].y}
                    stroke="rgba(255,255,255,0.08)"
                    strokeDasharray="2 3"
                  />
                  <line
                    x1={svgPlotWidth / 2}
                    y1={points[0].y}
                    x2={svgPlotWidth / 2}
                    y2={points[points.length - 1].y}
                    stroke="rgba(255,255,255,0.12)"
                    strokeDasharray="2 3"
                  />
                  <line
                    x1={svgPlotWidth - svgPlotPad}
                    y1={points[0].y}
                    x2={svgPlotWidth - svgPlotPad}
                    y2={points[points.length - 1].y}
                    stroke="rgba(255,255,255,0.08)"
                    strokeDasharray="2 3"
                  />

                  {/* Gradient Curve Segments connecting all rows seamlessly */}
                  {pathSegments.map((seg) => (
                    <path
                      key={seg.id}
                      d={seg.d}
                      fill="none"
                      stroke={`url(#${seg.id})`}
                      strokeWidth="3.2"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      opacity="0.95"
                    />
                  ))}

                  {/* Data Nodes at the exact vertical center of each row */}
                  {points.map((pt, i) => (
                    <g key={i}>
                      <circle
                        cx={pt.x}
                        cy={pt.y}
                        r="5.5"
                        fill={getScoreColor(pt.score)}
                        stroke="#ffffff"
                        strokeWidth="2"
                        filter="url(#comfortHalo)"
                      />
                    </g>
                  ))}
                </svg>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Empty State before search */}
      {!windowResponse && !loading && (
        <div className="ww-empty-state">
          <div className="ww-empty-icon-circle">
            <Compass size={44} color="#38bdf8" />
          </div>
          <h2>Ready to Search Optimal Weather Windows</h2>
          <p>
            Configure your passage start and destination, define your time window, and click{' '}
            <strong>Find Best Weather Windows</strong>. The table orders departures chronologically
            and renders an unbroken vertical comfort plot revealing when the ideal weather window opens.
          </p>
          <div className="ww-features-row">
            <div className="ww-feature-item">
              <Sparkles size={18} className="text-emerald" />
              <span>Vertical Comfort Plot</span>
            </div>
            <div className="ww-feature-item">
              <Waves size={18} className="text-sky" />
              <span>Wave Height &amp; Period Index</span>
            </div>
            <div className="ww-feature-item">
              <AlertTriangle size={18} className="text-amber" />
              <span>Gale &amp; Calm Warnings</span>
            </div>
            <div className="ww-feature-item">
              <Layers size={18} className="text-purple" />
              <span>Representative Weather Map</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

/**
 * Computes percentage breakdown for fine-grained points of sail:
 * - Close Hauled (< 60° TWA)
 * - Close Reach (60° – 80° TWA)
 * - Beam Reach (80° – 110° TWA)
 * - Broad Reach (110° – 160° TWA)
 * - Dead Downwind (160° – 180° TWA)
 */
function computePointOfSailDistribution(route: RouteResult) {
  const wps = route?.waypoints || [];
  if (wps.length === 0) {
    return {
      pctCloseHauled: 0,
      pctCloseReach: 0,
      pctBeamReach: 50,
      pctBroadReach: 50,
      pctDeadDownwind: 0,
      pctUpwind: 0,
      pctReaching: 100,
      pctDownwind: 0,
      topPos: 'beam_reach' as const,
      topLabel: 'Beam Reach',
      topPct: 100,
      topColor: POINT_OF_SAIL_METAS.beam_reach.color,
    };
  }

  let closeHauledCount = 0;
  let closeReachCount = 0;
  let beamReachCount = 0;
  let broadReachCount = 0;
  let deadDownwindCount = 0;

  for (const wp of wps) {
    const pos = getPointOfSail(wp.twa_deg);
    switch (pos) {
      case 'close_hauled':
        closeHauledCount++;
        break;
      case 'close_reach':
        closeReachCount++;
        break;
      case 'beam_reach':
        beamReachCount++;
        break;
      case 'broad_reach':
        broadReachCount++;
        break;
      case 'dead_downwind':
        deadDownwindCount++;
        break;
    }
  }

  const n = wps.length;
  const pctCloseHauled = (closeHauledCount / n) * 100;
  const pctCloseReach = (closeReachCount / n) * 100;
  const pctBeamReach = (beamReachCount / n) * 100;
  const pctBroadReach = (broadReachCount / n) * 100;
  const pctDeadDownwind = (deadDownwindCount / n) * 100;

  const pctUpwind = pctCloseHauled;
  const pctReaching = pctBeamReach + pctBroadReach;
  const pctDownwind = pctDeadDownwind;

  // Determine the dominant / top sailing point of sail
  const posCounts = [
    { key: 'beam_reach', pct: pctBeamReach, meta: POINT_OF_SAIL_METAS.beam_reach },
    { key: 'broad_reach', pct: pctBroadReach, meta: POINT_OF_SAIL_METAS.broad_reach },
    { key: 'close_reach', pct: pctCloseReach, meta: POINT_OF_SAIL_METAS.close_reach },
    { key: 'close_hauled', pct: pctCloseHauled, meta: POINT_OF_SAIL_METAS.close_hauled },
    { key: 'dead_downwind', pct: pctDeadDownwind, meta: POINT_OF_SAIL_METAS.dead_downwind },
  ];
  posCounts.sort((a, b) => b.pct - a.pct);
  const top = posCounts[0];

  return {
    pctCloseHauled,
    pctCloseReach,
    pctBeamReach,
    pctBroadReach,
    pctDeadDownwind,
    pctUpwind,
    pctReaching,
    pctDownwind,
    topPos: top.key,
    topLabel: top.meta.shortLabel,
    topPct: top.pct,
    topColor: top.meta.color,
  };
}

/**
 * RepresentativeWeatherMiniMap renders an embedded SVG overview of the route
 * with the boat position and wind field at the representative event moment.
 */
interface RepresentativeWeatherMiniMapProps {
  route: RouteResult;
  event: any;
  width?: number;
  height?: number;
}

const RepresentativeWeatherMiniMap: React.FC<RepresentativeWeatherMiniMapProps> = ({
  route,
  event,
  width = 320,
  height = 130,
}) => {
  const waypoints = route.waypoints || [];
  if (waypoints.length < 2) return null;

  // Compute bounding box
  let minLat = 90, maxLat = -90, minLon = 180, maxLon = -180;
  for (const wp of waypoints) {
    if (wp.lat < minLat) minLat = wp.lat;
    if (wp.lat > maxLat) maxLat = wp.lat;
    if (wp.lon < minLon) minLon = wp.lon;
    if (wp.lon > maxLon) maxLon = wp.lon;
  }

  const latSpan = Math.max(1.0, maxLat - minLat);
  const lonSpan = Math.max(1.0, maxLon - minLon);

  // Pad by 15%
  const padLat = latSpan * 0.15;
  const padLon = lonSpan * 0.15;
  const bounds = {
    minLat: minLat - padLat,
    maxLat: maxLat + padLat,
    minLon: minLon - padLon,
    maxLon: maxLon + padLon,
  };

  const svgWidth = width;
  const svgHeight = height;

  const project = (lat: number, lon: number): [number, number] => {
    const x = ((lon - bounds.minLon) / (bounds.maxLon - bounds.minLon)) * svgWidth;
    const y = ((bounds.maxLat - lat) / (bounds.maxLat - bounds.minLat)) * svgHeight;
    return [Math.max(4, Math.min(svgWidth - 4, x)), Math.max(4, Math.min(svgHeight - 4, y))];
  };

  // Build SVG path
  const pathD = waypoints.reduce((acc, wp, idx) => {
    const [x, y] = project(wp.lat, wp.lon);
    return idx === 0 ? `M ${x.toFixed(1)} ${y.toFixed(1)}` : `${acc} L ${x.toFixed(1)} ${y.toFixed(1)}`;
  }, '');

  const [startX, startY] = project(route.start_point.lat, route.start_point.lon);
  const [destX, destY] = project(route.dest_point.lat, route.dest_point.lon);

  // Representative event location
  const repLoc = event.location || waypoints[Math.floor(waypoints.length / 2)];
  const [boatX, boatY] = project(repLoc.lat, repLoc.lon);

  // Wind arrow vector at boat
  const windDirRad = (event.wind_dir_deg * Math.PI) / 180.0;
  const windVectorLen = Math.min(22, Math.max(10, event.wind_speed_kts * 0.7));
  const arrowEndX = boatX + Math.sin(windDirRad + Math.PI) * windVectorLen;
  const arrowEndY = boatY - Math.cos(windDirRad + Math.PI) * windVectorLen;

  return (
    <div className="ww-mini-map-container" title="Representative snapshot of passage conditions">
      <svg
        viewBox={`0 0 ${svgWidth} ${svgHeight}`}
        className="ww-mini-map-svg"
        preserveAspectRatio="xMidYMid meet"
        style={{ height }}
      >
        <defs>
          <linearGradient id="routeGradient" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#10b981" />
            <stop offset="50%" stopColor="#38bdf8" />
            <stop offset="100%" stopColor="#8b5cf6" />
          </linearGradient>
          <filter id="glowMini" x="-20%" y="-20%" width="140%" height="140%">
            <feGaussianBlur stdDeviation="2" result="blur" />
            <feComposite in="SourceGraphic" in2="blur" operator="over" />
          </filter>
        </defs>

        {/* Ocean Background & Grid Lines */}
        <rect width={svgWidth} height={svgHeight} fill="#091528" rx="6" />
        <line x1="0" y1={svgHeight / 2} x2={svgWidth} y2={svgHeight / 2} stroke="#172b4d" strokeDasharray="3 3" />
        <line x1={svgWidth / 2} y1="0" x2={svgWidth / 2} y2={svgHeight} stroke="#172b4d" strokeDasharray="3 3" />

        {/* Route Track */}
        <path
          d={pathD}
          fill="none"
          stroke="url(#routeGradient)"
          strokeWidth="2.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />

        {/* Start Point Marker */}
        <circle cx={startX} cy={startY} r="4" fill="#10b981" stroke="#ffffff" strokeWidth="1.5" />

        {/* Destination Point Marker */}
        <circle cx={destX} cy={destY} r="4" fill="#f43f5e" stroke="#ffffff" strokeWidth="1.5" />

        {/* Wind Barb / Arrow at Event location */}
        <line
          x1={boatX}
          y1={boatY}
          x2={arrowEndX}
          y2={arrowEndY}
          stroke="#facc15"
          strokeWidth="2.2"
          strokeLinecap="round"
        />

        {/* Boat Marker at Event location */}
        <circle
          cx={boatX}
          cy={boatY}
          r="5.5"
          fill="#38bdf8"
          stroke="#ffffff"
          strokeWidth="1.8"
          filter="url(#glowMini)"
        />
      </svg>

      <div className="ww-mini-map-meta">
        <span className="ww-mini-wind-tag">
          Wind: <strong>{Math.round(event.wind_speed_kts)} kts</strong> @ {Math.round(event.wind_dir_deg)}°
        </span>
        <span className="ww-mini-wave-tag">
          Waves: <strong>{event.wave_height_m}m</strong> &bull; {event.wave_period_s}s
        </span>
      </div>
    </div>
  );
};

function getScoreColor(score: number): string {
  if (score >= 80) return '#10b981'; // Emerald
  if (score >= 65) return '#38bdf8'; // Sky
  if (score >= 50) return '#f59e0b'; // Amber
  return '#ef4444'; // Red
}

function getConfidenceStyle(score: number) {
  if (score >= 75) {
    return {
      color: '#34d399',
      backgroundColor: 'rgba(16, 185, 129, 0.18)',
      borderColor: 'rgba(52, 211, 153, 0.4)',
    };
  }
  if (score >= 50) {
    return {
      color: '#facc15',
      backgroundColor: 'rgba(234, 179, 8, 0.18)',
      borderColor: 'rgba(250, 204, 21, 0.4)',
    };
  }
  return {
    color: '#f87171',
    backgroundColor: 'rgba(239, 68, 68, 0.18)',
    borderColor: 'rgba(248, 113, 113, 0.4)',
  };
}

function formatDateDisplay(isoStr?: string): string {
  if (!isoStr) return '--';
  try {
    const d = new Date(isoStr);
    const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
    const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    return `${days[d.getUTCDay()]}, ${months[d.getUTCMonth()]} ${d.getUTCDate()}`;
  } catch {
    return isoStr.slice(0, 10);
  }
}

function formatTimeUTC(isoStr?: string): string {
  if (!isoStr) return '--:--';
  try {
    const d = new Date(isoStr);
    const h = String(d.getUTCHours()).padStart(2, '0');
    const m = String(d.getUTCMinutes()).padStart(2, '0');
    return `${h}:${m} UTC`;
  } catch {
    return '--:--';
  }
}

function formatDurationHours(h: number): string {
  const totalHours = Math.round(h);
  const days = Math.floor(totalHours / 24);
  const remHours = totalHours % 24;
  if (days > 0) {
    return `${days}d ${remHours}h`;
  }
  return `${totalHours}h`;
}
