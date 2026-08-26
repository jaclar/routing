"""FastAPI application factory and lifecycle configuration."""

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from vpp.api.routes import router as api_v1_router


def create_app() -> FastAPI:
    """Create and configure the FastAPI application for Sailboat VPP."""
    app = FastAPI(
        title="Sailboat Velocity Prediction Program (VPP) API",
        description="""
A modern physics-based 3-DOF Velocity Prediction Program REST API for sailboats.

### Features
- **Equilibrium Solver**: 3-DOF force & moment balance (Surge, Sway, Roll).
- **Aero Models**: Hazen/ORC IMS formulation supporting Sloop, Ketch, and Cutter rigs.
- **Hydro Models**: ITTC-57 friction, Delft Systematic Yacht Hull Series (DSYHS) wave drag, appendage induced drag & leeway, and hydrostatic stability.
- **Sail Trim Optimizer**: Automatic sail flattening and reefing optimization.
- **Polars & VMG**: Grid solving, optimal upwind beating and downwind gybing VMG targets.
- **Exports & Plots**: ORC `.pol` format, CSV download, and PNG polar image streaming.
        """,
        version="0.1.0",
        docs_url="/docs",
        redoc_url="/redoc",
    )

    # CORS middleware
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    @app.get("/health", tags=["Health"])
    def health_check():
        """Service health check."""
        return {
            "status": "healthy",
            "service": "sail-vpp",
            "version": "0.1.0",
        }

    @app.get("/", tags=["Root"])
    def root():
        """Root endpoint with service summary and links."""
        return {
            "name": "Sailboat Velocity Prediction Program (VPP) API",
            "version": "0.1.0",
            "docs": "/docs",
            "health": "/health",
            "presets": "/api/v1/presets",
        }

    # Mount API v1 routes
    app.include_router(api_v1_router)

    return app


app = create_app()
