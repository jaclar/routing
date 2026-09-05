"""A fingerprint identifying the behaviour of the VPP model.

Solved polars outlive the code that produced them. A custom boat's polar is solved once and
then kept in the owner's browser indefinitely, so when the model changes those stored tables
quietly become predictions from a model that no longer exists. Stamping every solve with a
version lets a client notice that and re-solve.

The version is derived from what the model *does*, not from a constant somebody has to
remember to bump. A hand-maintained version number reintroduces the very failure this is
meant to catch: the model changes, nobody updates the constant, and the staleness goes
unnoticed. Hashing the source would over-trigger instead, changing on every comment edit.

So the fingerprint is a hash of the model's own output: a fixed reference boat solved over a
small fixed grid. It changes exactly when the physics changes, and not otherwise. Speeds are
rounded before hashing so that floating-point noise across platforms and library versions
cannot produce spurious mismatches.
"""

import hashlib
import threading
from typing import Optional

from vpp.core.boat import Appendages, Boat, Hull, Rig, Stability
from vpp.solver.vpp_solver import VPPSolver

# Deliberately small: this runs once per process and only needs to sample the model, not
# characterise a boat. The angles span upwind, reaching and running so a change anywhere in
# the envelope registers.
_PROBE_TWS = [8.0, 16.0]
_PROBE_TWA = [40.0, 90.0, 135.0, 180.0]

# Speeds are rounded to this many decimals before hashing. Fine enough that a real modelling
# change shows up, coarse enough that platform-level float noise does not.
_ROUNDING = 3

_lock = threading.Lock()
_cached: Optional[str] = None


def _reference_boat() -> Boat:
    """A fixed boat used only to probe the model.

    Defined inline rather than taken from the presets on purpose: retuning a preset changes
    that boat, not the physics, and must not invalidate everyone's stored polars.
    """
    return Boat(
        name="Model Fingerprint Reference",
        hull=Hull(
            loa=11.0,
            lwl=9.5,
            b_max=3.5,
            b_wl=3.0,
            draft_canoe=0.55,
            draft_total=2.0,
            displacement_mass=6000.0,
            prismatic_coef=0.56,
            form_factor_k=0.11,
        ),
        appendages=Appendages(
            keel_type="fin",
            keel_area=1.8,
            keel_span=1.5,
            rudder_area=0.8,
            rudder_span=1.3,
            effective_draft=1.95,
        ),
        rig=Rig(
            rig_type="sloop",
            main_p=13.0,
            main_e=4.3,
            fore_i=13.8,
            fore_j=4.2,
            mast_height_above_water=16.5,
            boom_height_above_water=1.9,
        ),
        stability=Stability(
            gmt=1.15,
            crew_mass=400.0,
            crew_hiking_distance=1.5,
            # Non-zero so the crew term is part of the fingerprint: a change to how crew
            # righting moment behaves must invalidate stored polars.
            crew_hiking_fraction=0.5,
        ),
    )


def model_version() -> str:
    """Return the current model fingerprint, computing it once per process."""
    global _cached

    with _lock:
        if _cached is not None:
            return _cached

    solver = VPPSolver(boat=_reference_boat())
    samples = []
    for tws in _PROBE_TWS:
        for twa in _PROBE_TWA:
            result = solver.solve_point(tws_kts=tws, twa_deg=twa)
            # Convergence is part of the signature: a change that fixes a failing point
            # changes the model's behaviour even when the speed lands nearby.
            samples.append(
                f"{tws:.1f}/{twa:.1f}="
                f"{round(result.v_boat_kts, _ROUNDING)}:"
                f"{round(result.heel_deg, _ROUNDING)}:"
                f"{int(bool(result.converged))}"
            )

    digest = hashlib.sha256("|".join(samples).encode("utf-8")).hexdigest()[:12]

    with _lock:
        if _cached is None:
            _cached = digest
        return _cached


def reset_cache() -> None:
    """Forget the computed fingerprint. Intended for tests."""
    global _cached
    with _lock:
        _cached = None
