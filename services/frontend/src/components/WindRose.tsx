import React, { useState, useMemo } from 'react';
import { RouteResult } from '../types';
import {
  POINT_OF_SAIL_CONFIG,
  getPointOfSail,
  getPointOfSailRangeLabel,
  PointOfSail,
} from '../config/pointOfSail';
import { Compass, Wind, Info } from 'lucide-react';

interface WindRoseProps {
  routeResult: RouteResult;
}

export interface TwaSectorData {
  index: number;
  startAngle: number;
  endAngle: number;
  midAngle: number;
  label: string;
  shortLabel: string;
  pos: PointOfSail;
  color: string;
  count: number;
  hours: number;
  distNM: number;
  pctTime: number;
  avgWindKts: number;
  maxWindKts: number;
}

// 6 Angular Sectors of 30° each covering 0° to 180° TWA
const SECTOR_SPAN_DEG = 30;
const SECTOR_COUNT = 6; // 0-30, 30-60, 60-90, 90-120, 120-150, 150-180

export const WindRose: React.FC<WindRoseProps> = ({ routeResult }) => {
  const [hoveredSector, setHoveredSector] = useState<TwaSectorData | null>(null);

  // Compute 6 30° TWA angle sectors and global passage point of sail telemetry
  const { sectors, dominantSector, totalStats } = useMemo(() => {
    const wps = routeResult?.waypoints || [];
    const n = wps.length;

    // Initialize 6 buckets of 30° each
    const buckets: {
      count: number;
      hours: number;
      distNM: number;
      sumWind: number;
      maxWind: number;
    }[] = Array.from({ length: SECTOR_COUNT }, () => ({
      count: 0,
      hours: 0,
      distNM: 0,
      sumWind: 0,
      maxWind: 0,
    }));

    let totalDurationHours = 0;
    let totalDistanceNM = 0;

    let totalUpwindHours = 0;
    let totalReachingHours = 0;
    let totalDownwindHours = 0;

    let totalUpwindDist = 0;
    let totalReachingDist = 0;
    let totalDownwindDist = 0;

    for (let i = 0; i < n; i++) {
      const wp = wps[i];
      const prevWp = i > 0 ? wps[i - 1] : wp;

      // Determine step time span in hours
      let stepHours = 1.0;
      if (i > 0) {
        const tCurr = new Date(wp.time).getTime();
        const tPrev = new Date(prevWp.time).getTime();
        stepHours = Math.max(0.1, (tCurr - tPrev) / (3600 * 1000));
      } else if (n > 1) {
        const t1 = new Date(wps[1].time).getTime();
        const t0 = new Date(wps[0].time).getTime();
        stepHours = Math.max(0.1, (t1 - t0) / (3600 * 1000));
      }

      const stepDist = Math.max(0, wp.distance_nm - prevWp.distance_nm);
      totalDurationHours += stepHours;
      totalDistanceNM += stepDist;

      // Absolute True Wind Angle (0° to 180°), independent of Port or Starboard tack
      const twa = Math.min(180, Math.max(0, Math.abs(wp.twa_deg)));
      const pos = getPointOfSail(twa);

      if (pos === 'upwind') {
        totalUpwindHours += stepHours;
        totalUpwindDist += stepDist;
      } else if (pos === 'reaching') {
        totalReachingHours += stepHours;
        totalReachingDist += stepDist;
      } else {
        totalDownwindHours += stepHours;
        totalDownwindDist += stepDist;
      }

      // Assign to 30° bucket (0..5)
      let bucketIdx = Math.floor(twa / SECTOR_SPAN_DEG);
      if (bucketIdx >= SECTOR_COUNT) bucketIdx = SECTOR_COUNT - 1;

      const b = buckets[bucketIdx];
      b.count++;
      b.hours += stepHours;
      b.distNM += stepDist;
      b.sumWind += wp.tws_kts;
      if (wp.tws_kts > b.maxWind) b.maxWind = wp.tws_kts;
    }

    const totalWeightHours = Math.max(0.1, totalDurationHours);

    // Build the 6 TwaSectorData items
    const sectorList: TwaSectorData[] = buckets.map((b, idx) => {
      const startAngle = idx * SECTOR_SPAN_DEG;
      const endAngle = (idx + 1) * SECTOR_SPAN_DEG;
      const midAngle = (startAngle + endAngle) / 2;

      // Determine Point of Sail category for this 30° sector (based on its midpoint)
      const pos = getPointOfSail(midAngle);
      const color = POINT_OF_SAIL_CONFIG.colors[pos];
      const pctTime = (b.hours / totalWeightHours) * 100;

      const label = `${startAngle}° – ${endAngle}° TWA`;
      const shortLabel = `${startAngle}°–${endAngle}°`;

      return {
        index: idx,
        startAngle,
        endAngle,
        midAngle,
        label,
        shortLabel,
        pos,
        color,
        count: b.count,
        hours: b.hours,
        distNM: b.distNM,
        pctTime,
        avgWindKts: b.count > 0 ? b.sumWind / b.count : 0,
        maxWindKts: b.maxWind,
      };
    });

    const dominant =
      [...sectorList].sort((a, b) => b.pctTime - a.pctTime)[0] || sectorList[0];

    const upwindPct = (totalUpwindHours / totalWeightHours) * 100;
    const reachingPct = (totalReachingHours / totalWeightHours) * 100;
    const downwindPct = (totalDownwindHours / totalWeightHours) * 100;

    return {
      sectors: sectorList,
      dominantSector: dominant,
      totalStats: {
        upwindPct,
        reachingPct,
        downwindPct,
        totalUpwindHours,
        totalReachingHours,
        totalDownwindHours,
        totalUpwindDist,
        totalReachingDist,
        totalDownwindDist,
        totalDurationHours,
        totalDistanceNM,
      },
    };
  }, [routeResult]);

  // Scale Configuration for SVG Percentage Rings
  const maxPct = Math.max(...sectors.map((s) => s.pctTime), 10);
  let ringStep = 10;
  if (maxPct > 60) ringStep = 20;
  else if (maxPct > 35) ringStep = 15;
  else if (maxPct > 20) ringStep = 10;
  else ringStep = 5;

  const numRings = Math.max(3, Math.ceil(maxPct / ringStep));
  const maxScalePct = numRings * ringStep;

  // SVG Semicircle Dial Dimensions (0° Top -> 90° Right -> 180° Bottom)
  const SVG_WIDTH = 370;
  const SVG_HEIGHT = 410;
  const CX = 75;
  const CY = 205;
  const MAX_RADIUS = 175;
  const LABEL_RADIUS = 205;

  // Polar to Cartesian conversion for semicircle:
  // 0° is Top (0, -r), 90° is Right (+r, 0), 180° is Bottom (0, +r)
  const polarToXY = (r: number, angleDeg: number) => {
    const rad = ((angleDeg - 90) * Math.PI) / 180.0;
    return {
      x: CX + r * Math.cos(rad),
      y: CY + r * Math.sin(rad),
    };
  };

  // Annular SVG wedge path from center to rOuter for sector angle span
  const createWedgePath = (
    rOuter: number,
    startAngleDeg: number,
    endAngleDeg: number
  ) => {
    if (rOuter <= 0.5) return '';

    const pStart = polarToXY(rOuter, startAngleDeg);
    const pEnd = polarToXY(rOuter, endAngleDeg);

    return `M ${CX} ${CY} L ${pStart.x.toFixed(2)} ${pStart.y.toFixed(2)} A ${rOuter.toFixed(2)} ${rOuter.toFixed(2)} 0 0 1 ${pEnd.x.toFixed(2)} ${pEnd.y.toFixed(2)} Z`;
  };

  const scaleRadius = (pct: number) => (Math.min(pct, maxScalePct) / maxScalePct) * MAX_RADIUS;

  // 7 Dial Spoke Marks at 0°, 30°, 60°, 90°, 120°, 150°, 180°
  const spokeAngles = [0, 30, 60, 90, 120, 150, 180];

  const dialMarks = [
    { angleDeg: 0, label: '0° Bow (Headwind)', textAnchor: 'middle', offsetY: -12, offsetX: 0, isMajor: true },
    { angleDeg: 30, label: '30°', textAnchor: 'start', offsetY: -6, offsetX: 8, isMajor: false },
    { angleDeg: 60, label: '60°', textAnchor: 'start', offsetY: -2, offsetX: 10, isMajor: false },
    { angleDeg: 90, label: '90° Beam', textAnchor: 'start', offsetY: 0, offsetX: 12, isMajor: true },
    { angleDeg: 120, label: '120°', textAnchor: 'start', offsetY: 4, offsetX: 10, isMajor: false },
    { angleDeg: 150, label: '150°', textAnchor: 'start', offsetY: 8, offsetX: 8, isMajor: false },
    { angleDeg: 180, label: '180° Stern (Run)', textAnchor: 'middle', offsetY: 16, offsetX: 0, isMajor: true },
  ];

  return (
    <div className="wind-rose-card">
      {/* Header */}
      <div className="wind-rose-header">
        <div className="wind-rose-title-group">
          <div className="wind-rose-icon-badge">
            <Wind size={18} color="#38bdf8" />
          </div>
          <div>
            <div className="wind-rose-main-title">
              <span>Wind Angle Rose (TWA)</span>
            </div>
            <p className="wind-rose-subtitle">
              Passage duration distribution across True Wind Angle buckets (Port &amp; Starboard combined)
            </p>
          </div>
        </div>
      </div>

      {/* Main Grid: Semicircle Polar Chart on Left + Telemetry and Legend on Right */}
      <div className="wind-rose-body-grid">
        {/* Left Column: Semicircle TWA Rose */}
        <div className="wind-rose-chart-column">
          <div className="wind-rose-svg-container">
            <svg
              viewBox={`0 0 ${SVG_WIDTH} ${SVG_HEIGHT}`}
              className="wind-rose-svg"
              onMouseLeave={() => setHoveredSector(null)}
            >
              {/* Background Semicircle Container Arc */}
              <path
                d={`M ${CX} ${CY - MAX_RADIUS} A ${MAX_RADIUS} ${MAX_RADIUS} 0 0 1 ${CX} ${CY + MAX_RADIUS} Z`}
                fill="rgba(15, 23, 42, 0.4)"
                stroke="rgba(148, 163, 184, 0.15)"
                strokeWidth="1"
              />

              {/* 1. SECTOR PETALS (DRAWN FIRST SO GRID IS ON TOP) */}
              <g className="wind-rose-petals">
                {sectors.map((s) => {
                  if (s.pctTime <= 0.01) return null;

                  // Small 0.6° angular gap for crisp segment borders
                  const startAngle = s.startAngle + 0.3;
                  const endAngle = s.endAngle - 0.3;
                  const rOuter = scaleRadius(s.pctTime);
                  const isHovered = hoveredSector?.index === s.index;

                  return (
                    <g
                      key={s.index}
                      className={`wind-rose-sector-group ${isHovered ? 'hovered' : ''}`}
                      onMouseEnter={() => setHoveredSector(s)}
                      style={{ cursor: 'pointer' }}
                    >
                      {/* Colored Wedge Segment */}
                      <path
                        d={createWedgePath(rOuter, startAngle, endAngle)}
                        fill={s.color}
                        fillOpacity={isHovered ? 0.95 : 0.82}
                        stroke={isHovered ? '#ffffff' : 'rgba(255, 255, 255, 0.15)'}
                        strokeWidth={isHovered ? 1.8 : 0.8}
                      />

                      {/* Transparent Hover Hitbox */}
                      <path
                        d={createWedgePath(MAX_RADIUS + 15, s.startAngle, s.endAngle)}
                        fill="transparent"
                      />
                    </g>
                  );
                })}
              </g>

              {/* 2. CONCENTRIC PERCENTAGE RINGS (DRAWN ABOVE SEGMENTS) */}
              <g className="wind-rose-grid-rings">
                {Array.from({ length: numRings }).map((_, i) => {
                  const ringVal = (i + 1) * ringStep;
                  const r = scaleRadius(ringVal);
                  return (
                    <g key={i}>
                      {/* Semicircle Arc from Top (0°) to Bottom (180°) */}
                      <path
                        d={`M ${CX} ${CY - r} A ${r} ${r} 0 0 1 ${CX} ${CY + r}`}
                        fill="none"
                        stroke="rgba(255, 255, 255, 0.22)"
                        strokeWidth="1.1"
                        strokeDasharray="3 3"
                        style={{ pointerEvents: 'none' }}
                      />
                      {/* Percentage Ring Label along 90° Beam axis */}
                      <text
                        x={CX + r + 3}
                        y={CY - 4}
                        fill="#94a3b8"
                        fontSize="9"
                        fontFamily="var(--font-mono)"
                        fontWeight="700"
                        textAnchor="start"
                        style={{ pointerEvents: 'none', userSelect: 'none' }}
                      >
                        {ringVal}%
                      </text>
                    </g>
                  );
                })}
                {/* 0% Center Point */}
                <circle cx={CX} cy={CY} r="3" fill="#cbd5e1" style={{ pointerEvents: 'none' }} />
              </g>

              {/* 3. RADIAL SPOKE LINES (DRAWN ABOVE SEGMENTS) */}
              <g className="wind-rose-spokes" style={{ pointerEvents: 'none' }}>
                {spokeAngles.map((angle) => {
                  const spokeEnd = polarToXY(MAX_RADIUS + 6, angle);
                  const isMajor = angle % 90 === 0;
                  return (
                    <line
                      key={angle}
                      x1={CX}
                      y1={CY}
                      x2={spokeEnd.x}
                      y2={spokeEnd.y}
                      stroke={isMajor ? 'rgba(255, 255, 255, 0.35)' : 'rgba(255, 255, 255, 0.16)'}
                      strokeWidth={isMajor ? '1.3' : '0.9'}
                    />
                  );
                })}
              </g>

              {/* 4. DIAL LABELS AROUND THE PERIMETER */}
              <g className="wind-rose-labels">
                {dialMarks.map((mark) => {
                  const pos = polarToXY(LABEL_RADIUS, mark.angleDeg);
                  const isHovered =
                    hoveredSector &&
                    hoveredSector.startAngle <= mark.angleDeg &&
                    hoveredSector.endAngle >= mark.angleDeg;

                  return (
                    <text
                      key={mark.angleDeg}
                      x={pos.x + mark.offsetX}
                      y={pos.y + mark.offsetY}
                      textAnchor={mark.textAnchor as any}
                      dominantBaseline="central"
                      fill={
                        isHovered
                          ? '#38bdf8'
                          : mark.isMajor
                          ? '#f8fafc'
                          : '#94a3b8'
                      }
                      fontSize={mark.isMajor ? '11' : '9.5'}
                      fontWeight={mark.isMajor ? '700' : '600'}
                      fontFamily="var(--font-sans)"
                      className="wind-rose-dir-label"
                      style={{ transition: 'all 0.15s ease' }}
                    >
                      {mark.label}
                    </text>
                  );
                })}
              </g>
            </svg>
          </div>

          {/* Constant-Height Interactive Hover Telemetry Bar */}
          <div className="wind-rose-hover-detail-bar">
            {hoveredSector ? (
              <div className="hover-detail-content">
                <div className="hover-detail-top-row">
                  <span className="hover-sector-name">{hoveredSector.label}</span>
                  <span
                    className="hover-pos-tag"
                    style={{
                      backgroundColor: `${hoveredSector.color}22`,
                      color: hoveredSector.color,
                      border: `1px solid ${hoveredSector.color}44`,
                    }}
                  >
                    {hoveredSector.pos.toUpperCase()} ({getPointOfSailRangeLabel(hoveredSector.pos)})
                  </span>
                </div>
                <div className="hover-detail-bottom-row">
                  <span className="hover-stat-pill font-mono">
                    <strong style={{ color: hoveredSector.color }}>
                      {hoveredSector.pctTime.toFixed(1)}%
                    </strong>{' '}
                    time ({hoveredSector.hours.toFixed(1)} hrs • {hoveredSector.distNM.toFixed(1)} NM)
                  </span>
                  {hoveredSector.avgWindKts > 0 && (
                    <span className="hover-stat-pill font-mono text-muted">
                      Avg Wind: {hoveredSector.avgWindKts.toFixed(1)} kts (Peak: {hoveredSector.maxWindKts.toFixed(1)} kts)
                    </span>
                  )}
                </div>
              </div>
            ) : (
              <div className="hover-detail-placeholder">
                <Info size={14} className="text-accent" />
                <span>Hover over any wind angle sector to inspect duration, distance, and wind telemetry</span>
              </div>
            )}
          </div>
        </div>

        {/* Right Column: Point of Sail Definitions & Dominant Angle Summary */}
        <div className="wind-rose-telemetry-column">
          {/* 1. Point of Sail Definitions Card */}
          <div className="wind-rose-legend-card">
            <span className="legend-card-title">POINT OF SAIL DEFINITIONS</span>

            <div className="legend-items-list">
              {/* Upwind */}
              <div className="legend-pos-item">
                <div className="legend-pos-header">
                  <div
                    className="legend-color-dot"
                    style={{ backgroundColor: POINT_OF_SAIL_CONFIG.colors.upwind }}
                  />
                  <span className="legend-pos-name">Upwind (Beating / Close-hauled)</span>
                </div>
                <div className="legend-pos-meta">
                  <span className="legend-range-tag">{getPointOfSailRangeLabel('upwind')} TWA</span>
                  <span
                    className="legend-pct-val"
                    style={{ color: POINT_OF_SAIL_CONFIG.colors.upwind }}
                  >
                    {totalStats.upwindPct.toFixed(1)}%
                  </span>
                </div>
                <div className="legend-sub-stats">
                  <span>{totalStats.totalUpwindHours.toFixed(1)} hrs</span>
                  <span>•</span>
                  <span>{totalStats.totalUpwindDist.toFixed(1)} NM sailed</span>
                </div>
              </div>

              {/* Reaching */}
              <div className="legend-pos-item">
                <div className="legend-pos-header">
                  <div
                    className="legend-color-dot"
                    style={{ backgroundColor: POINT_OF_SAIL_CONFIG.colors.reaching }}
                  />
                  <span className="legend-pos-name">Reaching (Beam / Broad Reach)</span>
                </div>
                <div className="legend-pos-meta">
                  <span className="legend-range-tag">{getPointOfSailRangeLabel('reaching')} TWA</span>
                  <span
                    className="legend-pct-val"
                    style={{ color: POINT_OF_SAIL_CONFIG.colors.reaching }}
                  >
                    {totalStats.reachingPct.toFixed(1)}%
                  </span>
                </div>
                <div className="legend-sub-stats">
                  <span>{totalStats.totalReachingHours.toFixed(1)} hrs</span>
                  <span>•</span>
                  <span>{totalStats.totalReachingDist.toFixed(1)} NM sailed</span>
                </div>
              </div>

              {/* Downwind */}
              <div className="legend-pos-item">
                <div className="legend-pos-header">
                  <div
                    className="legend-color-dot"
                    style={{ backgroundColor: POINT_OF_SAIL_CONFIG.colors.downwind }}
                  />
                  <span className="legend-pos-name">Downwind (Running / Deep)</span>
                </div>
                <div className="legend-pos-meta">
                  <span className="legend-range-tag">{getPointOfSailRangeLabel('downwind')} TWA</span>
                  <span
                    className="legend-pct-val"
                    style={{ color: POINT_OF_SAIL_CONFIG.colors.downwind }}
                  >
                    {totalStats.downwindPct.toFixed(1)}%
                  </span>
                </div>
                <div className="legend-sub-stats">
                  <span>{totalStats.totalDownwindHours.toFixed(1)} hrs</span>
                  <span>•</span>
                  <span>{totalStats.totalDownwindDist.toFixed(1)} NM sailed</span>
                </div>
              </div>
            </div>
          </div>

          {/* 2. Dominant Wind Angle Card */}
          <div className="wind-rose-dominant-card">
            <div className="dominant-card-header">
              <Compass size={15} color="#38bdf8" />
              <span>DOMINANT WIND REGIME</span>
            </div>
            <div className="dominant-sector-display">
              <div
                className="dominant-sector-badge"
                style={{
                  background:
                    dominantSector.pos === 'upwind'
                      ? 'linear-gradient(135deg, #0284c7 0%, #0369a1 100%)'
                      : dominantSector.pos === 'reaching'
                      ? 'linear-gradient(135deg, #059669 0%, #047857 100%)'
                      : 'linear-gradient(135deg, #7e22ce 0%, #6b21a8 100%)',
                }}
              >
                {dominantSector.shortLabel}
              </div>
              <div className="dominant-sector-info">
                <span className="dominant-sector-pct">
                  {dominantSector.pctTime.toFixed(1)}% of passage
                </span>
                <span className="dominant-sector-dur">
                  {dominantSector.hours.toFixed(1)}h duration • {dominantSector.distNM.toFixed(1)} NM
                </span>
                {dominantSector.avgWindKts > 0 && (
                  <span className="dominant-sector-wind">
                    Avg Wind: {dominantSector.avgWindKts.toFixed(1)} kts (Max: {dominantSector.maxWindKts.toFixed(1)} kts)
                  </span>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default WindRose;
