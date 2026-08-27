import { BoatDetail, BoatPreset, CustomBoatFile, LandmaskPolygon, LandmaskResponse, Point, RouteResult, SolveMatrixResponse, WeatherGridResponse } from '../types';

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

export async function fetchBoatDetail(presetId: string, customBoats?: BoatPreset[]): Promise<BoatDetail> {
  if (customBoats) {
    const custom = customBoats.find((b) => b.id === presetId);
    if (custom && custom.customBoat) {
      return custom.customBoat;
    }
  }

  const res = await fetch(`/api/v1/presets/${encodeURIComponent(presetId)}`);
  if (!res.ok) {
    throw new Error(`Failed to load specifications for yacht preset '${presetId}'`);
  }
  return await res.json();
}

export async function fetchPolarMatrix(
  presetIdOrBoat: string | BoatDetail | SolveMatrixResponse
): Promise<SolveMatrixResponse> {
  if (typeof presetIdOrBoat === 'object' && 'speed_matrix' in presetIdOrBoat) {
    return presetIdOrBoat;
  }

  const payload =
    typeof presetIdOrBoat === 'string'
      ? { preset_name: presetIdOrBoat }
      : { boat: presetIdOrBoat };

  const res = await fetch('/api/v1/solve/matrix', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const errText = await res.text();
    throw new Error(`Failed to compute polar matrix: ${errText}`);
  }
  return await res.json();
}

export async function fetchPlotImageBlob(
  plotType: 'polar' | 'curves' | 'resistance',
  presetIdOrBoat: string | BoatDetail | SolveMatrixResponse,
  heelDeg: number = 15
): Promise<string> {
  const url =
    plotType === 'resistance'
      ? `/api/v1/plot/resistance?heel_deg=${heelDeg}`
      : `/api/v1/plot/${plotType}`;

  let payload: any;
  if (typeof presetIdOrBoat === 'string') {
    payload = { preset_name: presetIdOrBoat };
  } else if ('speed_matrix' in presetIdOrBoat) {
    payload = {
      boat_name: presetIdOrBoat.boat_name,
      tws_list: presetIdOrBoat.tws_list,
      twa_list: presetIdOrBoat.twa_list,
      speed_matrix: presetIdOrBoat.speed_matrix,
    };
  } else {
    payload = { boat: presetIdOrBoat };
  }

  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });

  if (!res.ok) {
    throw new Error(`Failed to render ${plotType} plot`);
  }

  const blob = await res.blob();
  return URL.createObjectURL(blob);
}

export async function exportORCPolFile(
  presetIdOrBoat: string | BoatDetail | SolveMatrixResponse
): Promise<string> {
  let payload: any;
  if (typeof presetIdOrBoat === 'string') {
    payload = { preset_name: presetIdOrBoat };
  } else if ('speed_matrix' in presetIdOrBoat) {
    payload = {
      boat_name: presetIdOrBoat.boat_name,
      tws_list: presetIdOrBoat.tws_list,
      twa_list: presetIdOrBoat.twa_list,
      speed_matrix: presetIdOrBoat.speed_matrix,
    };
  } else {
    payload = { boat: presetIdOrBoat };
  }

  const res = await fetch('/api/v1/export/orc', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error('Failed to generate ORC .pol export');
  }
  return await res.text();
}

export async function exportCSVPolFile(
  presetIdOrBoat: string | BoatDetail | SolveMatrixResponse
): Promise<string> {
  let payload: any;
  if (typeof presetIdOrBoat === 'string') {
    payload = { preset_name: presetIdOrBoat };
  } else if ('speed_matrix' in presetIdOrBoat) {
    payload = {
      boat_name: presetIdOrBoat.boat_name,
      tws_list: presetIdOrBoat.tws_list,
      twa_list: presetIdOrBoat.twa_list,
      speed_matrix: presetIdOrBoat.speed_matrix,
    };
  } else {
    payload = { boat: presetIdOrBoat };
  }

  const res = await fetch('/api/v1/export/csv', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
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
  customBoat?: BoatDetail;
  customPolar?: {
    boat_name: string;
    tws_list: number[];
    twa_list: number[];
    speed_matrix: number[][];
  };
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
      custom_boat: params.customBoat,
      custom_polar: params.customPolar,
    }),
  });

  if (!res.ok) {
    const errText = await res.text();
    throw new Error(errText || 'Routing calculation failed');
  }

  return await res.json();
}

export function parsePolFile(content: string, filename: string = 'custom_polar.pol'): SolveMatrixResponse {
  const lines = content
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter((l) => l.length > 0 && !l.startsWith('#') && !l.startsWith('!') && !l.startsWith('//') && !l.startsWith(';'));

  if (lines.length < 2) {
    throw new Error('Invalid .pol file: File must contain at least a header line and one data row.');
  }

  // Detect delimiter: tab, semicolon, comma, or whitespace
  const firstLine = lines[0];
  let delimiter: RegExp | string = /\s+/;
  if (firstLine.includes('\t')) {
    delimiter = '\t';
  } else if (firstLine.includes(';')) {
    delimiter = ';';
  } else if (firstLine.includes(',')) {
    delimiter = ',';
  }

  // Header tokens
  const rawHeaderTokens = firstLine.split(delimiter).map((t) => t.trim()).filter(Boolean);
  const sampleDataTokens = lines[1].split(delimiter).map((t) => t.trim()).filter(Boolean);

  let twsTokens: string[];
  if (sampleDataTokens.length > 0 && rawHeaderTokens.length === sampleDataTokens.length - 1) {
    // Header only contains wind speeds without a column 0 label
    twsTokens = rawHeaderTokens;
  } else {
    // Standard format: token 0 is "twa/tws", "twa", "deg", etc.
    twsTokens = rawHeaderTokens.slice(1);
  }

  if (twsTokens.length === 0) {
    throw new Error('Invalid .pol header: could not parse wind speeds.');
  }

  const twsList: number[] = [];
  for (const tok of twsTokens) {
    const val = parseFloat(tok.replace(/[^0-9.]/g, ''));
    if (isNaN(val)) {
      throw new Error(`Invalid TWS value in header: "${tok}"`);
    }
    twsList.push(val);
  }

  // Parse TWA rows and speed columns
  const twaList: number[] = [];
  const rows: number[][] = []; // [twaIdx][twsIdx]

  for (let r = 1; r < lines.length; r++) {
    const rowTokens = lines[r].split(delimiter).map((t) => t.trim()).filter(Boolean);
    if (rowTokens.length < 2) continue;

    const twa = parseFloat(rowTokens[0].replace(/[^0-9.]/g, ''));
    if (isNaN(twa)) continue;

    const speedsForRow: number[] = [];
    for (let c = 0; c < twsList.length; c++) {
      const spdToken = rowTokens[c + 1];
      const spd = spdToken !== undefined ? parseFloat(spdToken.replace(/[^0-9.]/g, '')) : 0.0;
      speedsForRow.push(isNaN(spd) ? 0.0 : spd);
    }

    twaList.push(twa);
    rows.push(speedsForRow);
  }

  if (twaList.length === 0) {
    throw new Error('Invalid .pol file: no valid TWA rows found.');
  }

  // Transpose to [len(TWS)][len(TWA)] as required by SolveMatrixResponse
  const speedMatrix: number[][] = [];
  for (let i = 0; i < twsList.length; i++) {
    const rowForTws: number[] = [];
    for (let j = 0; j < twaList.length; j++) {
      rowForTws.push(rows[j][i] || 0.0);
    }
    speedMatrix.push(rowForTws);
  }

  // Calculate VMG targets (Upwind & Downwind)
  const upwindTargets: Record<string, any> = {};
  const downwindTargets: Record<string, any> = {};

  for (let i = 0; i < twsList.length; i++) {
    const tws = twsList[i];
    let bestUpVMG = -Infinity;
    let bestUpTWA = 40.0;
    let bestUpSpd = 0.0;

    let bestDownVMG = -Infinity;
    let bestDownTWA = 140.0;
    let bestDownSpd = 0.0;

    for (let j = 0; j < twaList.length; j++) {
      const twa = twaList[j];
      const spd = speedMatrix[i][j];
      const rad = (twa * Math.PI) / 180.0;
      const vmg = spd * Math.cos(rad);

      // Upwind search: TWA 28° to 75°
      if (twa >= 28 && twa <= 75) {
        if (vmg > bestUpVMG && spd > 0.05) {
          bestUpVMG = vmg;
          bestUpTWA = twa;
          bestUpSpd = spd;
        }
      }

      // Downwind search: TWA 110° to 180°
      if (twa >= 110 && twa <= 180) {
        const downVmg = spd * -Math.cos(rad);
        if (downVmg > bestDownVMG && spd > 0.05) {
          bestDownVMG = downVmg;
          bestDownTWA = twa;
          bestDownSpd = spd;
        }
      }
    }

    const key = tws.toFixed(1);
    upwindTargets[key] = {
      tws_kts: tws,
      target_twa_deg: bestUpTWA,
      target_v_boat_kts: bestUpSpd,
      target_vmg_kts: Math.max(0, bestUpVMG),
      is_upwind: true,
    };
    downwindTargets[key] = {
      tws_kts: tws,
      target_twa_deg: bestDownTWA,
      target_v_boat_kts: bestDownSpd,
      target_vmg_kts: Math.max(0, bestDownVMG),
      is_upwind: false,
    };
  }

  // Derive friendly boat name from filename
  const cleanName = filename
    .replace(/\.[^/.]+$/, '')
    .replace(/[_-]+/g, ' ')
    .trim();

  return {
    boat_name: cleanName || 'Custom POL Yacht',
    tws_list: twsList,
    twa_list: twaList,
    speed_matrix: speedMatrix,
    upwind_vmg_targets: upwindTargets,
    downwind_vmg_targets: downwindTargets,
  };
}

// LocalStorage & JSON file persistence helpers
const CUSTOM_BOATS_STORAGE_KEY = 'sailboat_custom_boats_v1';

export function loadCustomBoatsFromStorage(): BoatPreset[] {
  try {
    const data = localStorage.getItem(CUSTOM_BOATS_STORAGE_KEY);
    if (!data) return [];
    const parsed = JSON.parse(data);
    return Array.isArray(parsed) ? parsed : [];
  } catch (err) {
    console.error('Failed to load custom boats from localStorage:', err);
    return [];
  }
}

export function saveCustomBoatToStorage(boatPreset: BoatPreset): void {
  try {
    const existing = loadCustomBoatsFromStorage();
    const filtered = existing.filter((b) => b.id !== boatPreset.id);
    filtered.push(boatPreset);
    localStorage.setItem(CUSTOM_BOATS_STORAGE_KEY, JSON.stringify(filtered));
  } catch (err) {
    console.error('Failed to save custom boat to localStorage:', err);
  }
}

export function deleteCustomBoatFromStorage(id: string): void {
  try {
    const existing = loadCustomBoatsFromStorage();
    const filtered = existing.filter((b) => b.id !== id);
    localStorage.setItem(CUSTOM_BOATS_STORAGE_KEY, JSON.stringify(filtered));
  } catch (err) {
    console.error('Failed to delete custom boat from localStorage:', err);
  }
}

export function exportCustomBoatJSON(boat: BoatDetail, polars?: SolveMatrixResponse): void {
  const fileData: CustomBoatFile = {
    version: '1.0',
    format: 'sailboat-vpp-polar',
    created_at: new Date().toISOString(),
    boat,
    polars,
  };

  const jsonStr = JSON.stringify(fileData, null, 2);
  const blob = new Blob([jsonStr], { type: 'application/json;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  const safeName = (boat.name || 'custom_boat').replace(/[^a-zA-Z0-9_-]/g, '_');
  link.download = `${safeName}_vpp_data.json`;
  link.click();
  URL.revokeObjectURL(url);
}

export function parseAndValidateBoatJSON(content: string): CustomBoatFile {
  let data: any;
  try {
    data = JSON.parse(content);
  } catch (e: any) {
    throw new Error('Invalid JSON format: ' + e.message);
  }

  if (!data || typeof data !== 'object') {
    throw new Error('Invalid file content: expected a JSON object');
  }

  const boat = data.boat || data;
  if (!boat.hull || !boat.rig || !boat.appendages || !boat.stability) {
    throw new Error('Incomplete boat specification: missing hull, rig, appendages, or stability data');
  }

  if (typeof boat.hull.loa !== 'number' || typeof boat.hull.displacement_mass !== 'number') {
    throw new Error('Invalid hull specifications: LOA and displacement mass must be numeric');
  }

  return {
    version: data.version || '1.0',
    format: 'sailboat-vpp-polar',
    created_at: data.created_at || new Date().toISOString(),
    boat: {
      name: boat.name || 'Imported Custom Boat',
      hull: {
        loa: Number(boat.hull.loa),
        lwl: Number(boat.hull.lwl || boat.hull.loa * 0.85),
        b_max: Number(boat.hull.b_max),
        b_wl: Number(boat.hull.b_wl || boat.hull.b_max * 0.88),
        draft_canoe: Number(boat.hull.draft_canoe || boat.hull.draft_total * 0.35),
        draft_total: Number(boat.hull.draft_total),
        displacement_mass: Number(boat.hull.displacement_mass),
        wetted_surface: boat.hull.wetted_surface ? Number(boat.hull.wetted_surface) : undefined,
        prismatic_coef: Number(boat.hull.prismatic_coef || 0.56),
        form_factor_k: Number(boat.hull.form_factor_k || 0.12),
        lcb_fraction: Number(boat.hull.lcb_fraction || 0.52),
      },
      appendages: {
        keel_type: String(boat.appendages.keel_type || 'fin'),
        keel_area: Number(boat.appendages.keel_area || 1.8),
        keel_span: Number(boat.appendages.keel_span || 1.4),
        rudder_area: Number(boat.appendages.rudder_area || 0.8),
        rudder_span: Number(boat.appendages.rudder_span || 1.1),
        effective_draft: boat.appendages.effective_draft ? Number(boat.appendages.effective_draft) : undefined,
        wetted_surface: boat.appendages.wetted_surface ? Number(boat.appendages.wetted_surface) : undefined,
      },
      rig: {
        rig_type: String(boat.rig.rig_type || 'sloop'),
        main_p: Number(boat.rig.main_p),
        main_e: Number(boat.rig.main_e),
        fore_i: Number(boat.rig.fore_i),
        fore_j: Number(boat.rig.fore_j),
        mast_height_above_water: Number(boat.rig.mast_height_above_water || boat.rig.fore_i * 1.15),
        boom_height_above_water: Number(boat.rig.boom_height_above_water || 1.8),
        mizzen_p: boat.rig.mizzen_p ? Number(boat.rig.mizzen_p) : undefined,
        mizzen_e: boat.rig.mizzen_e ? Number(boat.rig.mizzen_e) : undefined,
        mizzen_mast_height: boat.rig.mizzen_mast_height ? Number(boat.rig.mizzen_mast_height) : undefined,
        mizzen_boom_height: boat.rig.mizzen_boom_height ? Number(boat.rig.mizzen_boom_height) : undefined,
      },
      stability: {
        gmt: Number(boat.stability.gmt || 1.1),
        crew_mass: Number(boat.stability.crew_mass || 350),
        crew_hiking_distance: Number(boat.stability.crew_hiking_distance || 1.5),
        crew_hiking_fraction: Number(boat.stability.crew_hiking_fraction || 0.8),
      },
    },
    polars: data.polars,
  };
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
