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
        # No crew hiking is modelled for the built-in presets: hiking belongs to a
        # full race setup, or to a boat whose owner configures it deliberately when
        # building custom polars. Crew mass is kept only to record who is aboard.
        crew_mass=280.0,  # 3-4 cruising crew
        crew_hiking_distance=1.45,
        crew_hiking_fraction=0.0,
    )

    return Boat(
        name="36ft Cruising Ketch",
        hull=hull,
        appendages=appendages,
        rig=rig,
        stability=stability,
    )


def create_36ft_sloop() -> Boat:
    """Create a 36-foot racing sloop.

    An inshore/offshore racer in the IRC 36 mould: light for her length, deep T-keel,
    generous rig, and sailed with a full crew on the rail.
    """
    hull = Hull(
        loa=10.90,
        lwl=9.65,          # near-plumb bow, long sailing waterline
        b_max=3.48,
        b_wl=2.95,         # narrow waterline, beam carried high for form stability when heeled
        draft_canoe=0.48,
        draft_total=2.30,
        displacement_mass=4900.0,
        prismatic_coef=0.57,
        form_factor_k=0.09,
    )

    appendages = Appendages(
        keel_type="fin_bulb",
        keel_area=1.60,
        keel_span=1.85,
        rudder_area=0.72,
        rudder_span=1.50,
        effective_draft=2.25,
    )

    rig = Rig(
        rig_type="sloop",
        main_p=13.9,
        main_e=4.55,
        fore_i=14.6,
        fore_j=4.25,
        mast_height_above_water=17.3,
        boom_height_above_water=1.85,
    )

    stability = Stability(
        gmt=1.15,
        # A raced boat: the crew are on the rail and their righting moment is part of how
        # she is sailed, so hiking is modelled here.
        crew_mass=550.0,  # 7 racing crew
        crew_hiking_distance=1.68,
        crew_hiking_fraction=0.85,
    )

    return Boat(
        name="36ft Racing Sloop",
        hull=hull,
        appendages=appendages,
        rig=rig,
        stability=stability,
    )


def create_40ft_performance_cruiser() -> Boat:
    """Create a modern 40-foot cruising boat.

    A contemporary production cruiser: beamy, moderately heavy once loaded for living
    aboard, on a shoal-ish cruising fin, and sailed short-handed from the cockpit.
    """
    hull = Hull(
        loa=12.20,
        lwl=11.10,         # plumb bow and broad transom, typical of the modern type
        b_max=4.05,
        b_wl=3.58,
        draft_canoe=0.60,
        draft_total=1.98,  # cruising fin, kept shallow for anchorages
        displacement_mass=8600.0,  # loaded: tankage, ground tackle, cruising gear
        prismatic_coef=0.58,
        form_factor_k=0.13,
    )

    appendages = Appendages(
        keel_type="fin_bulb",
        keel_area=2.35,
        keel_span=1.45,
        rudder_area=1.00,
        rudder_span=1.35,
        effective_draft=1.92,
    )

    rig = Rig(
        rig_type="sloop",
        main_p=14.6,
        main_e=4.90,
        fore_i=15.4,
        fore_j=4.50,
        mast_height_above_water=17.9,
        boom_height_above_water=2.10,
    )

    stability = Stability(
        gmt=1.35,  # beam carried aft gives a modern cruiser plenty of form stability
        # Cruising crew sit in the cockpit rather than on the rail, so no hiking moment is
        # modelled. Crew mass is recorded only to note who is aboard.
        crew_mass=400.0,  # cruising couple plus guests
        crew_hiking_distance=1.80,
        crew_hiking_fraction=0.0,
    )

    return Boat(
        name="40ft Cruiser",
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
        # This one is a genuine race boat, so hiking crew are part of how it is sailed
        # and are modelled. Solved without them it is so tender that its upwind row at
        # 25 knots collapses to a fraction of a knot, which is useless for routing.
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
