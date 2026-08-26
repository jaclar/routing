"""Solver package for 3-DOF equilibrium, trim optimization, and VPP execution."""

from vpp.solver.equilibrium import (
    EquilibriumState,
    compute_equilibrium_residuals,
    solve_equilibrium,
)
from vpp.solver.optimizer import (
    OptimizedTrimResult,
    optimize_sail_trim,
)
from vpp.solver.vpp_solver import (
    VPPPointResult,
    VPPSolver,
)

__all__ = [
    "EquilibriumState",
    "compute_equilibrium_residuals",
    "solve_equilibrium",
    "OptimizedTrimResult",
    "optimize_sail_trim",
    "VPPPointResult",
    "VPPSolver",
]
