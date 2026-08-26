"""Pre-configured sailboat models and presets."""

from vpp.core.boat import Hull, Appendages, Rig, Stability, Boat


def create_36ft_ketch() -> Boat:
    """Create a 36-foot cruising ketch with user specified dimensions.
    
    Specification:
    - LOA: 11.0 m (36.1 ft)
    - LWL: 9.2 m (30.2 ft)
    - Beam: 3.5 m
    - Draft: 1.5 m
    - Displacement: 7000 kg
    - Rig: Ketch (Main + Jib + Mizzen)
    """
    hull = Hull(
        loa=11.0,
        lwl=9.2,
        b_max=3.5,
        b_wl=3.10,
        draft_canoe=0.65,
        draft_total=1.5,
        displacement_mass=7000.0,
        prismatic_coef=0.55,
        form_factor_k=0.14,
        lcb_fraction=0.53,
    )

    appendages = Appendages(
        keel_type="long_fin",
        keel_area=2.20,
        keel_span=0.95,
        rudder_area=0.90,
        rudder_span=0.90,
        effective_draft=1.45,
    )

    rig = Rig(
        rig_type="ketch",
        main_p=11.5,
        main_e=4.0,
        fore_i=12.5,
        fore_j=4.3,
        mast_height_above_water=14.5,
        boom_height_above_water=1.8,
        mizzen_p=7.8,
        mizzen_e=2.8,
        mizzen_mast_height=9.8,
        mizzen_boom_height=1.6,
    )

    stability = Stability(
        gmt=1.05,
        crew_mass=280.0,  # 3-4 cruising crew
        crew_hiking_distance=1.45,
        crew_hiking_fraction=0.6,
    )

    return Boat(
        name="36ft Cruising Ketch",
        hull=hull,
        appendages=appendages,
        rig=rig,
        stability=stability,
    )


def create_36ft_sloop() -> Boat:
    """Create a 36-foot modern racer-cruiser sloop."""
    hull = Hull(
        loa=10.75,
        lwl=9.32,
        b_max=3.51,
        b_wl=3.05,
        draft_canoe=0.52,
        draft_total=2.10,
        displacement_mass=5500.0,
        prismatic_coef=0.56,
        form_factor_k=0.10,
    )

    appendages = Appendages(
        keel_type="fin_bulb",
        keel_area=1.70,
        keel_span=1.60,
        rudder_area=0.75,
        rudder_span=1.40,
        effective_draft=2.05,
    )

    rig = Rig(
        rig_type="sloop",
        main_p=13.3,
        main_e=4.2,
        fore_i=14.0,
        fore_j=4.1,
        mast_height_above_water=16.5,
        boom_height_above_water=1.9,
    )

    stability = Stability(
        gmt=1.20,
        crew_mass=550.0,  # 7 racing crew
        crew_hiking_distance=1.65,
        crew_hiking_fraction=0.85,
    )

    return Boat(
        name="36ft Racer-Cruiser Sloop",
        hull=hull,
        appendages=appendages,
        rig=rig,
        stability=stability,
    )


def create_40ft_performance_cruiser() -> Boat:
    """Create a 40-foot performance cruiser."""
    hull = Hull(
        loa=12.24,
        lwl=10.67,
        b_max=3.89,
        b_wl=3.40,
        draft_canoe=0.58,
        draft_total=2.45,
        displacement_mass=7500.0,
        prismatic_coef=0.57,
        form_factor_k=0.11,
    )

    appendages = Appendages(
        keel_type="bulb",
        keel_area=2.00,
        keel_span=1.90,
        rudder_area=0.90,
        rudder_span=1.60,
        effective_draft=2.40,
    )

    rig = Rig(
        rig_type="sloop",
        main_p=15.2,
        main_e=5.2,
        fore_i=16.0,
        fore_j=4.6,
        mast_height_above_water=18.8,
        boom_height_above_water=2.1,
    )

    stability = Stability(
        gmt=1.25,
        crew_mass=600.0,
        crew_hiking_distance=1.80,
        crew_hiking_fraction=0.75,
    )

    return Boat(
        name="40ft Performance Cruiser",
        hull=hull,
        appendages=appendages,
        rig=rig,
        stability=stability,
    )


def create_24ft_sportboat() -> Boat:
    """Create a 24-foot lightweight sportboat (planing monohull)."""
    hull = Hull(
        loa=7.32,
        lwl=6.86,
        b_max=2.50,
        b_wl=2.15,
        draft_canoe=0.30,
        draft_total=1.75,
        displacement_mass=850.0,
        prismatic_coef=0.58,
        form_factor_k=0.08,
    )

    appendages = Appendages(
        keel_type="lifting_bulb",
        keel_area=0.85,
        keel_span=1.45,
        rudder_area=0.42,
        rudder_span=1.20,
        effective_draft=1.70,
    )

    rig = Rig(
        rig_type="sloop",
        main_p=9.0,
        main_e=3.3,
        fore_i=8.8,
        fore_j=2.7,
        mast_height_above_water=10.5,
        boom_height_above_water=1.2,
    )

    stability = Stability(
        gmt=0.95,
        crew_mass=320.0,  # 4 hiking crew
        crew_hiking_distance=1.20,
        crew_hiking_fraction=0.95,
    )

    return Boat(
        name="24ft Sportboat",
        hull=hull,
        appendages=appendages,
        rig=rig,
        stability=stability,
    )
