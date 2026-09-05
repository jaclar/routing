"""The model fingerprint must track the physics, and nothing else."""

import numpy as np

from vpp.core import boat as boat_mod
from vpp.polars.model_version import model_version, reset_cache
from vpp.presets import boats as presets


def fresh_version() -> str:
    reset_cache()
    return model_version()


def test_version_is_stable_across_calls():
    reset_cache()
    assert model_version() == model_version()
    assert fresh_version() == fresh_version()


def test_version_changes_when_the_physics_changes():
    """A change to the model must invalidate polars solved by the previous one."""
    before = fresh_version()

    original = boat_mod.Stability.righting_moment_crew
    try:
        # Any real change to how a force or moment is computed should register.
        boat_mod.Stability.righting_moment_crew = lambda self, phi: original(self, phi) * 1.5
        after = fresh_version()
    finally:
        boat_mod.Stability.righting_moment_crew = original

    assert after != before, "a modified crew moment must produce a different fingerprint"
    assert fresh_version() == before, "restoring the model must restore the fingerprint"


def test_version_survives_retuning_a_preset():
    """Retuning a boat is not a model change and must not invalidate anyone's polars."""
    before = fresh_version()

    original = presets.create_36ft_sloop
    try:
        def heavier():
            b = original()
            b.hull.displacement_mass *= 1.4
            b.stability.crew_hiking_fraction = 0.1
            return b

        presets.create_36ft_sloop = heavier
        after = fresh_version()
    finally:
        presets.create_36ft_sloop = original

    assert after == before, "preset geometry must not feed the fingerprint"


def test_version_is_a_short_hex_digest():
    v = fresh_version()
    assert len(v) == 12
    int(v, 16)  # raises if it is not hex


def test_rounding_absorbs_float_noise():
    """Differences far below the rounding threshold must not change the fingerprint."""
    before = fresh_version()

    original = boat_mod.Stability.righting_moment_crew
    try:
        # A perturbation well under the rounding floor of the hashed speeds.
        boat_mod.Stability.righting_moment_crew = lambda self, phi: original(self, phi) * (1 + 1e-12)
        after = fresh_version()
    finally:
        boat_mod.Stability.righting_moment_crew = original

    assert after == before, "negligible numerical noise must not invalidate stored polars"
