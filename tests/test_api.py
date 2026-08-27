"""Integration and unit tests for the FastAPI VPP service."""

import pytest
from fastapi.testclient import TestClient
from vpp.api.app import app

client = TestClient(app)


def test_health_check():
    response = client.get("/health")
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
    assert data["service"] == "sail-vpp"


def test_root_endpoint():
    response = client.get("/")
    assert response.status_code == 200
    data = response.json()
    assert "docs" in data


def test_list_presets():
    response = client.get("/api/v1/presets")
    assert response.status_code == 200
    presets = response.json()
    assert len(presets) >= 4
    preset_ids = [p["id"] for p in presets]
    assert "36ft-ketch" in preset_ids
    assert "36ft-sloop" in preset_ids


def test_get_preset_detail():
    response = client.get("/api/v1/presets/36ft-ketch")
    assert response.status_code == 200
    boat = response.json()
    assert boat["name"] == "36ft Cruising Ketch"
    assert boat["hull"]["loa"] == 11.0
    assert boat["hull"]["b_max"] == 3.5
    assert boat["hull"]["draft_total"] == 1.5
    assert boat["hull"]["displacement_mass"] == 7000.0
    assert boat["rig"]["rig_type"] == "ketch"


def test_get_preset_not_found():
    response = client.get("/api/v1/presets/nonexistent-boat")
    assert response.status_code == 404


def test_api_solve_point_preset():
    payload = {
        "tws_kts": 14.0,
        "twa_deg": 45.0,
        "preset_name": "36ft-ketch",
    }
    response = client.post("/api/v1/solve/point", json=payload)
    assert response.status_code == 200
    res = response.json()
    assert res["converged"] is True
    assert 5.0 <= res["v_boat_kts"] <= 8.0
    assert res["heel_deg"] > 0.0
    assert res["vmg_kts"] > 0.0
    assert res["r_total_n"] > 0.0


def test_api_solve_point_custom_boat():
    # Fetch 36ft-ketch schema and modify
    preset_res = client.get("/api/v1/presets/36ft-ketch").json()
    preset_res["name"] = "Custom Modified Ketch"
    preset_res["hull"]["displacement_mass"] = 6500.0

    payload = {
        "tws_kts": 12.0,
        "twa_deg": 90.0,
        "boat": preset_res,
    }
    response = client.post("/api/v1/solve/point", json=payload)
    assert response.status_code == 200
    res = response.json()
    assert res["converged"] is True
    assert res["v_boat_kts"] > 5.0


def test_api_solve_matrix():
    payload = {
        "tws_list": [8.0, 14.0],
        "twa_list": [45.0, 90.0, 150.0],
        "preset_name": "36ft-ketch",
    }
    response = client.post("/api/v1/solve/matrix", json=payload)
    assert response.status_code == 200
    data = response.json()
    assert data["boat_name"] == "36ft Cruising Ketch"
    assert len(data["speed_matrix"]) == 2
    assert len(data["speed_matrix"][0]) == 3
    assert "8.0" in data["upwind_vmg_targets"]
    assert "14.0" in data["downwind_vmg_targets"]


def test_api_export_orc():
    payload = {
        "tws_list": [8.0, 14.0],
        "twa_list": [45.0, 90.0, 150.0],
        "preset_name": "36ft-ketch",
    }
    response = client.post("/api/v1/export/orc", json=payload)
    assert response.status_code == 200
    assert response.headers["content-type"].startswith("text/plain")
    content = response.text
    assert "twa/tws" in content
    assert "45.0" in content


def test_api_export_csv():
    payload = {
        "tws_list": [8.0, 14.0],
        "twa_list": [45.0, 90.0, 150.0],
        "preset_name": "36ft-ketch",
    }
    response = client.post("/api/v1/export/csv", json=payload)
    assert response.status_code == 200
    assert response.headers["content-type"].startswith("text/csv")
    content = response.text
    assert "tws_kts,twa_deg,v_boat_kts" in content


def test_api_plot_polar():
    payload = {
        "tws_list": [8.0, 14.0],
        "twa_list": [45.0, 90.0, 150.0],
        "preset_name": "36ft-ketch",
    }
    response = client.post("/api/v1/plot/polar", json=payload)
    assert response.status_code == 200
    assert response.headers["content-type"] == "image/png"
    # PNG signature \x89PNG
    assert response.content[:4] == b"\x89PNG"


def test_api_plot_curves():
    payload = {
        "tws_list": [8.0, 14.0],
        "twa_list": [45.0, 90.0, 150.0],
        "preset_name": "36ft-ketch",
    }
    response = client.post("/api/v1/plot/curves", json=payload)
    assert response.status_code == 200
    assert response.headers["content-type"] == "image/png"
    assert response.content[:4] == b"\x89PNG"


def test_api_plot_resistance():
    response = client.post("/api/v1/plot/resistance?heel_deg=15.0")
    assert response.status_code == 200
    assert response.headers["content-type"] == "image/png"
    assert response.content[:4] == b"\x89PNG"


def test_api_direct_polar_matrix():
    payload = {
        "boat_name": "Direct POL Boat",
        "tws_list": [6.0, 10.0],
        "twa_list": [40.0, 90.0, 140.0],
        "speed_matrix": [
            [4.0, 5.0, 4.5],
            [5.5, 7.0, 6.2],
        ],
    }
    response = client.post("/api/v1/solve/matrix", json=payload)
    assert response.status_code == 200
    data = response.json()
    assert data["boat_name"] == "Direct POL Boat"
    assert data["speed_matrix"] == [[4.0, 5.0, 4.5], [5.5, 7.0, 6.2]]
    assert "6.0" in data["upwind_vmg_targets"]
    assert "10.0" in data["downwind_vmg_targets"]


def test_api_direct_polar_plot():
    payload = {
        "boat_name": "Direct POL Boat",
        "tws_list": [6.0, 10.0],
        "twa_list": [40.0, 90.0, 140.0],
        "speed_matrix": [
            [4.0, 5.0, 4.5],
            [5.5, 7.0, 6.2],
        ],
    }
    response = client.post("/api/v1/plot/polar", json=payload)
    assert response.status_code == 200
    assert response.headers["content-type"] == "image/png"
    assert response.content[:4] == b"\x89PNG"

