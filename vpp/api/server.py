"""Uvicorn server launcher for the FastAPI VPP service."""

import uvicorn


def run_server(host: str = "0.0.0.0", port: int = 8000, reload: bool = False):
    """Run the VPP FastAPI service using uvicorn."""
    uvicorn.run(
        "vpp.api.app:app",
        host=host,
        port=port,
        reload=reload,
    )


if __name__ == "__main__":
    run_server(host="127.0.0.1", port=8000)
