"""Wind vector transformations between True Wind and Apparent Wind."""

from dataclasses import dataclass
import numpy as np
from vpp.core.environment import Environment


@dataclass
class ApparentWind:
    """Apparent wind parameters at the sail center of effort."""
    aws: float          # Apparent Wind Speed [m/s]
    awa_rad: float      # Apparent Wind Angle [radians]
    aws_eff: float      # Effective AWS accounting for heel [m/s]
    awa_eff_rad: float  # Effective AWA accounting for heel [radians]

    @property
    def awa_deg(self) -> float:
        """Apparent Wind Angle in degrees."""
        return np.rad2deg(self.awa_rad)

    @property
    def awa_eff_deg(self) -> float:
        """Effective Apparent Wind Angle in degrees."""
        return np.rad2deg(self.awa_eff_rad)


def compute_apparent_wind(
    tws_10m: float,
    twa_rad: float,
    v_boat: float,
    leeway_rad: float = 0.0,
    heel_rad: float = 0.0,
    z_ce: float = 6.0,
    env: Environment = Environment(),
) -> ApparentWind:
    """Compute apparent wind vector from true wind and boat motion.
    
    Args:
        tws_10m: True wind speed at 10m reference height [m/s].
        twa_rad: True wind angle [radians] (0 = head to wind, pi = dead downwind).
        v_boat: Boat forward speed through water [m/s].
        leeway_rad: Leeway drift angle [radians].
        heel_rad: Heel angle [radians].
        z_ce: Center of effort height above waterline [m].
        env: Environment instance with air density and wind shear.
        
    Returns:
        ApparentWind instance with speed and angle.
    """
    # Wind speed at center of effort height
    vt_ce = env.wind_speed_at_height(tws_10m, z_ce)

    # In boat horizontal plane coordinate system (x forward, y to leeward/starboard):
    # True wind vector relative to boat course (taking leeway into account)
    twa_course = twa_rad + leeway_rad
    
    # Apparent wind components (wind blowing *from* direction)
    # Forward component (opposing boat motion)
    v_ax = vt_ce * np.cos(twa_course) + v_boat * np.cos(leeway_rad)
    # Cross component (side wind)
    v_ay = vt_ce * np.sin(twa_course) - v_boat * np.sin(leeway_rad)

    aws = np.hypot(v_ax, v_ay)
    awa_rad = np.arctan2(v_ay, v_ax)

    # Correction for heel angle phi:
    # Lift and drag are generated in the plane inclined by the heel angle.
    # The effective wind component normal to mast is reduced by cos(phi).
    cos_phi = np.cos(heel_rad)
    sin_awa = np.sin(awa_rad)
    cos_awa = np.cos(awa_rad)

    aws_eff = aws * np.sqrt((cos_awa ** 2) + ((sin_awa * cos_phi) ** 2))
    awa_eff_rad = np.arctan2(sin_awa * cos_phi, cos_awa)

    return ApparentWind(
        aws=aws,
        awa_rad=awa_rad,
        aws_eff=aws_eff,
        awa_eff_rad=awa_eff_rad,
    )
