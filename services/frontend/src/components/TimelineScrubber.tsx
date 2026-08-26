import React, { useState, useEffect } from 'react';
import { RouteResult } from '../types';
import { Play, Pause, SkipBack, SkipForward } from 'lucide-react';

interface TimelineScrubberProps {
  routeResult: RouteResult;
  currentIndex: number;
  onIndexChange: (idx: number) => void;
}

export const TimelineScrubber: React.FC<TimelineScrubberProps> = ({
  routeResult,
  currentIndex,
  onIndexChange,
}) => {
  const [isPlaying, setIsPlaying] = useState(false);
  const totalWaypoints = routeResult.waypoints.length;
  const currentWp = routeResult.waypoints[currentIndex] || routeResult.waypoints[0];

  useEffect(() => {
    let interval: ReturnType<typeof setInterval> | null = null;
    if (isPlaying) {
      interval = setInterval(() => {
        onIndexChange((currentIndex + 1) % totalWaypoints);
      }, 350);
    }
    return () => {
      if (interval) clearInterval(interval);
    };
  }, [isPlaying, currentIndex, totalWaypoints, onIndexChange]);

  const formatDate = (isoStr: string) => {
    try {
      const d = new Date(isoStr);
      return d.toUTCString().replace('GMT', 'UTC').slice(0, 22);
    } catch {
      return isoStr;
    }
  };

  const getPointOfSail = (twa: number, heading: number, twd: number) => {
    let rel = (twd - heading) % 360;
    if (rel > 180) rel -= 360;
    if (rel < -180) rel += 360;

    const tack = rel >= 0 ? 'Starboard Tack' : 'Port Tack';
    const tackShort = rel >= 0 ? 'Stbd' : 'Port';

    let name = 'Close-Hauled';
    if (twa < 28) name = 'In Irons (No-Go)';
    else if (twa <= 55) name = 'Close-Hauled';
    else if (twa <= 80) name = 'Close Reach';
    else if (twa <= 110) name = 'Beam Reach';
    else if (twa <= 150) name = 'Broad Reach';
    else name = 'Running';

    return { name, tack, tackShort };
  };

  const pos = getPointOfSail(currentWp.twa_deg, currentWp.heading_deg, currentWp.twd_deg);

  return (
    <div className="timeline-bar">
      <div className="timeline-controls">
        <button
          className="play-btn"
          onClick={() => setIsPlaying(!isPlaying)}
          title={isPlaying ? 'Pause' : 'Play'}
        >
          {isPlaying ? <Pause size={18} /> : <Play size={18} />}
        </button>

        <button
          style={{ background: 'transparent', border: 'none', color: '#94a3b8', cursor: 'pointer' }}
          onClick={() => onIndexChange(0)}
          title="Restart"
        >
          <SkipBack size={18} />
        </button>

        <input
          type="range"
          min={0}
          max={totalWaypoints - 1}
          value={currentIndex}
          onChange={(e) => {
            setIsPlaying(false);
            onIndexChange(parseInt(e.target.value, 10));
          }}
          className="timeline-slider"
        />

        <button
          style={{ background: 'transparent', border: 'none', color: '#94a3b8', cursor: 'pointer' }}
          onClick={() => onIndexChange(totalWaypoints - 1)}
          title="End of Route"
        >
          <SkipForward size={18} />
        </button>
      </div>

      <div className="timeline-stats">
        <div className="stat-box">
          <span className="stat-label">TIME (UTC)</span>
          <span className="stat-value" style={{ fontSize: '0.75rem' }}>{formatDate(currentWp.time)}</span>
        </div>

        <div className="stat-box">
          <span className="stat-label">BOAT SPEED</span>
          <span className="stat-value" style={{ color: '#38bdf8' }}>{currentWp.boat_speed_kts.toFixed(2)} kts</span>
        </div>

        <div className="stat-box">
          <span className="stat-label">HEADING</span>
          <span className="stat-value">{currentWp.heading_deg.toFixed(1)}°</span>
        </div>

        <div className="stat-box">
          <span className="stat-label">TRUE WIND</span>
          <span className="stat-value">{currentWp.tws_kts.toFixed(1)} kts @ {currentWp.twd_deg.toFixed(0)}°</span>
        </div>

        <div className="stat-box">
          <span className="stat-label">TWA</span>
          <span className="stat-value">{currentWp.twa_deg.toFixed(1)}°</span>
        </div>

        <div className="stat-box">
          <span className="stat-label">POINT OF SAIL</span>
          <span className="stat-value" style={{ color: '#38bdf8', fontSize: '0.8rem' }}>
            {pos.name} <span style={{ color: '#94a3b8', fontSize: '0.7rem' }}>({pos.tackShort})</span>
          </span>
        </div>

        <div className="stat-box">
          <span className="stat-label">HEEL ANGLE</span>
          <span className="stat-value" style={{ color: currentWp.estimated_heel_deg > 20 ? '#f59e0b' : '#10b981' }}>
            {currentWp.estimated_heel_deg.toFixed(1)}°
          </span>
        </div>
      </div>
    </div>
  );
};

export default TimelineScrubber;
