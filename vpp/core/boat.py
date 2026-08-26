"""Boat geometric, hydrostatic, aerodynamic, and physical properties."""

from dataclasses import dataclass, field
from typing import Callable, Optional
import numpy as np
from vpp.core.units import G


@dataclass
class Hull:
    """Hull geometric and hydrostatic characteristics."""
    loa: float              # Length overall [m]
    lwl: float              # Length waterline [m]
    b_max: float            # Maximum beam [m]
    b_wl: float             # Waterline beam [m]
    draft_canoe: float      # Canoe body draft (excluding appendages) [m]
    draft_total: float      # Total draft with keel [m]
    displacement_mass: float  # Displacement mass [kg]
    wetted_surface: Optional[float] = None  # Wetted surface area [m^2]
    prismatic_coef: float = 0.56            # Prismatic coefficient Cp
    form_factor_k: float = 0.12             # 3D viscous form factor (1+k) = 1.12
    lcb_fraction: float = 0.52              # LCB position from Fwd perpendicular (0 to 1)

    def __post_init__(self):
        if self.wetted_surface is None:
            # Standard empirical approximation for sailing yacht wetted surface
            # S_w ~ c * sqrt(Vol * LWL), where c ~ 2.65 for typical monohulls
            vol = self.displacement_mass / 1025.0
            self.wetted_surface = 2.65 * np.sqrt(vol * self.lwl)

    @property
    def displacement_volume(self) -> float:
        """Displacement volume [m^3] in standard seawater (1025 kg/m^3)."""
        return self.displacement_mass / 1025.0

    @property
    def length_displacement_ratio(self) -> float:
        """Length-displacement ratio LWL / Vol^(1/3)."""
        return self.lwl / (self.displacement_volume ** (1.0 / 3.0))

    @property
    def beam_draft_ratio(self) -> float:
        """Waterline beam to canoe body draft ratio B_wl / T_c."""
        return self.b_wl / max(self.draft_canoe, 0.1)


@dataclass
class Appendages:
    """Keel and rudder characteristics for hydrodynamic lateral force and drag."""
    keel_type: str = "fin"          # "fin", "bulb", "long_keel", "full_keel"
    keel_area: float = 1.6          # Lateral profile area of keel [m^2]
    keel_span: float = 1.1          # Keel span [m]
    rudder_area: float = 0.8        # Lateral profile area of rudder [m^2]
    rudder_span: float = 1.0        # Rudder span [m]
    effective_draft: Optional[float] = None  # Effective hydrodynamic draft T_eff [m]
    wetted_surface: Optional[float] = None    # Total appendages wetted surface [m^2]

    def __post_init__(self):
        if self.wetted_surface is None:
            # Wetted surface of two-sided thin foil is approximately 2 * area * 1.05
            self.wetted_surface = 2.05 * (self.keel_area + self.rudder_area)
        if self.effective_draft is None:
            self.effective_draft = self.keel_span + 0.35

    @property
    def total_lateral_area(self) -> float:
        """Total appendages lateral projected area [m^2]."""
        return self.keel_area + self.rudder_area

    @property
    def effective_aspect_ratio(self) -> float:
        """Effective hydrodynamic aspect ratio including hull mirror plane."""
        # 2 * span^2 / area
        return 2.0 * (self.effective_draft ** 2) / max(self.total_lateral_area, 0.1)


@dataclass
class Rig:
    """Rig geometry and dimensions."""
    rig_type: str = "sloop"         # "sloop", "ketch", "cutter"
    main_p: float = 12.0            # Mainsail luff P [m]
    main_e: float = 4.2             # Mainsail foot E [m]
    fore_i: float = 13.0            # Foretriangle height I [m]
    fore_j: float = 4.4             # Foretriangle base J [m]
    mast_height_above_water: float = 15.0  # Main masthead height above DWL [m]
    boom_height_above_water: float = 1.8   # Boom height above DWL [m]
    # Ketch specific parameters
    mizzen_p: Optional[float] = None       # Mizzen luff P [m]
    mizzen_e: Optional[float] = None       # Mizzen foot E [m]
    mizzen_mast_height: Optional[float] = None  # Mizzen masthead height [m]
    mizzen_boom_height: float = 1.6        # Mizzen boom height [m]


@dataclass
class Stability:
    """Hydrostatic stability, righting moment curve, and crew hiking parameters."""
    gmt: float = 1.10               # Transverse metacentric height GM_T [m]
    crew_mass: float = 350.0        # Total crew mass [kg] (e.g. 4-5 crew or cruising couple)
    crew_hiking_distance: float = 1.5  # Transverse distance of hiking crew from centerline [m]
    crew_hiking_fraction: float = 0.8  # Fraction of crew active on windward rail
    custom_gz_fn: Optional[Callable[[float], float]] = None  # Custom GZ(phi) function [rad -> m]

    def righting_arm_gz(self, phi_rad: float) -> float:
        """Calculate hydrostatic righting arm GZ [m] as a function of heel angle in radians."""
        if self.custom_gz_fn is not None:
            return self.custom_gz_fn(phi_rad)
        
        # Standard naval architecture approximation for sailing yacht GZ curve:
        # GZ(phi) = GM_T * sin(phi) + 0.5 * BM_T * tan^2(phi) * sin(phi) for small/medium angles,
        # with deck immersion & keel CG roll behavior.
        # Clean formulation ensuring smooth, realistic GZ curve up to large heel:
        sin_phi = np.sin(phi_rad)
        cos_phi = np.cos(phi_rad)
        # S-curve modifier representing deck edge immersion and keel bulb righting leverage
        deck_edge_angle = np.deg2rad(30.0)
        phi_ratio = np.abs(phi_rad) / deck_edge_angle
        # Form stability term
        gz = self.gmt * sin_phi * (1.0 + 0.35 * (np.sin(phi_rad) ** 2))
        return gz

    def righting_moment_hull(self, displacement_mass: float, phi_rad: float) -> float:
        """Hydrostatic righting moment of the hull and keel [N*m]."""
        gz = self.righting_arm_gz(phi_rad)
        return displacement_mass * G * gz

    def righting_moment_crew(self, phi_rad: float) -> float:
        """Righting moment contributed by crew hiking on the windward rail [N*m]."""
        effective_crew_mass = self.crew_mass * self.crew_hiking_fraction
        return effective_crew_mass * G * self.crew_hiking_distance * np.cos(phi_rad)

    def total_righting_moment(self, displacement_mass: float, phi_rad: float) -> float:
        """Total righting moment (Hull + Crew) [N*m]."""
        return self.righting_moment_hull(displacement_mass, phi_rad) + self.righting_moment_crew(phi_rad)


@dataclass
class Boat:
    """Complete sailboat model."""
    name: str
    hull: Hull
    appendages: Appendages
    rig: Rig
    stability: Stability

    @property
    def total_wetted_surface(self) -> float:
        """Total wetted surface area of hull and appendages [m^2]."""
        return (self.hull.wetted_surface or 0.0) + (self.appendages.wetted_surface or 0.0)
