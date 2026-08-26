"""Frictional viscous resistance modeling using ITTC-1957 correlation line."""

from dataclasses import dataclass
import numpy as np
from vpp.core.boat import Boat
from vpp.core.environment import Environment


@dataclass
class FrictionalResistance:
    """Frictional resistance components."""
    r_v_total: float      # Total viscous resistance [N]
    r_v_hull: float       # Hull viscous resistance [N]
    r_v_appendages: float # Appendages viscous resistance [N]
    cf_hull: float        # ITTC-57 skin friction coefficient for hull
    cf_app: float         # ITTC-57 skin friction coefficient for appendages
    re_hull: float        # Hull Reynolds number
    re_app: float         # Appendages Reynolds number


def ittc_57_cf(reynolds: float) -> float:
    """Calculate skin friction coefficient according to ITTC-1957 correlation line.
    
    Cf = 0.075 / (log10(Re) - 2)^2
    """
    re_safe = max(reynolds, 1e4)
    log_re = np.log10(re_safe)
    denom = max(log_re - 2.0, 0.5)
    return 0.075 / (denom ** 2)


def compute_frictional_resistance(
    boat: Boat,
    v_boat_ms: float,
    env: Environment = Environment(),
) -> FrictionalResistance:
    """Compute 3D viscous resistance of hull and appendages.
    
    Args:
        boat: Boat instance.
        v_boat_ms: Boat forward speed [m/s].
        env: Environment instance.
        
    Returns:
        FrictionalResistance dataclass.
    """
    v_safe = max(v_boat_ms, 0.01)
    
    # 1. Hull friction
    # Effective characteristic length ~ 0.70 * LWL
    l_eff_hull = 0.70 * boat.hull.lwl
    re_hull = (v_safe * l_eff_hull) / env.nu_water
    cf_hull = ittc_57_cf(re_hull)
    
    # Viscous form factor (1 + k)
    form_factor = 1.0 + boat.hull.form_factor_k
    s_w_hull = boat.hull.wetted_surface or (2.65 * np.sqrt(boat.hull.displacement_volume * boat.hull.lwl))
    q_water = 0.5 * env.rho_water * (v_safe ** 2)
    r_v_hull = form_factor * cf_hull * q_water * s_w_hull

    # 2. Appendages friction
    # Effective mean chord length for keel/rudder
    chord_keel = np.sqrt(boat.appendages.keel_area)
    re_app = (v_safe * chord_keel) / env.nu_water
    cf_app = ittc_57_cf(re_app)
    # Form factor for streamlined hydrofoils ~ 1.06
    s_w_app = boat.appendages.wetted_surface or (2.05 * boat.appendages.total_lateral_area)
    r_v_app = 1.06 * cf_app * q_water * s_w_app

    r_v_total = r_v_hull + r_v_app

    return FrictionalResistance(
        r_v_total=r_v_total,
        r_v_hull=r_v_hull,
        r_v_appendages=r_v_app,
        cf_hull=cf_hull,
        cf_app=cf_app,
        re_hull=re_hull,
        re_app=re_app,
    )
