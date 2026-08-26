"""Hydrostatic stability and righting moment calculations."""

from dataclasses import dataclass
import numpy as np
from vpp.core.boat import Boat


@dataclass
class RightingMoments:
    """Hydrostatic and crew righting moments."""
    rm_total: float   # Total righting moment [N*m]
    rm_hull: float    # Hull & keel hydrostatic righting moment [N*m]
    rm_crew: float    # Crew hiking righting moment [N*m]
    gz_arm: float     # Righting arm GZ [m]
    heel_rad: float   # Heel angle [radians]

    @property
    def heel_deg(self) -> float:
        """Heel angle in degrees."""
        return np.rad2deg(self.heel_rad)


def compute_righting_moments(
    boat: Boat,
    heel_rad: float,
) -> RightingMoments:
    """Calculate total righting moment for the boat at a given heel angle.
    
    Args:
        boat: Boat instance.
        heel_rad: Heel angle [radians].
        
    Returns:
        RightingMoments dataclass.
    """
    gz = boat.stability.righting_arm_gz(heel_rad)
    rm_hull = boat.stability.righting_moment_hull(boat.hull.displacement_mass, heel_rad)
    rm_crew = boat.stability.righting_moment_crew(heel_rad)
    rm_total = rm_hull + rm_crew

    return RightingMoments(
        rm_total=rm_total,
        rm_hull=rm_hull,
        rm_crew=rm_crew,
        gz_arm=gz,
        heel_rad=heel_rad,
    )
