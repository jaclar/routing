import React, { useMemo, useRef, useEffect, useState } from 'react';
import { RouteResult } from '../types';

interface TimelineTableProps {
  routeResult: RouteResult | null;
  currentIndex: number;
  onIndexChange: (idx: number) => void;
}

interface TimelineColumnData {
  stepIndex: number;
  elapsedHours: number;
  timeStr: string;
  clockTime: string;
  dayOffsetStr: string;
  boatSpeedKts: number;
  twsKts: number;
  gustKts: number;
  waveHeightM: number;
  wavePeriodS: number;
  twdDeg: number;
  twaDeg: number;
  headingDeg: number;
  relWindDeg: number;
  tack: 'Stbd' | 'Port';
  waypointIndex: number;
}

const COLUMN_WIDTH_PX = 60;

/**
 * Maps true wind speed (kts) to RGB color matching the map's heatmap ramp:
 * 0 kts: Blue
 * 10 kts: Green
 * 20 kts: Yellow
 * 30 kts: Orange
 * 40 kts: Red
 * 50+ kts: Violet
 */
export function getWindColor(tws: number): string {
  if (tws <= 0) return 'rgb(29, 78, 216)';
  if (tws < 10) {
    const f = tws / 10;
    const r = Math.round(29 + f * (16 - 29));
    const g = Math.round(78 + f * (185 - 78));
    const b = Math.round(216 + f * (129 - 216));
    return `rgb(${r}, ${g}, ${b})`;
  } else if (tws < 20) {
    const f = (tws - 10) / 10;
    const r = Math.round(16 + f * (250 - 16));
    const g = Math.round(185 + f * (204 - 185));
    const b = Math.round(129 + f * (21 - 129));
    return `rgb(${r}, ${g}, ${b})`;
  } else if (tws < 30) {
    const f = (tws - 20) / 10;
    const r = Math.round(250 + f * (249 - 250));
    const g = Math.round(204 + f * (115 - 204));
    const b = Math.round(21 + f * (22 - 21));
    return `rgb(${r}, ${g}, ${b})`;
  } else if (tws < 40) {
    const f = (tws - 30) / 10;
    const r = Math.round(249 + f * (239 - 249));
    const g = Math.round(115 + f * (68 - 115));
    const b = Math.round(22 + f * (68 - 22));
    return `rgb(${r}, ${g}, ${b})`;
  } else if (tws < 50) {
    const f = (tws - 40) / 10;
    const r = Math.round(239 + f * (168 - 239));
    const g = Math.round(68 + f * (85 - 68));
    const b = Math.round(68 + f * (247 - 68));
    return `rgb(${r}, ${g}, ${b})`;
  } else {
    const f = Math.min((tws - 50) / 20, 1.0);
    const r = Math.round(168 + f * (139 - 168));
    const g = Math.round(85 + f * (92 - 85));
    const b = Math.round(247 + f * (246 - 247));
    return `rgb(${r}, ${g}, ${b})`;
  }
}

import { getPointOfSailColor } from '../config/pointOfSail';
export { getPointOfSailColor };

/**
 * Maps wave height and period to color representing sea state intensity / steepness:
 * - Steepness increases with higher wave height and lower wave period.
 * - Calm / Gentle: Soft Cyan (#38bdf8)
 * - Moderate: Emerald Green (#10b981)
 * - Choppy / Building: Amber Yellow (#eab308)
 * - Rough / Steep: Vivid Orange (#f97316)
 * - Very Rough / Violent: Crimson Red (#ef4444)
 */
export function getWaveIntensityColor(heightM: number, periodS: number): string {
  const h = Math.max(0, heightM);
  const t = Math.max(3.0, periodS || 7.0);
  // Intensity index based on wave height & steepness (H / T^0.9)
  const intensity = h * Math.pow(7.5 / t, 0.9);

  if (intensity < 0.9) {
    return '#38bdf8'; // Calm / Gentle
  } else if (intensity < 1.7) {
    return '#10b981'; // Moderate
  } else if (intensity < 2.6) {
    return '#eab308'; // Choppy / Building
  } else if (intensity < 3.8) {
    return '#f97316'; // Rough / Steep
  } else {
    return '#ef4444'; // Very Rough / Violent
  }
}

/**
 * Calculates adaptive time step size (hours) for timeline columns:
 * - At least 1 hour step
 * - Up to 4 hours for longer ocean passages
 */
export function getTimelineStepHours(totalDurationHours: number): number {
  if (totalDurationHours <= 36) {
    return 1;
  } else if (totalDurationHours <= 84) {
    return 2;
  } else if (totalDurationHours <= 168) {
    return 3;
  } else {
    return 4;
  }
}

/**
 * Interpolates circular angle in degrees smoothly
 */
function interpolateAngle(a: number, b: number, f: number): number {
  let diff = (b - a) % 360;
  if (diff > 180) diff -= 360;
  if (diff < -180) diff += 360;
  return (a + diff * f + 360) % 360;
}

export const TimelineTable: React.FC<TimelineTableProps> = ({
  routeResult,
  currentIndex,
  onIndexChange,
}) => {
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  const [viewportWidth, setViewportWidth] = useState<number>(600);

  // Monitor viewport container width dynamically
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;

    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        if (entry.contentRect.width > 0) {
          setViewportWidth(entry.contentRect.width);
        }
      }
    });

    ro.observe(container);
    setViewportWidth(container.clientWidth || 600);

    return () => ro.disconnect();
  }, []);

  // Generate sampled & interpolated timeline column data
  const columns: TimelineColumnData[] = useMemo(() => {
    if (!routeResult || !routeResult.waypoints || routeResult.waypoints.length === 0) {
      return [];
    }

    const wps = routeResult.waypoints;
    const totalDuration = Math.max(0.1, routeResult.total_duration_hours);
    const stepHours = getTimelineStepHours(totalDuration);
    const startTimeMs = new Date(routeResult.start_time).getTime();

    // Map each waypoint's elapsed time from passage start in hours
    const wpTimesHours = wps.map((wp) => {
      const wpMs = new Date(wp.time).getTime();
      return Math.max(0, (wpMs - startTimeMs) / (3600 * 1000));
    });

    const cols: TimelineColumnData[] = [];
    const numSteps = Math.ceil(totalDuration / stepHours);

    for (let s = 0; s <= numSteps; s++) {
      let targetElapsed = s * stepHours;
      if (targetElapsed > totalDuration) {
        targetElapsed = totalDuration;
      }

      // Find bracketing waypoints
      let wpAIdx = 0;
      let wpBIdx = 0;
      let f = 0;

      if (targetElapsed <= wpTimesHours[0]) {
        wpAIdx = 0;
        wpBIdx = 0;
        f = 0;
      } else if (targetElapsed >= wpTimesHours[wpTimesHours.length - 1]) {
        wpAIdx = wpTimesHours.length - 1;
        wpBIdx = wpTimesHours.length - 1;
        f = 0;
      } else {
        for (let i = 0; i < wpTimesHours.length - 1; i++) {
          if (wpTimesHours[i] <= targetElapsed && wpTimesHours[i + 1] >= targetElapsed) {
            wpAIdx = i;
            wpBIdx = i + 1;
            const span = wpTimesHours[i + 1] - wpTimesHours[i];
            f = span > 0 ? (targetElapsed - wpTimesHours[i]) / span : 0;
            break;
          }
        }
      }

      const wpA = wps[wpAIdx];
      const wpB = wps[wpBIdx];

      // Interpolate values
      const boatSpeedKts = wpA.boat_speed_kts * (1 - f) + wpB.boat_speed_kts * f;
      const twsKts = wpA.tws_kts * (1 - f) + wpB.tws_kts * f;

      const gustA = wpA.gust_kts !== undefined && wpA.gust_kts > 0 ? wpA.gust_kts : wpA.tws_kts * 1.25 + 1.5;
      const gustB = wpB.gust_kts !== undefined && wpB.gust_kts > 0 ? wpB.gust_kts : wpB.tws_kts * 1.25 + 1.5;
      const gustKts = gustA * (1 - f) + gustB * f;

      const waveA = wpA.wave_height_m !== undefined && wpA.wave_height_m > 0
        ? wpA.wave_height_m
        : Math.max(0.3, Math.round(Math.pow(wpA.tws_kts / 10.0, 1.3) * 0.5 * 10.0) / 10.0);
      const waveB = wpB.wave_height_m !== undefined && wpB.wave_height_m > 0
        ? wpB.wave_height_m
        : Math.max(0.3, Math.round(Math.pow(wpB.tws_kts / 10.0, 1.3) * 0.5 * 10.0) / 10.0);
      const waveHeightM = waveA * (1 - f) + waveB * f;

      const periodA = wpA.wave_period_s !== undefined && wpA.wave_period_s > 0
        ? wpA.wave_period_s
        : Math.max(4.0, Math.round((3.5 + Math.sqrt(wpA.tws_kts) * 1.2) * 10.0) / 10.0);
      const periodB = wpB.wave_period_s !== undefined && wpB.wave_period_s > 0
        ? wpB.wave_period_s
        : Math.max(4.0, Math.round((3.5 + Math.sqrt(wpB.tws_kts) * 1.2) * 10.0) / 10.0);
      const wavePeriodS = periodA * (1 - f) + periodB * f;

      const twdDeg = interpolateAngle(wpA.twd_deg, wpB.twd_deg, f);
      const headingDeg = interpolateAngle(wpA.heading_deg, wpB.heading_deg, f);
      const twaDeg = wpA.twa_deg * (1 - f) + wpB.twa_deg * f;

      // Relative wind calculation
      let relWindDeg = (twdDeg - headingDeg) % 360;
      if (relWindDeg > 180) relWindDeg -= 360;
      if (relWindDeg < -180) relWindDeg += 360;

      const tack: 'Stbd' | 'Port' = relWindDeg >= 0 ? 'Stbd' : 'Port';

      // Determine the closest actual waypoint index for click jumping
      const closestWpIdx = f < 0.5 ? wpAIdx : wpBIdx;

      // Time formatting
      const targetTimeMs = startTimeMs + targetElapsed * 3600 * 1000;
      const dateObj = new Date(targetTimeMs);
      const hoursStr = String(dateObj.getUTCHours()).padStart(2, '0');
      const minsStr = String(dateObj.getUTCMinutes()).padStart(2, '0');
      const clockTime = `${hoursStr}:${minsStr}`;

      const dayOffset = Math.floor(targetElapsed / 24);
      const dayOffsetStr = dayOffset > 0 ? `D${dayOffset + 1}` : '';

      cols.push({
        stepIndex: s,
        elapsedHours: targetElapsed,
        timeStr: dateObj.toISOString(),
        clockTime,
        dayOffsetStr,
        boatSpeedKts,
        twsKts,
        gustKts,
        waveHeightM,
        wavePeriodS,
        twdDeg,
        twaDeg,
        headingDeg,
        relWindDeg,
        tack,
        waypointIndex: closestWpIdx,
      });

      if (targetElapsed >= totalDuration) {
        break;
      }
    }

    return cols;
  }, [routeResult]);

  // Active step index closest to current position for subtle highlighting
  const currentWp = routeResult?.waypoints[currentIndex] || routeResult?.waypoints[0];
  const startTimeMs = routeResult ? new Date(routeResult.start_time).getTime() : 0;
  const currentWpMs = currentWp ? new Date(currentWp.time).getTime() : 0;
  const currentElapsed = Math.max(0, (currentWpMs - startTimeMs) / (3600 * 1000));
  const activeColIndex = columns.reduce((bestIdx, col, idx) => {
    const prevDiff = Math.abs(columns[bestIdx].elapsedHours - currentElapsed);
    const currDiff = Math.abs(col.elapsedHours - currentElapsed);
    return currDiff < prevDiff ? idx : bestIdx;
  }, 0);

  // Compute locked 1/3 camera tracking
  const { markerX, targetScrollLeft } = useMemo(() => {
    if (!routeResult || columns.length === 0) {
      return { markerX: 0, targetScrollLeft: 0 };
    }

    const totalDuration = Math.max(0.1, routeResult.total_duration_hours);
    const progress = Math.min(1, Math.max(0, currentElapsed / totalDuration));
    const totalContentWidth = columns.length * COLUMN_WIDTH_PX;
    const maxScrollLeft = Math.max(0, totalContentWidth - viewportWidth);
    const lockedLinePos = viewportWidth / 3;

    const targetXInContent =
      columns.length > 1
        ? progress * ((columns.length - 1) * COLUMN_WIDTH_PX) + COLUMN_WIDTH_PX / 2
        : COLUMN_WIDTH_PX / 2;

    const desiredScrollLeft = targetXInContent - lockedLinePos;
    const scrollLeft = Math.min(maxScrollLeft, Math.max(0, desiredScrollLeft));
    const mX = Math.min(viewportWidth - 1, Math.max(1, targetXInContent - scrollLeft));

    return { markerX: mX, targetScrollLeft: scrollLeft };
  }, [routeResult, columns, currentElapsed, viewportWidth]);

  // Synchronize scroll position
  useEffect(() => {
    if (scrollContainerRef.current) {
      scrollContainerRef.current.scrollLeft = targetScrollLeft;
    }
  }, [targetScrollLeft]);

  if (!routeResult || columns.length === 0) {
    return null;
  }

  return (
    <div className="timeline-table-root">
      {/* Fixed Sticky Header Labels Column with Units */}
      <div className="timeline-table-labels">
        <div className="timeline-label-cell label-time">TIME</div>
        <div className="timeline-label-cell label-wind">
          <span>WIND</span>
          <span className="unit-sub">kt</span>
        </div>
        <div className="timeline-label-cell label-boat">
          <span>BOAT</span>
          <span className="unit-sub">kt</span>
        </div>
        <div className="timeline-label-cell label-wave">
          <span>WAVE</span>
          <span className="unit-sub">m/s</span>
        </div>
        <div className="timeline-label-cell label-twa">
          <span>TWA</span>
          <span className="unit-sub">°</span>
        </div>
      </div>

      {/* Horizontally Scrollable Data Viewport */}
      <div className="timeline-table-viewport-wrapper">
        {/* Dynamic Vertical Indicator Line */}
        <div
          className="timeline-table-indicator-line"
          style={{ left: `${markerX}px` }}
        >
          <div className="indicator-pointer-top" />
          <div className="indicator-pointer-bottom" />
        </div>

        {/* Scrollable Column Track */}
        <div ref={scrollContainerRef} className="timeline-table-scroll-track">
          <div className="timeline-table-content">
            {columns.map((col, idx) => {
              const isActive = idx === activeColIndex;
              const windColor = getWindColor(col.twsKts);
              const gustColor = getWindColor(col.gustKts);
              const waveColor = getWaveIntensityColor(col.waveHeightM, col.wavePeriodS);
              const posColor = getPointOfSailColor(col.twaDeg);
              // Relative wind flow arrow over deck (Bow is to the left):
              // 0° (Headwind from Bow on Left) -> flow to Right (0°)
              // +90° (Starboard Beam from Top) -> flow to Bottom (+90°)
              // 180° (Downwind from Stern on Right) -> flow to Left (180°)
              // -90° (Port Beam from Bottom) -> flow to Top (-90°)
              const windFlowAngle = col.relWindDeg;

              return (
                <div
                  key={col.stepIndex}
                  className={`timeline-col ${isActive ? 'col-active' : ''}`}
                  style={{ width: `${COLUMN_WIDTH_PX}px` }}
                  onClick={() => onIndexChange(col.waypointIndex)}
                  title={`+${col.elapsedHours.toFixed(1)}h (${col.clockTime} UTC)\nWind: ${Math.round(col.twsKts)} kts (Gust: ${Math.round(col.gustKts)} kts) @ ${col.twdDeg.toFixed(0)}°\nBoat: ${col.boatSpeedKts.toFixed(1)} kts\nWave: ${col.waveHeightM.toFixed(1)}m @ ${Math.round(col.wavePeriodS)}s\nTWA: ${col.twaDeg.toFixed(0)}° (${col.tack})`}
                >
                  {/* Row 1: Time Passed in Sail + Clock Time */}
                  <div className="timeline-data-cell cell-time">
                    <span className="time-elapsed-tag">+{Math.round(col.elapsedHours)}h</span>
                    <span className="time-clock-tag">{col.clockTime}</span>
                  </div>

                  {/* Row 2: Divided Wind (TWS) & Gust Chip */}
                  <div className="timeline-data-cell cell-wind">
                    <div className="wind-divided-chip">
                      <span
                        className="wind-chip-segment wind-chip-tws"
                        style={{
                          backgroundColor: windColor,
                          color: '#020617',
                        }}
                        title={`Sustained: ${Math.round(col.twsKts)} kt`}
                      >
                        {Math.round(col.twsKts)}
                      </span>
                      <span
                        className="wind-chip-segment wind-chip-gust"
                        style={{
                          backgroundColor: gustColor,
                          color: '#020617',
                        }}
                        title={`Gust: ${Math.round(col.gustKts)} kt`}
                      >
                        {Math.round(col.gustKts)}
                      </span>
                    </div>
                  </div>

                  {/* Row 3: Boat Speed (knots) */}
                  <div className="timeline-data-cell cell-boat">
                    <span className="boat-speed-val">{col.boatSpeedKts.toFixed(1)}</span>
                  </div>

                  {/* Row 4: Wave Height (m) & Period (s) - Color Coded by Intensity */}
                  <div className="timeline-data-cell cell-wave">
                    <div className="wave-combined-cell">
                      <span className="wave-height-val" style={{ color: waveColor }}>
                        {col.waveHeightM.toFixed(1)}m
                      </span>
                      <span className="wave-period-val" style={{ color: waveColor, opacity: 0.85 }}>
                        {Math.round(col.wavePeriodS)}s
                      </span>
                    </div>
                  </div>

                  {/* Row 5: Combined TWA Directional Arrow & Degrees (Point of Sail Color) */}
                  <div className="timeline-data-cell cell-twa">
                    <div className="twa-combined-row">
                      <svg
                        width="13"
                        height="13"
                        viewBox="0 0 24 24"
                        style={{
                          transform: `rotate(${windFlowAngle}deg)`,
                          transformOrigin: 'center',
                          transition: 'transform 0.15s ease',
                        }}
                      >
                        {/* Horizontal flow arrow pointing Left to Right (Bow to Stern at 0°) */}
                        <path
                          d="M3 12 L21 12 M15 6 L21 12 M15 18 L21 12"
                          fill="none"
                          stroke={posColor}
                          strokeWidth="2.8"
                          strokeLinecap="round"
                          strokeLinejoin="round"
                        />
                      </svg>
                      <span className="twa-deg-val" style={{ color: posColor }}>
                        {Math.round(col.twaDeg)}°
                      </span>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
};

export default TimelineTable;
