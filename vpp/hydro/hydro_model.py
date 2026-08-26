"""Combined hydrodynamic forces, total resistance, and equilibrium response."""

from dataclasses import dataclass
import numpy as np
from vpp.core.boat import Boat
from vpp.core.environment import Environment
from vpp.hydro.friction import compute_frictional_resistance, FrictionalResistance
from vpp.hydro.residuary import compute_residuary_resistance, ResiduaryResistance
from vpp.hydro.induced import compute_induced_resistance, InducedHydrodynamics
from vpp.hydro.heel import compute_heel_resistance, HeelResistance
from vpp.hydro.stability import compute_righting_moments, RightingMoments


@dataclass
class HydroDynamics:
    """Complete hydrodynamic evaluation at a given speed, heel, and lateral load."""
    r_total: float                  # Total hydrodynamic resistance [N]
    r_viscous: float                # Viscous frictional resistance [N]
    r_residuary: float              # Residuary wave-making resistance [N]
    r_induced: float                # Appendage induced drag [N]
    r_heel: float                   # Heel added resistance [N]
    leeway_rad: float               # Equilibrium leeway angle [radians]
    righting_moment: float          # Total righting moment [N*m]
    friction_details: FrictionalResistance
    residuary_details: ResiduaryResistance
    induced_details: InducedHydrodynamics
    heel_details: HeelResistance
    stability_details: RightingMoments

    @property
    def leeway_deg(self) -> float:
        """Leeway angle in degrees."""
        return np.rad2deg(self.leeway_rad)


def compute_hydrodynamics(
    boat: Boat,
    v_boat_ms: float,
    heel_rad: float,
    side_force_n: float,
    env: Environment = Environment(),
) -> HydroDynamics:
    """Compute all hydrodynamic resistance components and stability response.
    
    Args:
        boat: Boat instance.
        v_boat_ms: Boat forward speed [m/s].
        heel_rad: Heel angle [radians].
        side_force_n: Lateral side force to be countered by appendages [N].
        env: Environment instance.
        
    Returns:
        HydroDynamics dataclass.
    """
    v_safe = max(v_boat_ms, 0.01)

    # 1. Viscous resistance
    fric = compute_frictional_resistance(boat, v_safe, env)

    # 2. Residuary wave resistance
    res = compute_residuary_resistance(boat, v_safe, env)

    # 3. Induced resistance & leeway
    ind = compute_induced_resistance(boat, v_safe, side_force_n, env)

    # 4. Added resistance due to heel
    heel_res = compute_heel_resistance(boat, v_safe, heel_rad, env)

    # 5. Total resistance
    r_total = fric.r_v_total + res.r_r + ind.r_i + heel_res.delta_r_heel

    # 6. Righting moments
    stab = compute_righting_moments(boat, heel_rad)

    return HydroDynamics(
        r_total=r_total,
        r_viscous=fric.r_v_total,
        r_residuary=res.r_r,
        r_induced=ind.r_i,
        r_heel=heel_res.delta_r_heel,
        leeway_rad=ind.leeway_rad,
        righting_moment=stab.rm_total,
        friction_details=fric,
        residuary_details=res,
        induced_details=ind,
        heel_details=heel_res,
        stability_details=stab,
    )
