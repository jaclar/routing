"""Plotting routines for sailing polar diagrams, Cartesian curves, and resistance breakdowns."""

from pathlib import Path
from typing import Optional, Union, List
import numpy as np
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

from vpp.polars.polar_data import PolarTable
from vpp.core.boat import Boat
from vpp.core.units import kts_to_ms
from vpp.hydro.hydro_model import compute_hydrodynamics


def plot_polar_diagram(
    polar_table: PolarTable,
    save_path: Optional[Union[str, Path]] = None,
    show: bool = False,
    dpi: int = 200,
) -> plt.Figure:
    """Generate a high-quality sailing polar diagram.
    
    Args:
        polar_table: PolarTable instance.
        save_path: Optional path to save image file (e.g. PNG).
        show: If True, calls plt.show().
        dpi: Resolution of exported image.
        
    Returns:
        Matplotlib Figure object.
    """
    fig, ax = plt.subplots(figsize=(9, 10), subplot_kw={"projection": "polar"})

    # Convention: 0 deg at top (Head to wind), clockwise orientation
    ax.set_theta_zero_location("N")
    ax.set_theta_direction(-1)

    # Plot for starboard side (0 to 180 deg) or mirror for full 360 deg
    colors = plt.cm.plasma(np.linspace(0.1, 0.9, len(polar_table.tws_list)))

    upwind_twas = []
    upwind_spds = []
    downwind_twas = []
    downwind_spds = []

    for i, tws in enumerate(polar_table.tws_list):
        twa_rad = np.deg2rad(polar_table.twa_list)
        speeds = polar_table.speed_table[i, :]

        # Mirror for both port and starboard
        twa_full = np.concatenate([-twa_rad[::-1], twa_rad])
        speeds_full = np.concatenate([speeds[::-1], speeds])

        ax.plot(
            twa_full,
            speeds_full,
            label=f"{tws:.0f} kts",
            color=colors[i],
            linewidth=2.2,
        )

        # Collect VMG targets for target lines
        if tws in polar_table.upwind_targets:
            tgt_up = polar_table.upwind_targets[tws]
            upwind_twas.append(np.deg2rad(tgt_up.target_twa_deg))
            upwind_spds.append(tgt_up.target_v_boat_kts)
        if tws in polar_table.downwind_targets:
            tgt_down = polar_table.downwind_targets[tws]
            downwind_twas.append(np.deg2rad(tgt_down.target_twa_deg))
            downwind_spds.append(tgt_down.target_v_boat_kts)

    # Plot optimal VMG lines
    if upwind_twas:
        ax.plot(upwind_twas, upwind_spds, "o--", color="#0055d4", linewidth=1.5, markersize=4.5, label="Upwind VMG")
        # Mirror on port
        ax.plot([-t for t in upwind_twas], upwind_spds, "o--", color="#0055d4", linewidth=1.5, markersize=4.5)

    if downwind_twas:
        ax.plot(downwind_twas, downwind_spds, "s--", color="#d40055", linewidth=1.5, markersize=4.5, label="Downwind VMG")
        # Mirror on port
        ax.plot([-t for t in downwind_twas], downwind_spds, "s--", color="#d40055", linewidth=1.5, markersize=4.5)

    # Visual in-irons zone (< 28 deg) in neutral gray without labels
    max_speed = float(np.nanmax(polar_table.speed_table)) if polar_table.speed_table.size > 0 else 10.0
    if not np.isfinite(max_speed) or max_speed <= 0:
        max_speed = 10.0
    nogo_theta = np.linspace(np.deg2rad(-28.0), np.deg2rad(28.0), 60)
    nogo_r = np.full_like(nogo_theta, max_speed * 1.05)
    ax.fill_between(nogo_theta, 0, nogo_r, color="#64748b", alpha=0.18, zorder=1)

    ax.set_thetagrids(
        np.arange(0, 360, 30),
        labels=["0° (Head)", "30°", "60°", "90° (Beam)", "120°", "150°", "180° (Run)", "150°", "120°", "90°", "60°", "30°"],
    )
    ax.set_title(f"Polar Performance Diagram: {polar_table.boat_name}", fontsize=14, fontweight="bold", pad=20)
    ax.legend(loc="upper right", bbox_to_anchor=(1.25, 1.05), title="True Wind (TWS)", framealpha=0.9)
    ax.grid(True, linestyle=":", alpha=0.6)

    fig.tight_layout()

    if save_path:
        p = Path(save_path)
        p.parent.mkdir(parents=True, exist_ok=True)
        fig.savefig(p, dpi=dpi, bbox_inches="tight")

    if show:
        plt.show()

    return fig


def plot_performance_curves(
    polar_table: PolarTable,
    save_path: Optional[Union[str, Path]] = None,
    show: bool = False,
    dpi: int = 200,
) -> plt.Figure:
    """Generate 4-panel Cartesian performance curves (Speed, Heel, Leeway, VMG vs TWA)."""
    fig, axes = plt.subplots(2, 2, figsize=(13, 9), sharex=True)
    colors = plt.cm.plasma(np.linspace(0.1, 0.9, len(polar_table.tws_list)))

    twa_arr = np.array(polar_table.twa_list)

    for i, tws in enumerate(polar_table.tws_list):
        speeds = []
        heels = []
        leeways = []
        vmgs = []

        for twa in twa_arr:
            res = polar_table.get_point(tws, twa)
            if res is not None:
                speeds.append(res.v_boat_kts)
                heels.append(res.heel_deg)
                leeways.append(res.leeway_deg)
                vmgs.append(res.vmg_kts)
            else:
                speeds.append(np.nan)
                heels.append(np.nan)
                leeways.append(np.nan)
                vmgs.append(np.nan)

        c = colors[i]
        lbl = f"{tws:.0f} kts"

        # 1. Boat speed
        axes[0, 0].plot(twa_arr, speeds, label=lbl, color=c, linewidth=2.0)
        # 2. Heel angle
        axes[0, 1].plot(twa_arr, heels, label=lbl, color=c, linewidth=2.0)
        # 3. Leeway angle
        axes[1, 0].plot(twa_arr, leeways, label=lbl, color=c, linewidth=2.0)
        # 4. VMG
        axes[1, 1].plot(twa_arr, vmgs, label=lbl, color=c, linewidth=2.0)

    # Subplot styling
    for r in range(2):
        for col in range(2):
            axes[r, col].axvspan(0, 28, color="#ef4444", alpha=0.08)

    axes[0, 0].set_ylabel("Boat Speed [kts]", fontsize=11, fontweight="bold")
    axes[0, 0].set_title("Boat Speed vs TWA", fontsize=12)
    axes[0, 0].grid(True, linestyle=":", alpha=0.6)
    axes[0, 0].legend(title="TWS", fontsize=9, loc="lower right")

    axes[0, 1].set_ylabel("Heel Angle [deg]", fontsize=11, fontweight="bold")
    axes[0, 1].set_title("Heel Angle vs TWA", fontsize=12)
    axes[0, 1].grid(True, linestyle=":", alpha=0.6)

    axes[1, 0].set_xlabel("True Wind Angle (TWA) [deg]", fontsize=11, fontweight="bold")
    axes[1, 0].set_ylabel("Leeway Angle [deg]", fontsize=11, fontweight="bold")
    axes[1, 0].set_title("Leeway Angle vs TWA", fontsize=12)
    axes[1, 0].grid(True, linestyle=":", alpha=0.6)

    axes[1, 1].set_xlabel("True Wind Angle (TWA) [deg]", fontsize=11, fontweight="bold")
    axes[1, 1].set_ylabel("Velocity Made Good (VMG) [kts]", fontsize=11, fontweight="bold")
    axes[1, 1].set_title("VMG vs TWA (+Upwind / -Downwind)", fontsize=12)
    axes[1, 1].axhline(0, color="gray", linestyle="--", alpha=0.5)
    axes[1, 1].grid(True, linestyle=":", alpha=0.6)

    fig.suptitle(f"Performance Curves: {polar_table.boat_name}", fontsize=14, fontweight="bold")
    fig.tight_layout()

    if save_path:
        p = Path(save_path)
        p.parent.mkdir(parents=True, exist_ok=True)
        fig.savefig(p, dpi=dpi, bbox_inches="tight")

    if show:
        plt.show()

    return fig


def plot_resistance_breakdown(
    boat: Boat,
    speeds_kts: Optional[List[float]] = None,
    heel_deg: float = 15.0,
    save_path: Optional[Union[str, Path]] = None,
    show: bool = False,
    dpi: int = 200,
) -> plt.Figure:
    """Generate resistance component breakdown vs boat speed."""
    if speeds_kts is None:
        v_hull_nominal = (0.40 * np.sqrt(9.81 * boat.hull.lwl)) * (3600.0 / 1852.0)
        max_spd = max(v_hull_nominal * 1.5, 12.0)
        speeds_kts = list(np.linspace(1.0, max_spd, 50))

    r_visc = []
    r_wave = []
    r_ind = []
    r_heel = []
    r_tot = []

    heel_rad = np.deg2rad(heel_deg)

    for spd in speeds_kts:
        v_ms = kts_to_ms(spd)
        # Representative lateral force scaling with speed
        side_force = 0.5 * 1025.0 * (v_ms ** 2) * boat.appendages.total_lateral_area * np.sin(np.deg2rad(3.0))
        hydro = compute_hydrodynamics(boat, v_ms, heel_rad, side_force)

        r_visc.append(hydro.r_viscous)
        r_wave.append(hydro.r_residuary)
        r_ind.append(hydro.r_induced)
        r_heel.append(hydro.r_heel)
        r_tot.append(hydro.r_total)

    fig, ax = plt.subplots(figsize=(10, 6))

    ax.plot(speeds_kts, r_tot, "k-", linewidth=2.5, label="Total Resistance")
    ax.plot(speeds_kts, r_visc, "b--", linewidth=1.8, label="Viscous Friction (Rv)")
    ax.plot(speeds_kts, r_wave, "r--", linewidth=1.8, label="Wave-Making / Residuary (Rr)")
    ax.plot(speeds_kts, r_ind, "g--", linewidth=1.8, label="Induced Drag (Ri)")
    ax.plot(speeds_kts, r_heel, "m--", linewidth=1.8, label="Heel Added Drag (Rheel)")

    # Hull speed line Fn = 0.40 -> V = 0.40 * sqrt(g * LWL) in knots
    hull_speed_kts = (0.40 * np.sqrt(9.81 * boat.hull.lwl)) * (3600.0 / 1852.0)
    ax.axvline(hull_speed_kts, color="gray", linestyle=":", label=f"Hull Speed ({hull_speed_kts:.1f} kts)")

    ax.set_xlabel("Boat Speed [kts]", fontsize=11, fontweight="bold")
    ax.set_ylabel("Resistance [N]", fontsize=11, fontweight="bold")
    ax.set_title(f"Hydrodynamic Resistance Breakdown: {boat.name} (Heel = {heel_deg}°)", fontsize=13, fontweight="bold")
    ax.grid(True, linestyle=":", alpha=0.6)
    ax.legend(loc="upper left")

    fig.tight_layout()

    if save_path:
        p = Path(save_path)
        p.parent.mkdir(parents=True, exist_ok=True)
        fig.savefig(p, dpi=dpi, bbox_inches="tight")

    if show:
        plt.show()

    return fig
