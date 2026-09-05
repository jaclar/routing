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

export interface MemberOutcome {
  member_id: number;
  total_duration_hours: number;
  total_distance_nm?: number;
  average_speed_kts: number;
  max_wind_kts: number;
  total_tacks?: number;
  waypoints?: Waypoint[];
  trajectory?: Point[];
}

export interface EnsembleComparison {
  mean_duration_hours: number;
  std_duration_hours: number;
  min_duration_hours: number;
  max_duration_hours: number;
  iqr_duration_hours: number;
  p10_duration_hours: number;
  p90_duration_hours: number;
  fastest_member_id: number;
  slowest_member_id: number;
  member_count: number;
  members?: MemberOutcome[];
}

export interface UncertaintyEnvelope {
  left_boundary?: Point[];
  right_boundary?: Point[];
  polygon: Point[];
  confidence_level?: string;
  max_lateral_nm?: number;
}

export interface WaypointConfidence {
  index: number;
  time: string;
  score: number;
  score_strategy_a: number;
  score_strategy_b?: number;
  lateral_uncertainty_nm?: number;
  wind_speed_mean_kts: number;
  wind_speed_std_kts: number;
  wind_speed_p10_kts: number;
  wind_speed_p90_kts: number;
  wind_dir_spread_deg: number;
  gale_probability: number;
  strong_wind_probability: number;
  member_speed_mean_kts?: number;
  member_speed_std_kts?: number;
  member_speed_p10_kts?: number;
  member_speed_p90_kts?: number;
}

export interface StatisticalComparison {
  mean_duration_hours: number;
  std_duration_hours: number;
  min_duration_hours: number;
  max_duration_hours: number;
  iqr_duration_hours: number;
}

export interface RouteConfidence {
  overall_score: number;
  category: 'Very High' | 'High' | 'Moderate' | 'Low' | 'High Uncertainty' | string;
  score_strategy_a: number;
  score_strategy_b?: number;
  agreement_score?: number;
  model_id: string;
  num_members: number;
  waypoints: WaypointConfidence[];
  statistical_comparison?: StatisticalComparison;
  ensemble_comparison?: EnsembleComparison;
  uncertainty_envelope?: UncertaintyEnvelope;
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
  confidence_score?: number;
  confidence_score_a?: number;
  confidence_score_b?: number;
  wind_speed_std_kts?: number;
  wind_speed_p10_kts?: number;
  wind_speed_p90_kts?: number;
  wind_dir_spread_deg?: number;
  gale_probability?: number;
  lateral_uncertainty_nm?: number;
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
  confidence?: RouteConfidence;
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

export interface RepresentativeWeatherEvent {
  type: 'mid_passage' | 'peak_wind' | string;
  time: string;
  description: string;
  location: Point;
  wind_speed_kts: number;
  wind_dir_deg: number;
  wave_height_m: number;
  wave_period_s: number;
  grid_min_lat?: number;
  grid_max_lat?: number;
  grid_min_lon?: number;
  grid_max_lon?: number;
  grid_lat_step?: number;
  grid_lon_step?: number;
  weather_grid?: WindCondition[][];
}

export interface WindowCandidate {
  departure_time: string;
  arrival_time: string;
  duration_hours: number;
  distance_nm: number;
  comfort_score: number;
  comfort_rank: number;
  confidence_score: number;

  upwind_fraction: number;
  close_reach_fraction: number;
  beam_reach_fraction: number;
  broad_reach_fraction: number;
  downwind_fraction: number;

  avg_wind_kts: number;
  max_wind_kts: number;
  avg_wave_height_m: number;
  max_wave_height_m: number;
  avg_wave_period_s: number;
  max_heel_deg: number;
  total_tacks: number;
  total_gybes: number;

  gale_warning: boolean;
  gale_warning_detail?: string;
  low_wind_warning: boolean;
  low_wind_warning_detail?: string;
  night_arrival_warning?: boolean;
  night_arrival_warning_detail?: string;

  representative_event: RepresentativeWeatherEvent;
  route: RouteResult;
}

export interface WeatherWindowRequest {
  start: Point;
  dest: Point;
  earliest_departure: string;
  latest_departure?: string;
  boat_preset?: string;
  model?: string;
  custom_boat?: BoatDetail;
  custom_polar?: {
    boat_name: string;
    tws_list: number[];
    twa_list: number[];
    speed_matrix: number[][];
  };
}

export interface WeatherWindowResponse {
  start: Point;
  dest: Point;
  direct_distance_nm: number;
  time_step_hours: number;
  departure_step_hours: number;
  evaluated_departures: number;
  earliest_departure: string;
  latest_departure: string;
  windows: WindowCandidate[];
}

