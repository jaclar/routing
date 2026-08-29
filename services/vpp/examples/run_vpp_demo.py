"""Demonstration script: Run VPP on the 36-foot Cruising Ketch and export polars and plots."""

import sys
from pathlib import Path

# Ensure root directory is on python path
repo_root = Path(__file__).resolve().parent.parent
if str(repo_root) not in sys.path:
    sys.path.insert(0, str(repo_root))

from vpp import (
    create_36ft_ketch,
    VPPSolver,
    generate_polar_table,
    export_to_csv,
    export_to_orc_pol,
    export_to_json,
    plot_polar_diagram,
    plot_performance_curves,
    plot_resistance_breakdown,
)


def main():
    print("=" * 70)
    print(" Sailboat Velocity Prediction Program (VPP) - 36ft Ketch Demonstration")
    print("=" * 70)

    # 1. Instantiate the 36ft Ketch test boat
    ketch = create_36ft_ketch()
    print(f"\nBoat: {ketch.name}")
    print(f"  Length Overall (LOA): {ketch.hull.loa:.1f} m ({ketch.hull.loa * 3.28084:.1f} ft)")
    print(f"  Length Waterline (LWL): {ketch.hull.lwl:.1f} m ({ketch.hull.lwl * 3.28084:.1f} ft)")
    print(f"  Beam: {ketch.hull.b_max:.2f} m")
    print(f"  Draft: {ketch.hull.draft_total:.2f} m")
    print(f"  Displacement: {ketch.hull.displacement_mass:.0f} kg ({ketch.hull.displacement_volume:.2f} m^3)")
    print(f"  Rig Type: {ketch.rig.rig_type.upper()} (Main + Mizzen + Foretriangle)")

    # 2. Setup VPP Solver
    solver = VPPSolver(boat=ketch, max_heel_deg=26.0)
    print(f"  Upwind Sail Set: {[s.name for s in solver.upwind_sails.sails]} (Total Area: {solver.upwind_sails.total_area:.1f} m^2)")
    print(f"  Downwind Sail Set: {[s.name for s in solver.downwind_sails.sails]} (Total Area: {solver.downwind_sails.total_area:.1f} m^2)")

    # 3. Solve Polar Matrix
    tws_list = [6.0, 8.0, 10.0, 12.0, 14.0, 16.0, 20.0, 25.0]
    twa_list = [30.0, 35.0, 40.0, 45.0, 52.0, 60.0, 70.0, 80.0, 90.0, 110.0, 120.0, 135.0, 150.0, 165.0, 180.0]

    print(f"\nSolving 3-DOF Equilibrium Polar Matrix ({len(tws_list)} TWS x {len(twa_list)} TWA = {len(tws_list)*len(twa_list)} points)...")
    polars = generate_polar_table(solver, tws_list=tws_list, twa_list=twa_list)
    print("✓ Polar solution complete!")

    # 4. Display VMG Targets Table
    print("\n" + "=" * 70)
    print(f"{'TWS [kts]':<10} | {'Opt Beat TWA':<12} | {'Beat Spd':<10} | {'Up VMG':<10} | {'Opt Gybe TWA':<12} | {'Gybe Spd':<10} | {'Down VMG':<10}")
    print("-" * 70)
    for tws in tws_list:
        up = polars.upwind_targets[tws]
        down = polars.downwind_targets[tws]
        print(f"{tws:<10.1f} | {up.target_twa_deg:<12.1f} | {up.target_v_boat_kts:<10.2f} | {up.target_vmg_kts:<10.2f} | {down.target_twa_deg:<12.1f} | {down.target_v_boat_kts:<10.2f} | {down.target_vmg_kts:<10.2f}")
    print("=" * 70)

    # 5. Display Speed Table (ORC format preview)
    print("\nSpeed Matrix [knots]:")
    header = f"{'TWA':<6}" + "".join([f"{tws:>7.0f}k" for tws in tws_list])
    print(header)
    print("-" * len(header))
    for j, twa in enumerate(twa_list):
        row = f"{twa:<6.0f}" + "".join([f"{polars.speed_table[i, j]:>8.2f}" for i in range(len(tws_list))])
        print(row)

    # 6. Export Results
    out_dir = Path("output")
    out_dir.mkdir(exist_ok=True)

    csv_file = out_dir / "ketch36_polars.csv"
    pol_file = out_dir / "ketch36_polars.pol"
    json_file = out_dir / "ketch36_polars.json"
    plot_polar_file = out_dir / "ketch36_polar_diagram.png"
    plot_curves_file = out_dir / "ketch36_performance_curves.png"
    plot_res_file = out_dir / "ketch36_resistance_breakdown.png"

    print("\nExporting files...")
    export_to_csv(polars, csv_file)
    export_to_orc_pol(polars, pol_file)
    export_to_json(polars, json_file)
    print(f"  ✓ Saved CSV: {csv_file}")
    print(f"  ✓ Saved ORC Polar: {pol_file}")
    print(f"  ✓ Saved JSON: {json_file}")

    print("\nGenerating performance plots...")
    plot_polar_diagram(polars, save_path=plot_polar_file)
    plot_performance_curves(polars, save_path=plot_curves_file)
    plot_resistance_breakdown(ketch, save_path=plot_res_file)
    print(f"  ✓ Saved Polar Plot: {plot_polar_file}")
    print(f"  ✓ Saved Cartesian Curves: {plot_curves_file}")
    print(f"  ✓ Saved Resistance Breakdown: {plot_res_file}")

    print("\n✓ VPP Demo completed successfully!")


if __name__ == "__main__":
    main()
