"""FastAPI REST service package for Sailboat VPP."""

from vpp.api.app import app, create_app
from vpp.api.server import run_server
from vpp.api.schemas import (
    BoatSchema,
    HullSchema,
    AppendagesSchema,
    RigSchema,
    StabilitySchema,
    SolvePointRequest,
    SolvePointResponse,
    SolveMatrixRequest,
    SolveMatrixResponse,
    VMGTargetResponse,
    PresetSummaryResponse,
)

__all__ = [
    "app",
    "create_app",
    "run_server",
    "BoatSchema",
    "HullSchema",
    "AppendagesSchema",
    "RigSchema",
    "StabilitySchema",
    "SolvePointRequest",
    "SolvePointResponse",
    "SolveMatrixRequest",
    "SolveMatrixResponse",
    "VMGTargetResponse",
    "PresetSummaryResponse",
]
