"""Environmental parameters for VPP calculations."""

from dataclasses import dataclass


@dataclass
class Environment:
    """Atmospheric and marine environmental properties."""
    rho_water: float = 1025.0       # Seawater density [kg/m^3]
    rho_air: float = 1.225          # Air density at sea level [kg/m^3]
    nu_water: float = 1.188e-6      # Seawater kinematic viscosity at 15 deg C [m^2/s]
    ref_wind_height: float = 10.0   # Standard reference height for TWS [m]
    wind_shear_exponent: float = 0.142857  # 1/7 power law for atmospheric boundary layer
    g: float = 9.80665              # Gravitational acceleration [m/s^2]

    def wind_speed_at_height(self, tws_10m: float, z: float) -> float:
        """Calculate True Wind Speed at height z above sea level.
        
        Args:
            tws_10m: True wind speed at reference height 10m [m/s or kts].
            z: Height above waterline [m].
            
        Returns:
            Wind speed at height z in the same units as tws_10m.
        """
        z_safe = max(z, 0.5)
        return tws_10m * (z_safe / self.ref_wind_height) ** self.wind_shear_exponent
