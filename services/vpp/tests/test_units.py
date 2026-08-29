"""Unit tests for unit conversions and physical constants."""

import pytest
import numpy as np
from vpp.core.units import (
    kts_to_ms,
    ms_to_kts,
    deg_to_rad,
    rad_to_deg,
    ft_to_m,
    m_to_ft,
)


def test_speed_conversions():
    assert pytest.approx(kts_to_ms(1.0), rel=1e-5) == 0.514444
    assert pytest.approx(ms_to_kts(1.0), rel=1e-5) == 1.943844
    assert pytest.approx(ms_to_kts(kts_to_ms(12.5)), rel=1e-6) == 12.5


def test_angle_conversions():
    assert pytest.approx(deg_to_rad(180.0), rel=1e-6) == np.pi
    assert pytest.approx(rad_to_deg(np.pi / 2.0), rel=1e-6) == 90.0
    assert pytest.approx(rad_to_deg(deg_to_rad(45.0)), rel=1e-6) == 45.0


def test_length_conversions():
    assert pytest.approx(ft_to_m(36.0), rel=1e-4) == 10.9728
    assert pytest.approx(m_to_ft(1.5), rel=1e-4) == 4.92126
