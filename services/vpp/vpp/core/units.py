"""Unit conversion constants and helper functions for VPP."""

from __future__ import annotations
from typing import Union
import numpy as np

# Physical constants
G: float = 9.80665  # Standard gravity acceleration [m/s^2]
KNOTS_TO_MS: float = 1852.0 / 3600.0  # 1 knot in m/s (~0.514444)
MS_TO_KNOTS: float = 3600.0 / 1852.0  # 1 m/s in knots (~1.943844)
FEET_TO_METERS: float = 0.3048
METERS_TO_FEET: float = 1.0 / 0.3048
LBS_TO_KG: float = 0.45359237
KG_TO_LBS: float = 1.0 / 0.45359237


def kts_to_ms(knots: Union[float, np.ndarray]) -> Union[float, np.ndarray]:
    """Convert knots to meters per second."""
    return knots * KNOTS_TO_MS


def ms_to_kts(ms: Union[float, np.ndarray]) -> Union[float, np.ndarray]:
    """Convert meters per second to knots."""
    return ms * MS_TO_KNOTS


def deg_to_rad(deg: Union[float, np.ndarray]) -> Union[float, np.ndarray]:
    """Convert degrees to radians."""
    return np.deg2rad(deg)


def rad_to_deg(rad: Union[float, np.ndarray]) -> Union[float, np.ndarray]:
    """Convert radians to degrees."""
    return np.rad2deg(rad)


def ft_to_m(feet: Union[float, np.ndarray]) -> Union[float, np.ndarray]:
    """Convert feet to meters."""
    return feet * FEET_TO_METERS


def m_to_ft(meters: Union[float, np.ndarray]) -> Union[float, np.ndarray]:
    """Convert meters to feet."""
    return meters * METERS_TO_FEET
