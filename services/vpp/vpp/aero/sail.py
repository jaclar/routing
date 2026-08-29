"""Sail representation, geometry, and inventory management."""

from __future__ import annotations
from dataclasses import dataclass
from enum import Enum
from typing import List, Optional, Tuple
from vpp.core.boat import Rig


class SailType(str, Enum):
    MAIN = "main"
    JIB = "jib"
    GENOA = "genoa"
    MIZZEN = "mizzen"
    SPINNAKER = "spinnaker"
    ASYMMETRIC = "asymmetric"
    CODE_ZERO = "code_zero"


@dataclass
class Sail:
    """Geometric and aerodynamic properties of an individual sail."""
    name: str
    sail_type: SailType
    area: float              # Nominal sail area [m^2]
    z_ce: float              # Center of effort height above DWL [m]
    aspect_ratio: float      # Effective aspect ratio of sail
    cl_max: float = 1.4      # Maximum lift coefficient
    cd_0: float = 0.02       # Profile/parasitic drag coefficient
    is_downwind: bool = False  # True if sail is dedicated for reaching/running (e.g. spinnaker)

    def effective_area(self, reef: float) -> float:
        """Effective sail area with reefing factor (0.5 <= reef <= 1.0)."""
        return self.area * (reef ** 2)

    def effective_z_ce(self, reef: float) -> float:
        """Effective center of effort height with reefing factor."""
        return self.z_ce * (0.3 + 0.7 * reef)


@dataclass
class SailSet:
    """A combined set of sails flown together (e.g. Main + Jib or Main + Mizzen + Spinnaker)."""
    name: str
    sails: List[Sail]
    is_downwind_set: bool = False

    @property
    def total_area(self) -> float:
        """Sum of nominal sail areas [m^2]."""
        return sum(s.area for s in self.sails)

    def effective_total_area(self, reef: float) -> float:
        """Sum of effective sail areas with reefing."""
        return sum(s.effective_area(reef) for s in self.sails)

    def combined_z_ce(self, reef: float) -> float:
        """Area-weighted center of effort height [m]."""
        eff_areas = [s.effective_area(reef) for s in self.sails]
        tot_area = sum(eff_areas)
        if tot_area <= 1e-6:
            return 5.0
        eff_zces = [s.effective_z_ce(reef) for s in self.sails]
        return sum(a * z for a, z in zip(eff_areas, eff_zces)) / tot_area


def create_sails_from_rig(
    rig: Rig,
    spin_area_multiplier: float = 1.7,
    mizzen_spin_area: Optional[float] = None,
) -> Tuple[SailSet, SailSet]:
    """Generate default Upwind and Downwind SailSets from rig dimensions.
    
    Args:
        rig: Rig dataclass with P, E, I, J, mast heights.
        spin_area_multiplier: Factor on I*J for spinnaker area.
        mizzen_spin_area: Optional additional staysail/mizzen downwind sail area.
        
    Returns:
        tuple (upwind_sail_set, downwind_sail_set)
    """
    # Mainsail
    # Roach factor ~ 1.08
    main_area = 0.5 * rig.main_p * rig.main_e * 1.08
    main_ar = (rig.main_p ** 2) / max(main_area, 1.0)
    main_z_ce = rig.boom_height_above_water + (rig.main_p * 0.38)
    mainsail = Sail(
        name="Mainsail",
        sail_type=SailType.MAIN,
        area=main_area,
        z_ce=main_z_ce,
        aspect_ratio=main_ar,
        cl_max=1.45,
        cd_0=0.018,
    )

    # Jib / Headsail (100% - 110% foretriangle)
    jib_area = 0.5 * rig.fore_i * rig.fore_j * 1.04
    jib_ar = (rig.fore_i ** 2) / max(jib_area, 1.0)
    jib_z_ce = 1.0 + (rig.fore_i * 0.36)
    jib = Sail(
        name="Jib",
        sail_type=SailType.JIB,
        area=jib_area,
        z_ce=jib_z_ce,
        aspect_ratio=jib_ar,
        cl_max=1.35,
        cd_0=0.015,
    )

    # Spinnaker / Asymmetric
    spin_area = 0.5 * rig.fore_i * rig.fore_j * spin_area_multiplier
    spin_ar = (rig.fore_i ** 2) / max(spin_area, 1.0)
    spin_z_ce = 1.0 + (rig.fore_i * 0.44)
    spinnaker = Sail(
        name="Spinnaker",
        sail_type=SailType.ASYMMETRIC,
        area=spin_area,
        z_ce=spin_z_ce,
        aspect_ratio=spin_ar,
        cl_max=1.60,
        cd_0=0.035,
        is_downwind=True,
    )

    upwind_sails = [mainsail, jib]
    downwind_sails = [mainsail, spinnaker]

    # Ketch support: if mizzen dimensions are present, add Mizzen sail
    if rig.mizzen_p is not None and rig.mizzen_e is not None and rig.mizzen_p > 0:
        mizzen_area = 0.5 * rig.mizzen_p * rig.mizzen_e * 1.05
        mizzen_ar = (rig.mizzen_p ** 2) / max(mizzen_area, 1.0)
        mizzen_z_ce = rig.mizzen_boom_height + (rig.mizzen_p * 0.38)
        mizzen = Sail(
            name="Mizzen",
            sail_type=SailType.MIZZEN,
            area=mizzen_area,
            z_ce=mizzen_z_ce,
            aspect_ratio=mizzen_ar,
            cl_max=1.30,
            cd_0=0.020,
        )
        upwind_sails.append(mizzen)
        downwind_sails.append(mizzen)

    upwind_set = SailSet(name="Upwind (Main+Jib)", sails=upwind_sails, is_downwind_set=False)
    downwind_set = SailSet(name="Downwind (Main+Spin)", sails=downwind_sails, is_downwind_set=True)

    return upwind_set, downwind_set
