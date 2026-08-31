/**
 * Global Point of Sail & Wind Angle Configuration
 *
 * Deploy-time configurable via environment variables:
 * - VITE_REACHING_START_DEG: TWA in degrees where Reaching begins (default: 90.0)
 * - VITE_REACHING_STOP_DEG: TWA in degrees where Reaching ends and Downwind begins (default: 150.0)
 */

export interface PointOfSailConfig {
  reachingStartDeg: number;
  reachingStopDeg: number;
  colors: {
    upwind: string;
    reaching: string;
    downwind: string;
  };
  labels: {
    upwind: string;
    reaching: string;
    downwind: string;
  };
}

// Read deploy-time environment variables or use defaults (Reaching: 90° to 150°)
const envReachingStart =
  typeof import.meta !== 'undefined' && (import.meta as any).env?.VITE_REACHING_START_DEG
    ? parseFloat((import.meta as any).env.VITE_REACHING_START_DEG)
    : 90.0;

const envReachingStop =
  typeof import.meta !== 'undefined' && (import.meta as any).env?.VITE_REACHING_STOP_DEG
    ? parseFloat((import.meta as any).env.VITE_REACHING_STOP_DEG)
    : 150.0;

export const POINT_OF_SAIL_CONFIG: PointOfSailConfig = {
  reachingStartDeg: isNaN(envReachingStart) ? 90.0 : envReachingStart,
  reachingStopDeg: isNaN(envReachingStop) ? 150.0 : envReachingStop,
  colors: {
    upwind: '#38bdf8',    // Cyan / Sky
    reaching: '#10b981',  // Emerald Green
    downwind: '#a855f7',  // Purple / Violet
  },
  labels: {
    upwind: 'Upwind',
    reaching: 'Reaching',
    downwind: 'Downwind',
  },
};

export type PointOfSail = 'upwind' | 'reaching' | 'downwind';

/**
 * Returns point of sail category based on True Wind Angle (TWA in degrees):
 * - Upwind: TWA < reachingStartDeg (e.g. < 90°)
 * - Reaching: reachingStartDeg <= TWA < reachingStopDeg (e.g. 90° <= TWA < 150°)
 * - Downwind: TWA >= reachingStopDeg (e.g. >= 150°)
 */
export function getPointOfSail(twaDeg: number): PointOfSail {
  const twa = Math.abs(twaDeg);
  if (twa < POINT_OF_SAIL_CONFIG.reachingStartDeg) {
    return 'upwind';
  } else if (twa < POINT_OF_SAIL_CONFIG.reachingStopDeg) {
    return 'reaching';
  } else {
    return 'downwind';
  }
}

/**
 * Returns theme color for a given True Wind Angle (TWA in degrees)
 */
export function getPointOfSailColor(twaDeg: number): string {
  const pos = getPointOfSail(twaDeg);
  return POINT_OF_SAIL_CONFIG.colors[pos];
}

/**
 * Returns a human-readable angle range string for each point of sail
 */
export function getPointOfSailRangeLabel(pos: PointOfSail): string {
  const start = POINT_OF_SAIL_CONFIG.reachingStartDeg;
  const stop = POINT_OF_SAIL_CONFIG.reachingStopDeg;
  switch (pos) {
    case 'upwind':
      return `0° – ${start}°`;
    case 'reaching':
      return `${start}° – ${stop}°`;
    case 'downwind':
      return `${stop}° – 180°`;
  }
}
