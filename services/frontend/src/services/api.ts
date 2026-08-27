import { BoatDetail, BoatPreset, LandmaskPolygon, LandmaskResponse, Point, RouteResult, SolveMatrixResponse, WeatherGridResponse } from '../types';

export const ROUTE_PRESETS = [
  {
    name: 'Prickly Bay (Grenada) to Chaguaramas (Trinidad)',
    startName: 'Prickly Bay (Grenada)',
    start: { lat: 11.975, lon: -61.765 },
    destName: 'Chaguaramas (Trinidad)',
    dest: { lat: 10.675, lon: -61.645 },
  },
  {
    name: 'Newport to Bermuda (Classic Ocean Race)',
    startName: 'Newport (Brenton Reef)',
    start: { lat: 41.45, lon: -71.35 },
    destName: "Bermuda (St. David's)",
    dest: { lat: 32.40, lon: -64.55 },
  },
  {
    name: 'Cowes to Fastnet Rock (Rolex Fastnet Race)',
    startName: 'Cowes (Isle of Wight)',
    start: { lat: 50.76, lon: -1.30 },
    destName: 'Fastnet Rock (SW Ireland)',
    dest: { lat: 51.38, lon: -9.60 },
  },
  {
    name: 'Lisbon to Madeira Island (Atlantic Crossing)',
    startName: 'Lisbon (Cascais)',
    start: { lat: 38.69, lon: -9.42 },
    destName: 'Madeira (Funchal)',
    dest: { lat: 32.64, lon: -16.90 },
  },
  {
    name: 'San Francisco to Hawaii (Transpac Passage)',
    startName: 'San Francisco (Golden Gate)',
    start: { lat: 37.81, lon: -122.50 },
    destName: 'Honolulu (Diamond Head)',
    dest: { lat: 21.25, lon: -157.80 },
  },
];

export function calcDirectDistanceNM(p1: Point, p2: Point): number {
  const R = 6371000; // meters
  const dLat = ((p2.lat - p1.lat) * Math.PI) / 180;
  const dLon = ((p2.lon - p1.lon) * Math.PI) / 180;
  const a =
    Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.cos((p1.lat * Math.PI) / 180) *
      Math.cos((p2.lat * Math.PI) / 180) *
      Math.sin(dLon / 2) *
      Math.sin(dLon / 2);
  const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
  return (R * c) / 1852; // NM
}

/**
 * Returns a sane default isochrone time step (in hours) based on great-circle passage distance:
 * - <= 100 NM:   5 minutes  (5/60 h)   -> High precision for inshore & island passages (e.g. Grenada -> Trinidad)
 * - 100-250 NM:  15 minutes (15/60 h)  -> Precision for channels & straits
 * - 250-500 NM:  30 minutes (0.5 h)    -> Fastnet, coastal races, medium passages (e.g. Cowes -> Fastnet)
 * - 500-1200 NM: 1 hour     (1.0 h)    -> Offshore & Bermuda crossings (e.g. Newport -> Bermuda)
 * - > 1200 NM:   2 hours    (2.0 h)    -> Ocean crossings (e.g. San Francisco -> Hawaii)
 */
export function getSaneDefaultTimeStepHours(distNM: number): number {
  if (distNM <= 100) return 5 / 60;
  if (distNM <= 250) return 15 / 60;
  if (distNM <= 500) return 30 / 60;
  if (distNM <= 1200) return 1.0;
  return 2.0;
}

export async function fetchPresets(): Promise<BoatPreset[]> {
  try {
    const res = await fetch('/api/v1/presets');
    if (!res.ok) throw new Error('Failed to fetch presets');
    return await res.json();
  } catch (err) {
    console.warn('Fallback to local presets:', err);
    return [
      {
        id: '36ft-ketch',
        name: '36ft Cruising Ketch',
        loa_m: 11.0,
        beam_m: 3.5,
        draft_m: 1.5,
        displacement_kg: 7000,
        rig_type: 'ketch',
      },
      {
        id: '36ft-sloop',
        name: '36ft Racer-Cruiser Sloop',
        loa_m: 10.75,
        beam_m: 3.51,
        draft_m: 2.1,
        displacement_kg: 5500,
        rig_type: 'sloop',
      },
      {
        id: '40ft-cruiser',
        name: '40ft Performance Cruiser',
        loa_m: 12.24,
        beam_m: 3.89,
        draft_m: 2.45,
        displacement_kg: 7500,
        rig_type: 'sloop',
      },
      {
        id: '24ft-sportboat',
        name: '24ft Sportboat',
        loa_m: 7.32,
        beam_m: 2.5,
        draft_m: 1.75,
        displacement_kg: 850,
        rig_type: 'sloop',
      },
    ];
  }
}

export async function fetchBoatDetail(presetId: string): Promise<BoatDetail> {
  const res = await fetch(`/api/v1/presets/${encodeURIComponent(presetId)}`);
  if (!res.ok) {
    throw new Error(`Failed to load specifications for yacht preset '${presetId}'`);
  }
  return await res.json();
}

export async function fetchPolarMatrix(presetId: string): Promise<SolveMatrixResponse> {
  const res = await fetch('/api/v1/solve/matrix', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ preset_name: presetId }),
  });
  if (!res.ok) {
    throw new Error(`Failed to compute polar matrix for yacht '${presetId}'`);
  }
  return await res.json();
}

export async function fetchPlotImageBlob(
  plotType: 'polar' | 'curves' | 'resistance',
  presetId: string,
  heelDeg: number = 15
): Promise<string> {
  const url =
    plotType === 'resistance'
      ? `/api/v1/plot/resistance?heel_deg=${heelDeg}`
      : `/api/v1/plot/${plotType}`;

  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ preset_name: presetId }),
  });

  if (!res.ok) {
    throw new Error(`Failed to render ${plotType} plot`);
  }

  const blob = await res.blob();
  return URL.createObjectURL(blob);
}

export async function exportORCPolFile(presetId: string): Promise<string> {
  const res = await fetch('/api/v1/export/orc', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ preset_name: presetId }),
  });
  if (!res.ok) {
    throw new Error('Failed to generate ORC .pol export');
  }
  return await res.text();
}

export async function exportCSVPolFile(presetId: string): Promise<string> {
  const res = await fetch('/api/v1/export/csv', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ preset_name: presetId }),
  });
  if (!res.ok) {
    throw new Error('Failed to generate CSV polar export');
  }
  return await res.text();
}

export async function calculateRoute(params: {
  start: Point;
  dest: Point;
  startTime?: string;
  boatPreset: string;
  timeStepHours?: number;
  tackPenaltyMinutes?: number;
  gybePenaltyMinutes?: number;
}): Promise<RouteResult> {
  const res = await fetch('/api/v1/route', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      start: params.start,
      dest: params.dest,
      start_time: params.startTime ? new Date(params.startTime).toISOString() : undefined,
      boat_preset: params.boatPreset,
      time_step_hours: params.timeStepHours || 2.0,
      tack_penalty_minutes: params.tackPenaltyMinutes,
      gybe_penalty_minutes: params.gybePenaltyMinutes,
    }),
  });

  if (!res.ok) {
    const errText = await res.text();
    throw new Error(errText || 'Routing calculation failed');
  }

  return await res.json();
}

export async function fetchWeatherGrid(params: {
  minLat: number;
  maxLat: number;
  minLon: number;
  maxLon: number;
  latStep?: number;
  lonStep?: number;
  time?: string;
}): Promise<WeatherGridResponse> {
  const res = await fetch('/api/v1/weather/grid', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      min_lat: params.minLat,
      max_lat: params.maxLat,
      min_lon: params.minLon,
      max_lon: params.maxLon,
      lat_step: params.latStep || 2.0,
      lon_step: params.lonStep || 2.0,
      time: params.time ? new Date(params.time).toISOString() : undefined,
    }),
  });

  if (!res.ok) {
    throw new Error('Failed to fetch weather grid');
  }

  return await res.json();
}

export async function fetchLandmaskPolygons(bounds?: {
  minLat: number;
  maxLat: number;
  minLon: number;
  maxLon: number;
}): Promise<LandmaskPolygon[]> {
  try {
    let url = '/api/v1/landmask/polygons';
    if (bounds) {
      url += `?min_lat=${bounds.minLat.toFixed(2)}&max_lat=${bounds.maxLat.toFixed(2)}&min_lon=${bounds.minLon.toFixed(2)}&max_lon=${bounds.maxLon.toFixed(2)}`;
    }
    const res = await fetch(url);
    if (!res.ok) throw new Error('Failed to fetch landmask polygons');
    const data: LandmaskResponse = await res.json();
    return data.polygons || [];
  } catch (err) {
    console.warn('Failed to fetch landmask polygons:', err);
    return [];
  }
}
