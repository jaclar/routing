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
    # True where the solver reached equilibrium. False entries in speed_table were estimated
    # from neighbouring converged points rather than solved; see generate_polar_table.
    converged_mask: Optional[np.ndarray] = None

    @property
    def converged_fraction(self) -> float:
        """Share of grid points the solver actually converged on."""
        if self.converged_mask is None:
            return 1.0
        return float(self.converged_mask.mean())

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
    converged_mask = np.zeros((len(tws_list), len(twa_list)), dtype=bool)
    upwind_targets: Dict[float, VMGTarget] = {}
    downwind_targets: Dict[float, VMGTarget] = {}

    for i, tws in enumerate(tws_list):
        last_result: Optional[VPPPointResult] = None

        for j, twa in enumerate(twa_list):
            res = solver.solve_point(
                tws_kts=tws,
                twa_deg=twa,
                warm_start_state=last_result,
            )
            matrix[(tws, twa)] = res
            speed_table[i, j] = res.v_boat_kts
            converged_mask[i, j] = bool(res.converged)

            # Only warm-start from a state the solver actually trusted. Seeding the next
            # point with a failed iterate propagates the failure along the row.
            if res.converged:
                last_result = res

    speed_table = _estimate_unconverged(speed_table, converged_mask, tws_list, twa_list)

    # VMG targets are derived from the repaired table, so a failed solve cannot nominate
    # itself as the optimal beating or gybing angle.
    for i, tws in enumerate(tws_list):
        up_target, down_target = compute_vmg_targets(tws, twa_list, speed_table[i, :].tolist())
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
        converged_mask=converged_mask,
    )


def _estimate_unconverged(
    speed_table: np.ndarray,
    converged_mask: np.ndarray,
    tws_list: List[float],
    twa_list: List[float],
) -> np.ndarray:
    """Replace points the solver failed on with estimates from the points around them.

    A failed solve returns whatever iterate the optimizer stopped on, which carries no
    physical meaning: for light, heavily-canvassed boats it can exceed hull speed several
    times over. Boat speed varies smoothly with wind angle at a fixed wind speed, so a
    failed point is far better estimated from its converged neighbours than trusted.

    Points inside the no-go zone converge on a boat speed of zero and so anchor the
    interpolation correctly rather than being filled in.
    """
    repaired = speed_table.copy()
    estimated = np.zeros_like(converged_mask)
    twa = np.asarray(twa_list, dtype=float)
    tws = np.asarray(tws_list, dtype=float)

    def fill_between(values, coords, good, bad):
        """Interpolate bad entries that lie between converged anchors.

        Only points inside the converged span are touched. np.interp would otherwise hold
        the endpoint flat beyond the last anchor, which past the deepest converged angle
        would assign reaching speeds to a boat running dead downwind.
        """
        if good.sum() < 2:
            return np.zeros_like(bad)
        lo, hi = coords[good].min(), coords[good].max()
        inside = bad & (coords >= lo) & (coords <= hi)
        if inside.any():
            values[inside] = np.interp(coords[inside], coords[good], values[good])
        return inside

    # Along the wind-angle axis first: that is the smoother direction. Points inside the
    # no-go zone converge on zero, so they are excluded when judging whether a row carries
    # enough real information to interpolate from, while still anchoring its shape.
    for i in range(repaired.shape[0]):
        good = converged_mask[i, :]
        informative = good & (repaired[i, :] > 0.0)
        if informative.sum() >= 2:
            estimated[i, :] |= fill_between(repaired[i, :], twa, good, ~good)

    # Whatever is still unsolved is tried against the same wind angle at other wind speeds.
    for j in range(repaired.shape[1]):
        good = converged_mask[:, j]
        bad = ~converged_mask[:, j] & ~estimated[:, j]
        if bad.any():
            estimated[:, j] |= fill_between(repaired[:, j], tws, good, bad)

    return repaired
