"""Aerodynamic modeling package for VPP."""

from vpp.aero.wind import ApparentWind, compute_apparent_wind
from vpp.aero.sail import SailType, Sail, SailSet, create_sails_from_rig
from vpp.aero.coefficients import (
    AeroCoefficients,
    compute_sail_coefficients,
    compute_parasitic_windage_drag_area,
)
from vpp.aero.aero_model import AeroForces, compute_aero_forces

__all__ = [
    "ApparentWind",
    "compute_apparent_wind",
    "SailType",
    "Sail",
    "SailSet",
    "create_sails_from_rig",
    "AeroCoefficients",
    "compute_sail_coefficients",
    "compute_parasitic_windage_drag_area",
    "AeroForces",
    "compute_aero_forces",
]
