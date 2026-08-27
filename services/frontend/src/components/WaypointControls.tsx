import React from 'react';
import { Point } from '../types';
import { calcDirectDistanceNM } from '../services/api';
import { MapPin } from 'lucide-react';

interface WaypointControlsProps {
  startPoint: Point;
  destPoint: Point;
  placementMode: 'start' | 'dest';
  onSelectPlacementMode: (mode: 'start' | 'dest') => void;
}

export const WaypointControls: React.FC<WaypointControlsProps> = ({
  startPoint,
  destPoint,
  placementMode,
  onSelectPlacementMode,
}) => {
  const directDistNM = calcDirectDistanceNM(startPoint, destPoint);

  return (
    <div className="waypoint-controls-card">
      <div className="waypoint-controls-header">
        <div className="waypoint-controls-title">
          <MapPin size={13} className="text-accent" />
          <span>PASSAGE PINS</span>
        </div>
        <span className="waypoint-dist-badge">Direct: {directDistNM.toFixed(1)} NM</span>
      </div>

      <div className="waypoint-buttons-stack">
        <button
          type="button"
          className={`waypoint-pill-btn start-pill ${placementMode === 'start' ? 'active' : ''}`}
          onClick={() => onSelectPlacementMode('start')}
          title="Click to activate placing Start point on next map click"
        >
          <span className="pill-dot pill-dot-start"></span>
          <span className="pill-label">START:</span>
          <span className="pill-coords">{startPoint.lat.toFixed(3)}°, {startPoint.lon.toFixed(3)}°</span>
        </button>

        <button
          type="button"
          className={`waypoint-pill-btn dest-pill ${placementMode === 'dest' ? 'active' : ''}`}
          onClick={() => onSelectPlacementMode('dest')}
          title="Click to activate placing Finish point on next map click"
        >
          <span className="pill-dot pill-dot-dest"></span>
          <span className="pill-label">FINISH:</span>
          <span className="pill-coords">{destPoint.lat.toFixed(3)}°, {destPoint.lon.toFixed(3)}°</span>
        </button>
      </div>

      <div className="waypoint-instruction-badge">
        {placementMode === 'start' ? (
          <span>🟢 Click map to set <strong>Start</strong> (or drag ⚓)</span>
        ) : (
          <span>🏁 Click map to set <strong>Finish</strong> (or drag 🏁)</span>
        )}
      </div>
    </div>
  );
};
