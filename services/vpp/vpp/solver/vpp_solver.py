"""Top-level Velocity Prediction Program solver."""

from dataclasses import dataclass
from typing import Optional, Any
import numpy as np

from vpp.core.boat import Boat
from vpp.core.environment import Environment
from vpp.core.units import kts_to_ms, ms_to_kts, deg_to_rad
from vpp.aero.sail import SailSet, create_sails_from_rig
from vpp.solver.optimizer import optimize_sail_trim


@dataclass
class VPPPointResult:
    """Detailed VPP solution for a specific (TWS, TWA) point."""
    tws_kts: float
    twa_deg: float
    v_boat_kts: float
    v_boat_ms: float
    vmg_kts: float            # Velocity Made Good = V_boat * cos(TWA)
    heel_deg: float
    leeway_deg: float
    sail_set_name: str
    flat: float
    reef: float
    aws_kts: float
    awa_deg: float
    f_x_n: float              # Forward drive force [N]
    r_total_n: float          # Total resistance [N]
    r_viscous_n: float        # Frictional resistance [N]
    r_residuary_n: float      # Wave resistance [N]
    r_induced_n: float        # Induced drag [N]
    r_heel_n: float           # Heel added resistance [N]
    heeling_moment_nm: float  # Heeling moment [N*m]
    righting_moment_nm: float # Righting moment [N*m]
    converged: bool


class VPPSolver:
    """Velocity Prediction Program solver managing yacht configurations, physics, and polar matrices."""

    def __init__(
        self,
        boat: Boat,
        upwind_sails: Optional[SailSet] = None,
        downwind_sails: Optional[SailSet] = None,
        env: Environment = Environment(),
        max_heel_deg: float = 28.0,
    ):
        self.boat = boat
        self.env = env
        self.max_heel_rad = deg_to_rad(max_heel_deg)

        if upwind_sails is None or downwind_sails is None:
            default_upwind, default_downwind = create_sails_from_rig(boat.rig)
            self.upwind_sails = upwind_sails or default_upwind
            self.downwind_sails = downwind_sails or default_downwind
        else:
            self.upwind_sails = upwind_sails
            self.downwind_sails = downwind_sails

    def solve_point(
        self,
        tws_kts: float,
        twa_deg: float,
        warm_start_state: Optional[Any] = None,
    ) -> VPPPointResult:
        """Solve equilibrium performance for a single wind speed and wind angle.
        
        Args:
            tws_kts: True Wind Speed in knots.
            twa_deg: True Wind Angle in degrees (0 to 180).
            warm_start_state: Optional initial guess from adjacent point.
            
        Returns:
            VPPPointResult containing full performance metrics.
        """
        if twa_deg < 28.0:
            # In irons / aerodynamic no-go zone (sails cannot generate forward lift)
            return VPPPointResult(
                tws_kts=tws_kts,
                twa_deg=twa_deg,
                v_boat_kts=0.0,
                v_boat_ms=0.0,
                vmg_kts=0.0,
                heel_deg=0.0,
                leeway_deg=0.0,
                sail_set_name="In Irons (No-Go)",
                flat=1.0,
                reef=1.0,
                aws_kts=tws_kts,
                awa_deg=twa_deg,
                f_x_n=0.0,
                r_total_n=0.0,
                r_viscous_n=0.0,
                r_residuary_n=0.0,
                r_induced_n=0.0,
                r_heel_n=0.0,
                heeling_moment_nm=0.0,
                righting_moment_nm=0.0,
                converged=True,
            )

        tws_ms = kts_to_ms(tws_kts)
        twa_rad = deg_to_rad(twa_deg)

        x0 = None
        if warm_start_state is not None and warm_start_state.v_boat_ms > 0.1:
            x0 = (warm_start_state.v_boat_ms, deg_to_rad(warm_start_state.heel_deg), deg_to_rad(warm_start_state.leeway_deg))

        # Always evaluate upwind sail set
        upwind_res = optimize_sail_trim(
            boat=self.boat,
            sail_set=self.upwind_sails,
            tws_ms=tws_ms,
            twa_rad=twa_rad,
            max_heel_rad=self.max_heel_rad,
            x0=x0,
            env=self.env,
        )

        best_res = upwind_res

        # If reaching or running (TWA >= 45 deg), also test downwind sails (Spinnaker)
        if twa_deg >= 45.0:
            downwind_res = optimize_sail_trim(
                boat=self.boat,
                sail_set=self.downwind_sails,
                tws_ms=tws_ms,
                twa_rad=twa_rad,
                max_heel_rad=self.max_heel_rad,
                x0=x0,
                env=self.env,
            )
            # Pick downwind if faster or if upwind failed to converge
            if downwind_res.state.converged and (
                not best_res.state.converged or downwind_res.state.v_boat_ms > best_res.state.v_boat_ms
            ):
                best_res = downwind_res

        st = best_res.state
        v_boat_kts = ms_to_kts(st.v_boat_ms)
        vmg_kts = v_boat_kts * np.cos(twa_rad)

        return VPPPointResult(
            tws_kts=tws_kts,
            twa_deg=twa_deg,
            v_boat_kts=v_boat_kts,
            v_boat_ms=st.v_boat_ms,
            vmg_kts=vmg_kts,
            heel_deg=st.heel_deg,
            leeway_deg=st.leeway_deg,
            sail_set_name=best_res.sail_set_name,
            flat=best_res.flat,
            reef=best_res.reef,
            aws_kts=ms_to_kts(st.aero.apparent_wind.aws),
            awa_deg=st.aero.apparent_wind.awa_deg,
            f_x_n=st.aero.f_x,
            r_total_n=st.hydro.r_total,
            r_viscous_n=st.hydro.r_viscous,
            r_residuary_n=st.hydro.r_residuary,
            r_induced_n=st.hydro.r_induced,
            r_heel_n=st.hydro.r_heel,
            heeling_moment_nm=st.aero.m_x,
            righting_moment_nm=st.hydro.righting_moment,
            converged=st.converged,
        )
