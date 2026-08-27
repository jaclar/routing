import React from 'react';
import { BoatPreset, Point, RouteResult } from '../types';
import { ROUTE_PRESETS, calcDirectDistanceNM, getSaneDefaultTimeStepHours } from '../services/api';
import { Compass, Navigation, Wind, Anchor, Download, Play, Gauge, Sparkles, RefreshCw } from 'lucide-react';

interface SidebarProps {
  presets: BoatPreset[];
  selectedPresetId: string;
  onSelectPreset: (id: string) => void;
  startPoint: Point;
  destPoint: Point;
  onStartChange: (p: Point) => void;
  onDestChange: (p: Point) => void;
  departureTime: string;
  onDepartureTimeChange: (t: string) => void;
  timeStepHours: number;
  onTimeStepChange: (h: number) => void;
  tackPenaltyMinutes: number;
  onTackPenaltyChange: (m: number) => void;
  gybePenaltyMinutes: number;
  onGybePenaltyChange: (m: number) => void;
  onCalculateRoute: () => void;
  loading: boolean;
  routeResult: RouteResult | null;
}

export const Sidebar: React.FC<SidebarProps> = ({
  presets,
  selectedPresetId,
  onSelectPreset,
  startPoint,
  destPoint,
  onStartChange,
  onDestChange,
  departureTime,
  onDepartureTimeChange,
  timeStepHours,
  onTimeStepChange,
  tackPenaltyMinutes,
  onTackPenaltyChange,
  gybePenaltyMinutes,
  onGybePenaltyChange,
  onCalculateRoute,
  loading,
  routeResult,
}) => {
  const currentBoat = presets.find((p) => p.id === selectedPresetId);

  const directDistNM = calcDirectDistanceNM(startPoint, destPoint);
  const recommendedStep = getSaneDefaultTimeStepHours(directDistNM);

  const formatStepLabel = (hours: number) => {
    const mins = Math.round(hours * 60);
    if (mins < 60) return `${mins} min`;
    return `${hours} hr`;
  };

  const matchingPresetIdx = ROUTE_PRESETS.findIndex(
    (p) =>
      Math.abs(p.start.lat - startPoint.lat) < 0.005 &&
      Math.abs(p.start.lon - startPoint.lon) < 0.005 &&
      Math.abs(p.dest.lat - destPoint.lat) < 0.005 &&
      Math.abs(p.dest.lon - destPoint.lon) < 0.005
  );
  const selectedPresetValue = matchingPresetIdx >= 0 ? matchingPresetIdx.toString() : 'custom';

  const handleRoutePresetSelect = (e: React.ChangeEvent<HTMLSelectElement>) => {
    if (e.target.value === 'custom') return;
    const idx = parseInt(e.target.value, 10);
    if (!isNaN(idx) && ROUTE_PRESETS[idx]) {
      const preset = ROUTE_PRESETS[idx];
      onStartChange(preset.start);
      onDestChange(preset.dest);
      const d = calcDirectDistanceNM(preset.start, preset.dest);
      onTimeStepChange(getSaneDefaultTimeStepHours(d));
    }
  };

  const handleExportGPX = () => {
    if (!routeResult) return;
    const gpx = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="SailVPP-Routing" xmlns="http://www.topografix.com/GPX/1/1">
  <metadata>
    <name>${routeResult.boat_name} Route</name>
    <time>${routeResult.start_time}</time>
  </metadata>
  <rte>
    <name>${routeResult.boat_name} Optimal Weather Route</name>
    ${routeResult.waypoints
      .map(
        (wp) =>
          `<rtept lat="${wp.lat}" lon="${wp.lon}"><time>${wp.time}</time><name>Spd ${wp.boat_speed_kts.toFixed(1)}k</name></rtept>`
      )
      .join('\n    ')}
  </rte>
</gpx>`;
    const blob = new Blob([gpx], { type: 'application/gpx+xml' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `route_${routeResult.boat_name.replace(/\s+/g, '_')}.gpx`;
    a.click();
  };

  const setCruisingMode = () => {
    onTackPenaltyChange(5.0);
    onGybePenaltyChange(8.0);
  };

  const setRacingMode = () => {
    onTackPenaltyChange(1.5);
    onGybePenaltyChange(2.0);
  };

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <Compass className="text-accent" size={24} color="#38bdf8" />
        <div>
          <h1>Sailboat Weather Routing</h1>
          <span className="badge">Isochrone + GFS + VPP</span>
        </div>
      </div>

      <div className="sidebar-content">
        {/* 1. Boat Selection */}
        <div className="section-card">
          <div className="section-title">
            <Anchor size={16} />
            <span>Boat Configuration</span>
          </div>

          <div className="input-group">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <label>Select Yacht Model</label>
              {currentBoat?.isCustom && (
                <span className="custom-boat-badge" style={{ fontSize: '0.65rem', padding: '1px 5px' }}>
                  Custom VPP
                </span>
              )}
            </div>
            <select
              className="select-field"
              value={selectedPresetId}
              onChange={(e) => onSelectPreset(e.target.value)}
            >
              <optgroup label="Built-in Standard Presets">
                {presets
                  .filter((p) => !p.isCustom)
                  .map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name} ({p.rig_type.toUpperCase()})
                    </option>
                  ))}
              </optgroup>
              {presets.some((p) => p.isCustom) && (
                <optgroup label="Custom User Yachts">
                  {presets
                    .filter((p) => p.isCustom)
                    .map((p) => (
                      <option key={p.id} value={p.id}>
                        🛠️ {p.name} ({p.rig_type.toUpperCase()})
                      </option>
                    ))}
                </optgroup>
              )}
            </select>
          </div>

          {currentBoat && (
            <div style={{ fontSize: '0.75rem', color: '#94a3b8', display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '4px', background: 'rgba(0,0,0,0.2)', padding: '8px', borderRadius: '6px' }}>
              <div>LOA: <b style={{ color: '#f8fafc' }}>{currentBoat.loa_m}m</b></div>
              <div>Beam: <b style={{ color: '#f8fafc' }}>{currentBoat.beam_m}m</b></div>
              <div>Draft: <b style={{ color: '#f8fafc' }}>{currentBoat.draft_m}m</b></div>
              <div>Disp: <b style={{ color: '#f8fafc' }}>{currentBoat.displacement_kg.toLocaleString()}kg</b></div>
            </div>
          )}
        </div>

        {/* 2. Route Coordinates & Preset Selection */}
        <div className="section-card">
          <div className="section-title">
            <Navigation size={16} />
            <span>Passage & Waypoints</span>
          </div>

          <div className="input-group">
            <label>Passage Route Preset</label>
            <select
              className="select-field"
              value={selectedPresetValue}
              onChange={handleRoutePresetSelect}
            >
              {selectedPresetValue === 'custom' && (
                <option value="custom">📍 Custom Coordinates (Map Placed / Dragged)</option>
              )}
              {ROUTE_PRESETS.map((p, idx) => (
                <option key={idx} value={idx.toString()}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>

          <div className="input-group">
            <label>Start Location (Lat / Lon)</label>
            <div className="coords-row">
              <input
                type="number"
                step="0.001"
                className="input-field"
                value={startPoint.lat}
                onChange={(e) => onStartChange({ ...startPoint, lat: parseFloat(e.target.value) || 0 })}
              />
              <input
                type="number"
                step="0.001"
                className="input-field"
                value={startPoint.lon}
                onChange={(e) => onStartChange({ ...startPoint, lon: parseFloat(e.target.value) || 0 })}
              />
            </div>
          </div>

          <div className="input-group">
            <label>Destination Location (Lat / Lon)</label>
            <div className="coords-row">
              <input
                type="number"
                step="0.001"
                className="input-field"
                value={destPoint.lat}
                onChange={(e) => onDestChange({ ...destPoint, lat: parseFloat(e.target.value) || 0 })}
              />
              <input
                type="number"
                step="0.001"
                className="input-field"
                value={destPoint.lon}
                onChange={(e) => onDestChange({ ...destPoint, lon: parseFloat(e.target.value) || 0 })}
              />
            </div>
          </div>

          <div className="map-click-hint-box">
            <Navigation size={13} className="text-accent" />
            <span>Click on map to place Start &amp; Finish, or drag pins on water.</span>
          </div>
        </div>

        {/* 3. Routing Controls & Maneuver Penalties */}
        <div className="section-card">
          <div className="section-title">
            <Wind size={16} />
            <span>Weather & Time Parameters</span>
          </div>

          <div className="input-group">
            <label>Departure Date & Time (UTC)</label>
            <input
              type="datetime-local"
              className="input-field"
              value={departureTime}
              onChange={(e) => onDepartureTimeChange(e.target.value)}
            />
          </div>

          <div className="input-group">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                <span>Isochrone Time Step</span>
              </label>
              <span className="auto-step-indicator" title={`Automatic sane default for ${directDistNM.toFixed(0)} NM passage`}>
                <Sparkles size={11} className="text-accent" /> Auto: {formatStepLabel(recommendedStep)} ({directDistNM.toFixed(0)} NM)
              </span>
            </div>

            <div style={{ display: 'flex', gap: '6px' }}>
              <select
                className="select-field"
                style={{ flex: 1 }}
                value={timeStepHours}
                onChange={(e) => onTimeStepChange(parseFloat(e.target.value))}
              >
                <option value={5 / 60}>
                  5 Minutes {Math.abs(recommendedStep - 5 / 60) < 0.001 ? '★ (Auto Default)' : '— Short (<100 NM)'}
                </option>
                <option value={10 / 60}>
                  10 Minutes {Math.abs(recommendedStep - 10 / 60) < 0.001 ? '★ (Auto Default)' : '— Coastal / Inshore'}
                </option>
                <option value={15 / 60}>
                  15 Minutes {Math.abs(recommendedStep - 15 / 60) < 0.001 ? '★ (Auto Default)' : '— Channel (100–250 NM)'}
                </option>
                <option value={30 / 60}>
                  30 Minutes {Math.abs(recommendedStep - 30 / 60) < 0.001 ? '★ (Auto Default)' : '— Offshore (250–500 NM)'}
                </option>
                <option value={1}>
                  1 Hour {Math.abs(recommendedStep - 1) < 0.001 ? '★ (Auto Default)' : '— Ocean (500–1200 NM)'}
                </option>
                <option value={2}>
                  2 Hours {Math.abs(recommendedStep - 2) < 0.001 ? '★ (Auto Default)' : '— Trans-Ocean (>1200 NM)'}
                </option>
                <option value={6}>
                  6 Hours — Fast Preview
                </option>
              </select>

              {Math.abs(timeStepHours - recommendedStep) >= 0.001 && (
                <button
                  type="button"
                  className="btn-reset-auto-step"
                  onClick={() => onTimeStepChange(recommendedStep)}
                  title={`Reset to recommended auto default (${formatStepLabel(recommendedStep)})`}
                >
                  <RefreshCw size={13} />
                </button>
              )}
            </div>
          </div>

          {/* Maneuver Penalties (Cruising vs Racing) */}
          <div className="input-group" style={{ borderTop: '1px solid rgba(148, 163, 184, 0.15)', paddingTop: '10px', marginTop: '4px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '4px' }}>
              <label style={{ color: '#38bdf8', fontWeight: 600, display: 'flex', alignItems: 'center', gap: '4px' }}>
                <Gauge size={13} /> Maneuver Penalties
              </label>
              <div style={{ display: 'flex', gap: '4px' }}>
                <button
                  type="button"
                  onClick={setCruisingMode}
                  style={{
                    background: tackPenaltyMinutes >= 4.0 ? '#0284c7' : 'rgba(15, 23, 42, 0.6)',
                    color: '#fff',
                    border: '1px solid var(--border-color)',
                    fontSize: '0.65rem',
                    padding: '2px 6px',
                    borderRadius: '4px',
                    cursor: 'pointer',
                  }}
                >
                  Cruising
                </button>
                <button
                  type="button"
                  onClick={setRacingMode}
                  style={{
                    background: tackPenaltyMinutes < 4.0 && tackPenaltyMinutes > 0 ? '#0284c7' : 'rgba(15, 23, 42, 0.6)',
                    color: '#fff',
                    border: '1px solid var(--border-color)',
                    fontSize: '0.65rem',
                    padding: '2px 6px',
                    borderRadius: '4px',
                    cursor: 'pointer',
                  }}
                >
                  Racing
                </button>
              </div>
            </div>

            <div className="coords-row">
              <div className="input-group">
                <label style={{ fontSize: '0.75rem' }}>Tack Penalty (min)</label>
                <input
                  type="number"
                  step="0.5"
                  min="0"
                  max="60"
                  className="input-field"
                  value={tackPenaltyMinutes}
                  onChange={(e) => onTackPenaltyChange(Math.max(0, parseFloat(e.target.value) || 0))}
                />
              </div>
              <div className="input-group">
                <label style={{ fontSize: '0.75rem' }}>Gybe Penalty (min)</label>
                <input
                  type="number"
                  step="0.5"
                  min="0"
                  max="60"
                  className="input-field"
                  value={gybePenaltyMinutes}
                  onChange={(e) => onGybePenaltyChange(Math.max(0, parseFloat(e.target.value) || 0))}
                />
              </div>
            </div>
          </div>

          <button className="btn-primary" onClick={onCalculateRoute} disabled={loading} style={{ marginTop: '6px' }}>
            <Play size={18} />
            <span>{loading ? 'Propagating Isochrones...' : 'Calculate Optimal Route'}</span>
          </button>
        </div>

        {/* 4. Solved Route Summary Metrics */}
        {routeResult && (
          <div className="section-card" style={{ borderColor: 'rgba(56, 189, 248, 0.4)' }}>
            <div className="section-title" style={{ justifyContent: 'space-between' }}>
              <span>Route Summary</span>
              <button
                onClick={handleExportGPX}
                style={{ background: 'transparent', border: 'none', color: '#38bdf8', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: '4px', fontSize: '0.75rem' }}
              >
                <Download size={14} /> GPX
              </button>
            </div>

            <div className="metrics-grid">
              <div className="metric-card">
                <div className="metric-title">Passage Time</div>
                <div className="metric-value">
                  {Math.floor(routeResult.total_duration_hours / 24)}d {Math.round(routeResult.total_duration_hours % 24)}h
                </div>
              </div>

              <div className="metric-card">
                <div className="metric-title">Distance Sailed</div>
                <div className="metric-value">{Math.round(routeResult.total_distance_nm)} NM</div>
              </div>

              <div className="metric-card">
                <div className="metric-title">Average Speed</div>
                <div className="metric-value">{routeResult.average_speed_kts.toFixed(2)} kts</div>
              </div>

              <div className="metric-card">
                <div className="metric-title">Max Wind</div>
                <div className="metric-value">{routeResult.max_wind_kts.toFixed(1)} kts</div>
              </div>

              <div className="metric-card">
                <div className="metric-title">Tacks</div>
                <div className="metric-value" style={{ color: '#10b981' }}>{routeResult.total_tacks}</div>
              </div>

              <div className="metric-card">
                <div className="metric-title">Gybes</div>
                <div className="metric-value" style={{ color: '#f59e0b' }}>{routeResult.total_gybes}</div>
              </div>
            </div>
          </div>
        )}
      </div>
    </aside>
  );
};
