# Sailboat Velocity Prediction Program (VPP) Microservice

Modern aero-hydrodynamic Velocity Prediction Program (VPP) and FastAPI service in Python for calculating sailboat polar performance, equilibrium states, and VMG targets.

## Features
- **3-DOF Equilibrium Solver**: Balances aerodynamic drive/side/heeling forces against hydrodynamic drag/lift/righting moments.
- **Aero & Hydro Models**: Hazen/ORC aero sail formulations, ITTC-1957 friction, and DSYHS residuary resistance.
- **Polar Table Generator**: Solves 2D performance matrix across wind speeds and angles.
- **FastAPI Service**: REST API endpoints for single point solving, matrix calculation, ORC `.pol` / CSV export, and polar diagrams.

## Quickstart
```bash
# Install package in editable mode
pip install -e .

# Run test suite
pytest -v

# Start FastAPI server
uvicorn vpp.api.app:app --host 0.0.0.0 --port 8000 --reload
```
