import React, { useState } from 'react';
import { Layers, Wind, Eye, Clock, ShieldAlert, Minus } from 'lucide-react';

interface LayerTogglesProps {
  showIsochrones: boolean;
  onToggleIsochrones: () => void;
  showWindGrid: boolean;
  onToggleWindGrid: () => void;
  showLandmask: boolean;
  onToggleLandmask: () => void;
  activeTime?: string;
}

export const LayerToggles: React.FC<LayerTogglesProps> = ({
  showIsochrones,
  onToggleIsochrones,
  showWindGrid,
  onToggleWindGrid,
  showLandmask,
  onToggleLandmask,
  activeTime,
}) => {
  // Minimized by default on mobile screens (width <= 768px)
  const [isCollapsed, setIsCollapsed] = useState<boolean>(() => {
    return typeof window !== 'undefined' ? window.innerWidth <= 768 : false;
  });

  const formatTime = (iso?: string) => {
    if (!iso) return '';
    try {
      const d = new Date(iso);
      return d.toUTCString().replace('GMT', 'UTC').slice(0, 22);
    } catch {
      return iso;
    }
  };

  // When collapsed (mobile default): show only the symbol
  if (isCollapsed) {
    return (
      <button
        type="button"
        className="floating-overlay-icon-btn"
        onClick={() => setIsCollapsed(false)}
        title="Open Map Layers"
      >
        <Layers size={18} color="#38bdf8" />
      </button>
    );
  }

  return (
    <div className="layer-toggles">
      <div className="layer-toggles-header">
        <div className="layer-toggles-title">
          <Layers size={14} color="#38bdf8" />
          <span>MAP LAYERS</span>
        </div>
        <button
          type="button"
          className="btn-overlay-minimize"
          onClick={() => setIsCollapsed(true)}
          title="Minimize layers panel"
        >
          <Minus size={14} />
        </button>
      </div>

      <label className="toggle-item">
        <input
          type="checkbox"
          checked={showIsochrones}
          onChange={onToggleIsochrones}
          style={{ accentColor: '#38bdf8' }}
        />
        <Eye size={14} color="#38bdf8" />
        <span>Isochrone Fronts</span>
      </label>

      <label className="toggle-item">
        <input
          type="checkbox"
          checked={showWindGrid}
          onChange={onToggleWindGrid}
          style={{ accentColor: '#10b981' }}
        />
        <Wind size={14} color="#10b981" />
        <span>GFS Wind &amp; Barbs</span>
      </label>

      <label className="toggle-item">
        <input
          type="checkbox"
          checked={showLandmask}
          onChange={onToggleLandmask}
          style={{ accentColor: '#f59e0b' }}
        />
        <ShieldAlert size={14} color="#f59e0b" />
        <span>Landmass Polygons</span>
      </label>

      {showWindGrid && (
        <div className="wind-scale-container">
          <div className="wind-scale-labels">
            <span>0k</span>
            <span>10k</span>
            <span>20k</span>
            <span>30k</span>
            <span>40k</span>
            <span>50k+</span>
          </div>
          <div className="wind-scale-gradient-bar" />
        </div>
      )}

      {activeTime && showWindGrid && (
        <div className="active-forecast-timestamp">
          <Clock size={12} />
          <span>{formatTime(activeTime)}</span>
        </div>
      )}
    </div>
  );
};

export default LayerToggles;
