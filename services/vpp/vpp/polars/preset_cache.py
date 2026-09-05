"""In-memory cache of solved preset polar tables.

This service is the single source of truth for the built-in presets' polars: the routing
engine asks for them over HTTP rather than carrying its own copy, which would silently drift
whenever the VPP model changed.

Solving one preset takes roughly one to three seconds, so serving it from the solver on every
request would put that on the critical path of every route calculation. A preset's geometry
never changes within a process, so each distinct request shape is solved at most once and
then served from memory.
"""

import threading
from typing import Callable, Dict, List, Optional, Tuple

from vpp.core.boat import Boat
from vpp.polars.polar_data import PolarTable, generate_polar_table
from vpp.solver.vpp_solver import VPPSolver

_CacheKey = Tuple[str, Tuple[float, ...], Tuple[float, ...], float]

_lock = threading.Lock()
_cache: Dict[_CacheKey, PolarTable] = {}


def _key(
    preset_id: str,
    tws_list: Optional[List[float]],
    twa_list: Optional[List[float]],
    max_heel_deg: float,
) -> _CacheKey:
    # The grid is part of the identity: a caller asking for a different set of wind speeds or
    # angles is asking for a different table, not the same one resampled.
    return (
        preset_id,
        tuple(tws_list or ()),
        tuple(twa_list or ()),
        float(max_heel_deg),
    )


def get_preset_polar(
    preset_id: str,
    boat_factory: Callable[[], Boat],
    tws_list: Optional[List[float]] = None,
    twa_list: Optional[List[float]] = None,
    max_heel_deg: float = 28.0,
) -> PolarTable:
    """Return the solved polar for a preset, solving it only on the first request."""
    key = _key(preset_id, tws_list, twa_list, max_heel_deg)

    with _lock:
        cached = _cache.get(key)
    if cached is not None:
        return cached

    # Solved outside the lock: holding it for seconds would stall every other request. Two
    # callers racing on a cold entry may both solve, which wastes a little work but is
    # harmless, and setdefault makes sure they agree on the result afterwards.
    solver = VPPSolver(boat=boat_factory(), max_heel_deg=max_heel_deg)
    table = generate_polar_table(solver, tws_list=tws_list, twa_list=twa_list)

    with _lock:
        return _cache.setdefault(key, table)


def cache_size() -> int:
    """Number of solved tables currently held. Exposed for diagnostics and tests."""
    with _lock:
        return len(_cache)


def clear() -> None:
    """Drop every cached table. Intended for tests."""
    with _lock:
        _cache.clear()
