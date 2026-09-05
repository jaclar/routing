import React, { useState, useMemo } from 'react';
import { RouteResult } from '../types';
import {
  POINT_OF_SAIL_METAS,
  getPointOfSailRangeLabel,
  PointOfSail,
} from '../config/pointOfSail';
import { Compass, Wind, Info } from 'lucide-react';

interface WindRoseProps {
  routeResult: RouteResult;
  compact?: boolean;
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

// 12 Even 15° Angular Sectors spanning 0° to 180° TWA:
// Points of Sail borders align directly with one or more 15° sectors:
// - Close Hauled:  0°–15°, 15°–30°, 30°–45°, 45°–60° (4 sectors)
// - Close Reach:   60°–75° (1 sector)
// - Beam Reach:    75°–90°, 90°–105° (2 sectors, centered on 90° beam)
// - Broad Reach:   105°–120°, 120°–135°, 135°–150° (3 sectors)
// - Dead Downwind: 150°–165°, 165°–180° (2 sectors)
const TWA_15DEG_SECTORS_DEF: {
  pos: PointOfSail;
  startAngle: number;
  endAngle: number;
  label: string;
  shortLabel: string;
}[] = [
  { pos: 'close_hauled', startAngle: 0, endAngle: 15, label: '0° – 15° (Pinching / Irons)', shortLabel: '0°–15°' },
  { pos: 'close_hauled', startAngle: 15, endAngle: 30, label: '15° – 30° (Close Hauled / Beating)', shortLabel: '15°–30°' },
  { pos: 'close_hauled', startAngle: 30, endAngle: 45, label: '30° – 45° (Close Hauled / Beating)', shortLabel: '30°–45°' },
  { pos: 'close_hauled', startAngle: 45, endAngle: 60, label: '45° – 60° (Full & By / Close Hauled)', shortLabel: '45°–60°' },
  { pos: 'close_reach', startAngle: 60, endAngle: 75, label: '60° – 75° (Close Reach)', shortLabel: '60°–75°' },
  { pos: 'beam_reach', startAngle: 75, endAngle: 90, label: '75° – 90° (Beam Reach Forward)', shortLabel: '75°–90°' },
  { pos: 'beam_reach', startAngle: 90, endAngle: 105, label: '90° – 105° (Beam Reach Aft)', shortLabel: '90°–105°' },
  { pos: 'broad_reach', startAngle: 105, endAngle: 120, label: '105° – 120° (Broad Reach)', shortLabel: '105°–120°' },
  { pos: 'broad_reach', startAngle: 120, endAngle: 135, label: '120° – 135° (Broad Reach)', shortLabel: '120°–135°' },
  { pos: 'broad_reach', startAngle: 135, endAngle: 150, label: '135° – 150° (Deep Broad Reach)', shortLabel: '135°–150°' },
  { pos: 'dead_downwind', startAngle: 150, endAngle: 165, label: '150° – 165° (Training Run / Downwind)', shortLabel: '150°–165°' },
  { pos: 'dead_downwind', startAngle: 165, endAngle: 180, label: '165° – 180° (Dead Downwind / Stern)', shortLabel: '165°–180°' },
];

export const WindRose: React.FC<WindRoseProps> = ({ routeResult, compact = false }) => {
  const [hoveredSector, setHoveredSector] = useState<TwaSectorData | null>(null);

  // Compute the 12 even 15° TWA angle sectors and passage point of sail telemetry
  const { sectors, dominantSector, posBreakdown } = useMemo(() => {
    const wps = routeResult?.waypoints || [];
    const n = wps.length;

    // Initialize 12 buckets matching the 15° sector boundaries
    const buckets = TWA_15DEG_SECTORS_DEF.map(() => ({
      count: 0,
      hours: 0,
      distNM: 0,
      sumWind: 0,
      maxWind: 0,
    }));

    let totalDurationHours = 0;
    let totalDistanceNM = 0;

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

      // 15° even dissection: index is floor(twa / 15), clamped to 0..11
      const bucketIdx = Math.min(11, Math.floor(twa / 15.0));

      const b = buckets[bucketIdx];
      b.count++;
      b.hours += stepHours;
      b.distNM += stepDist;
      b.sumWind += wp.tws_kts;
      if (wp.tws_kts > b.maxWind) b.maxWind = wp.tws_kts;
    }

    const totalWeightHours = Math.max(0.1, totalDurationHours);

    // Build the 12 TwaSectorData items
    const sectorList: TwaSectorData[] = TWA_15DEG_SECTORS_DEF.map((def, idx) => {
      const b = buckets[idx];
      const midAngle = (def.startAngle + def.endAngle) / 2;
      const meta = POINT_OF_SAIL_METAS[def.pos];
      const pctTime = (b.hours / totalWeightHours) * 100;

      return {
        index: idx,
        startAngle: def.startAngle,
        endAngle: def.endAngle,
        midAngle,
        label: `${def.label} • ${meta.label}`,
        shortLabel: def.shortLabel,
        pos: def.pos,
        color: meta.color,
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

    // Aggregate telemetry by the 5 Points of Sail (each encompassing 1 or more 15° sectors)
    const posKeys: PointOfSail[] = ['close_hauled', 'close_reach', 'beam_reach', 'broad_reach', 'dead_downwind'];
    const posBreakdown = posKeys.map((key) => {
      const matchingSectors = sectorList.filter((s) => s.pos === key);
      const hours = matchingSectors.reduce((acc, s) => acc + s.hours, 0);
      const distNM = matchingSectors.reduce((acc, s) => acc + s.distNM, 0);
      const pctTime = matchingSectors.reduce((acc, s) => acc + s.pctTime, 0);

      return {
        pos: key,
        meta: POINT_OF_SAIL_METAS[key],
        pctTime,
        hours,
        distNM,
      };
    });

    return {
      sectors: sectorList,
      dominantSector: dominant,
      posBreakdown,
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
  const SVG_WIDTH = compact ? 260 : 370;
  const SVG_HEIGHT = compact ? 280 : 410;
  const CX = compact ? 45 : 75;
  const CY = compact ? 140 : 205;
  const MAX_RADIUS = compact ? 115 : 175;
  const LABEL_RADIUS = compact ? 135 : 205;

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

  // 15° even spoke lines for all 12 dissections (0°, 15°, 30°, 45°, 60°, 75°, 90°, 105°, 120°, 135°, 150°, 165°, 180°)
  // Point of Sail transition boundaries: 0°, 60°, 75°, 105°, 150°, 180°
  const spokeAngles = [0, 15, 30, 45, 60, 75, 90, 105, 120, 135, 150, 165, 180];
  const boundaryAngles = new Set([0, 60, 75, 90, 105, 150, 180]);

  const dialMarks = compact
    ? [
        { angleDeg: 0, label: '0° Bow', textAnchor: 'middle', offsetY: -8, offsetX: 0, isMajor: true },
        { angleDeg: 30, label: '30°', textAnchor: 'start', offsetY: -4, offsetX: 5, isMajor: false },
        { angleDeg: 60, label: '60°', textAnchor: 'start', offsetY: -2, offsetX: 6, isMajor: true },
        { angleDeg: 75, label: '75°', textAnchor: 'start', offsetY: -1, offsetX: 6, isMajor: false },
        { angleDeg: 90, label: '90° Beam', textAnchor: 'start', offsetY: 0, offsetX: 7, isMajor: true },
        { angleDeg: 105, label: '105°', textAnchor: 'start', offsetY: 2, offsetX: 6, isMajor: false },
        { angleDeg: 120, label: '120°', textAnchor: 'start', offsetY: 4, offsetX: 5, isMajor: false },
        { angleDeg: 150, label: '150°', textAnchor: 'start', offsetY: 6, offsetX: 6, isMajor: true },
        { angleDeg: 180, label: '180° Stern', textAnchor: 'middle', offsetY: 12, offsetX: 0, isMajor: true },
      ]
    : [
        { angleDeg: 0, label: '0° Bow (Headwind)', textAnchor: 'middle', offsetY: -12, offsetX: 0, isMajor: true },
        { angleDeg: 30, label: '30°', textAnchor: 'start', offsetY: -6, offsetX: 6, isMajor: false },
        { angleDeg: 60, label: '60° Close Reach', textAnchor: 'start', offsetY: -3, offsetX: 8, isMajor: true },
        { angleDeg: 75, label: '75° Beam Start', textAnchor: 'start', offsetY: -1, offsetX: 9, isMajor: false },
        { angleDeg: 90, label: '90° Pure Beam', textAnchor: 'start', offsetY: 0, offsetX: 10, isMajor: true },
        { angleDeg: 105, label: '105° Broad Start', textAnchor: 'start', offsetY: 2, offsetX: 9, isMajor: false },
        { angleDeg: 120, label: '120°', textAnchor: 'start', offsetY: 4, offsetX: 8, isMajor: false },
        { angleDeg: 135, label: '135°', textAnchor: 'start', offsetY: 6, offsetX: 7, isMajor: false },
        { angleDeg: 150, label: '150° Downwind', textAnchor: 'start', offsetY: 7, offsetX: 8, isMajor: true },
        { angleDeg: 180, label: '180° Stern (Dead Run)', textAnchor: 'middle', offsetY: 16, offsetX: 0, isMajor: true },
      ];

  return (
    <div className={`wind-rose-card ${compact ? 'compact' : ''}`}>
      {/* Header */}
      <div className="wind-rose-header">
        <div className="wind-rose-title-group">
          <div className="wind-rose-icon-badge">
            <Wind size={compact ? 15 : 18} color="#06b6d4" />
          </div>
          <div>
            <div className="wind-rose-main-title">
              <span>Wind Angle Rose (TWA)</span>
              {compact && dominantSector && (
                <span className="wind-rose-badge">
                  {dominantSector.pctTime.toFixed(0)}% {dominantSector.shortLabel}
                </span>
              )}
            </div>
            {!compact && (
              <p className="wind-rose-subtitle">
                Passage duration distribution across True Wind Angle buckets (Port &amp; Starboard combined)
              </p>
            )}
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
                        stroke={isHovered ? '#ffffff' : 'rgba(255, 255, 255, 0.2)'}
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
                        y={CY - 3}
                        fill="#94a3b8"
                        fontSize={compact ? '7.5' : '9'}
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
                <circle cx={CX} cy={CY} r={compact ? '2.5' : '3'} fill="#cbd5e1" style={{ pointerEvents: 'none' }} />
              </g>

              {/* 3. RADIAL SPOKE LINES (DRAWN ABOVE SEGMENTS) */}
              <g className="wind-rose-spokes" style={{ pointerEvents: 'none' }}>
                {spokeAngles.map((angle) => {
                  const isBoundary = boundaryAngles.has(angle);
                  const isMajor = angle === 0 || angle === 90 || angle === 180;
                  const spokeEnd = polarToXY(MAX_RADIUS + (isMajor ? 7 : isBoundary ? 5 : 3), angle);
                  return (
                    <line
                      key={angle}
                      x1={CX}
                      y1={CY}
                      x2={spokeEnd.x}
                      y2={spokeEnd.y}
                      stroke={
                        isMajor
                          ? 'rgba(255, 255, 255, 0.45)'
                          : isBoundary
                          ? 'rgba(255, 255, 255, 0.28)'
                          : 'rgba(255, 255, 255, 0.12)'
                      }
                      strokeWidth={isMajor ? '1.4' : isBoundary ? '1.0' : '0.6'}
                      strokeDasharray={!isBoundary && !isMajor ? '2 2' : undefined}
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
                      fontSize={compact ? (mark.isMajor ? '8.5' : '7.5') : (mark.isMajor ? '11' : '9.5')}
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
                    {POINT_OF_SAIL_METAS[hoveredSector.pos].label.toUpperCase()} ({getPointOfSailRangeLabel(hoveredSector.pos)})
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
                <Info size={13} className="text-accent" />
                <span>Hover over any wind angle sector to inspect duration, distance, and wind speed</span>
              </div>
            )}
          </div>
        </div>

        {/* Right Column: Point of Sail Definitions & Dominant Angle Summary */}
        <div className="wind-rose-telemetry-column">
          {/* 1. Point of Sail Definitions Card */}
          <div className="wind-rose-legend-card">
            <span className="legend-card-title">POINT OF SAIL BREAKDOWN</span>

            <div className="legend-items-list">
              {posBreakdown.map((item) => (
                <div
                  key={item.pos}
                  className={`legend-pos-item ${hoveredSector?.pos === item.pos ? 'highlighted' : ''}`}
                  onMouseEnter={() => {
                    const matched = sectors.find((s) => s.pos === item.pos);
                    if (matched) setHoveredSector(matched);
                  }}
                  onMouseLeave={() => setHoveredSector(null)}
                  style={{ cursor: 'pointer' }}
                >
                  <div className="legend-pos-header">
                    <div
                      className="legend-color-dot"
                      style={{ backgroundColor: item.meta.color }}
                    />
                    <span className="legend-pos-name">{item.meta.label}</span>
                  </div>
                  <div className="legend-pos-meta">
                    <span className="legend-range-tag">{getPointOfSailRangeLabel(item.pos)} TWA</span>
                    <span
                      className="legend-pct-val"
                      style={{ color: item.meta.color }}
                    >
                      {item.pctTime.toFixed(1)}%
                    </span>
                  </div>
                  <div className="legend-sub-stats">
                    <span>{item.hours.toFixed(1)} hrs</span>
                    <span>•</span>
                    <span>{item.distNM.toFixed(1)} NM sailed</span>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* 2. Dominant Wind Angle Card */}
          <div className="wind-rose-dominant-card">
            <div className="dominant-card-header">
              <Compass size={14} color="#06b6d4" />
              <span>DOMINANT WIND REGIME</span>
            </div>
            <div className="dominant-sector-display">
              <div
                className="dominant-sector-badge"
                style={{
                  background: dominantSector.color,
                  boxShadow: `0 4px 14px ${dominantSector.color}44`,
                }}
              >
                {dominantSector.shortLabel}
              </div>
              <div className="dominant-sector-info">
                <span className="dominant-sector-pct">
                  {dominantSector.pctTime.toFixed(1)}% of passage ({POINT_OF_SAIL_METAS[dominantSector.pos].label})
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
