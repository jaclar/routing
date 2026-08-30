export interface Point {
  lat: number;
  lon: number;
}

export type WeatherModelId = 'gfs_0p25' | 'ifs_0p25' | 'icon_global' | string;

export interface WeatherModelMeta {
  id: WeatherModelId;
  name: string;
  shortName: string;
  center: string;
  resolution: string;
  color: string;
  lightColor: string;
  badgeBg: string;
  badgeBorder: string;
}

export const WEATHER_MODELS: Record<string, WeatherModelMeta> = {
  gfs_0p25: {
    id: 'gfs_0p25',
    name: 'NOAA GFS (0.25°)',
    shortName: 'GFS 0.25°',
    center: 'NOAA NCEP (USA)',
    resolution: '0.25° Global',
    color: '#0284c7',
    lightColor: '#38bdf8',
    badgeBg: 'rgba(56, 189, 248, 0.15)',
    badgeBorder: 'rgba(56, 189, 248, 0.4)',
  },
  ifs_0p25: {
    id: 'ifs_0p25',
    name: 'ECMWF IFS (0.25°)',
    shortName: 'ECMWF 0.25°',
    center: 'ECMWF (Europe)',
    resolution: '0.25° Global',
    color: '#8b5cf6',
    lightColor: '#a855f7',
    badgeBg: 'rgba(168, 85, 247, 0.15)',
    badgeBorder: 'rgba(168, 85, 247, 0.4)',
  },
  icon_global: {
    id: 'icon_global',
    name: 'DWD ICON (Global)',
    shortName: 'ICON Global',
    center: 'DWD (Germany)',
    resolution: '0.25° Regular',
    color: '#f59e0b',
    lightColor: '#fbbf24',
    badgeBg: 'rgba(245, 158, 11, 0.15)',
    badgeBorder: 'rgba(245, 158, 11, 0.4)',
  },
};

export const DEFAULT_WEATHER_MODEL: WeatherModelId = 'gfs_0p25';

export interface BoatPreset {
  id: string;
  name: string;
  loa_m: number;
  beam_m: number;
  draft_m: number;
  displacement_kg: number;
  rig_type: string;
  isCustom?: boolean;
  isPolFileOnly?: boolean;
  customBoat?: BoatDetail;
  polarData?: SolveMatrixResponse;
}

export interface CustomBoatFile {
  version: string;
  format: 'sailboat-vpp-polar';
  created_at: string;
  boat: BoatDetail;
  polars?: SolveMatrixResponse;
}

export interface HullDetail {
  loa: number;
  lwl: number;
  b_max: number;
  b_wl: number;
  draft_canoe: number;
  draft_total: number;
  displacement_mass: number;
  wetted_surface?: number;
  prismatic_coef: number;
  form_factor_k: number;
  lcb_fraction: number;
}

export interface AppendagesDetail {
  keel_type: string;
  keel_area: number;
  keel_span: number;
  rudder_area: number;
  rudder_span: number;
  effective_draft?: number;
  wetted_surface?: number;
}

export interface RigDetail {
  rig_type: string;
  main_p: number;
  main_e: number;
  fore_i: number;
  fore_j: number;
  mast_height_above_water: number;
  boom_height_above_water: number;
  mizzen_p?: number;
  mizzen_e?: number;
  mizzen_mast_height?: number;
  mizzen_boom_height?: number;
}

export interface StabilityDetail {
  gmt: number;
  crew_mass: number;
  crew_hiking_distance: number;
  crew_hiking_fraction: number;
}

export interface BoatDetail {
  name: string;
  hull: HullDetail;
  appendages: AppendagesDetail;
  rig: RigDetail;
  stability: StabilityDetail;
}

export interface VMGTarget {
  tws_kts: number;
  target_twa_deg: number;
  target_v_boat_kts: number;
  target_vmg_kts: number;
  is_upwind: boolean;
}

export interface SolveMatrixResponse {
  boat_name: string;
  tws_list: number[];
  twa_list: number[];
  speed_matrix: number[][]; // [twsIndex][twaIndex] in knots
  upwind_vmg_targets: Record<string, VMGTarget>;
  downwind_vmg_targets: Record<string, VMGTarget>;
}

export interface WindCondition {
  tws_kts: number;
  twd_deg: number;
  u_ms: number;
  v_ms: number;
}

export interface WeatherGridResponse {
  model?: string;
  time: string;
  min_lat: number;
  max_lat: number;
  min_lon: number;
  max_lon: number;
  lat_step: number;
  lon_step: number;
  grid: WindCondition[][];
}

export interface Waypoint {
  lat: number;
  lon: number;
  time: string;
  heading_deg: number;
  boat_speed_kts: number;
  tws_kts: number;
  twd_deg: number;
  twa_deg: number;
  distance_nm: number;
  distance_to_dest_nm: number;
  estimated_heel_deg: number;
  maneuver?: 'none' | 'tack' | 'gybe';
  gust_kts?: number;
  wave_height_m?: number;
  wave_period_s?: number;
}

export interface IsochroneWave {
  step_index: number;
  time: string;
  points: Point[];
}

export interface RouteResult {
  boat_name: string;
  start_point: Point;
  dest_point: Point;
  start_time: string;
  arrival_time: string;
  total_duration_hours: number;
  total_distance_nm: number;
  direct_distance_nm: number;
  average_speed_kts: number;
  max_wind_kts: number;
  total_tacks: number;
  total_gybes: number;
  tack_penalty_minutes: number;
  gybe_penalty_minutes: number;
  waypoints: Waypoint[];
  isochrones: IsochroneWave[];
  destination_reached: boolean;
  model_id?: WeatherModelId;
}

export type MultiRouteResult = Record<string, RouteResult>;

export interface MultiModelRouteResponse extends RouteResult {
  active_model: string;
  routes: Record<string, RouteResult>;
}

export interface RoutePreset {
  name: string;
  startName: string;
  start: Point;
  destName: string;
  dest: Point;
}

export interface LandmaskPolygon {
  name: string;
  min_lat: number;
  max_lat: number;
  min_lon: number;
  max_lon: number;
  vertices: Point[];
}

export interface LandmaskResponse {
  polygons: LandmaskPolygon[];
}
