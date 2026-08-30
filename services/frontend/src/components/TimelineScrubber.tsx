import React, { useState, useEffect, useRef } from 'react';
import { RouteResult } from '../types';
import {
  Play,
  Pause,
  SkipBack,
  SkipForward,
  Clock,
  Download,
  RotateCw,
  Calendar,
  Edit2,
  X,
  Check,
  ChevronUp,
  ChevronDown,
} from 'lucide-react';

export type AnimationSpeed = 0.5 | 1 | 2;

const SPEED_TO_DURATION_SEC: Record<AnimationSpeed, number> = {
  0.5: 30,
  1: 20,
  2: 10,
};

interface TimelineScrubberProps {
  routeResult: RouteResult | null;
  currentIndex: number;
  onIndexChange: (idx: number) => void;
  departureTime: string;
  onDepartureTimeChange: (timeStr: string) => void;
  onCalculateRoute: () => void;
  loading: boolean;
  isRecalculateActive?: boolean;
}

export const TimelineScrubber: React.FC<TimelineScrubberProps> = ({
  routeResult,
  currentIndex,
  onIndexChange,
  departureTime,
  onDepartureTimeChange,
  onCalculateRoute,
  loading,
  isRecalculateActive,
}) => {
  const [isPlaying, setIsPlaying] = useState<boolean>(false);
  const [speedMultiplier, setSpeedMultiplier] = useState<AnimationSpeed>(1);

  // Mobile drawer expanded state (default collapsed on mobile)
  const [isMobileExpanded, setIsMobileExpanded] = useState<boolean>(false);

  // State for Departure Time Change Dialog Modal
  const [isDateModalOpen, setIsDateModalOpen] = useState<boolean>(false);
  const [tempDepartureTime, setTempDepartureTime] = useState<string>(departureTime);

  const totalWaypoints = routeResult ? routeResult.waypoints.length : 0;
  const currentWp = routeResult && routeResult.waypoints[currentIndex] ? routeResult.waypoints[currentIndex] : null;
  const durationSec = SPEED_TO_DURATION_SEC[speedMultiplier];

  const animRef = useRef<number | null>(null);
  const startTimeRef = useRef<number | null>(null);
  const currentIndexRef = useRef<number>(currentIndex);
  const isPlayingRef = useRef<boolean>(isPlaying);
  const durationSecRef = useRef<number>(durationSec);

  currentIndexRef.current = currentIndex;
  isPlayingRef.current = isPlaying;
  durationSecRef.current = durationSec;

  const normalizeTimeStr = (t?: string) => {
    if (!t) return '';
    try {
      return new Date(t).toISOString().slice(0, 16);
    } catch {
      return t.slice(0, 16);
    }
  };

  // Recalculate is active when any setting (boat, penalties, departure time, waypoints) has changed
  const isDepartureChanged = routeResult
    ? normalizeTimeStr(departureTime) !== normalizeTimeStr(routeResult.start_time)
    : false;

  const isRecalcActive = isRecalculateActive !== undefined ? isRecalculateActive : isDepartureChanged;

  const handlePlayToggle = () => {
    if (!isPlaying) {
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

  const handleOpenDateModal = () => {
    setTempDepartureTime(departureTime);
    setIsDateModalOpen(true);
  };

  const handleApplyDepartureOnly = () => {
    onDepartureTimeChange(tempDepartureTime);
    setIsDateModalOpen(false);
  };

  const handleApplyAndRecalculate = () => {
    onDepartureTimeChange(tempDepartureTime);
    setIsDateModalOpen(false);
    setTimeout(() => {
      onCalculateRoute();
    }, 50);
  };

  const adjustTempHours = (hours: number) => {
    try {
      const d = new Date(tempDepartureTime);
      d.setHours(d.getHours() + hours);
      setTempDepartureTime(d.toISOString().slice(0, 16));
    } catch {
      // fallback
    }
  };

  // If no route calculated yet: render the bottom panel with departure & calculate button
  if (!routeResult) {
    return (
      <div className="timeline-bar uncalculated-dock mobile-bottom-sheet">
        <div className="uncalculated-dock-inner">
          
          {/* Departure Date & Time */}
          <div className="dock-input-group">
            <label className="dock-label">
              <Calendar size={13} color="#38bdf8" />
              <span>Departure Time (UTC)</span>
            </label>
            <input
              type="datetime-local"
              className="input-field dock-time-input"
              value={departureTime}
              onChange={(e) => onDepartureTimeChange(e.target.value)}
            />
          </div>

          {/* Calculate Route Action Button */}
          <button
            className="btn-primary dock-calc-btn"
            onClick={onCalculateRoute}
            disabled={loading}
          >
            {loading ? (
              <>
                <RotateCw size={16} className="animate-spin" />
                <span>Propagating...</span>
              </>
            ) : (
              <>
                <Play size={16} />
                <span>Calculate Route</span>
              </>
            )}
          </button>

        </div>
      </div>
    );
  }

  // Active Route Scrubber View
  const pos = currentWp ? getPointOfSail(currentWp.twa_deg, currentWp.heading_deg, currentWp.twd_deg) : { name: '', tackShort: '' };
  const progressPercent = totalWaypoints > 1 ? ((currentIndex / (totalWaypoints - 1)) * 100).toFixed(0) : '0';

  return (
    <div className={`timeline-bar mobile-bottom-sheet ${isMobileExpanded ? 'mobile-expanded' : 'mobile-collapsed'}`}>
      {/* Top Playback & Control Strip */}
      <div className="timeline-controls">
        {/* Play/Pause Button */}
        <button
          className="play-btn"
          onClick={handlePlayToggle}
          title={isPlaying ? 'Pause Animation' : 'Play Passage Animation'}
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

        {/* Desktop Speed Switcher (0.5x, 1x, 2x) */}
        <div className="speed-controls-group desktop-only-control">
          <Clock size={13} className="speed-clock-icon" />
          <div className="speed-segmented-bar">
            <button
              type="button"
              className={`speed-pill-btn ${speedMultiplier === 0.5 ? 'active' : ''}`}
              onClick={() => handleSpeedChange(0.5)}
              title="0.5x Speed — 30 seconds"
            >
              0.5x
            </button>
            <button
              type="button"
              className={`speed-pill-btn ${speedMultiplier === 1 ? 'active' : ''}`}
              onClick={() => handleSpeedChange(1)}
              title="1x Speed — 20 seconds"
            >
              1x
            </button>
            <button
              type="button"
              className={`speed-pill-btn ${speedMultiplier === 2 ? 'active' : ''}`}
              onClick={() => handleSpeedChange(2)}
              title="2x Speed — 10 seconds"
            >
              2x
            </button>
          </div>
        </div>

        {/* Progress % */}
        <span className="timeline-progress-badge">{progressPercent}%</span>

        {/* Desktop Action Buttons: Recalculate & GPX Export */}
        <div className="dock-actions-cluster desktop-only-control">
          <button
            type="button"
            className={`dock-recalc-btn ${isRecalcActive ? 'active-changed' : 'disabled'}`}
            onClick={onCalculateRoute}
            disabled={!isRecalcActive || loading}
            title={
              isRecalcActive
                ? 'Settings or departure time changed. Click to recalculate optimal route'
                : 'Route is up to date with current parameters'
            }
          >
            <RotateCw size={13} className={loading ? 'animate-spin' : ''} />
            <span>{loading ? 'Solving...' : 'Recalculate'}</span>
          </button>

          <button
            type="button"
            className="dock-gpx-btn"
            onClick={handleExportGPX}
            title="Download GPX route"
          >
            <Download size={13} />
            <span>GPX</span>
          </button>
        </div>

        {/* Mobile Drawer Expand Toggle Button (Chevron Up/Down) */}
        <button
          type="button"
          className="mobile-expand-toggle-btn"
          onClick={() => setIsMobileExpanded(!isMobileExpanded)}
          title={isMobileExpanded ? 'Collapse controls' : 'Expand speed, departure & passage settings'}
        >
          {isMobileExpanded ? <ChevronDown size={18} /> : <ChevronUp size={18} />}
        </button>
      </div>

      {/* Mobile Expanded Settings Drawer */}
      {isMobileExpanded && (
        <div className="mobile-expanded-drawer">
          {/* Row 1: Speed selector centered under scrubber */}
          <div className="mobile-speed-centered-row">
            <div className="speed-segmented-bar">
              <button
                type="button"
                className={`speed-pill-btn ${speedMultiplier === 0.5 ? 'active' : ''}`}
                onClick={() => handleSpeedChange(0.5)}
              >
                0.5x
              </button>
              <button
                type="button"
                className={`speed-pill-btn ${speedMultiplier === 1 ? 'active' : ''}`}
                onClick={() => handleSpeedChange(1)}
              >
                1x
              </button>
              <button
                type="button"
                className={`speed-pill-btn ${speedMultiplier === 2 ? 'active' : ''}`}
                onClick={() => handleSpeedChange(2)}
              >
                2x
              </button>
            </div>
          </div>

          {/* Row 2: Departure, Recalculate, and GPX buttons in one line */}
          <div className="mobile-actions-unified-row">
            <button
              type="button"
              className="mobile-unified-btn btn-departure-trigger"
              onClick={handleOpenDateModal}
              title="Change departure time"
            >
              <Calendar size={13} />
              <span>Departure</span>
            </button>

            <button
              type="button"
              className={`dock-recalc-btn mobile-unified-btn ${isRecalcActive ? 'active-changed' : 'disabled'}`}
              onClick={onCalculateRoute}
              disabled={!isRecalcActive || loading}
              title={isRecalcActive ? 'Click to recalculate route' : 'Route is up to date'}
            >
              <RotateCw size={13} className={loading ? 'animate-spin' : ''} />
              <span>{loading ? 'Solving...' : 'Recalculate'}</span>
            </button>

            <button
              type="button"
              className="dock-gpx-btn mobile-unified-btn"
              onClick={handleExportGPX}
              title="Download GPX route"
            >
              <Download size={13} />
              <span>GPX</span>
            </button>
          </div>

          {/* Row 3: Current passage state displayed smaller below */}
          {currentWp && (
            <div className="mobile-compact-passage-state">
              <div className="compact-state-chip">
                <span className="compact-state-label">SPEED</span>
                <strong className="compact-state-val text-sky">{currentWp.boat_speed_kts.toFixed(1)}k</strong>
              </div>
              <div className="compact-state-chip">
                <span className="compact-state-label">HDG</span>
                <strong className="compact-state-val">{currentWp.heading_deg.toFixed(0)}°</strong>
              </div>
              <div className="compact-state-chip">
                <span className="compact-state-label">WIND</span>
                <strong className="compact-state-val">{currentWp.tws_kts.toFixed(1)}k @ {currentWp.twd_deg.toFixed(0)}°</strong>
              </div>
              <div className="compact-state-chip">
                <span className="compact-state-label">SAIL</span>
                <strong className="compact-state-val text-sky">{pos.name} ({pos.tackShort})</strong>
              </div>
              <div className="compact-state-chip">
                <span className="compact-state-label">HEEL</span>
                <strong
                  className="compact-state-val"
                  style={{ color: currentWp.estimated_heel_deg > 20 ? '#f59e0b' : '#10b981' }}
                >
                  {currentWp.estimated_heel_deg.toFixed(1)}°
                </strong>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Desktop-Only Real-time Passage Telemetry Grid (Hidden on Mobile) */}
      {currentWp && (
        <div className="timeline-stats desktop-only-telemetry">
          {/* Clickable Departure Time Box with Edit Pencil Indicator */}
          <div
            className="stat-box clickable-stat-box"
            onClick={handleOpenDateModal}
            title="Click to change departure start time"
          >
            <div className="stat-label-with-icon">
              <span className="stat-label">DEPARTURE TIME</span>
              <Edit2 size={10} color="#38bdf8" />
            </div>
            <span className="stat-value time-stat-clickable" style={{ fontSize: '0.75rem' }}>
              {formatDate(departureTime || routeResult.start_time)}
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
      )}

      {/* Departure Time Change Modal Dialog */}
      {isDateModalOpen && (
        <div className="modal-backdrop" onClick={() => setIsDateModalOpen(false)}>
          <div className="departure-time-modal" onClick={(e) => e.stopPropagation()}>
            <div className="departure-modal-header">
              <div className="departure-modal-title">
                <Calendar size={18} color="#38bdf8" />
                <span>Change Departure Time (UTC)</span>
              </div>
              <button
                type="button"
                className="btn-modal-close"
                onClick={() => setIsDateModalOpen(false)}
              >
                <X size={18} />
              </button>
            </div>

            <div className="departure-modal-body">
              <p className="departure-modal-desc">
                Select a new departure date and time in UTC. Updating will recalculate the optimal route against the GFS weather forecast starting at this time.
              </p>

              <div className="input-group">
                <label className="departure-modal-label">Departure Date &amp; Time (UTC)</label>
                <input
                  type="datetime-local"
                  className="input-field departure-datetime-input"
                  value={tempDepartureTime}
                  onChange={(e) => setTempDepartureTime(e.target.value)}
                  autoFocus
                />
              </div>

              {/* Quick Offset Shortcuts */}
              <div className="quick-time-shortcuts">
                <span className="shortcuts-label">Quick offsets:</span>
                <div className="shortcuts-buttons">
                  <button
                    type="button"
                    className="btn-time-shortcut"
                    onClick={() => setTempDepartureTime(new Date().toISOString().slice(0, 16))}
                  >
                    Now (UTC)
                  </button>
                  <button
                    type="button"
                    className="btn-time-shortcut"
                    onClick={() => adjustTempHours(6)}
                  >
                    +6h
                  </button>
                  <button
                    type="button"
                    className="btn-time-shortcut"
                    onClick={() => adjustTempHours(12)}
                  >
                    +12h
                  </button>
                  <button
                    type="button"
                    className="btn-time-shortcut"
                    onClick={() => adjustTempHours(24)}
                  >
                    +24h
                  </button>
                  <button
                    type="button"
                    className="btn-time-shortcut"
                    onClick={() => adjustTempHours(-6)}
                  >
                    -6h
                  </button>
                </div>
              </div>
            </div>

            <div className="departure-modal-footer">
              <button
                type="button"
                className="btn-modal-cancel"
                onClick={() => setIsDateModalOpen(false)}
              >
                Cancel
              </button>
              <button
                type="button"
                className="btn-modal-cancel"
                style={{ borderColor: 'rgba(56, 189, 248, 0.3)', color: '#38bdf8' }}
                onClick={handleApplyDepartureOnly}
              >
                <Check size={14} />
                <span>Apply Time</span>
              </button>
              <button
                type="button"
                className="btn-primary"
                onClick={handleApplyAndRecalculate}
                disabled={loading}
              >
                <RotateCw size={14} className={loading ? 'animate-spin' : ''} />
                <span>Update &amp; Recalculate</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default TimelineScrubber;
