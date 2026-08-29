"""Unit tests for polar matrix generation, VMG targets, and file export."""

import os
import tempfile
from vpp.presets.boats import create_36ft_ketch
from vpp.solver.vpp_solver import VPPSolver
from vpp.polars.polar_data import generate_polar_table
from vpp.polars.exporter import export_to_csv, export_to_orc_pol, export_to_json


def test_polar_table_generation():
    ketch = create_36ft_ketch()
    solver = VPPSolver(ketch)

    tws_list = [8.0, 14.0]
    twa_list = [40.0, 60.0, 90.0, 135.0, 160.0]

    polars = generate_polar_table(solver, tws_list=tws_list, twa_list=twa_list)
    assert polars.boat_name == "36ft Cruising Ketch"
    assert polars.speed_table.shape == (2, 5)

    # Check that 14 kts wind yields higher speeds than 8 kts wind
    for j in range(len(twa_list)):
        assert polars.speed_table[1, j] > polars.speed_table[0, j]

    # Check VMG targets
    assert 8.0 in polars.upwind_targets
    assert 8.0 in polars.downwind_targets
    tgt_up = polars.upwind_targets[8.0]
    assert tgt_up.target_vmg_kts > 0.0
    assert 35.0 <= tgt_up.target_twa_deg <= 55.0

    # Bilinear interpolation
    spd_interp = polars.interpolate_speed(11.0, 75.0)
    assert polars.speed_table[0, 2] < spd_interp < polars.speed_table[1, 2] + 2.0


def test_polar_exporters():
    ketch = create_36ft_ketch()
    solver = VPPSolver(ketch)
    polars = generate_polar_table(solver, tws_list=[8.0, 14.0], twa_list=[45.0, 90.0, 150.0])

    with tempfile.TemporaryDirectory() as tmpdir:
        csv_path = os.path.join(tmpdir, "test_polar.csv")
        pol_path = os.path.join(tmpdir, "test_polar.pol")
        json_path = os.path.join(tmpdir, "test_polar.json")

        export_to_csv(polars, csv_path)
        export_to_orc_pol(polars, pol_path)
        export_to_json(polars, json_path)

        assert os.path.exists(csv_path) and os.path.getsize(csv_path) > 0
        assert os.path.exists(pol_path) and os.path.getsize(pol_path) > 0
        assert os.path.exists(json_path) and os.path.getsize(json_path) > 0

        # Verify ORC pol format content
        with open(pol_path, "r") as f:
            lines = f.readlines()
            assert "twa/tws" in lines[0]
            assert len(lines) == 4  # 1 header + 3 twa rows
