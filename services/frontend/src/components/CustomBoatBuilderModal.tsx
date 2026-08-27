import React, { useState } from 'react';
import { BoatDetail, BoatPreset, SolveMatrixResponse } from '../types';
import { fetchPolarMatrix } from '../services/api';
import {
  Anchor,
  Gauge,
  Layers,
  Ruler,
  Save,
  Wind,
  X,
  Sparkles,
  RefreshCw,
  Info,
} from 'lucide-react';

interface CustomBoatBuilderModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSaveBoat: (boatPreset: BoatPreset) => void;
  initialBoat?: BoatDetail;
  builtinPresets: BoatPreset[];
}

const DEFAULT_SLOOP: BoatDetail = {
  name: 'Custom 38ft Cruiser',
  hull: {
    loa: 11.5,
    lwl: 9.8,
    b_max: 3.65,
    b_wl: 3.15,
    draft_canoe: 0.55,
    draft_total: 2.1,
    displacement_mass: 6500,
    prismatic_coef: 0.56,
    form_factor_k: 0.12,
    lcb_fraction: 0.52,
  },
  appendages: {
    keel_type: 'fin_bulb',
    keel_area: 1.85,
    keel_span: 1.55,
    rudder_area: 0.8,
    rudder_span: 1.3,
  },
  rig: {
    rig_type: 'sloop',
    main_p: 14.0,
    main_e: 4.6,
    fore_i: 14.8,
    fore_j: 4.4,
    mast_height_above_water: 17.2,
    boom_height_above_water: 1.9,
  },
  stability: {
    gmt: 1.15,
    crew_mass: 400,
    crew_hiking_distance: 1.6,
    crew_hiking_fraction: 0.75,
  },
};

export const CustomBoatBuilderModal: React.FC<CustomBoatBuilderModalProps> = ({
  isOpen,
  onClose,
  onSaveBoat,
  initialBoat,
  builtinPresets,
}) => {
  const [boat, setBoat] = useState<BoatDetail>(() => initialBoat || DEFAULT_SLOOP);
  const [activeTab, setActiveTab] = useState<'hull' | 'appendages' | 'rig' | 'stability'>('hull');
  const [calculating, setCalculating] = useState<boolean>(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  if (!isOpen) return null;

  // Clone from built-in preset template
  const handleLoadTemplate = async (presetId: string) => {
    try {
      setCalculating(true);
      const res = await fetch(`/api/v1/presets/${encodeURIComponent(presetId)}`);
      if (!res.ok) throw new Error('Failed to load preset template');
      const data: BoatDetail = await res.json();
      setBoat({
        ...data,
        name: `Custom ${data.name}`,
      });
      setErrorMsg(null);
    } catch (err: any) {
      setErrorMsg(`Failed to load preset: ${err.message}`);
    } finally {
      setCalculating(false);
    }
  };

  // Sail Area calculations
  const mainArea = 0.5 * boat.rig.main_p * boat.rig.main_e;
  const jibArea = 0.5 * boat.rig.fore_i * boat.rig.fore_j;
  const mizzenArea =
    boat.rig.rig_type === 'ketch' && boat.rig.mizzen_p && boat.rig.mizzen_e
      ? 0.5 * boat.rig.mizzen_p * boat.rig.mizzen_e
      : 0;
  const totalUpwindArea = mainArea + jibArea + mizzenArea;

  // Hydrostatic indicators
  const displM3 = boat.hull.displacement_mass / 1025.0;
  const l_d_ratio = displM3 > 0 ? (boat.hull.lwl / Math.cbrt(displM3)).toFixed(2) : '-';
  const sa_d_ratio =
    displM3 > 0 ? (totalUpwindArea / Math.pow(displM3, 2 / 3)).toFixed(1) : '-';

  const handleSaveAndCalculate = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMsg(null);
    setCalculating(true);

    try {
      // Calculate full polar matrix via VPP solver
      const matrix: SolveMatrixResponse = await fetchPolarMatrix(boat);

      const customId = `custom-${Date.now()}`;
      const preset: BoatPreset = {
        id: customId,
        name: boat.name.trim() || 'Custom Yacht',
        loa_m: boat.hull.loa,
        beam_m: boat.hull.b_max,
        draft_m: boat.hull.draft_total,
        displacement_kg: boat.hull.displacement_mass,
        rig_type: boat.rig.rig_type,
        isCustom: true,
        customBoat: boat,
        polarData: matrix,
      };

      onSaveBoat(preset);
      onClose();
    } catch (err: any) {
      console.error('VPP solve failed for custom boat:', err);
      setErrorMsg(err.message || 'Failed to solve polar matrix. Please check parameters.');
    } finally {
      setCalculating(false);
    }
  };

  return (
    <div className="custom-modal-backdrop">
      <div className="custom-modal-card">
        {/* Header */}
        <div className="custom-modal-header">
          <div className="custom-modal-title">
            <Anchor size={20} className="text-accent" />
            <div>
              <h3>Custom Sailboat Polar Builder</h3>
              <p>Design vessel dimensions and solve 3-DOF aerohydrodynamic polars</p>
            </div>
          </div>
          <button type="button" className="btn-modal-close" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        {/* Template Quick Loader */}
        <div className="template-loader-bar">
          <Sparkles size={15} color="#38bdf8" />
          <span className="template-label">Start from base template:</span>
          <select
            className="template-select"
            defaultValue=""
            onChange={(e) => {
              if (e.target.value) handleLoadTemplate(e.target.value);
            }}
          >
            <option value="" disabled>
              Select preset to clone...
            </option>
            {builtinPresets.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} ({p.loa_m}m / {p.displacement_kg}kg)
              </option>
            ))}
          </select>
        </div>

        {errorMsg && (
          <div className="custom-modal-error">
            <Info size={16} />
            <span>{errorMsg}</span>
          </div>
        )}

        <form onSubmit={handleSaveAndCalculate} className="custom-modal-body">
          {/* Top General Name & Rig */}
          <div className="form-row-general">
            <div className="input-group flex-2">
              <label>Yacht Name</label>
              <input
                type="text"
                required
                className="input-field"
                value={boat.name}
                onChange={(e) => setBoat({ ...boat, name: e.target.value })}
                placeholder="e.g. My 38ft Performance Cruiser"
              />
            </div>

            <div className="input-group flex-1">
              <label>Rig Configuration</label>
              <select
                className="select-field"
                value={boat.rig.rig_type}
                onChange={(e) =>
                  setBoat({
                    ...boat,
                    rig: { ...boat.rig, rig_type: e.target.value },
                  })
                }
              >
                <option value="sloop">Sloop (Main + Jib)</option>
                <option value="ketch">Ketch (Main + Jib + Mizzen)</option>
                <option value="cutter">Cutter (Main + Staysail + Yankee)</option>
              </select>
            </div>
          </div>

          {/* Section Tabs */}
          <div className="custom-modal-tabs">
            <button
              type="button"
              className={`modal-tab-btn ${activeTab === 'hull' ? 'active' : ''}`}
              onClick={() => setActiveTab('hull')}
            >
              <Ruler size={15} />
              <span>1. Hull Geometry</span>
            </button>
            <button
              type="button"
              className={`modal-tab-btn ${activeTab === 'appendages' ? 'active' : ''}`}
              onClick={() => setActiveTab('appendages')}
            >
              <Layers size={15} />
              <span>2. Keel & Rudder</span>
            </button>
            <button
              type="button"
              className={`modal-tab-btn ${activeTab === 'rig' ? 'active' : ''}`}
              onClick={() => setActiveTab('rig')}
            >
              <Wind size={15} />
              <span>3. Rig & Sail Plan</span>
            </button>
            <button
              type="button"
              className={`modal-tab-btn ${activeTab === 'stability' ? 'active' : ''}`}
              onClick={() => setActiveTab('stability')}
            >
              <Gauge size={15} />
              <span>4. Stability & Crew</span>
            </button>
          </div>

          {/* Tab 1: Hull Geometry */}
          {activeTab === 'hull' && (
            <div className="tab-pane">
              <div className="form-grid-3">
                <div className="input-group">
                  <label>Length Overall LOA [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="3.0"
                    max="45.0"
                    required
                    className="input-field"
                    value={boat.hull.loa}
                    onChange={(e) => {
                      const loa = parseFloat(e.target.value) || 0;
                      setBoat({
                        ...boat,
                        hull: {
                          ...boat.hull,
                          loa,
                          lwl: Number((loa * 0.85).toFixed(2)),
                        },
                      });
                    }}
                  />
                  <span className="input-hint">{(boat.hull.loa * 3.28084).toFixed(1)} ft</span>
                </div>

                <div className="input-group">
                  <label>Waterline Length LWL [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="2.5"
                    max="42.0"
                    required
                    className="input-field"
                    value={boat.hull.lwl}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        hull: { ...boat.hull, lwl: parseFloat(e.target.value) || 0 },
                      })
                    }
                  />
                  <span className="input-hint">{(boat.hull.lwl * 3.28084).toFixed(1)} ft</span>
                </div>

                <div className="input-group">
                  <label>Displacement Mass [kg]</label>
                  <input
                    type="number"
                    step="50"
                    min="200"
                    max="150000"
                    required
                    className="input-field"
                    value={boat.hull.displacement_mass}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        hull: {
                          ...boat.hull,
                          displacement_mass: parseFloat(e.target.value) || 0,
                        },
                      })
                    }
                  />
                  <span className="input-hint">
                    {(boat.hull.displacement_mass / 1000).toFixed(2)} tonnes /{' '}
                    {(boat.hull.displacement_mass * 2.20462).toFixed(0)} lbs
                  </span>
                </div>

                <div className="input-group">
                  <label>Max Beam B_max [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="1.0"
                    max="15.0"
                    required
                    className="input-field"
                    value={boat.hull.b_max}
                    onChange={(e) => {
                      const b_max = parseFloat(e.target.value) || 0;
                      setBoat({
                        ...boat,
                        hull: {
                          ...boat.hull,
                          b_max,
                          b_wl: Number((b_max * 0.88).toFixed(2)),
                        },
                      });
                    }}
                  />
                  <span className="input-hint">{(boat.hull.b_max * 3.28084).toFixed(1)} ft</span>
                </div>

                <div className="input-group">
                  <label>Waterline Beam B_wl [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.8"
                    max="14.0"
                    required
                    className="input-field"
                    value={boat.hull.b_wl}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        hull: { ...boat.hull, b_wl: parseFloat(e.target.value) || 0 },
                      })
                    }
                  />
                  <span className="input-hint">{(boat.hull.b_wl * 3.28084).toFixed(1)} ft</span>
                </div>

                <div className="input-group">
                  <label>Total Draft with Keel [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.5"
                    max="6.0"
                    required
                    className="input-field"
                    value={boat.hull.draft_total}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        hull: {
                          ...boat.hull,
                          draft_total: parseFloat(e.target.value) || 0,
                        },
                      })
                    }
                  />
                  <span className="input-hint">{(boat.hull.draft_total * 3.28084).toFixed(1)} ft</span>
                </div>

                <div className="input-group">
                  <label>Canoe Body Draft [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.2"
                    max="2.5"
                    required
                    className="input-field"
                    value={boat.hull.draft_canoe}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        hull: {
                          ...boat.hull,
                          draft_canoe: parseFloat(e.target.value) || 0,
                        },
                      })
                    }
                  />
                  <span className="input-hint">Hull depth excluding keel</span>
                </div>

                <div className="input-group">
                  <label>Prismatic Coef (Cp)</label>
                  <input
                    type="number"
                    step="0.01"
                    min="0.48"
                    max="0.65"
                    required
                    className="input-field"
                    value={boat.hull.prismatic_coef}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        hull: {
                          ...boat.hull,
                          prismatic_coef: parseFloat(e.target.value) || 0.56,
                        },
                      })
                    }
                  />
                  <span className="input-hint">Fineness of ends (0.52–0.58)</span>
                </div>

                <div className="input-group">
                  <label>Form Factor (1+k)</label>
                  <input
                    type="number"
                    step="0.01"
                    min="0.05"
                    max="0.25"
                    required
                    className="input-field"
                    value={boat.hull.form_factor_k}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        hull: {
                          ...boat.hull,
                          form_factor_k: parseFloat(e.target.value) || 0.12,
                        },
                      })
                    }
                  />
                  <span className="input-hint">3D viscous drag modifier</span>
                </div>
              </div>

              {/* Hydro ratios summary */}
              <div className="spec-summary-banner">
                <div>
                  <span className="banner-label">Displacement Volume</span>
                  <span className="banner-val">{displM3.toFixed(2)} m³</span>
                </div>
                <div>
                  <span className="banner-label">L/D Ratio</span>
                  <span className="banner-val">{l_d_ratio}</span>
                </div>
                <div>
                  <span className="banner-label">Beam/Draft</span>
                  <span className="banner-val">
                    {(boat.hull.b_wl / Math.max(boat.hull.draft_canoe, 0.1)).toFixed(2)}
                  </span>
                </div>
              </div>
            </div>
          )}

          {/* Tab 2: Appendages */}
          {activeTab === 'appendages' && (
            <div className="tab-pane">
              <div className="form-grid-3">
                <div className="input-group">
                  <label>Keel Profile / Type</label>
                  <select
                    className="select-field"
                    value={boat.appendages.keel_type}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        appendages: {
                          ...boat.appendages,
                          keel_type: e.target.value,
                        },
                      })
                    }
                  >
                    <option value="fin">Fin Keel</option>
                    <option value="fin_bulb">Fin with Bulb</option>
                    <option value="bulb">T-Bulb Performance</option>
                    <option value="long_fin">Cruising Long Fin</option>
                    <option value="full_keel">Full Traditional Keel</option>
                  </select>
                </div>

                <div className="input-group">
                  <label>Keel Lateral Area [m²]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.3"
                    max="10.0"
                    required
                    className="input-field"
                    value={boat.appendages.keel_area}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        appendages: {
                          ...boat.appendages,
                          keel_area: parseFloat(e.target.value) || 0,
                        },
                      })
                    }
                  />
                  <span className="input-hint">Profile planform area</span>
                </div>

                <div className="input-group">
                  <label>Keel Span / Depth [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.3"
                    max="5.0"
                    required
                    className="input-field"
                    value={boat.appendages.keel_span}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        appendages: {
                          ...boat.appendages,
                          keel_span: parseFloat(e.target.value) || 0,
                        },
                      })
                    }
                  />
                  <span className="input-hint">Foil span from canoe body</span>
                </div>

                <div className="input-group">
                  <label>Rudder Lateral Area [m²]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.2"
                    max="5.0"
                    required
                    className="input-field"
                    value={boat.appendages.rudder_area}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        appendages: {
                          ...boat.appendages,
                          rudder_area: parseFloat(e.target.value) || 0,
                        },
                      })
                    }
                  />
                </div>

                <div className="input-group">
                  <label>Rudder Span [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.2"
                    max="4.0"
                    required
                    className="input-field"
                    value={boat.appendages.rudder_span}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        appendages: {
                          ...boat.appendages,
                          rudder_span: parseFloat(e.target.value) || 0,
                        },
                      })
                    }
                  />
                </div>
              </div>
            </div>
          )}

          {/* Tab 3: Rig & Sail Plan */}
          {activeTab === 'rig' && (
            <div className="tab-pane">
              <div className="form-grid-3">
                <div className="input-group">
                  <label>Mainsail Luff P [m]</label>
                  <input
                    type="number"
                    step="0.1"
                    min="4.0"
                    max="35.0"
                    required
                    className="input-field"
                    value={boat.rig.main_p}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        rig: { ...boat.rig, main_p: parseFloat(e.target.value) || 0 },
                      })
                    }
                  />
                </div>

                <div className="input-group">
                  <label>Mainsail Foot E [m]</label>
                  <input
                    type="number"
                    step="0.1"
                    min="1.5"
                    max="15.0"
                    required
                    className="input-field"
                    value={boat.rig.main_e}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        rig: { ...boat.rig, main_e: parseFloat(e.target.value) || 0 },
                      })
                    }
                  />
                </div>

                <div className="input-group">
                  <label>Foretriangle Height I [m]</label>
                  <input
                    type="number"
                    step="0.1"
                    min="4.0"
                    max="38.0"
                    required
                    className="input-field"
                    value={boat.rig.fore_i}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        rig: { ...boat.rig, fore_i: parseFloat(e.target.value) || 0 },
                      })
                    }
                  />
                </div>

                <div className="input-group">
                  <label>Foretriangle Base J [m]</label>
                  <input
                    type="number"
                    step="0.1"
                    min="1.5"
                    max="14.0"
                    required
                    className="input-field"
                    value={boat.rig.fore_j}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        rig: { ...boat.rig, fore_j: parseFloat(e.target.value) || 0 },
                      })
                    }
                  />
                </div>

                <div className="input-group">
                  <label>Masthead Height (Air Draft) [m]</label>
                  <input
                    type="number"
                    step="0.1"
                    min="5.0"
                    max="45.0"
                    required
                    className="input-field"
                    value={boat.rig.mast_height_above_water}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        rig: {
                          ...boat.rig,
                          mast_height_above_water: parseFloat(e.target.value) || 0,
                        },
                      })
                    }
                  />
                </div>

                <div className="input-group">
                  <label>Boom Height Above DWL [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.5"
                    max="4.0"
                    required
                    className="input-field"
                    value={boat.rig.boom_height_above_water}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        rig: {
                          ...boat.rig,
                          boom_height_above_water: parseFloat(e.target.value) || 0,
                        },
                      })
                    }
                  />
                </div>
              </div>

              {/* Ketch Specific Inputs */}
              {boat.rig.rig_type === 'ketch' && (
                <div
                  style={{
                    marginTop: '12px',
                    padding: '12px',
                    background: 'rgba(15, 23, 42, 0.4)',
                    border: '1px solid var(--border-color)',
                    borderRadius: '8px',
                  }}
                >
                  <h4 style={{ fontSize: '0.8rem', color: '#38bdf8', marginBottom: '8px' }}>
                    Ketch Mizzen Mast Dimensions
                  </h4>
                  <div className="form-grid-3">
                    <div className="input-group">
                      <label>Mizzen Luff P [m]</label>
                      <input
                        type="number"
                        step="0.1"
                        min="2.0"
                        max="25.0"
                        className="input-field"
                        value={boat.rig.mizzen_p || 7.5}
                        onChange={(e) =>
                          setBoat({
                            ...boat,
                            rig: {
                              ...boat.rig,
                              mizzen_p: parseFloat(e.target.value) || 0,
                            },
                          })
                        }
                      />
                    </div>

                    <div className="input-group">
                      <label>Mizzen Foot E [m]</label>
                      <input
                        type="number"
                        step="0.1"
                        min="1.0"
                        max="10.0"
                        className="input-field"
                        value={boat.rig.mizzen_e || 2.8}
                        onChange={(e) =>
                          setBoat({
                            ...boat,
                            rig: {
                              ...boat.rig,
                              mizzen_e: parseFloat(e.target.value) || 0,
                            },
                          })
                        }
                      />
                    </div>

                    <div className="input-group">
                      <label>Mizzen Mast Height [m]</label>
                      <input
                        type="number"
                        step="0.1"
                        min="3.0"
                        max="28.0"
                        className="input-field"
                        value={boat.rig.mizzen_mast_height || 9.5}
                        onChange={(e) =>
                          setBoat({
                            ...boat,
                            rig: {
                              ...boat.rig,
                              mizzen_mast_height: parseFloat(e.target.value) || 0,
                            },
                          })
                        }
                      />
                    </div>
                  </div>
                </div>
              )}

              {/* Sail Area Breakdown */}
              <div className="spec-summary-banner" style={{ marginTop: '12px' }}>
                <div>
                  <span className="banner-label">Mainsail Area</span>
                  <span className="banner-val">{mainArea.toFixed(1)} m²</span>
                </div>
                <div>
                  <span className="banner-label">Foretriangle Area</span>
                  <span className="banner-val">{jibArea.toFixed(1)} m²</span>
                </div>
                {boat.rig.rig_type === 'ketch' && (
                  <div>
                    <span className="banner-label">Mizzen Area</span>
                    <span className="banner-val">{mizzenArea.toFixed(1)} m²</span>
                  </div>
                )}
                <div>
                  <span className="banner-label">Total Upwind Area</span>
                  <span className="banner-val font-bold" style={{ color: '#38bdf8' }}>
                    {totalUpwindArea.toFixed(1)} m²
                  </span>
                </div>
                <div>
                  <span className="banner-label">SA/D Ratio</span>
                  <span className="banner-val">{sa_d_ratio}</span>
                </div>
              </div>
            </div>
          )}

          {/* Tab 4: Stability & Crew */}
          {activeTab === 'stability' && (
            <div className="tab-pane">
              <div className="form-grid-2">
                <div className="input-group">
                  <label>Transverse Metacentric Height (GM_T) [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.5"
                    max="2.5"
                    required
                    className="input-field"
                    value={boat.stability.gmt}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        stability: {
                          ...boat.stability,
                          gmt: parseFloat(e.target.value) || 1.1,
                        },
                      })
                    }
                  />
                  <span className="input-hint">Initial upright stability (0.9–1.4m)</span>
                </div>

                <div className="input-group">
                  <label>Total Crew Mass [kg]</label>
                  <input
                    type="number"
                    step="25"
                    min="75"
                    max="1500"
                    required
                    className="input-field"
                    value={boat.stability.crew_mass}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        stability: {
                          ...boat.stability,
                          crew_mass: parseFloat(e.target.value) || 350,
                        },
                      })
                    }
                  />
                  <span className="input-hint">
                    ~{(boat.stability.crew_mass / 80).toFixed(0)} crew members on board
                  </span>
                </div>

                <div className="input-group">
                  <label>Crew Hiking Leverage Distance [m]</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.5"
                    max="5.0"
                    required
                    className="input-field"
                    value={boat.stability.crew_hiking_distance}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        stability: {
                          ...boat.stability,
                          crew_hiking_distance: parseFloat(e.target.value) || 1.5,
                        },
                      })
                    }
                  />
                  <span className="input-hint">Distance of windward deck from centerline</span>
                </div>

                <div className="input-group">
                  <label>Crew Active Hiking Fraction (0.0 – 1.0)</label>
                  <input
                    type="number"
                    step="0.05"
                    min="0.0"
                    max="1.0"
                    required
                    className="input-field"
                    value={boat.stability.crew_hiking_fraction}
                    onChange={(e) =>
                      setBoat({
                        ...boat,
                        stability: {
                          ...boat.stability,
                          crew_hiking_fraction: parseFloat(e.target.value) || 0.8,
                        },
                      })
                    }
                  />
                  <span className="input-hint">
                    {Math.round(boat.stability.crew_hiking_fraction * 100)}% of crew active on rail
                  </span>
                </div>
              </div>
            </div>
          )}

          {/* Footer Actions */}
          <div className="custom-modal-footer">
            <button
              type="button"
              className="btn-modal-cancel"
              onClick={onClose}
              disabled={calculating}
            >
              Cancel
            </button>
            <button
              type="submit"
              className="btn-modal-submit"
              disabled={calculating}
            >
              {calculating ? (
                <>
                  <RefreshCw size={16} className="spin" />
                  <span>Solving 3-DOF VPP Polars...</span>
                </>
              ) : (
                <>
                  <Save size={16} />
                  <span>Calculate Polars & Save Yacht</span>
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default CustomBoatBuilderModal;
