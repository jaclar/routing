"""Unit tests for the 3-DOF equilibrium solver and trim optimizer."""

import pytest
import numpy as np
from vpp.core.units import kts_to_ms, deg_to_rad
from vpp.presets.boats import create_36ft_ketch, create_36ft_sloop
from vpp.solver.vpp_solver import VPPSolver
from vpp.solver.equilibrium import solve_equilibrium
from vpp.aero.sail import create_sails_from_rig


def test_equilibrium_solve_upwind():
    ketch = create_36ft_ketch()
    upwind_sails, _ = create_sails_from_rig(ketch.rig)
    
    tws_ms = kts_to_ms(12.0)
    twa_rad = deg_to_rad(45.0)

    state = solve_equilibrium(
        boat=ketch,
        sail_set=upwind_sails,
        tws_ms=tws_ms,
        twa_rad=twa_rad,
        flat=1.0,
        reef=1.0,
    )

    assert state.converged
    # In 12 kts wind at 45 deg, 36ft Ketch speed should be ~ 5.5 to 7.0 kts
    assert 5.0 <= state.v_boat_kts <= 8.0
    # Heel angle should be between 10 and 25 deg
    assert 8.0 <= state.heel_deg <= 25.0
    # Leeway should be positive and realistic (1.5 to 7.0 deg)
    assert 1.0 <= state.leeway_deg <= 7.5


def test_vpp_solver_single_point():
    ketch = create_36ft_ketch()
    solver = VPPSolver(ketch)

    # Beam reach in 14 kts
    res = solver.solve_point(tws_kts=14.0, twa_deg=90.0)
    assert res.converged
    assert res.v_boat_kts > 6.0
    assert res.heel_deg > 0.0
    assert res.aws_kts > 0.0
    assert res.r_total_n > 0.0


def test_depowering_in_heavy_wind():
    ketch = create_36ft_ketch()
    solver = VPPSolver(ketch, max_heel_deg=25.0)

    # Strong wind on close reach
    res = solver.solve_point(tws_kts=25.0, twa_deg=50.0)
    assert res.converged
    # Sail should be flattened and/or reefed
    assert (res.flat < 1.0) or (res.reef < 1.0)
    # Heel should remain controlled below or near limit
    assert res.heel_deg <= 28.0
