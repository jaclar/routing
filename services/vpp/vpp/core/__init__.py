"""Core definitions, units, environment, and boat data models."""

from vpp.core.units import (
    G,
    KNOTS_TO_MS,
    MS_TO_KNOTS,
    kts_to_ms,
    ms_to_kts,
    deg_to_rad,
    rad_to_deg,
    ft_to_m,
    m_to_ft,
)
from vpp.core.environment import Environment
from vpp.core.boat import Hull, Appendages, Rig, Stability, Boat

__all__ = [
    "G",
    "KNOTS_TO_MS",
    "MS_TO_KNOTS",
    "kts_to_ms",
    "ms_to_kts",
    "deg_to_rad",
    "rad_to_deg",
    "ft_to_m",
    "m_to_ft",
    "Environment",
    "Hull",
    "Appendages",
    "Rig",
    "Stability",
    "Boat",
]
