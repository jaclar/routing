"""Unit tests for aerodynamics, wind triangles, and sail forces."""

import pytest
import numpy as np
from vpp.core.environment import Environment
from vpp.core.units import deg_to_rad, kts_to_ms
from vpp.aero.wind import compute_apparent_wind
from vpp.aero.sail import create_sails_from_rig, SailType
from vpp.aero.aero_model import compute_aero_forces
from vpp.presets.boats import create_36ft_ketch


def test_apparent_wind_head_to_wind():
    env = Environment(wind_shear_exponent=0.0)
    # Boat at 3 m/s, true wind 5 m/s head to wind (TWA = 0)
    app = compute_apparent_wind(
        tws_10m=5.0,
        twa_rad=0.0,
        v_boat=3.0,
        leeway_rad=0.0,
        heel_rad=0.0,
        z_ce=5.0,
        env=env,
    )
    assert pytest.approx(app.aws, rel=1e-3) == 8.0
    assert pytest.approx(app.awa_deg, abs=1e-3) == 0.0


def test_apparent_wind_dead_downwind():
    env = Environment(wind_shear_exponent=0.0)
    # Boat at 3 m/s, true wind 5 m/s dead downwind (TWA = 180 deg)
    app = compute_apparent_wind(
        tws_10m=5.0,
        twa_rad=np.pi,
        v_boat=3.0,
        leeway_rad=0.0,
        heel_rad=0.0,
        z_ce=5.0,
        env=env,
    )
    assert pytest.approx(app.aws, rel=1e-3) == 2.0
    assert pytest.approx(app.awa_deg, abs=1e-3) == 180.0


def test_ketch_rig_sails():
    ketch = create_36ft_ketch()
    upwind_set, downwind_set = create_sails_from_rig(ketch.rig)
    
    # Ketch upwind should have Mainsail, Jib, and Mizzen
    sail_types = [s.sail_type for s in upwind_set.sails]
    assert SailType.MAIN in sail_types
    assert SailType.JIB in sail_types
    assert SailType.MIZZEN in sail_types
    assert len(upwind_set.sails) == 3

    # Check total upwind sail area is within realistic cruising ketch range (60-70 m^2)
    assert 55.0 <= upwind_set.total_area <= 75.0


def test_aero_forces_generation():
    ketch = create_36ft_ketch()
    upwind_set, _ = create_sails_from_rig(ketch.rig)
    
    tws_ms = kts_to_ms(15.0)
    twa_rad = deg_to_rad(45.0)
    v_boat = kts_to_ms(6.5)

    forces = compute_aero_forces(
        boat=ketch,
        sail_set=upwind_set,
        tws_ms=tws_ms,
        twa_rad=twa_rad,
        v_boat_ms=v_boat,
        heel_rad=deg_to_rad(15.0),
        leeway_rad=deg_to_rad(3.5),
    )

    # In 15 kts wind on close reach, drive force and side force must be positive
    assert forces.f_x > 0.0
    assert forces.f_y > 0.0
    assert forces.m_x > 0.0
    assert forces.apparent_wind.aws > tws_ms
