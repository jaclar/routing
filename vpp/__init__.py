"""Sailboat Velocity Prediction Program (VPP) in Python."""

__version__ = "0.1.0"

from vpp.core.units import (
    G,
    kts_to_ms,
    ms_to_kts,
    deg_to_rad,
    rad_to_deg,
    ft_to_m,
    m_to_ft,
)
from vpp.core.environment import Environment
from vpp.core.boat import Hull, Appendages, Rig, Stability, Boat
from vpp.aero.wind import ApparentWind, compute_apparent_wind
from vpp.aero.sail import Sail, SailSet, SailType, create_sails_from_rig
from vpp.aero.aero_model import AeroForces, compute_aero_forces
from vpp.hydro.hydro_model import HydroDynamics, compute_hydrodynamics
from vpp.solver.equilibrium import EquilibriumState, solve_equilibrium
from vpp.solver.vpp_solver import VPPPointResult, VPPSolver
from vpp.polars.polar_data import PolarTable, VMGTarget, generate_polar_table
from vpp.polars.exporter import export_to_csv, export_to_orc_pol, export_to_json
from vpp.polars.plotter import (
    plot_polar_diagram,
    plot_performance_curves,
    plot_resistance_breakdown,
)
from vpp.presets.boats import (
    create_36ft_ketch,
    create_36ft_sloop,
    create_40ft_performance_cruiser,
    create_24ft_sportboat,
)
from vpp.api.app import app, create_app

__all__ = [
    "app",
    "create_app",
    "Environment",
    "Hull",
    "Appendages",
    "Rig",
    "Stability",
    "Boat",
    "Sail",
    "SailSet",
    "SailType",
    "create_sails_from_rig",
    "AeroForces",
    "compute_aero_forces",
    "HydroDynamics",
    "compute_hydrodynamics",
    "EquilibriumState",
    "solve_equilibrium",
    "VPPPointResult",
    "VPPSolver",
    "PolarTable",
    "VMGTarget",
    "generate_polar_table",
    "export_to_csv",
    "export_to_orc_pol",
    "export_to_json",
    "plot_polar_diagram",
    "plot_performance_curves",
    "plot_resistance_breakdown",
    "create_36ft_ketch",
    "create_36ft_sloop",
    "create_40ft_performance_cruiser",
    "create_24ft_sportboat",
    "kts_to_ms",
    "ms_to_kts",
    "deg_to_rad",
    "rad_to_deg",
]
