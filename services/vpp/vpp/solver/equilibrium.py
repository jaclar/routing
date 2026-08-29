"""Nonlinear 3-DOF equilibrium solver for sailboat steady-state motion."""

from dataclasses import dataclass
from typing import Optional, Tuple
import numpy as np
from scipy.optimize import least_squares

from vpp.core.boat import Boat
from vpp.core.environment import Environment
from vpp.aero.sail import SailSet
from vpp.aero.aero_model import compute_aero_forces, AeroForces
from vpp.hydro.hydro_model import compute_hydrodynamics, HydroDynamics


@dataclass
class EquilibriumState:
    """Equilibrium solution at a specific wind condition and sail trim."""
    v_boat_ms: float
    heel_rad: float
    leeway_rad: float
    aero: AeroForces
    hydro: HydroDynamics
    converged: bool
    residual_norm: float

    @property
    def v_boat_kts(self) -> float:
        """Boat speed in knots."""
        return self.v_boat_ms * (3600.0 / 1852.0)

    @property
    def heel_deg(self) -> float:
        """Heel angle in degrees."""
        return np.rad2deg(self.heel_rad)

    @property
    def leeway_deg(self) -> float:
        """Leeway angle in degrees."""
        return np.rad2deg(self.leeway_rad)


def compute_equilibrium_residuals(
    x: np.ndarray,
    boat: Boat,
    sail_set: SailSet,
    tws_ms: float,
    twa_rad: float,
    flat: float = 1.0,
    reef: float = 1.0,
    env: Environment = Environment(),
) -> Tuple[np.ndarray, AeroForces, HydroDynamics]:
    """Calculate the 3-DOF force and moment balance residuals.
    
    State vector x = [v_boat_ms, heel_rad, leeway_rad]
    
    Residuals:
        R1 = (F_x_aero - R_total) / R_ref
        R2 = (F_y_aero - F_y_hydro) / R_ref
        R3 = (M_x_aero - RM_total) / M_ref
    """
    v_boat = max(x[0], 0.05)
    heel = float(np.clip(x[1], 0.0, np.deg2rad(65.0)))
    leeway = float(np.clip(x[2], -np.deg2rad(20.0), np.deg2rad(20.0)))

    # Compute Aerodynamic forces
    aero = compute_aero_forces(
        boat=boat,
        sail_set=sail_set,
        tws_ms=tws_ms,
        twa_rad=twa_rad,
        v_boat_ms=v_boat,
        heel_rad=heel,
        leeway_rad=leeway,
        flat=flat,
        reef=reef,
        env=env,
    )

    # Compute Hydrodynamic forces & moments
    hydro = compute_hydrodynamics(
        boat=boat,
        v_boat_ms=v_boat,
        heel_rad=heel,
        side_force_n=aero.f_y,
        env=env,
    )

    # Scaling reference for well-conditioned optimization
    # Normal force scale ~ weight * 0.05, Moment scale ~ weight * GZ_ref
    ref_force = max(boat.hull.displacement_mass * 9.80665 * 0.02, 100.0)
    ref_moment = ref_force * max(boat.hull.b_max * 0.5, 1.0)

    # 1. Surge equation: Fx_aero - R_total = 0
    res_surge = (aero.f_x - hydro.r_total) / ref_force

    # 2. Sway equation: leeway angle matches required side force
    res_sway = (leeway - hydro.leeway_rad) / np.deg2rad(2.0)

    # 3. Roll equation: Heeling moment - Righting moment = 0
    res_roll = (aero.m_x - hydro.righting_moment) / ref_moment

    residuals = np.array([res_surge, res_sway, res_roll])
    return residuals, aero, hydro


def solve_equilibrium(
    boat: Boat,
    sail_set: SailSet,
    tws_ms: float,
    twa_rad: float,
    flat: float = 1.0,
    reef: float = 1.0,
    x0: Optional[Tuple[float, float, float]] = None,
    env: Environment = Environment(),
) -> EquilibriumState:
    """Solve for steady sailing equilibrium (V_boat, Heel, Leeway).
    
    Args:
        boat: Boat instance.
        sail_set: SailSet instance.
        tws_ms: True wind speed [m/s].
        twa_rad: True wind angle [radians].
        flat: Flattening factor (0.4 to 1.0).
        reef: Reefing factor (0.5 to 1.0).
        x0: Optional initial guess (v_boat_ms, heel_rad, leeway_rad).
        env: Environment instance.
        
    Returns:
        EquilibriumState dataclass with solved kinematics and forces.
    """
    if x0 is None:
        # Smart initial guess based on wind speed and angle
        # Typical cruising speed ~ 50-70% of TWS up to hull speed
        v_guess = min(max(tws_ms * 0.45, 1.5), 1.25 * np.sqrt(9.81 * boat.hull.lwl))
        # Heel guess ~ 10-20 deg on beat/reach, lower downwind
        twa_deg = np.rad2deg(twa_rad)
        if twa_deg < 60.0:
            heel_guess = np.deg2rad(min(tws_ms * 1.8, 22.0))
        elif twa_deg < 120.0:
            heel_guess = np.deg2rad(min(tws_ms * 2.2, 25.0))
        else:
            heel_guess = np.deg2rad(min(tws_ms * 0.8, 10.0))
        leeway_guess = np.deg2rad(3.0 if twa_deg < 90 else 1.0)
        x_init = np.array([v_guess, heel_guess, leeway_guess])
    else:
        x_init = np.array(x0)

    def residual_func(x_vec: np.ndarray) -> np.ndarray:
        res, _, _ = compute_equilibrium_residuals(
            x=x_vec,
            boat=boat,
            sail_set=sail_set,
            tws_ms=tws_ms,
            twa_rad=twa_rad,
            flat=flat,
            reef=reef,
            env=env,
        )
        return res

    # Bounds: v in [0.05, 20.0] m/s, heel in [0.0, 60 deg], leeway in [-10, 15 deg]
    lower_bounds = [0.05, 0.0, np.deg2rad(-5.0)]
    upper_bounds = [20.0, np.deg2rad(60.0), np.deg2rad(20.0)]

    # Use robust bounded least squares
    res = least_squares(
        residual_func,
        x0=x_init,
        bounds=(lower_bounds, upper_bounds),
        ftol=1e-6,
        xtol=1e-6,
        gtol=1e-6,
        max_nfev=200,
    )

    x_sol = res.x
    _, final_aero, final_hydro = compute_equilibrium_residuals(
        x=x_sol,
        boat=boat,
        sail_set=sail_set,
        tws_ms=tws_ms,
        twa_rad=twa_rad,
        flat=flat,
        reef=reef,
        env=env,
    )

    residual_norm = float(np.linalg.norm(res.fun))
    converged = bool(res.success and residual_norm < 0.25)

    return EquilibriumState(
        v_boat_ms=float(x_sol[0]),
        heel_rad=float(x_sol[1]),
        leeway_rad=float(x_sol[2]),
        aero=final_aero,
        hydro=final_hydro,
        converged=converged,
        residual_norm=residual_norm,
    )
