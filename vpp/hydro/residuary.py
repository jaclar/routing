"""Residuary and wave-making resistance based on Delft Systematic Yacht Hull Series (DSYHS)."""

from dataclasses import dataclass
import numpy as np
from vpp.core.boat import Boat
from vpp.core.environment import Environment
from vpp.core.units import G


@dataclass
class ResiduaryResistance:
    """Residuary (wave-making) resistance evaluation."""
    r_r: float          # Residuary resistance [N]
    fn: float           # Froude number Fn = V / sqrt(g * LWL)
    cr: float           # Residuary resistance coefficient R_r / (Delta * g)


def compute_residuary_resistance(
    boat: Boat,
    v_boat_ms: float,
    env: Environment = Environment(),
) -> ResiduaryResistance:
    """Compute residuary / wave-making resistance using DSYHS formulations.
    
    Args:
        boat: Boat instance.
        v_boat_ms: Boat forward speed [m/s].
        env: Environment instance.
        
    Returns:
        ResiduaryResistance dataclass.
    """
    g = env.g
    lwl = boat.hull.lwl
    fn = v_boat_ms / np.sqrt(g * lwl)

    if fn <= 0.05:
        return ResiduaryResistance(r_r=0.0, fn=fn, cr=0.0)

    # Dimensionless hull parameters
    # Length-Displacement ratio: L_wl / Vol^(1/3) (typically 4.5 - 7.5)
    lvr = boat.hull.length_displacement_ratio
    # Beam-to-canoe-draft ratio: B_wl / T_c (typically 3.0 - 6.0)
    btr = boat.hull.beam_draft_ratio
    # Prismatic coefficient Cp (typically 0.52 - 0.60)
    cp = boat.hull.prismatic_coef

    # Delft Series polynomial representation of Cr = Rr / (Delta * g)
    # The curve features an exponential rise near "hull speed" (Fn ~ 0.35 - 0.45)
    # and a plateau for light displacement semi-planing hulls at Fn > 0.50.
    
    # Base wave drag exponent function of Froude number
    # Low speed (Fn < 0.25): negligible
    # Medium speed (0.25 <= Fn < 0.45): rapid wave accumulation
    # High speed (Fn >= 0.45): transition / semi-planing
    if fn < 0.20:
        cr_base = 0.0004 * (fn / 0.20) ** 4.0
    elif fn < 0.35:
        # Pre-hump region
        t = (fn - 0.20) / 0.15
        cr_base = 0.0004 + 0.0120 * (t ** 3.2)
    elif fn < 0.45:
        # Main displacement resistance hump (hull speed barrier)
        t = (fn - 0.35) / 0.10
        cr_base = 0.0124 + 0.0520 * (t ** 2.2)
    else:
        # Post-hump / semi-planing regime
        t = fn - 0.45
        cr_base = 0.0644 + 0.075 * t

    # Influence coefficients from Delft regression:
    # 1. Slenderness / Length-Volume ratio effect: heavier boats (lower LVR) have higher wave drag
    f_lvr = (5.8 / max(lvr, 3.5)) ** 1.85

    # 2. Beam-Draft ratio effect: wider/shallower hulls generate slightly wider wave patterns
    f_btr = (btr / 4.0) ** 0.35

    # 3. Prismatic coefficient effect: higher Cp fills out the ends, reducing wave resistance at higher Fn
    if fn > 0.35:
        f_cp = 1.0 - 1.2 * (cp - 0.56)
    else:
        f_cp = 1.0 + 0.8 * (cp - 0.56)

    cr = cr_base * f_lvr * f_btr * f_cp
    cr = max(cr, 0.0)

    # Total residuary resistance in Newtons: Rr = Cr * (Mass * g)
    displacement_weight_n = boat.hull.displacement_mass * g
    r_r = cr * displacement_weight_n

    return ResiduaryResistance(
        r_r=r_r,
        fn=fn,
        cr=cr,
    )
