import React, { useState, useEffect, useRef, useMemo } from 'react';
import { MultiRouteResult, RouteResult, WEATHER_MODELS, WeatherModelId } from '../types';
import { TimelineTable } from './TimelineTable';
import {
  Play,
  Pause,
  SkipBack,
  SkipForward,
  RotateCw,
  Calendar,
  X,
  ChevronDown,
  Check,
  Share2,
  Shield,
} from 'lucide-react';

export type AnimationSpeed = 0.5 | 1 | 2;

const SPEED_TO_DURATION_SEC: Record<AnimationSpeed, number> = {
  0.5: 30,
  1: 20,
  2: 10,
};

interface TimelineScrubberProps {
  routeResult: RouteResult | null;
  multiRouteResult?: MultiRouteResult | null;
  activeModel?: WeatherModelId;
  onActiveModelChange?: (modelId: WeatherModelId) => void;
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
  multiRouteResult,
  activeModel = 'gfs_0p25',
  onActiveModelChange,
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

  // State for Departure Time Change Dialog Modal
  const [isDateModalOpen, setIsDateModalOpen] = useState<boolean>(false);
  const [tempDepartureTime, setTempDepartureTime] = useState<string>(departureTime);

  // State for Model Selector Dropdown
  const [isModelDropdownOpen, setIsModelDropdownOpen] = useState<boolean>(false);
  const modelDropdownRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (modelDropdownRef.current && !modelDropdownRef.current.contains(event.target as Node)) {
        setIsModelDropdownOpen(false);
      }
    };
    if (isModelDropdownOpen) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [isModelDropdownOpen]);

  const totalWaypoints = routeResult ? routeResult.waypoints.length : 0;
  const durationSec = SPEED_TO_DURATION_SEC[speedMultiplier];

  const animRef = useRef<number | null>(null);
  const startTimeRef = useRef<number | null>(null);
  const currentIndexRef = useRef<number>(currentIndex);
  const isPlayingRef = useRef<boolean>(isPlaying);
  const durationSecRef = useRef<number>(durationSec);

  currentIndexRef.current = currentIndex;
  isPlayingRef.current = isPlaying;
  durationSecRef.current = durationSec;

  const confidenceGradient = useMemo(() => {
    if (!routeResult || !routeResult.waypoints || routeResult.waypoints.length === 0) {
      return 'linear-gradient(to right, #10b981, #eab308)';
    }
    const wps = routeResult.waypoints;
    const n = wps.length;
    const stops: string[] = [];
    const step = Math.max(1, Math.floor(n / 20));

    for (let i = 0; i < n; i += step) {
      const wp = wps[i];
      const score = wp.confidence_score ?? 85;
      const pct = ((i / (n - 1)) * 100).toFixed(1);
      const color = score >= 75 ? '#10b981' : score >= 50 ? '#eab308' : '#ef4444';
      stops.push(`${color} ${pct}%`);
    }
    const lastScore = wps[n - 1].confidence_score ?? 85;
    const lastColor = lastScore >= 75 ? '#10b981' : lastScore >= 50 ? '#eab308' : '#ef4444';
    stops.push(`${lastColor} 100%`);

    return `linear-gradient(to right, ${stops.join(', ')})`;
  }, [routeResult]);

  const normalizeTimeStr = (t?: string) => {
    if (!t) return '';
    try {
      return new Date(t).toISOString().slice(0, 16);
    } catch {
      return t.slice(0, 16);
    }
  };

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

  const handleExportOrShareGPX = async () => {
    if (!routeResult) return;
    const gpx = `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="SailVPP-Routing" xmlns="http://www.topografix.com/GPX/1/1">
  <metadata>
    <name>${routeResult.boat_name} (${activeModel}) Route</name>
    <time>${routeResult.start_time}</time>
  </metadata>
  <rte>
    <name>${routeResult.boat_name} Optimal Weather Route [${activeModel}]</name>
    ${routeResult.waypoints
      .map(
        (wp) =>
          `<rtept lat="${wp.lat}" lon="${wp.lon}"><time>${wp.time}</time><name>Spd ${wp.boat_speed_kts.toFixed(1)}k</name></rtept>`
      )
      .join('\n    ')}
  </rte>
</gpx>`;
    const fileName = `route_${routeResult.boat_name.replace(/\s+/g, '_')}_${activeModel}.gpx`;
    const blob = new Blob([gpx], { type: 'application/gpx+xml' });

    // Web Share API (native sheet on iOS / Android)
    if (typeof navigator !== 'undefined' && navigator.share) {
      try {
        const file = new File([blob], fileName, { type: 'application/gpx+xml' });
        if (navigator.canShare && navigator.canShare({ files: [file] })) {
          await navigator.share({
            title: `${routeResult.boat_name} Route (${activeModel})`,
            text: `Sailboat weather route for ${routeResult.boat_name} (${activeModel}, ${routeResult.total_duration_hours.toFixed(1)}h)`,
            files: [file],
          });
          return;
        }
      } catch (err: any) {
        if (err.name === 'AbortError') {
          return; // User cancelled share sheet
        }
      }
    }

    // Direct download fallback
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = fileName;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };

  const handleOpenDateModal = () => {
    setTempDepartureTime(departureTime);
    setIsDateModalOpen(true);
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
                <span>Propagating Multi-Models...</span>
              </>
            ) : (
              <>
                <Play size={16} />
                <span>Calculate Routes</span>
              </>
            )}
          </button>
        </div>
      </div>
    );
  }

  const currentWaypoint = routeResult?.waypoints[currentIndex] || routeResult?.waypoints[0];

  const currentConfScore = currentWaypoint?.confidence_score ?? 85;
  const currentConfColor = currentConfScore >= 75 ? '#34d399' : currentConfScore >= 50 ? '#facc15' : '#f87171';
  const currentConfBg = currentConfScore >= 75 ? 'rgba(16, 185, 129, 0.15)' : currentConfScore >= 50 ? 'rgba(234, 179, 8, 0.15)' : 'rgba(239, 68, 68, 0.15)';
  const currentConfBorder = currentConfScore >= 75 ? 'rgba(52, 211, 153, 0.35)' : currentConfScore >= 50 ? 'rgba(250, 204, 21, 0.35)' : 'rgba(248, 113, 113, 0.35)';

  // Active Route Scrubber View
  return (
    <div className="timeline-bar mobile-bottom-sheet">
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

        {/* Timeline Slider with Ensemble Confidence Track */}
        <div className="timeline-slider-wrapper">
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
          <div
            className="confidence-slider-track"
            style={{ background: confidenceGradient }}
            title="Ensemble Predictability Gradient (Green = High Confidence, Yellow = Moderate, Red = High Uncertainty)"
          />
        </div>

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

        {/* Live Waypoint Confidence Pill */}
        {currentWaypoint && (
          <div
            className="waypoint-confidence-pill"
            style={{
              backgroundColor: currentConfBg,
              borderColor: currentConfBorder,
            }}
            title={`Waypoint Predictability: ${currentConfScore.toFixed(0)}%\n• Strategy A (Statistical): ${(currentWaypoint.confidence_score_a ?? currentConfScore).toFixed(0)}%\n• Strategy B (Member Sim): ${(currentWaypoint.confidence_score_b ?? currentConfScore).toFixed(0)}%\n• Wind Speed Spread: ±${(currentWaypoint.wind_speed_std_kts ?? 1.5).toFixed(1)} kt (P10: ${(currentWaypoint.wind_speed_p10_kts ?? currentWaypoint.tws_kts).toFixed(1)}k, P90: ${(currentWaypoint.wind_speed_p90_kts ?? currentWaypoint.tws_kts).toFixed(1)}k)\n• Wind Direction Spread: ±${(currentWaypoint.wind_dir_spread_deg ?? 8).toFixed(0)}°\n• Gale Risk: ${((currentWaypoint.gale_probability ?? 0) * 100).toFixed(0)}%`}
          >
            <Shield size={13} color={currentConfColor} />
            <span className="confidence-score-val" style={{ color: currentConfColor }}>
              {currentConfScore.toFixed(0)}%
            </span>
            <span className="confidence-score-label">Conf</span>
          </div>
        )}

        {/* Desktop Speed Switcher (.5x, 1x, 2x) */}
        <div className="speed-controls-group desktop-only-control">
          <div className="speed-segmented-bar">
            <button
              type="button"
              className={`speed-pill-btn ${speedMultiplier === 0.5 ? 'active' : ''}`}
              onClick={() => handleSpeedChange(0.5)}
              title="0.5x Speed — 30s"
            >
              .5x
            </button>
            <button
              type="button"
              className={`speed-pill-btn ${speedMultiplier === 1 ? 'active' : ''}`}
              onClick={() => handleSpeedChange(1)}
              title="1x Speed — 20s"
            >
              1x
            </button>
            <button
              type="button"
              className={`speed-pill-btn ${speedMultiplier === 2 ? 'active' : ''}`}
              onClick={() => handleSpeedChange(2)}
              title="2x Speed — 10s"
            >
              2x
            </button>
          </div>
        </div>

        {/* Compact Model Selector Dropdown */}
        {multiRouteResult && Object.keys(multiRouteResult).length > 1 && (
          <div className="model-dropdown-container" ref={modelDropdownRef}>
            {(() => {
              const activeMeta = WEATHER_MODELS[activeModel] || {
                id: activeModel,
                name: activeModel.toUpperCase(),
                shortName: activeModel,
                color: '#38bdf8',
                lightColor: '#38bdf8',
                badgeBg: 'rgba(56, 189, 248, 0.15)',
              };
              const activeRoute = multiRouteResult[activeModel];
              const activeDur = activeRoute?.total_duration_hours || 0;
              const activeDurStr = activeDur >= 24
                ? `${Math.floor(activeDur / 24)}d ${Math.round(activeDur % 24)}h`
                : `${activeDur.toFixed(1)}h`;

              return (
                <>
                  <button
                    type="button"
                    className={`model-dropdown-trigger-btn ${isModelDropdownOpen ? 'open' : ''}`}
                    onClick={() => setIsModelDropdownOpen(!isModelDropdownOpen)}
                    title={`Active weather model: ${activeMeta.name}. Click to switch model`}
                    style={{
                      borderColor: isModelDropdownOpen ? activeMeta.lightColor : 'rgba(148, 163, 184, 0.25)',
                    }}
                  >
                    <span
                      className="model-status-dot"
                      style={{ backgroundColor: activeMeta.lightColor }}
                    />
                    <span className="model-btn-name">{activeMeta.shortName}</span>
                    <span className="model-btn-duration" style={{ color: activeMeta.lightColor }}>
                      {activeDurStr}
                    </span>
                    <ChevronDown size={13} className={`model-dropdown-chevron ${isModelDropdownOpen ? 'rotated' : ''}`} />
                  </button>

                  {isModelDropdownOpen && (
                    <div className="model-dropdown-menu">
                      <div className="model-dropdown-header">WEATHER MODEL</div>
                      {Object.entries(multiRouteResult).map(([mId, r]) => {
                        const meta = WEATHER_MODELS[mId] || {
                          id: mId,
                          name: mId.toUpperCase(),
                          shortName: mId,
                          color: '#0284c7',
                          lightColor: '#38bdf8',
                          badgeBg: 'rgba(56, 189, 248, 0.15)',
                        };
                        const isSelected = mId === activeModel;
                        const durHours = r?.total_duration_hours || 0;
                        const durStr = durHours >= 24
                          ? `${Math.floor(durHours / 24)}d ${Math.round(durHours % 24)}h`
                          : `${durHours.toFixed(1)}h`;

                        return (
                          <button
                            key={mId}
                            type="button"
                            className={`model-dropdown-item ${isSelected ? 'selected' : ''}`}
                            onClick={() => {
                              onActiveModelChange?.(mId);
                              setIsModelDropdownOpen(false);
                            }}
                          >
                            <div className="model-dropdown-item-left">
                              <span
                                className="model-status-dot"
                                style={{ backgroundColor: meta.lightColor }}
                              />
                              <div className="model-dropdown-item-text">
                                <span className="model-dropdown-item-name">{meta.name}</span>
                              </div>
                            </div>
                            <div className="model-dropdown-item-right">
                              <span className="model-dropdown-item-dur" style={{ color: isSelected ? meta.lightColor : 'var(--text-muted)' }}>
                                {durStr}
                              </span>
                              {isSelected && <Check size={13} color={meta.lightColor} />}
                            </div>
                          </button>
                        );
                      })}
                    </div>
                  )}
                </>
              );
            })()}
          </div>
        )}

        {/* Action Buttons: Recalculate & GPX Export / Share */}
        <div className="dock-actions-cluster">
          <button
            type="button"
            className={`dock-recalc-btn ${isRecalcActive ? 'active-changed' : ''}`}
            onClick={handleOpenDateModal}
            disabled={loading}
            title={loading ? 'Recalculating routes...' : 'Change departure time & recalculate routes'}
            aria-label="Recalculate route"
          >
            <RotateCw size={14} className={loading ? 'animate-spin' : ''} />
          </button>

          <button
            type="button"
            className="dock-gpx-btn"
            onClick={handleExportOrShareGPX}
            title="Share or download GPX route"
            aria-label="Share or download GPX route"
          >
            <Share2 size={14} />
          </button>
        </div>
      </div>

      {/* Compact Passage Timeline Table Row */}
      <TimelineTable
        routeResult={routeResult}
        currentIndex={currentIndex}
        onIndexChange={onIndexChange}
      />

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
                Select a new departure date and time in UTC. Updating will recalculate optimal routes across all available weather models (NOAA GFS, ECMWF IFS, DWD ICON).
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
                className="btn-primary"
                onClick={handleApplyAndRecalculate}
                disabled={loading}
              >
                <RotateCw size={14} className={loading ? 'animate-spin' : ''} />
                <span>{loading ? 'Updating...' : 'Update'}</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default TimelineScrubber;
