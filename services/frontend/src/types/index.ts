export interface Point {
  lat: number;
  lon: number;
}

export interface BoatPreset {
  id: string;
  name: string;
  loa_m: number;
  beam_m: number;
  draft_m: number;
  displacement_kg: number;
  rig_type: string;
}

export interface WindCondition {
  tws_kts: number;
  twd_deg: number;
  u_ms: number;
  v_ms: number;
}

export interface WeatherGridResponse {
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
}

export interface RoutePreset {
  name: string;
  startName: string;
  start: Point;
  destName: string;
  dest: Point;
}
