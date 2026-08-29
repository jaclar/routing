"""Polar diagram data structures, interpolation, and VMG target calculations."""

from dataclasses import dataclass
from typing import List, Dict, Tuple, Optional
import numpy as np
from scipy.interpolate import RegularGridInterpolator

from vpp.solver.vpp_solver import VPPPointResult, VPPSolver


@dataclass
class VMGTarget:
    """Optimal Velocity Made Good target point."""
    tws_kts: float
    target_twa_deg: float
    target_v_boat_kts: float
    target_vmg_kts: float
    is_upwind: bool


@dataclass
class PolarTable:
    """Full performance matrix across true wind speeds and angles."""
    boat_name: str
    tws_list: List[float]                       # List of TWS [knots]
    twa_list: List[float]                       # List of TWA [degrees]
    matrix: Dict[Tuple[float, float], VPPPointResult]  # (tws, twa) -> VPPPointResult
    speed_table: np.ndarray                     # 2D array [n_tws, n_twa] in knots
    upwind_targets: Dict[float, VMGTarget]      # tws -> upwind VMGTarget
    downwind_targets: Dict[float, VMGTarget]    # tws -> downwind VMGTarget

    def get_point(self, tws_kts: float, twa_deg: float) -> Optional[VPPPointResult]:
        """Retrieve the exact grid point result if available."""
        return self.matrix.get((tws_kts, twa_deg))

    def create_interpolator(self) -> RegularGridInterpolator:
        """Create a 2D bilinear interpolator for continuous (tws, twa) speed lookup."""
        return RegularGridInterpolator(
            (np.array(self.tws_list), np.array(self.twa_list)),
            self.speed_table,
            bounds_error=False,
            fill_value=None,
        )

    def interpolate_speed(self, tws_kts: float, twa_deg: float) -> float:
        """Interpolate boat speed for arbitrary (TWS, TWA) with aerodynamic no-go zone."""
        angle = abs(float(twa_deg) % 360.0)
        if angle > 180.0:
            angle = 360.0 - angle
        if angle <= 22.0:
            return 0.0

        interp = self.create_interpolator()
        val = float(interp(np.array([[tws_kts, max(angle, 30.0)]]))[0])
        if angle < 28.0:
            frac = (angle - 22.0) / 6.0
            val = val * (frac * frac)
        return max(val, 0.0)


def compute_vmg_targets(
    tws_kts: float,
    twa_list: List[float],
    speeds: List[float],
) -> Tuple[VMGTarget, VMGTarget]:
    """Find the optimal upwind beating and downwind gybing angles for max VMG.
    
    Args:
        tws_kts: True Wind Speed in knots.
        twa_list: List of TWA angles [degrees].
        speeds: List of corresponding boat speeds [knots].
        
    Returns:
        tuple (upwind_target, downwind_target)
    """
    twa_arr = np.array(twa_list)
    spd_arr = np.array(speeds)
    twa_rad = np.deg2rad(twa_arr)

    # 1. Upwind VMG: VMG = V * cos(TWA) in range TWA in [30, 65]
    upwind_mask = (twa_arr >= 30.0) & (twa_arr <= 70.0)
    if np.any(upwind_mask):
        sub_twa = twa_arr[upwind_mask]
        sub_spd = spd_arr[upwind_mask]
        sub_vmg = sub_spd * np.cos(np.deg2rad(sub_twa))
        best_idx = int(np.argmax(sub_vmg))
        upwind_target = VMGTarget(
            tws_kts=tws_kts,
            target_twa_deg=float(sub_twa[best_idx]),
            target_v_boat_kts=float(sub_spd[best_idx]),
            target_vmg_kts=float(sub_vmg[best_idx]),
            is_upwind=True,
        )
    else:
        upwind_target = VMGTarget(tws_kts, 45.0, 5.0, 3.5, True)

    # 2. Downwind VMG: VMG = -V * cos(TWA) in range TWA in [120, 180]
    downwind_mask = (twa_arr >= 120.0) & (twa_arr <= 180.0)
    if np.any(downwind_mask):
        sub_twa = twa_arr[downwind_mask]
        sub_spd = spd_arr[downwind_mask]
        sub_vmg = -sub_spd * np.cos(np.deg2rad(sub_twa))
        best_idx = int(np.argmax(sub_vmg))
        downwind_target = VMGTarget(
            tws_kts=tws_kts,
            target_twa_deg=float(sub_twa[best_idx]),
            target_v_boat_kts=float(sub_spd[best_idx]),
            target_vmg_kts=float(sub_vmg[best_idx]),
            is_upwind=False,
        )
    else:
        downwind_target = VMGTarget(tws_kts, 150.0, 6.0, 5.0, False)

    return upwind_target, downwind_target


def generate_polar_table(
    solver: VPPSolver,
    tws_list: Optional[List[float]] = None,
    twa_list: Optional[List[float]] = None,
) -> PolarTable:
    """Generate a complete polar diagram across wind speeds and angles.
    
    Args:
        solver: VPPSolver instance.
        tws_list: List of TWS in knots. Defaults to [6, 8, 10, 12, 14, 16, 20, 25].
        twa_list: List of TWA in degrees. Defaults to [0, 20, 25, 30, 35, 40, 45, 52, 60, 70, 80, 90, 110, 120, 135, 150, 165, 180].
        
    Returns:
        PolarTable containing all solved data and VMG targets.
    """
    if tws_list is None:
        tws_list = [6.0, 8.0, 10.0, 12.0, 14.0, 16.0, 20.0, 25.0]
    if twa_list is None:
        twa_list = [0.0, 20.0, 25.0, 30.0, 35.0, 40.0, 45.0, 52.0, 60.0, 70.0, 80.0, 90.0, 110.0, 120.0, 135.0, 150.0, 165.0, 180.0]

    matrix: Dict[Tuple[float, float], VPPPointResult] = {}
    speed_table = np.zeros((len(tws_list), len(twa_list)))
    upwind_targets: Dict[float, VMGTarget] = {}
    downwind_targets: Dict[float, VMGTarget] = {}

    for i, tws in enumerate(tws_list):
        last_result: Optional[VPPPointResult] = None
        speeds_for_tws: List[float] = []

        for j, twa in enumerate(twa_list):
            res = solver.solve_point(
                tws_kts=tws,
                twa_deg=twa,
                warm_start_state=last_result,
            )
            matrix[(tws, twa)] = res
            speed_table[i, j] = res.v_boat_kts
            speeds_for_tws.append(res.v_boat_kts)
            last_result = res

        up_target, down_target = compute_vmg_targets(tws, twa_list, speeds_for_tws)
        upwind_targets[tws] = up_target
        downwind_targets[tws] = down_target

    return PolarTable(
        boat_name=solver.boat.name,
        tws_list=tws_list,
        twa_list=twa_list,
        matrix=matrix,
        speed_table=speed_table,
        upwind_targets=upwind_targets,
        downwind_targets=downwind_targets,
    )
