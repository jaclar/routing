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
    # Low speed (Fn < 0.18): negligible viscous wavelets
    # Medium speed (0.18 <= Fn < 0.40): rapid wave accumulation up to hull speed (Fn ~ 0.40)
    # High speed (Fn >= 0.40): steep displacement barrier for heavy cruisers vs dynamic lift for light skiffs
    if fn < 0.18:
        cr_base = 0.0003 * (fn / 0.18) ** 3.5
    elif fn < 0.30:
        # Pre-hump region
        t = (fn - 0.18) / 0.12
        cr_base = 0.0003 + 0.0080 * (t ** 2.8)
    elif fn < 0.40:
        # Main displacement resistance hump (hull speed barrier at Fn ~ 0.40)
        t = (fn - 0.30) / 0.10
        cr_base = 0.0083 + 0.0750 * (t ** 2.2)
    else:
        # Post-hump / semi-planing vs displacement regime:
        # Heavily governed by Length-Volume slenderness ratio (LVR) and displacement
        t = fn - 0.40
        # Planing ability index: 0.0 for heavy displacement (LVR <= 5.8), 1.0 for ultralight sportboats (LVR >= 7.0)
        planing_ability = max(0.0, min(1.0, (lvr - 5.8) / 1.4))
        # Non-planing heavy displacement hull: severe stern squat, wave train & energy dissipation barrier
        cr_displacement = 0.0833 + 1.25 * (t ** 1.3) + 12.0 * (t ** 2.5)
        # Planing hull: climbs out of displacement wave trough
        cr_planing = 0.0833 + 0.090 * t
        cr_base = cr_displacement * (1.0 - planing_ability) + cr_planing * planing_ability

    # Influence coefficients from Delft regression:
    # 1. Slenderness / Length-Volume ratio effect: heavier boats (lower LVR) have higher wave drag
    f_lvr = (5.8 / max(lvr, 3.5)) ** 2.2

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
