"""Heel-induced resistance formulation based on Delft Series regressions."""

from dataclasses import dataclass
import numpy as np
from vpp.core.boat import Boat
from vpp.core.environment import Environment


@dataclass
class HeelResistance:
    """Heel resistance increment."""
    delta_r_heel: float  # Added resistance due to heel [N]
    heel_rad: float      # Heel angle [radians]
    c_heel: float        # Dimensionless heel resistance coefficient


def compute_heel_resistance(
    boat: Boat,
    v_boat_ms: float,
    heel_rad: float,
    env: Environment = Environment(),
) -> HeelResistance:
    """Compute added resistance due to heel angle based on Delft Series.
    
    Args:
        boat: Boat instance.
        v_boat_ms: Boat forward speed [m/s].
        heel_rad: Heel angle [radians].
        env: Environment instance.
        
    Returns:
        HeelResistance dataclass.
    """
    if abs(heel_rad) < 1e-4 or v_boat_ms < 0.05:
        return HeelResistance(delta_r_heel=0.0, heel_rad=heel_rad, c_heel=0.0)

    g = env.g
    fn = v_boat_ms / np.sqrt(g * boat.hull.lwl)
    
    # Geometric influence terms
    btr = boat.hull.beam_draft_ratio
    lvr = boat.hull.length_displacement_ratio
    s_w = boat.total_wetted_surface

    # DSYHS formulation for heel resistance coefficient
    # Increases with sin^2(phi) and Froude number
    sin_sq_phi = np.sin(heel_rad) ** 2
    c_heel = 6.74e-4 * sin_sq_phi * (fn + 0.15) * np.sqrt(btr / 4.0) * np.sqrt(5.8 / max(lvr, 3.5))

    q_water = 0.5 * env.rho_water * (v_boat_ms ** 2)
    delta_r_heel = c_heel * q_water * s_w

    return HeelResistance(
        delta_r_heel=delta_r_heel,
        heel_rad=heel_rad,
        c_heel=c_heel,
    )
