/**
 * Global Point of Sail & True Wind Angle (TWA) Configuration
 *
 * Fine-grained definitions aligned with 15° even dissections:
 * - close_hauled:  < 60° TWA (4 × 15°: 0°–60°) -> Red (#ef4444)
 * - close_reach:   60° – 75° TWA (1 × 15°: 60°–75°) -> Amber / Orange (#f59e0b)
 * - beam_reach:    75° – 105° TWA (2 × 15° centered on 90°: 75°–105°) -> Emerald Green (#10b981)
 * - broad_reach:   105° – 150° TWA (3 × 15°: 105°–150°) -> Sky / Cyan (#06b6d4)
 * - dead_downwind: 150° – 180° TWA (2 × 15°: 150°–180°) -> Purple / Violet (#a855f7)
 */

export type PointOfSail =
  | 'close_hauled'
  | 'close_reach'
  | 'beam_reach'
  | 'broad_reach'
  | 'dead_downwind';

export interface PointOfSailMeta {
  key: PointOfSail;
  label: string;
  shortLabel: string;
  minTwaDeg: number;
  maxTwaDeg: number;
  color: string;
  comfortWeight: number; // For ranking / reference
}

export const POINT_OF_SAIL_METAS: Record<PointOfSail, PointOfSailMeta> = {
  close_hauled: {
    key: 'close_hauled',
    label: 'Close Hauled',
    shortLabel: 'Close Hauled',
    minTwaDeg: 0,
    maxTwaDeg: 60,
    color: '#ef4444', // Red / Coral (Least comfortable, slamming)
    comfortWeight: 15,
  },
  close_reach: {
    key: 'close_reach',
    label: 'Close Reach',
    shortLabel: 'Close Reach',
    minTwaDeg: 60,
    maxTwaDeg: 75,
    color: '#f59e0b', // Amber / Orange (Moderate comfort)
    comfortWeight: 40,
  },
  beam_reach: {
    key: 'beam_reach',
    label: 'Beam Reach',
    shortLabel: 'Beam Reach',
    minTwaDeg: 75,
    maxTwaDeg: 105,
    color: '#10b981', // Emerald Green (Sweet spot, very good)
    comfortWeight: 100,
  },
  broad_reach: {
    key: 'broad_reach',
    label: 'Broad Reach',
    shortLabel: 'Broad Reach',
    minTwaDeg: 105,
    maxTwaDeg: 150,
    color: '#06b6d4', // Cyan / Sky Blue (Very good / gentle ride)
    comfortWeight: 95,
  },
  dead_downwind: {
    key: 'dead_downwind',
    label: 'Dead Downwind',
    shortLabel: 'Downwind',
    minTwaDeg: 150,
    maxTwaDeg: 180,
    color: '#a855f7', // Purple / Violet (Next best after reaching)
    comfortWeight: 75,
  },
};

export const POINT_OF_SAIL_ORDER: PointOfSail[] = [
  'close_hauled',
  'close_reach',
  'beam_reach',
  'broad_reach',
  'dead_downwind',
];

export const POINT_OF_SAIL_CONFIG = {
  colors: {
    close_hauled: POINT_OF_SAIL_METAS.close_hauled.color,
    close_reach: POINT_OF_SAIL_METAS.close_reach.color,
    beam_reach: POINT_OF_SAIL_METAS.beam_reach.color,
    broad_reach: POINT_OF_SAIL_METAS.broad_reach.color,
    dead_downwind: POINT_OF_SAIL_METAS.dead_downwind.color,
    // Aliases
    upwind: POINT_OF_SAIL_METAS.close_hauled.color,
    reaching: POINT_OF_SAIL_METAS.beam_reach.color,
    downwind: POINT_OF_SAIL_METAS.dead_downwind.color,
  },
  labels: {
    close_hauled: POINT_OF_SAIL_METAS.close_hauled.label,
    close_reach: POINT_OF_SAIL_METAS.close_reach.label,
    beam_reach: POINT_OF_SAIL_METAS.beam_reach.label,
    broad_reach: POINT_OF_SAIL_METAS.broad_reach.label,
    dead_downwind: POINT_OF_SAIL_METAS.dead_downwind.label,
    upwind: 'Upwind',
    reaching: 'Reaching',
    downwind: 'Downwind',
  },
};

/**
 * Returns the fine-grained Point of Sail for a given True Wind Angle (TWA in degrees):
 * - close_hauled:  < 60°
 * - close_reach:   60° <= TWA < 75°
 * - beam_reach:    75° <= TWA < 105°
 * - broad_reach:   105° <= TWA < 150°
 * - dead_downwind: 150° <= TWA <= 180°
 */
export function getPointOfSail(twaDeg: number): PointOfSail {
  const twa = Math.min(180, Math.max(0, Math.abs(twaDeg)));
  if (twa < 60.0) {
    return 'close_hauled';
  } else if (twa < 75.0) {
    return 'close_reach';
  } else if (twa < 105.0) {
    return 'beam_reach';
  } else if (twa < 150.0) {
    return 'broad_reach';
  } else {
    return 'dead_downwind';
  }
}

/**
 * Returns theme color for a given True Wind Angle (TWA in degrees)
 */
export function getPointOfSailColor(twaDeg: number): string {
  const pos = getPointOfSail(twaDeg);
  return POINT_OF_SAIL_METAS[pos].color;
}

/**
 * Returns a human-readable angle range string for each point of sail
 */
export function getPointOfSailRangeLabel(pos: PointOfSail | 'upwind' | 'reaching' | 'downwind'): string {
  switch (pos) {
    case 'close_hauled':
      return '< 60°';
    case 'close_reach':
      return '60° – 75°';
    case 'beam_reach':
      return '75° – 105°';
    case 'broad_reach':
      return '105° – 150°';
    case 'dead_downwind':
      return '150° – 180°';
    case 'upwind':
      return '< 60°';
    case 'reaching':
      return '75° – 150°';
    case 'downwind':
      return '150° – 180°';
    default:
      return '';
  }
}
