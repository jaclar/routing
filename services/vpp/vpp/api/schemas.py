"""Pydantic data models and schemas for FastAPI VPP service."""

from __future__ import annotations
from typing import List, Optional, Dict, Any
from pydantic import BaseModel, Field

from vpp.core.boat import Hull, Appendages, Rig, Stability, Boat
from vpp.presets.boats import (
    create_36ft_ketch,
    create_36ft_sloop,
    create_40ft_performance_cruiser,
    create_24ft_sportboat,
)


class HullSchema(BaseModel):
    loa: float = Field(..., description="Length overall [m]", gt=0)
    lwl: float = Field(..., description="Length waterline [m]", gt=0)
    b_max: float = Field(..., description="Maximum beam [m]", gt=0)
    b_wl: float = Field(..., description="Waterline beam [m]", gt=0)
    draft_canoe: float = Field(..., description="Canoe body draft [m]", gt=0)
    draft_total: float = Field(..., description="Total draft with keel [m]", gt=0)
    displacement_mass: float = Field(..., description="Displacement mass [kg]", gt=0)
    wetted_surface: Optional[float] = Field(None, description="Wetted surface area [m^2]")
    prismatic_coef: float = Field(0.56, description="Prismatic coefficient Cp")
    form_factor_k: float = Field(0.12, description="3D viscous form factor k")
    lcb_fraction: float = Field(0.52, description="LCB position fraction from bow")

    def to_domain(self) -> Hull:
        return Hull(
            loa=self.loa,
            lwl=self.lwl,
            b_max=self.b_max,
            b_wl=self.b_wl,
            draft_canoe=self.draft_canoe,
            draft_total=self.draft_total,
            displacement_mass=self.displacement_mass,
            wetted_surface=self.wetted_surface,
            prismatic_coef=self.prismatic_coef,
            form_factor_k=self.form_factor_k,
            lcb_fraction=self.lcb_fraction,
        )


class AppendagesSchema(BaseModel):
    keel_type: str = Field("fin", description="Type of keel")
    keel_area: float = Field(..., description="Lateral profile area of keel [m^2]", gt=0)
    keel_span: float = Field(..., description="Keel span [m]", gt=0)
    rudder_area: float = Field(..., description="Lateral profile area of rudder [m^2]", gt=0)
    rudder_span: float = Field(..., description="Rudder span [m]", gt=0)
    effective_draft: Optional[float] = Field(None, description="Hydrodynamic effective draft [m]")
    wetted_surface: Optional[float] = Field(None, description="Total appendages wetted surface [m^2]")

    def to_domain(self) -> Appendages:
        return Appendages(
            keel_type=self.keel_type,
            keel_area=self.keel_area,
            keel_span=self.keel_span,
            rudder_area=self.rudder_area,
            rudder_span=self.rudder_span,
            effective_draft=self.effective_draft,
            wetted_surface=self.wetted_surface,
        )


class RigSchema(BaseModel):
    rig_type: str = Field("sloop", description="Rig type: sloop, ketch, cutter")
    main_p: float = Field(..., description="Mainsail luff P [m]", gt=0)
    main_e: float = Field(..., description="Mainsail foot E [m]", gt=0)
    fore_i: float = Field(..., description="Foretriangle height I [m]", gt=0)
    fore_j: float = Field(..., description="Foretriangle base J [m]", gt=0)
    mast_height_above_water: float = Field(..., description="Main masthead height above DWL [m]", gt=0)
    boom_height_above_water: float = Field(1.8, description="Boom height above DWL [m]")
    mizzen_p: Optional[float] = Field(None, description="Mizzen luff P [m] (for ketch)")
    mizzen_e: Optional[float] = Field(None, description="Mizzen foot E [m] (for ketch)")
    mizzen_mast_height: Optional[float] = Field(None, description="Mizzen masthead height [m]")
    mizzen_boom_height: float = Field(1.6, description="Mizzen boom height [m]")

    def to_domain(self) -> Rig:
        return Rig(
            rig_type=self.rig_type,
            main_p=self.main_p,
            main_e=self.main_e,
            fore_i=self.fore_i,
            fore_j=self.fore_j,
            mast_height_above_water=self.mast_height_above_water,
            boom_height_above_water=self.boom_height_above_water,
            mizzen_p=self.mizzen_p,
            mizzen_e=self.mizzen_e,
            mizzen_mast_height=self.mizzen_mast_height,
            mizzen_boom_height=self.mizzen_boom_height,
        )


class StabilitySchema(BaseModel):
    gmt: float = Field(..., description="Transverse metacentric height GM_T [m]", gt=0)
    crew_mass: float = Field(350.0, description="Total crew mass [kg]")
    crew_hiking_distance: float = Field(1.5, description="Hiking distance from centerline [m]")
    crew_hiking_fraction: float = Field(0.8, description="Fraction of crew active hiking")

    def to_domain(self) -> Stability:
        return Stability(
            gmt=self.gmt,
            crew_mass=self.crew_mass,
            crew_hiking_distance=self.crew_hiking_distance,
            crew_hiking_fraction=self.crew_hiking_fraction,
        )


class BoatSchema(BaseModel):
    name: str = Field("Custom Yacht", description="Boat name")
    hull: HullSchema
    appendages: AppendagesSchema
    rig: RigSchema
    stability: StabilitySchema

    def to_domain(self) -> Boat:
        return Boat(
            name=self.name,
            hull=self.hull.to_domain(),
            appendages=self.appendages.to_domain(),
            rig=self.rig.to_domain(),
            stability=self.stability.to_domain(),
        )

    @classmethod
    def from_domain(cls, boat: Boat) -> BoatSchema:
        return cls(
            name=boat.name,
            hull=HullSchema(
                loa=boat.hull.loa,
                lwl=boat.hull.lwl,
                b_max=boat.hull.b_max,
                b_wl=boat.hull.b_wl,
                draft_canoe=boat.hull.draft_canoe,
                draft_total=boat.hull.draft_total,
                displacement_mass=boat.hull.displacement_mass,
                wetted_surface=boat.hull.wetted_surface,
                prismatic_coef=boat.hull.prismatic_coef,
                form_factor_k=boat.hull.form_factor_k,
                lcb_fraction=boat.hull.lcb_fraction,
            ),
            appendages=AppendagesSchema(
                keel_type=boat.appendages.keel_type,
                keel_area=boat.appendages.keel_area,
                keel_span=boat.appendages.keel_span,
                rudder_area=boat.appendages.rudder_area,
                rudder_span=boat.appendages.rudder_span,
                effective_draft=boat.appendages.effective_draft,
                wetted_surface=boat.appendages.wetted_surface,
            ),
            rig=RigSchema(
                rig_type=boat.rig.rig_type,
                main_p=boat.rig.main_p,
                main_e=boat.rig.main_e,
                fore_i=boat.rig.fore_i,
                fore_j=boat.rig.fore_j,
                mast_height_above_water=boat.rig.mast_height_above_water,
                boom_height_above_water=boat.rig.boom_height_above_water,
                mizzen_p=boat.rig.mizzen_p,
                mizzen_e=boat.rig.mizzen_e,
                mizzen_mast_height=boat.rig.mizzen_mast_height,
                mizzen_boom_height=boat.rig.mizzen_boom_height,
            ),
            stability=StabilitySchema(
                gmt=boat.stability.gmt,
                crew_mass=boat.stability.crew_mass,
                crew_hiking_distance=boat.stability.crew_hiking_distance,
                crew_hiking_fraction=boat.stability.crew_hiking_fraction,
            ),
        )


PRESETS_MAP: Dict[str, Any] = {
    "36ft-ketch": create_36ft_ketch,
    "36ft-sloop": create_36ft_sloop,
    "40ft-cruiser": create_40ft_performance_cruiser,
    "24ft-sportboat": create_24ft_sportboat,
}


def resolve_boat(boat_schema: Optional[BoatSchema], preset_name: Optional[str]) -> Boat:
    """Resolve Boat instance from explicit schema or preset identifier."""
    if boat_schema is not None:
        return boat_schema.to_domain()
    if preset_name is not None:
        key = preset_name.lower().strip()
        if key in PRESETS_MAP:
            return PRESETS_MAP[key]()
        raise ValueError(f"Unknown preset '{preset_name}'. Available: {list(PRESETS_MAP.keys())}")
    # Default to 36ft ketch
    return create_36ft_ketch()


# Request / Response Models

class SolvePointRequest(BaseModel):
    tws_kts: float = Field(..., description="True Wind Speed [knots]", gt=0)
    twa_deg: float = Field(..., description="True Wind Angle [degrees]", ge=0, le=180)
    boat: Optional[BoatSchema] = Field(None, description="Custom boat configuration")
    preset_name: Optional[str] = Field("36ft-ketch", description="Preset boat name (e.g. 36ft-ketch, 36ft-sloop)")
    max_heel_deg: float = Field(28.0, description="Maximum heel angle limit [degrees]")


class SolvePointResponse(BaseModel):
    tws_kts: float
    twa_deg: float
    v_boat_kts: float
    v_boat_ms: float
    vmg_kts: float
    heel_deg: float
    leeway_deg: float
    sail_set_name: str
    flat: float
    reef: float
    aws_kts: float
    awa_deg: float
    f_x_n: float
    r_total_n: float
    r_viscous_n: float
    r_residuary_n: float
    r_induced_n: float
    r_heel_n: float
    heeling_moment_nm: float
    righting_moment_nm: float
    converged: bool


class SolveMatrixRequest(BaseModel):
    tws_list: Optional[List[float]] = Field(
        default=[6.0, 8.0, 10.0, 12.0, 14.0, 16.0, 20.0, 25.0],
        description="List of True Wind Speeds in knots",
    )
    twa_list: Optional[List[float]] = Field(
        default=[0.0, 10.0, 20.0, 25.0, 30.0, 35.0, 40.0, 45.0, 52.0, 60.0, 70.0, 80.0, 90.0, 110.0, 120.0, 135.0, 150.0, 165.0, 180.0],
        description="List of True Wind Angles in degrees",
    )
    boat: Optional[BoatSchema] = Field(None, description="Custom boat configuration")
    preset_name: Optional[str] = Field("36ft-ketch", description="Preset boat name")
    max_heel_deg: float = Field(28.0, description="Maximum heel limit [degrees]")
    speed_matrix: Optional[List[List[float]]] = Field(None, description="Precomputed speed matrix [len(tws), len(twa)] in knots")
    boat_name: Optional[str] = Field(None, description="Display boat name for polar table and plots")


class VMGTargetResponse(BaseModel):
    tws_kts: float
    target_twa_deg: float
    target_v_boat_kts: float
    target_vmg_kts: float
    is_upwind: bool


class SolveMatrixResponse(BaseModel):
    boat_name: str
    tws_list: List[float]
    twa_list: List[float]
    speed_matrix: List[List[float]]  # [len(tws), len(twa)] in knots
    upwind_vmg_targets: Dict[str, VMGTargetResponse]
    downwind_vmg_targets: Dict[str, VMGTargetResponse]
    # Fingerprint of the model that produced this table. A client holding a stored polar can
    # compare it against the live value to notice that the polar predates a model change.
    model_version: str = ""


class ModelVersionResponse(BaseModel):
    model_version: str


class PresetSummaryResponse(BaseModel):
    id: str
    name: str
    loa_m: float
    beam_m: float
    draft_m: float
    displacement_kg: float
    rig_type: str
