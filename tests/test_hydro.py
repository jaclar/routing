"""Unit tests for hydrodynamic resistance components and hydrostatic stability."""

import pytest
import numpy as np
from vpp.core.units import kts_to_ms, deg_to_rad
from vpp.hydro.friction import compute_frictional_resistance, ittc_57_cf
from vpp.hydro.residuary import compute_residuary_resistance
from vpp.hydro.induced import compute_induced_resistance
from vpp.hydro.heel import compute_heel_resistance
from vpp.hydro.stability import compute_righting_moments
from vpp.hydro.hydro_model import compute_hydrodynamics
from vpp.presets.boats import create_36ft_ketch


def test_ittc_cf_monotonic():
    # As Reynolds number increases, Cf should decrease monotonically
    cf_low = ittc_57_cf(1e5)
    cf_med = ittc_57_cf(1e6)
    cf_high = ittc_57_cf(1e7)
    assert cf_low > cf_med > cf_high
    assert 0.002 <= cf_high <= 0.008


def test_resistance_increases_with_speed():
    ketch = create_36ft_ketch()
    v1 = kts_to_ms(4.0)
    v2 = kts_to_ms(7.0)

    fric1 = compute_frictional_resistance(ketch, v1)
    fric2 = compute_frictional_resistance(ketch, v2)
    assert fric2.r_v_total > fric1.r_v_total

    res1 = compute_residuary_resistance(ketch, v1)
    res2 = compute_residuary_resistance(ketch, v2)
    assert res2.r_r > res1.r_r


def test_hydrostatic_righting_moment():
    ketch = create_36ft_ketch()
    stab_0 = compute_righting_moments(ketch, deg_to_rad(0.0))
    stab_10 = compute_righting_moments(ketch, deg_to_rad(10.0))
    stab_20 = compute_righting_moments(ketch, deg_to_rad(20.0))

    # At 0 deg heel, hull righting moment is 0 (crew may have hiking moment)
    assert stab_0.rm_hull == 0.0
    assert stab_10.rm_total < stab_20.rm_total
    assert stab_20.rm_total > 10000.0  # Realistic righting moment for 7000kg boat


def test_total_hydrodynamics():
    ketch = create_36ft_ketch()
    v_boat = kts_to_ms(6.0)
    heel = deg_to_rad(15.0)
    side_force = 3500.0  # Newtons

    hydro = compute_hydrodynamics(ketch, v_boat, heel, side_force)
    assert hydro.r_total > 0.0
    assert hydro.r_viscous > 0.0
    assert hydro.r_residuary > 0.0
    assert hydro.r_induced > 0.0
    assert hydro.r_heel > 0.0
    assert 0.0 < hydro.leeway_deg < 10.0
