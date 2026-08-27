"""FastAPI router and endpoint implementations for VPP calculations, presets, exports, and plots."""

import io
from typing import List, Optional
import numpy as np
from fastapi import APIRouter, HTTPException, Query, Response
from fastapi.responses import PlainTextResponse

from vpp.solver.vpp_solver import VPPSolver
from vpp.polars.polar_data import PolarTable, compute_vmg_targets, generate_polar_table
from vpp.polars.exporter import export_to_orc_pol, export_to_csv
from vpp.polars.plotter import (
    plot_polar_diagram,
    plot_performance_curves,
    plot_resistance_breakdown,
)
from vpp.api.schemas import (
    BoatSchema,
    SolvePointRequest,
    SolvePointResponse,
    SolveMatrixRequest,
    SolveMatrixResponse,
    VMGTargetResponse,
    PresetSummaryResponse,
    PRESETS_MAP,
    resolve_boat,
)

router = APIRouter(prefix="/api/v1", tags=["VPP"])


@router.get("/presets", response_model=List[PresetSummaryResponse])
def list_presets():
    """List all available built-in yacht presets."""
    summaries = []
    for pid, factory in PRESETS_MAP.items():
        b = factory()
        summaries.append(
            PresetSummaryResponse(
                id=pid,
                name=b.name,
                loa_m=b.hull.loa,
                beam_m=b.hull.b_max,
                draft_m=b.hull.draft_total,
                displacement_kg=b.hull.displacement_mass,
                rig_type=b.rig.rig_type,
            )
        )
    return summaries


@router.get("/presets/{preset_id}", response_model=BoatSchema)
def get_preset(preset_id: str):
    """Get full geometry and specification for a yacht preset."""
    key = preset_id.lower().strip()
    if key not in PRESETS_MAP:
        raise HTTPException(status_code=404, detail=f"Preset '{preset_id}' not found. Available: {list(PRESETS_MAP.keys())}")
    boat = PRESETS_MAP[key]()
    return BoatSchema.from_domain(boat)


@router.post("/solve/point", response_model=SolvePointResponse)
def solve_point(req: SolvePointRequest):
    """Compute 3-DOF equilibrium boat speed, heel, leeway, and forces for a single wind condition."""
    try:
        boat = resolve_boat(req.boat, req.preset_name)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    solver = VPPSolver(boat=boat, max_heel_deg=req.max_heel_deg)
    res = solver.solve_point(tws_kts=req.tws_kts, twa_deg=req.twa_deg)

    return SolvePointResponse(
        tws_kts=res.tws_kts,
        twa_deg=res.twa_deg,
        v_boat_kts=res.v_boat_kts,
        v_boat_ms=res.v_boat_ms,
        vmg_kts=res.vmg_kts,
        heel_deg=res.heel_deg,
        leeway_deg=res.leeway_deg,
        sail_set_name=res.sail_set_name,
        flat=res.flat,
        reef=res.reef,
        aws_kts=res.aws_kts,
        awa_deg=res.awa_deg,
        f_x_n=res.f_x_n,
        r_total_n=res.r_total_n,
        r_viscous_n=res.r_viscous_n,
        r_residuary_n=res.r_residuary_n,
        r_induced_n=res.r_induced_n,
        r_heel_n=res.r_heel_n,
        heeling_moment_nm=res.heeling_moment_nm,
        righting_moment_nm=res.righting_moment_nm,
        converged=res.converged,
    )


def get_or_generate_polar_table(req: SolveMatrixRequest) -> PolarTable:
    """Retrieve polar table from precomputed matrix (e.g. uploaded .pol) or solve via 3-DOF VPP."""
    if req.speed_matrix is not None and req.tws_list and req.twa_list:
        speed_table = np.array(req.speed_matrix, dtype=float)
        upwind_targets = {}
        downwind_targets = {}
        for i, tws in enumerate(req.tws_list):
            speeds = speed_table[i, :].tolist()
            up, dn = compute_vmg_targets(float(tws), [float(a) for a in req.twa_list], speeds)
            upwind_targets[float(tws)] = up
            downwind_targets[float(tws)] = dn
        name = req.boat_name or (req.boat.name if req.boat else (req.preset_name or "Custom POL Yacht"))
        return PolarTable(
            boat_name=name,
            tws_list=[float(x) for x in req.tws_list],
            twa_list=[float(x) for x in req.twa_list],
            matrix={},
            speed_table=speed_table,
            upwind_targets=upwind_targets,
            downwind_targets=downwind_targets,
        )

    boat = resolve_boat(req.boat, req.preset_name)
    solver = VPPSolver(boat=boat, max_heel_deg=req.max_heel_deg)
    return generate_polar_table(solver, tws_list=req.tws_list, twa_list=req.twa_list)


@router.post("/solve/matrix", response_model=SolveMatrixResponse)
def solve_matrix(req: SolveMatrixRequest):
    """Compute or format full polar matrix and optimal upwind/downwind VMG targets."""
    try:
        polars = get_or_generate_polar_table(req)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    upwind_targets = {
        str(tws): VMGTargetResponse(
            tws_kts=tgt.tws_kts,
            target_twa_deg=tgt.target_twa_deg,
            target_v_boat_kts=tgt.target_v_boat_kts,
            target_vmg_kts=tgt.target_vmg_kts,
            is_upwind=tgt.is_upwind,
        )
        for tws, tgt in polars.upwind_targets.items()
    }

    downwind_targets = {
        str(tws): VMGTargetResponse(
            tws_kts=tgt.tws_kts,
            target_twa_deg=tgt.target_twa_deg,
            target_v_boat_kts=tgt.target_v_boat_kts,
            target_vmg_kts=tgt.target_vmg_kts,
            is_upwind=tgt.is_upwind,
        )
        for tws, tgt in polars.downwind_targets.items()
    }

    return SolveMatrixResponse(
        boat_name=polars.boat_name,
        tws_list=polars.tws_list,
        twa_list=polars.twa_list,
        speed_matrix=polars.speed_table.tolist(),
        upwind_vmg_targets=upwind_targets,
        downwind_vmg_targets=downwind_targets,
    )


@router.post("/export/orc", response_class=PlainTextResponse)
def export_orc(req: SolveMatrixRequest):
    """Generate standard ORC / OpenCPN / Expedition .pol polar file."""
    try:
        polars = get_or_generate_polar_table(req)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    buf = io.StringIO()
    header = ["twa/tws"] + [f"{tws:.1f}" for tws in polars.tws_list]
    lines = ["\t".join(header)]

    for j, twa in enumerate(polars.twa_list):
        row = [f"{twa:.1f}"]
        for i in range(len(polars.tws_list)):
            row.append(f"{polars.speed_table[i, j]:.2f}")
        lines.append("\t".join(row))

    pol_text = "\n".join(lines) + "\n"
    return PlainTextResponse(
        content=pol_text,
        media_type="text/plain",
        headers={"Content-Disposition": f"attachment; filename={polars.boat_name.replace(' ', '_')}.pol"},
    )


@router.post("/export/csv", response_class=PlainTextResponse)
def export_csv_data(req: SolveMatrixRequest):
    """Export polar results to CSV."""
    try:
        polars = get_or_generate_polar_table(req)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    import csv
    buf = io.StringIO()
    writer = csv.writer(buf)
    writer.writerow(["tws_kts", "twa_deg", "v_boat_kts", "vmg_kts", "heel_deg", "leeway_deg", "sail_set_name", "flat", "reef"])

    for i, tws in enumerate(polars.tws_list):
        for j, twa in enumerate(polars.twa_list):
            res = polars.get_point(tws, twa)
            if res is not None:
                writer.writerow([res.tws_kts, res.twa_deg, f"{res.v_boat_kts:.3f}", f"{res.vmg_kts:.3f}", f"{res.heel_deg:.2f}", f"{res.leeway_deg:.2f}", res.sail_set_name, f"{res.flat:.2f}", f"{res.reef:.2f}"])
            else:
                spd = polars.speed_table[i, j]
                vmg = spd * np.cos(np.deg2rad(twa))
                writer.writerow([tws, twa, f"{spd:.3f}", f"{vmg:.3f}", "0.00", "0.00", "Main+Jib", "1.00", "1.00"])

    return PlainTextResponse(
        content=buf.getvalue(),
        media_type="text/csv",
        headers={"Content-Disposition": f"attachment; filename={polars.boat_name.replace(' ', '_')}.csv"},
    )


@router.post("/plot/polar")
def plot_polar_image(req: SolveMatrixRequest):
    """Generate and return polar performance diagram as PNG image stream."""
    try:
        polars = get_or_generate_polar_table(req)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    fig = plot_polar_diagram(polars, show=False)
    buf = io.BytesIO()
    fig.savefig(buf, format="png", dpi=180, bbox_inches="tight")
    import matplotlib.pyplot as plt
    plt.close(fig)
    buf.seek(0)

    return Response(content=buf.getvalue(), media_type="image/png")


@router.post("/plot/curves")
def plot_curves_image(req: SolveMatrixRequest):
    """Generate and return Cartesian performance curves as PNG image stream."""
    try:
        polars = get_or_generate_polar_table(req)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    fig = plot_performance_curves(polars, show=False)
    buf = io.BytesIO()
    fig.savefig(buf, format="png", dpi=180, bbox_inches="tight")
    import matplotlib.pyplot as plt
    plt.close(fig)
    buf.seek(0)

    return Response(content=buf.getvalue(), media_type="image/png")


@router.post("/plot/resistance")
def plot_resistance_image(
    req: Optional[SolveMatrixRequest] = None,
    heel_deg: float = Query(15.0, description="Heel angle in degrees"),
):
    """Generate and return hydrodynamic resistance breakdown as PNG image stream."""
    boat = resolve_boat(req.boat if req else None, req.preset_name if req else None)

    fig = plot_resistance_breakdown(boat, heel_deg=heel_deg, show=False)
    buf = io.BytesIO()
    fig.savefig(buf, format="png", dpi=180, bbox_inches="tight")
    import matplotlib.pyplot as plt
    plt.close(fig)
    buf.seek(0)

    return Response(content=buf.getvalue(), media_type="image/png")
