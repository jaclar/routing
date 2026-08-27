import React, { useState, useEffect, useRef } from 'react';
import { RouteResult } from '../types';
import { Play, Pause, SkipBack, SkipForward, Clock } from 'lucide-react';

interface TimelineScrubberProps {
  routeResult: RouteResult;
  currentIndex: number;
  onIndexChange: (idx: number) => void;
}

export type AnimationSpeed = 0.5 | 1 | 2;

// Speed to constant duration mapping as requested:
// 0.5x -> 30s
// 1x   -> 20s
// 2x   -> 10s
const SPEED_TO_DURATION_SEC: Record<AnimationSpeed, number> = {
  0.5: 30,
  1: 20,
  2: 10,
};

export const TimelineScrubber: React.FC<TimelineScrubberProps> = ({
  routeResult,
  currentIndex,
  onIndexChange,
}) => {
  const [isPlaying, setIsPlaying] = useState<boolean>(false);
  const [speedMultiplier, setSpeedMultiplier] = useState<AnimationSpeed>(1);

  const totalWaypoints = routeResult.waypoints.length;
  const currentWp = routeResult.waypoints[currentIndex] || routeResult.waypoints[0];
  const durationSec = SPEED_TO_DURATION_SEC[speedMultiplier];

  const animRef = useRef<number | null>(null);
  const startTimeRef = useRef<number | null>(null);
  const currentIndexRef = useRef<number>(currentIndex);
  const isPlayingRef = useRef<boolean>(isPlaying);
  const durationSecRef = useRef<number>(durationSec);

  currentIndexRef.current = currentIndex;
  isPlayingRef.current = isPlaying;
  durationSecRef.current = durationSec;

  const handlePlayToggle = () => {
    if (!isPlaying) {
      // If at or past the end, start over from 0
      if (currentIndex >= totalWaypoints - 1) {
        onIndexChange(0);
        currentIndexRef.current = 0;
      }
      setIsPlaying(true);
    } else {
      setIsPlaying(false);
    }
  };

  const handleSpeedChange = (newSpeed: AnimationSpeed) => {
    setSpeedMultiplier(newSpeed);
    const newDurationSec = SPEED_TO_DURATION_SEC[newSpeed];
    if (isPlayingRef.current && totalWaypoints > 1) {
      // Adjust start time to preserve current progress without jumping
      const progress = currentIndexRef.current / (totalWaypoints - 1);
      startTimeRef.current = performance.now() - progress * (newDurationSec * 1000);
    }
  };

  useEffect(() => {
    if (!isPlaying || totalWaypoints <= 1) {
      if (animRef.current) {
        cancelAnimationFrame(animRef.current);
        animRef.current = null;
      }
      return;
    }

    const totalDurMs = durationSecRef.current * 1000;
    const initialProgress = currentIndexRef.current / (totalWaypoints - 1);
    startTimeRef.current = performance.now() - initialProgress * totalDurMs;

    const tick = (now: number) => {
      if (!startTimeRef.current) startTimeRef.current = now;
      const elapsed = now - startTimeRef.current;
      const progress = Math.min(1, elapsed / (durationSecRef.current * 1000));
      const targetIndex = Math.min(
        totalWaypoints - 1,
        Math.floor(progress * (totalWaypoints - 1))
      );

      if (targetIndex !== currentIndexRef.current) {
        onIndexChange(targetIndex);
      }

      if (progress < 1) {
        animRef.current = requestAnimationFrame(tick);
      } else {
        onIndexChange(totalWaypoints - 1);
        setIsPlaying(false);
      }
    };

    animRef.current = requestAnimationFrame(tick);

    return () => {
      if (animRef.current) {
        cancelAnimationFrame(animRef.current);
        animRef.current = null;
      }
    };
  }, [isPlaying, totalWaypoints, onIndexChange]);

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
  const progressPercent = totalWaypoints > 1 ? ((currentIndex / (totalWaypoints - 1)) * 100).toFixed(0) : '0';

  return (
    <div className="timeline-bar">
      <div className="timeline-controls">
        {/* Play/Pause Button */}
        <button
          className="play-btn"
          onClick={handlePlayToggle}
          title={isPlaying ? 'Pause Animation' : 'Play Constant-Time Animation'}
        >
          {isPlaying ? <Pause size={18} /> : <Play size={18} />}
        </button>

        {/* Skip to Start */}
        <button
          className="timeline-nav-btn"
          onClick={() => {
            setIsPlaying(false);
            onIndexChange(0);
          }}
          title="Restart from Departure"
        >
          <SkipBack size={18} />
        </button>

        {/* Timeline Slider */}
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

        {/* Skip to Destination */}
        <button
          className="timeline-nav-btn"
          onClick={() => {
            setIsPlaying(false);
            onIndexChange(totalWaypoints - 1);
          }}
          title="Jump to Destination"
        >
          <SkipForward size={18} />
        </button>

        {/* Constant-Time Animation Speed Switcher (0.5x: 30s | 1x: 20s | 2x: 10s) */}
        <div className="speed-controls-group">
          <Clock size={13} className="speed-clock-icon" />
          <div className="speed-segmented-bar">
            <button
              type="button"
              className={`speed-pill-btn ${speedMultiplier === 0.5 ? 'active' : ''}`}
              onClick={() => handleSpeedChange(0.5)}
              title="0.5x Speed — 30 seconds total visualization"
            >
              0.5x
            </button>
            <button
              type="button"
              className={`speed-pill-btn ${speedMultiplier === 1 ? 'active' : ''}`}
              onClick={() => handleSpeedChange(1)}
              title="1x Speed (Standard) — 20 seconds total visualization"
            >
              1x
            </button>
            <button
              type="button"
              className={`speed-pill-btn ${speedMultiplier === 2 ? 'active' : ''}`}
              onClick={() => handleSpeedChange(2)}
              title="2x Speed (Fast) — 10 seconds total visualization"
            >
              2x
            </button>
          </div>
        </div>

        {/* Progress % */}
        <span className="timeline-progress-badge">{progressPercent}%</span>
      </div>

      {/* Real-time Passage Telemetry Grid */}
      <div className="timeline-stats">
        <div className="stat-box">
          <span className="stat-label">TIME (UTC)</span>
          <span className="stat-value" style={{ fontSize: '0.75rem' }}>
            {formatDate(currentWp.time)}
          </span>
        </div>

        <div className="stat-box">
          <span className="stat-label">BOAT SPEED</span>
          <span className="stat-value" style={{ color: '#38bdf8' }}>
            {currentWp.boat_speed_kts.toFixed(2)} kts
          </span>
        </div>

        <div className="stat-box">
          <span className="stat-label">HEADING</span>
          <span className="stat-value">{currentWp.heading_deg.toFixed(1)}°</span>
        </div>

        <div className="stat-box">
          <span className="stat-label">TRUE WIND</span>
          <span className="stat-value">
            {currentWp.tws_kts.toFixed(1)} kts @ {currentWp.twd_deg.toFixed(0)}°
          </span>
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
          <span
            className="stat-value"
            style={{ color: currentWp.estimated_heel_deg > 20 ? '#f59e0b' : '#10b981' }}
          >
            {currentWp.estimated_heel_deg.toFixed(1)}°
          </span>
        </div>
      </div>
    </div>
  );
};

export default TimelineScrubber;
