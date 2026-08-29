"""Aerodynamic lift, drag, and windage coefficient models."""

from dataclasses import dataclass
import numpy as np
from vpp.aero.sail import Sail


@dataclass
class AeroCoefficients:
    """Calculated lift and drag coefficients for a sail or sail set."""
    cl: float        # Total lift coefficient (perpendicular to apparent wind)
    cd: float        # Total drag coefficient (parallel to apparent wind)
    cd_induced: float  # Induced drag component
    cd_profile: float  # Profile drag component


def compute_sail_coefficients(
    sail: Sail,
    awa_rad: float,
    flat: float = 1.0,
    effective_aspect_ratio: float = 4.5,
) -> AeroCoefficients:
    """Compute lift and drag coefficients for a sail based on Hazen/ORC formulations.
    
    Args:
        sail: Sail instance.
        awa_rad: Effective apparent wind angle [radians].
        flat: Flattening factor (0.4 <= flat <= 1.0) to reduce lift/camber.
        effective_aspect_ratio: Rig effective aspect ratio.
        
    Returns:
        AeroCoefficients object with cl, cd, cd_induced, cd_profile.
    """
    awa_deg = float(np.clip(np.abs(np.rad2deg(awa_rad)), 0.0, 180.0))
    cl_max = sail.cl_max * flat
    cd_0 = sail.cd_0

    if sail.is_downwind:
        # Spinnaker / Asymmetric formulation
        if awa_deg < 35.0:
            # Cannot fly spinnaker too tight to the wind
            cl = 0.0
            cd_prof = cd_0
        elif awa_deg < 80.0:
            # Transition to full shape
            fraction = (awa_deg - 35.0) / 45.0
            cl = cl_max * fraction
            cd_prof = cd_0 + 0.15 * fraction
        elif awa_deg <= 130.0:
            # Maximum reaching lift
            cl = cl_max
            cd_prof = cd_0 + 0.20 + 0.20 * ((awa_deg - 80.0) / 50.0)
        else:
            # Transition from lift to pure drag at dead downwind (180 deg)
            fraction = (180.0 - awa_deg) / 50.0
            cl = cl_max * max(fraction, 0.0)
            # Drag peaks near 180 deg
            cd_prof = 0.40 + 0.85 * ((awa_deg - 130.0) / 50.0)
    else:
        # Upwind sails (Mainsail, Jib, Mizzen)
        if awa_deg < 20.0:
            # Pre-stall / luffing range
            cl = cl_max * (awa_deg / 20.0)
            cd_prof = cd_0 * 1.5
        elif awa_deg <= 50.0:
            # Optimal upwind lift range
            cl = cl_max
            cd_prof = cd_0
        elif awa_deg <= 90.0:
            # Reaching: flow remains mostly attached, gradual reduction
            cl = cl_max * np.cos(np.deg2rad(awa_deg - 50.0) * 0.75)
            cd_prof = cd_0 + 0.08 * ((awa_deg - 50.0) / 40.0)
        elif awa_deg <= 180.0:
            # Broad reaching to running: transitioning to bluff body drag
            fraction = (180.0 - awa_deg) / 90.0
            cl = cl_max * 0.70 * np.sin(np.deg2rad(180.0 - awa_deg))
            cd_prof = cd_0 + 0.08 + 0.65 * ((awa_deg - 90.0) / 90.0)
        else:
            cl = 0.0
            cd_prof = cd_0

    # Induced drag: Cd_i = Cl^2 / (pi * AR_eff)
    # Induced drag efficiency factor e ~ 0.85 - 0.90
    ar = max(effective_aspect_ratio, 1.0)
    cd_induced = (cl ** 2) / (np.pi * ar * 0.88)
    cd_total = cd_prof + cd_induced

    return AeroCoefficients(
        cl=cl,
        cd=cd_total,
        cd_induced=cd_induced,
        cd_profile=cd_prof,
    )


def compute_parasitic_windage_drag_area(
    loa: float,
    b_max: float,
    mast_height: float,
    freeboard: float = 1.2,
) -> float:
    """Compute effective parasitic windage area (Cd * Area) for hull, deckhouse, and rig.
    
    Returns:
        Effective parasitic drag area [m^2].
    """
    # Hull front/lateral projected windage area
    a_hull = 0.5 * b_max * freeboard
    cd_hull = 0.6
    
    # Mast and rigging aerodynamic drag area
    a_mast = 0.12 * mast_height
    cd_mast = 1.0

    return (a_hull * cd_hull) + (a_mast * cd_mast)
