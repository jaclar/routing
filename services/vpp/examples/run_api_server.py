"""Start the FastAPI VPP server."""

import sys
from pathlib import Path

# Ensure root directory is on python path
repo_root = Path(__file__).resolve().parent.parent
if str(repo_root) not in sys.path:
    sys.path.insert(0, str(repo_root))

from vpp.api.server import run_server

if __name__ == "__main__":
    print("Starting Sailboat VPP FastAPI service on http://127.0.0.1:8000 ...")
    print("Interactive Swagger documentation: http://127.0.0.1:8000/docs")
    print("ReDoc documentation: http://127.0.0.1:8000/redoc")
    run_server(host="127.0.0.1", port=8000, reload=False)
