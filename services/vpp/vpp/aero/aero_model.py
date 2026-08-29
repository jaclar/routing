"""Aerodynamic force and moment evaluation for sailboat rigs and sails."""

from dataclasses import dataclass
import numpy as np
from vpp.core.boat import Boat
from vpp.core.environment import Environment
from vpp.aero.wind import compute_apparent_wind, ApparentWind
from vpp.aero.sail import SailSet
from vpp.aero.coefficients import (
    compute_sail_coefficients,
    compute_parasitic_windage_drag_area,
)


@dataclass
class AeroForces:
    """Aerodynamic forces and moments in boat coordinates."""
    f_x: float           # Forward drive force [N] (positive forward)
    f_y: float           # Lateral side force [N] (positive to leeward)
    m_x: float           # Heeling moment [N*m] (positive causing heel)
    total_lift: float    # Total aerodynamic lift [N]
    total_drag: float    # Total aerodynamic drag [N]
    z_ce: float          # Combined center of effort height [m]
    apparent_wind: ApparentWind  # Apparent wind object


def compute_aero_forces(
    boat: Boat,
    sail_set: SailSet,
    tws_ms: float,
    twa_rad: float,
    v_boat_ms: float,
    heel_rad: float = 0.0,
    leeway_rad: float = 0.0,
    flat: float = 1.0,
    reef: float = 1.0,
    env: Environment = Environment(),
) -> AeroForces:
    """Evaluate total aerodynamic forces and moments acting on the sailboat.
    
    Args:
        boat: Boat instance.
        sail_set: SailSet instance with active sails.
        tws_ms: True wind speed at 10m [m/s].
        twa_rad: True wind angle [radians].
        v_boat_ms: Boat forward speed [m/s].
        heel_rad: Heel angle [radians].
        leeway_rad: Leeway drift angle [radians].
        flat: Camber flattening factor (0.4 to 1.0).
        reef: Reefing factor (0.5 to 1.0).
        env: Environment instance.
        
    Returns:
        AeroForces dataclass.
    """
    # 1. Combined Center of Effort height
    z_ce = sail_set.combined_z_ce(reef)

    # 2. Apparent wind at the center of effort
    app_wind = compute_apparent_wind(
        tws_10m=tws_ms,
        twa_rad=twa_rad,
        v_boat=v_boat_ms,
        leeway_rad=leeway_rad,
        heel_rad=heel_rad,
        z_ce=z_ce,
        env=env,
    )

    # 3. Dynamic pressure based on effective AWS
    q_eff = 0.5 * env.rho_air * (app_wind.aws_eff ** 2)

    # 4. Effective rig aspect ratio accounting for end-plate deck effect
    total_eff_area = sail_set.effective_total_area(reef)
    mast_h = boat.rig.mast_height_above_water
    eff_aspect_ratio = ((1.10 * mast_h) ** 2) / max(total_eff_area, 1.0)

    # 5. Calculate Lift and Drag for each sail in the set
    sum_lift = 0.0
    sum_drag = 0.0

    for sail in sail_set.sails:
        sail_area_eff = sail.effective_area(reef)
        coeffs = compute_sail_coefficients(
            sail=sail,
            awa_rad=app_wind.awa_eff_rad,
            flat=flat,
            effective_aspect_ratio=eff_aspect_ratio,
        )
        sum_lift += q_eff * sail_area_eff * coeffs.cl
        sum_drag += q_eff * sail_area_eff * coeffs.cd

    # 6. Parasitic windage drag of hull, deck, and rigging
    q_raw = 0.5 * env.rho_air * (app_wind.aws ** 2)
    windage_cd_area = compute_parasitic_windage_drag_area(
        loa=boat.hull.loa,
        b_max=boat.hull.b_max,
        mast_height=boat.rig.mast_height_above_water,
    )
    windage_drag = q_raw * windage_cd_area
    total_drag = sum_drag + windage_drag

    # 7. Convert Lift and Drag into Drive (Fx) and Side (Fy) forces
    # In apparent wind coordinate frame:
    # Lift is perpendicular to effective apparent wind
    # Drag is parallel to effective apparent wind
    sin_awa = np.sin(app_wind.awa_eff_rad)
    cos_awa = np.cos(app_wind.awa_eff_rad)

    # Forward drive force along centerline
    f_x = sum_lift * sin_awa - total_drag * cos_awa

    # Lateral force (to leeward)
    # Roll inclination cos(phi) reduces horizontal projected force
    f_y = (sum_lift * cos_awa + total_drag * sin_awa) * np.cos(heel_rad)

    # 8. Heeling moment
    # Center of lateral resistance (CLR) depth below waterline
    z_clr = 0.45 * boat.hull.draft_total
    # Moment arm: (Z_CE * cos(phi) + Z_CLR)
    heeling_arm = z_ce * np.cos(heel_rad) + z_clr
    m_x = f_y * heeling_arm

    return AeroForces(
        f_x=f_x,
        f_y=f_y,
        m_x=m_x,
        total_lift=sum_lift,
        total_drag=total_drag,
        z_ce=z_ce,
        apparent_wind=app_wind,
    )
