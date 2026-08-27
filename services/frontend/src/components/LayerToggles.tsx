import React from 'react';
import { Layers, Wind, Eye, Clock, ShieldAlert } from 'lucide-react';

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
  const formatTime = (iso?: string) => {
    if (!iso) return '';
    try {
      const d = new Date(iso);
      return d.toUTCString().replace('GMT', 'UTC').slice(0, 22);
    } catch {
      return iso;
    }
  };

  return (
    <div className="layer-toggles">
      <div style={{ fontSize: '0.75rem', fontWeight: 600, color: '#94a3b8', display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '2px' }}>
        <Layers size={14} /> MAP LAYERS
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
        <span>GFS Wind & Barbs</span>
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
        <div style={{ borderTop: '1px solid rgba(148, 163, 184, 0.15)', paddingTop: '8px', marginTop: '4px', display: 'flex', flexDirection: 'column', gap: '4px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.62rem', color: '#94a3b8', fontFamily: 'var(--font-mono)' }}>
            <span>0k</span>
            <span>10k</span>
            <span>20k</span>
            <span>30k</span>
            <span>40k</span>
            <span>50k+</span>
          </div>
          <div
            style={{
              height: '8px',
              borderRadius: '4px',
              background: 'linear-gradient(to right, rgb(29,78,216) 0%, rgb(16,185,129) 20%, rgb(250,204,21) 40%, rgb(249,115,22) 60%, rgb(239,68,68) 80%, rgb(168,85,247) 100%)',
              border: '1px solid rgba(255, 255, 255, 0.25)',
              boxShadow: 'inset 0 1px 2px rgba(0,0,0,0.3)',
            }}
          />
        </div>
      )}

      {activeTime && showWindGrid && (
        <div style={{ fontSize: '0.7rem', color: '#38bdf8', borderTop: '1px solid rgba(148, 163, 184, 0.15)', paddingTop: '6px', marginTop: '2px', display: 'flex', alignItems: 'center', gap: '4px' }}>
          <Clock size={12} />
          <span>{formatTime(activeTime)}</span>
        </div>
      )}
    </div>
  );
};
