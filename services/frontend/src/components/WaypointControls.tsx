import React, { useState } from 'react';
import { Point } from '../types';
import { ROUTE_PRESETS, calcDirectDistanceNM, getSaneDefaultTimeStepHours } from '../services/api';
import { Navigation, Flag, Crosshair, Minus } from 'lucide-react';

interface WaypointControlsProps {
  startPoint: Point;
  destPoint: Point;
  onStartChange: (p: Point) => void;
  onDestChange: (p: Point) => void;
  placementMode: 'start' | 'dest';
  onSelectPlacementMode: (mode: 'start' | 'dest') => void;
  onTimeStepChange?: (hours: number) => void;
}

export const WaypointControls: React.FC<WaypointControlsProps> = ({
  startPoint,
  destPoint,
  onStartChange,
  onDestChange,
  placementMode,
  onSelectPlacementMode,
  onTimeStepChange,
}) => {
  // Minimized by default on mobile screens (width <= 768px)
  const [isCollapsed, setIsCollapsed] = useState<boolean>(() => {
    return typeof window !== 'undefined' ? window.innerWidth <= 768 : false;
  });

  const directDistNM = calcDirectDistanceNM(startPoint, destPoint);

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
      if (onTimeStepChange) {
        const d = calcDirectDistanceNM(preset.start, preset.dest);
        onTimeStepChange(getSaneDefaultTimeStepHours(d));
      }
    }
  };

  const handleStartLatChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const lat = parseFloat(e.target.value);
    if (!isNaN(lat) && lat >= -90 && lat <= 90) {
      onStartChange({ ...startPoint, lat });
    }
  };

  const handleStartLonChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const lon = parseFloat(e.target.value);
    if (!isNaN(lon) && lon >= -180 && lon <= 180) {
      onStartChange({ ...startPoint, lon });
    }
  };

  const handleDestLatChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const lat = parseFloat(e.target.value);
    if (!isNaN(lat) && lat >= -90 && lat <= 90) {
      onDestChange({ ...destPoint, lat });
    }
  };

  const handleDestLonChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const lon = parseFloat(e.target.value);
    if (!isNaN(lon) && lon >= -180 && lon <= 180) {
      onDestChange({ ...destPoint, lon });
    }
  };

  // When collapsed (mobile default): show only the symbol with distance badge
  if (isCollapsed) {
    return (
      <button
        type="button"
        className="floating-overlay-icon-btn"
        onClick={() => setIsCollapsed(false)}
        title="Open Passage Controls"
      >
        <Navigation size={18} color="#38bdf8" />
      </button>
    );
  }

  return (
    <div className="waypoint-controls-card">
      {/* Header: PASSAGE + distance badge on left, Minus minimize on far right */}
      <div className="waypoint-controls-header">
        <div className="waypoint-controls-title">
          <Navigation size={14} color="#38bdf8" />
          <span>PASSAGE</span>
          <span className="waypoint-dist-badge">{directDistNM.toFixed(1)} NM</span>
        </div>
        <button
          type="button"
          className="btn-overlay-minimize"
          onClick={() => setIsCollapsed(true)}
          title="Minimize passage panel"
        >
          <Minus size={14} />
        </button>
      </div>

      {/* Preset Selector */}
      <div className="passage-preset-select-wrapper">
        <select
          className="select-field passage-preset-select"
          value={selectedPresetValue}
          onChange={handleRoutePresetSelect}
        >
          {selectedPresetValue === 'custom' && (
            <option value="custom">📍 Custom Route (Map / Coords)</option>
          )}
          {ROUTE_PRESETS.map((p, idx) => (
            <option key={idx} value={idx.toString()}>
              {p.name}
            </option>
          ))}
        </select>
      </div>

      {/* In-Place Editable Start Coordinates */}
      <div className="passage-coord-block">
        <div className="passage-coord-row-header">
          <div className="passage-point-tag">
            <span className="pill-dot pill-dot-start"></span>
            <label className="passage-coord-label">START</label>
          </div>
          <button
            type="button"
            className={`passage-map-pick-btn ${placementMode === 'start' ? 'active' : ''}`}
            onClick={() => onSelectPlacementMode('start')}
            title="Click to set start on next map click"
          >
            <Crosshair size={11} />
            <span>{placementMode === 'start' ? 'Click Map' : 'Set on Map'}</span>
          </button>
        </div>

        <div className="passage-inputs-grid">
          <div className="passage-input-wrapper">
            <span className="passage-input-prefix">Lat</span>
            <input
              type="number"
              step="0.001"
              min="-90"
              max="90"
              className="passage-coord-input"
              value={startPoint.lat}
              onChange={handleStartLatChange}
            />
          </div>
          <div className="passage-input-wrapper">
            <span className="passage-input-prefix">Lon</span>
            <input
              type="number"
              step="0.001"
              min="-180"
              max="180"
              className="passage-coord-input"
              value={startPoint.lon}
              onChange={handleStartLonChange}
            />
          </div>
        </div>
      </div>

      {/* In-Place Editable Stop / Destination Coordinates */}
      <div className="passage-coord-block">
        <div className="passage-coord-row-header">
          <div className="passage-point-tag">
            <span className="pill-dot pill-dot-dest"></span>
            <label className="passage-coord-label">STOP</label>
          </div>
          <button
            type="button"
            className={`passage-map-pick-btn ${placementMode === 'dest' ? 'active' : ''}`}
            onClick={() => onSelectPlacementMode('dest')}
            title="Click to set stop on next map click"
          >
            <Flag size={11} />
            <span>{placementMode === 'dest' ? 'Click Map' : 'Set on Map'}</span>
          </button>
        </div>

        <div className="passage-inputs-grid">
          <div className="passage-input-wrapper">
            <span className="passage-input-prefix">Lat</span>
            <input
              type="number"
              step="0.001"
              min="-90"
              max="90"
              className="passage-coord-input"
              value={destPoint.lat}
              onChange={handleDestLatChange}
            />
          </div>
          <div className="passage-input-wrapper">
            <span className="passage-input-prefix">Lon</span>
            <input
              type="number"
              step="0.001"
              min="-180"
              max="180"
              className="passage-coord-input"
              value={destPoint.lon}
              onChange={handleDestLonChange}
            />
          </div>
        </div>
      </div>

      <div className="waypoint-instruction-badge">
        <span>Click map or drag pins ⚓ / 🏁 on water</span>
      </div>
    </div>
  );
};

export default WaypointControls;
