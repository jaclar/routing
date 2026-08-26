import React, { useEffect, useState } from 'react';
import {
  BoatDetail,
  BoatPreset,
  SolveMatrixResponse,
} from '../types';
import {
  exportCSVPolFile,
  exportORCPolFile,
  fetchBoatDetail,
  fetchPlotImageBlob,
  fetchPolarMatrix,
} from '../services/api';
import {
  Anchor,
  Compass,
  Download,
  FileSpreadsheet,
  Gauge,
  Layers,
  LineChart,
  Navigation,
  RefreshCw,
  Sliders,
  Wind,
} from 'lucide-react';

interface VPPInspectorProps {
  presets: BoatPreset[];
  selectedPresetId: string;
  onSelectPreset: (presetId: string) => void;
}

export const VPPInspector: React.FC<VPPInspectorProps> = ({
  presets,
  selectedPresetId,
  onSelectPreset,
}) => {
  const [boatDetail, setBoatDetail] = useState<BoatDetail | null>(null);
  const [polarData, setPolarData] = useState<SolveMatrixResponse | null>(null);
  const [activePlotTab, setActivePlotTab] = useState<'polar' | 'curves' | 'resistance'>('polar');
  const [heelAngle, setHeelAngle] = useState<number>(15);
  const [plotImages, setPlotImages] = useState<{
    polar?: string;
    curves?: string;
    resistance?: string;
  }>({});
  const [loading, setLoading] = useState<boolean>(true);
  const [loadingPlot, setLoadingPlot] = useState<boolean>(false);
  const [exporting, setExporting] = useState<boolean>(false);

  // 1. Fetch Boat Specification and Polar Matrix on preset change
  useEffect(() => {
    let isMounted = true;
    setLoading(true);

    Promise.all([
      fetchBoatDetail(selectedPresetId),
      fetchPolarMatrix(selectedPresetId),
    ])
      .then(([boat, matrix]) => {
        if (isMounted) {
          setBoatDetail(boat);
          setPolarData(matrix);
          setLoading(false);
        }
      })
      .catch((err) => {
        console.error('Failed to load VPP data:', err);
        if (isMounted) setLoading(false);
      });

    return () => {
      isMounted = false;
    };
  }, [selectedPresetId]);

  // 2. Fetch Active Plot Image
  useEffect(() => {
    let isMounted = true;
    setLoadingPlot(true);

    fetchPlotImageBlob(activePlotTab, selectedPresetId, heelAngle)
      .then((blobUrl) => {
        if (isMounted) {
          setPlotImages((prev) => ({ ...prev, [activePlotTab]: blobUrl }));
          setLoadingPlot(false);
        }
      })
      .catch((err) => {
        console.error(`Failed to load ${activePlotTab} plot:`, err);
        if (isMounted) setLoadingPlot(false);
      });

    return () => {
      isMounted = false;
    };
  }, [activePlotTab, selectedPresetId, heelAngle]);

  const handleExportORC = async () => {
    try {
      setExporting(true);
      const text = await exportORCPolFile(selectedPresetId);
      const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${selectedPresetId}.pol`;
      link.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      alert(`Export ORC failed: ${err}`);
    } finally {
      setExporting(false);
    }
  };

  const handleExportCSV = async () => {
    try {
      setExporting(true);
      const text = await exportCSVPolFile(selectedPresetId);
      const blob = new Blob([text], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${selectedPresetId}_polars.csv`;
      link.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      alert(`Export CSV failed: ${err}`);
    } finally {
      setExporting(false);
    }
  };

  // Helper for matrix cell color
  const getSpeedHeatmapColor = (speed: number, maxSpeed: number = 9.0) => {
    const norm = Math.min(Math.max(speed / maxSpeed, 0), 1);
    if (speed < 0.1) return 'rgba(15, 23, 42, 0.4)';
    const r = Math.round(2 + norm * 54);
    const g = Math.round(132 + norm * 57);
    const b = Math.round(199 + norm * 49);
    return `rgba(${r}, ${g}, ${b}, ${0.15 + norm * 0.45})`;
  };

  return (
    <div className="vpp-inspector-container">
      {/* Top Header Bar */}
      <div className="vpp-top-bar">
        <div className="vpp-title-section">
          <div className="vpp-icon-badge">
            <Gauge size={22} color="#38bdf8" />
          </div>
          <div>
            <h2>Sailboat VPP & Performance Polars</h2>
            <p>3-DOF Equilibrium Solver (Aero-Hydro Coupling & Wave Resistance Models)</p>
          </div>
        </div>

        <div className="vpp-actions-bar">
          {/* Preset Selector */}
          <div className="vpp-preset-dropdown">
            <Anchor size={16} color="#94a3b8" />
            <select
              value={selectedPresetId}
              onChange={(e) => onSelectPreset(e.target.value)}
              className="vpp-select"
            >
              {presets.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} ({p.rig_type.toUpperCase()})
                </option>
              ))}
            </select>
          </div>

          {/* Export Buttons */}
          <button
            className="vpp-btn-export"
            onClick={handleExportORC}
            disabled={exporting}
            title="Download standard ORC/Expedition .pol file"
          >
            <Download size={15} />
            <span>Export .POL</span>
          </button>

          <button
            className="vpp-btn-export"
            onClick={handleExportCSV}
            disabled={exporting}
            title="Download point-by-point CSV dataset"
          >
            <FileSpreadsheet size={15} />
            <span>Export CSV</span>
          </button>
        </div>
      </div>

      {loading ? (
        <div className="vpp-loading-state">
          <RefreshCw size={32} className="spin" color="#38bdf8" />
          <span>Solving 3-DOF Aero-Hydro Equilibrium Equations...</span>
        </div>
      ) : (
        <div className="vpp-content-layout">
          {/* Left Column: Boat Specs & VMG Targets */}
          <div className="vpp-left-pane">
            {/* 1. Geometry & Naval Architecture Specifications */}
            {boatDetail && (
              <div className="vpp-card">
                <div className="vpp-card-header">
                  <Layers size={16} color="#38bdf8" />
                  <h3>Yacht Specifications & Geometry</h3>
                </div>
                <div className="vpp-specs-grid">
                  <div className="vpp-spec-item">
                    <span className="spec-label">LOA / LWL</span>
                    <span className="spec-value">
                      {boatDetail.hull.loa.toFixed(2)}m / {boatDetail.hull.lwl.toFixed(2)}m
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Beam (Bmax / Bwl)</span>
                    <span className="spec-value">
                      {boatDetail.hull.b_max.toFixed(2)}m / {boatDetail.hull.b_wl.toFixed(2)}m
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Draft Total / Canoe</span>
                    <span className="spec-value">
                      {boatDetail.hull.draft_total.toFixed(2)}m / {boatDetail.hull.draft_canoe.toFixed(2)}m
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Displacement Mass</span>
                    <span className="spec-value">
                      {Math.round(boatDetail.hull.displacement_mass).toLocaleString()} kg
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Prismatic Coef (Cp)</span>
                    <span className="spec-value">{boatDetail.hull.prismatic_coef.toFixed(3)}</span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Metacentric Height (GMT)</span>
                    <span className="spec-value">{boatDetail.stability.gmt.toFixed(2)}m</span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Mainsail P x E</span>
                    <span className="spec-value">
                      {boatDetail.rig.main_p.toFixed(2)}m x {boatDetail.rig.main_e.toFixed(2)}m
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Foretriangle I x J</span>
                    <span className="spec-value">
                      {boatDetail.rig.fore_i.toFixed(2)}m x {boatDetail.rig.fore_j.toFixed(2)}m
                    </span>
                  </div>
                  {boatDetail.rig.mizzen_p && (
                    <div className="vpp-spec-item">
                      <span className="spec-label">Mizzen P x E</span>
                      <span className="spec-value">
                        {boatDetail.rig.mizzen_p.toFixed(2)}m x {boatDetail.rig.mizzen_e?.toFixed(2)}m
                      </span>
                    </div>
                  )}
                  <div className="vpp-spec-item">
                    <span className="spec-label">Mast Height</span>
                    <span className="spec-value">
                      {boatDetail.rig.mast_height_above_water.toFixed(1)}m above DWL
                    </span>
                  </div>
                </div>
              </div>
            )}

            {/* 2. Optimal Upwind & Downwind VMG Targets */}
            {polarData && (
              <div className="vpp-card">
                <div className="vpp-card-header">
                  <Compass size={16} color="#10b981" />
                  <h3>Optimal VMG Targets</h3>
                </div>
                <div className="vpp-vmg-table-wrap">
                  <table className="vpp-table">
                    <thead>
                      <tr>
                        <th>TWS</th>
                        <th className="upwind-head">Upwind TWA</th>
                        <th className="upwind-head">Upwind Vs</th>
                        <th className="upwind-head">Upwind VMG</th>
                        <th className="downwind-head">Downwind TWA</th>
                        <th className="downwind-head">Downwind Vs</th>
                        <th className="downwind-head">Downwind VMG</th>
                      </tr>
                    </thead>
                    <tbody>
                      {polarData.tws_list.map((tws) => {
                        const up = polarData.upwind_vmg_targets[tws.toFixed(1)] || polarData.upwind_vmg_targets[String(tws)];
                        const dn = polarData.downwind_vmg_targets[tws.toFixed(1)] || polarData.downwind_vmg_targets[String(tws)];
                        return (
                          <tr key={tws}>
                            <td className="font-bold">{tws} kts</td>
                            <td className="upwind-val">{up ? `${up.target_twa_deg.toFixed(1)}°` : '-'}</td>
                            <td className="upwind-val">{up ? `${up.target_v_boat_kts.toFixed(2)}k` : '-'}</td>
                            <td className="upwind-val font-bold">{up ? `${up.target_vmg_kts.toFixed(2)}k` : '-'}</td>
                            <td className="downwind-val">{dn ? `${dn.target_twa_deg.toFixed(1)}°` : '-'}</td>
                            <td className="downwind-val">{dn ? `${dn.target_v_boat_kts.toFixed(2)}k` : '-'}</td>
                            <td className="downwind-val font-bold">{dn ? `${dn.target_vmg_kts.toFixed(2)}k` : '-'}</td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </div>

          {/* Right Column: Graphs & Polar Matrix Table */}
          <div className="vpp-right-pane">
            {/* 1. High-Resolution Visual Graphs */}
            <div className="vpp-card">
              <div className="vpp-plot-header">
                <div className="vpp-plot-tabs">
                  <button
                    className={`vpp-tab-btn ${activePlotTab === 'polar' ? 'active' : ''}`}
                    onClick={() => setActivePlotTab('polar')}
                  >
                    <Navigation size={15} />
                    <span>Polar Diagram</span>
                  </button>
                  <button
                    className={`vpp-tab-btn ${activePlotTab === 'curves' ? 'active' : ''}`}
                    onClick={() => setActivePlotTab('curves')}
                  >
                    <LineChart size={15} />
                    <span>Performance Curves</span>
                  </button>
                  <button
                    className={`vpp-tab-btn ${activePlotTab === 'resistance' ? 'active' : ''}`}
                    onClick={() => setActivePlotTab('resistance')}
                  >
                    <Wind size={15} />
                    <span>Hydro Resistance</span>
                  </button>
                </div>

                {activePlotTab === 'resistance' && (
                  <div className="vpp-heel-control">
                    <Sliders size={14} color="#38bdf8" />
                    <span>Heel Angle: {heelAngle}°</span>
                    <input
                      type="range"
                      min={0}
                      max={30}
                      step={5}
                      value={heelAngle}
                      onChange={(e) => setHeelAngle(parseInt(e.target.value, 10))}
                      className="vpp-slider"
                    />
                  </div>
                )}
              </div>

              <div className="vpp-plot-display">
                {loadingPlot ? (
                  <div className="vpp-plot-loading">
                    <RefreshCw size={28} className="spin" color="#38bdf8" />
                    <span>Rendering Matplotlib Vector Graph...</span>
                  </div>
                ) : plotImages[activePlotTab] ? (
                  <img
                    src={plotImages[activePlotTab]}
                    alt={`${activePlotTab} graph`}
                    className="vpp-plot-image"
                  />
                ) : (
                  <div className="vpp-plot-empty">Graph unavailable</div>
                )}
              </div>
            </div>

            {/* 2. Interactive Polar Matrix Heatmap Table */}
            {polarData && (
              <div className="vpp-card">
                <div className="vpp-card-header">
                  <FileSpreadsheet size={16} color="#38bdf8" />
                  <h3>Polar Boat Speed Matrix (Knots)</h3>
                </div>
                <div className="vpp-matrix-scroll">
                  <table className="vpp-matrix-table">
                    <thead>
                      <tr>
                        <th className="sticky-col">TWA \ TWS</th>
                        {polarData.tws_list.map((tws) => (
                          <th key={tws}>{tws} kt</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {polarData.twa_list.map((twa, twaIdx) => (
                        <tr key={twa} className={twa < 28 ? 'nogo-row' : ''}>
                          <td className={`sticky-col font-bold ${twa < 28 ? 'nogo-header' : ''}`}>
                            {twa}° {twa < 28 ? (twa === 0 ? '(Head/Irons)' : '(No-Go)') : ''}
                          </td>
                          {polarData.tws_list.map((tws, twsIdx) => {
                            const speed = polarData.speed_matrix[twsIdx]?.[twaIdx] || 0;
                            return (
                              <td
                                key={`${twa}_${tws}`}
                                style={{
                                  backgroundColor: twa < 28 ? 'rgba(239, 68, 68, 0.05)' : getSpeedHeatmapColor(speed),
                                  color: twa < 28 ? '#64748b' : undefined,
                                }}
                                className={`speed-cell ${twa < 28 ? 'nogo-cell' : ''}`}
                                title={twa < 28 ? `TWA: ${twa}° (In-Irons No-Go Zone) -> 0.00 kts` : `TWA: ${twa}°, TWS: ${tws} kts -> Boat Speed: ${speed.toFixed(2)} kts`}
                              >
                                {speed > 0.05 ? speed.toFixed(2) : '0.00'}
                              </td>
                            );
                          })}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
};

export default VPPInspector;
