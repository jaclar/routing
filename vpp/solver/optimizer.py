"""Sail trim and de-powering optimizer for maximum equilibrium boat speed."""

from dataclasses import dataclass
from typing import Optional, Tuple
import numpy as np
from scipy.optimize import minimize

from vpp.core.boat import Boat
from vpp.core.environment import Environment
from vpp.aero.sail import SailSet
from vpp.solver.equilibrium import solve_equilibrium, EquilibriumState


@dataclass
class OptimizedTrimResult:
    """Optimal trim solution for a specific sail set and wind condition."""
    state: EquilibriumState
    flat: float          # Flattening factor (0.4 to 1.0)
    reef: float          # Reefing factor (0.5 to 1.0)
    sail_set_name: str   # Name of sail set used


def optimize_sail_trim(
    boat: Boat,
    sail_set: SailSet,
    tws_ms: float,
    twa_rad: float,
    max_heel_rad: float = np.deg2rad(28.0),
    x0: Optional[Tuple[float, float, float]] = None,
    env: Environment = Environment(),
) -> OptimizedTrimResult:
    """Optimize sail flattening and reefing to maximize speed within heel limits.
    
    Args:
        boat: Boat instance.
        sail_set: SailSet instance.
        tws_ms: True wind speed [m/s].
        twa_rad: True wind angle [radians].
        max_heel_rad: Maximum allowed heel angle before penalty [radians].
        x0: Initial guess for equilibrium solver.
        env: Environment instance.
        
    Returns:
        OptimizedTrimResult with optimal state and trim parameters.
    """
    # 1. First test full sail (flat = 1.0, reef = 1.0)
    base_state = solve_equilibrium(
        boat=boat,
        sail_set=sail_set,
        tws_ms=tws_ms,
        twa_rad=twa_rad,
        flat=1.0,
        reef=1.0,
        x0=x0,
        env=env,
    )

    # If heel is moderate and solver converged well, full sail is optimal
    if base_state.heel_rad <= max_heel_rad and base_state.converged:
        return OptimizedTrimResult(
            state=base_state,
            flat=1.0,
            reef=1.0,
            sail_set_name=sail_set.name,
        )

    # 2. Optimization over (flat, reef)
    # We want to maximize v_boat_ms while penalizing excessive heel > max_heel_rad
    best_state = base_state
    best_flat = 1.0
    best_reef = 1.0
    best_score = -1e9

    # Fast 2D grid search followed by local refinement
    flats = [1.0, 0.85, 0.70, 0.55, 0.40]
    reefs = [1.0, 0.90, 0.80, 0.70, 0.60]

    current_x0 = (base_state.v_boat_ms, base_state.heel_rad, base_state.leeway_rad)

    for r in reefs:
        for f in flats:
            state = solve_equilibrium(
                boat=boat,
                sail_set=sail_set,
                tws_ms=tws_ms,
                twa_rad=twa_rad,
                flat=f,
                reef=r,
                x0=current_x0,
                env=env,
            )
            if state.converged:
                # Update warm start
                current_x0 = (state.v_boat_ms, state.heel_rad, state.leeway_rad)
                # Score function: prioritize speed, heavily penalize heel over max_heel_rad
                excess_heel = max(state.heel_rad - max_heel_rad, 0.0)
                score = state.v_boat_ms - 15.0 * (excess_heel ** 2)

                if score > best_score:
                    best_score = score
                    best_state = state
                    best_flat = f
                    best_reef = r

    return OptimizedTrimResult(
        state=best_state,
        flat=best_flat,
        reef=best_reef,
        sail_set_name=sail_set.name,
    )
