"""Exporters for polar data to CSV, JSON, and standard ORC/OpenCPN/Expedition .pol formats."""

import csv
import json
from pathlib import Path
from typing import Union
from vpp.polars.polar_data import PolarTable


def export_to_csv(polar_table: PolarTable, filepath: Union[str, Path]) -> None:
    """Export detailed polar point results to a CSV file.
    
    Args:
        polar_table: PolarTable instance.
        filepath: Destination file path.
    """
    path = Path(filepath)
    path.parent.mkdir(parents=True, exist_ok=True)

    fieldnames = [
        "tws_kts",
        "twa_deg",
        "v_boat_kts",
        "vmg_kts",
        "heel_deg",
        "leeway_deg",
        "sail_set_name",
        "flat",
        "reef",
        "aws_kts",
        "awa_deg",
        "f_x_n",
        "r_total_n",
        "r_viscous_n",
        "r_residuary_n",
        "r_induced_n",
        "r_heel_n",
        "heeling_moment_nm",
        "righting_moment_nm",
        "converged",
    ]

    with open(path, mode="w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        for tws in polar_table.tws_list:
            for twa in polar_table.twa_list:
                res = polar_table.get_point(tws, twa)
                if res is not None:
                    writer.writerow({
                        "tws_kts": f"{res.tws_kts:.1f}",
                        "twa_deg": f"{res.twa_deg:.1f}",
                        "v_boat_kts": f"{res.v_boat_kts:.3f}",
                        "vmg_kts": f"{res.vmg_kts:.3f}",
                        "heel_deg": f"{res.heel_deg:.2f}",
                        "leeway_deg": f"{res.leeway_deg:.2f}",
                        "sail_set_name": res.sail_set_name,
                        "flat": f"{res.flat:.2f}",
                        "reef": f"{res.reef:.2f}",
                        "aws_kts": f"{res.aws_kts:.2f}",
                        "awa_deg": f"{res.awa_deg:.2f}",
                        "f_x_n": f"{res.f_x_n:.1f}",
                        "r_total_n": f"{res.r_total_n:.1f}",
                        "r_viscous_n": f"{res.r_viscous_n:.1f}",
                        "r_residuary_n": f"{res.r_residuary_n:.1f}",
                        "r_induced_n": f"{res.r_induced_n:.1f}",
                        "r_heel_n": f"{res.r_heel_n:.1f}",
                        "heeling_moment_nm": f"{res.heeling_moment_nm:.1f}",
                        "righting_moment_nm": f"{res.righting_moment_nm:.1f}",
                        "converged": res.converged,
                    })


def export_to_orc_pol(polar_table: PolarTable, filepath: Union[str, Path], delimiter: str = "\t") -> None:
    """Export polar table to standard ORC / OpenCPN / Expedition .pol format.
    
    Format:
    twa/tws  6.0  8.0  10.0 ...
    30.0     3.8  4.7  5.4  ...
    ...
    """
    path = Path(filepath)
    path.parent.mkdir(parents=True, exist_ok=True)

    header = ["twa/tws"] + [f"{tws:.1f}" for tws in polar_table.tws_list]
    lines = [delimiter.join(header)]

    for j, twa in enumerate(polar_table.twa_list):
        row = [f"{twa:.1f}"]
        for i in range(len(polar_table.tws_list)):
            spd = polar_table.speed_table[i, j]
            row.append(f"{spd:.2f}")
        lines.append(delimiter.join(row))

    with open(path, mode="w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")


def export_to_json(polar_table: PolarTable, filepath: Union[str, Path]) -> None:
    """Export polar table and VMG targets to JSON format."""
    path = Path(filepath)
    path.parent.mkdir(parents=True, exist_ok=True)

    upwind_data = {}
    for tws, tgt in polar_table.upwind_targets.items():
        upwind_data[str(tws)] = {
            "target_twa_deg": tgt.target_twa_deg,
            "target_v_boat_kts": tgt.target_v_boat_kts,
            "target_vmg_kts": tgt.target_vmg_kts,
        }

    downwind_data = {}
    for tws, tgt in polar_table.downwind_targets.items():
        downwind_data[str(tws)] = {
            "target_twa_deg": tgt.target_twa_deg,
            "target_v_boat_kts": tgt.target_v_boat_kts,
            "target_vmg_kts": tgt.target_vmg_kts,
        }

    data = {
        "boat_name": polar_table.boat_name,
        "tws_list": polar_table.tws_list,
        "twa_list": polar_table.twa_list,
        "speed_matrix": polar_table.speed_table.tolist(),
        "upwind_vmg_targets": upwind_data,
        "downwind_vmg_targets": downwind_data,
    }

    with open(path, mode="w", encoding="utf-8") as f:
        json.dump(data, f, indent=2)
