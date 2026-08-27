import React, { useEffect, useState, useRef } from 'react';
import {
  BoatDetail,
  BoatPreset,
  SolveMatrixResponse,
} from '../types';
import {
  exportCSVPolFile,
  exportORCPolFile,
  exportCustomBoatJSON,
  fetchBoatDetail,
  fetchPlotImageBlob,
  fetchPolarMatrix,
  parseAndValidateBoatJSON,
} from '../services/api';
import { CustomBoatBuilderModal } from './CustomBoatBuilderModal';
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
  Plus,
  Upload,
  Edit,
  Trash2,
  FileJson,
  CheckCircle2,
  AlertTriangle,
} from 'lucide-react';

interface VPPInspectorProps {
  presets: BoatPreset[];
  selectedPresetId: string;
  onSelectPreset: (presetId: string) => void;
  onAddCustomBoat: (preset: BoatPreset) => void;
  onDeleteCustomBoat?: (presetId: string) => void;
}

export const VPPInspector: React.FC<VPPInspectorProps> = ({
  presets,
  selectedPresetId,
  onSelectPreset,
  onAddCustomBoat,
  onDeleteCustomBoat,
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

  // Modal & File Upload States
  const [isBuilderOpen, setIsBuilderOpen] = useState<boolean>(false);
  const [editingBoat, setEditingBoat] = useState<BoatDetail | undefined>(undefined);
  const [statusNotification, setStatusNotification] = useState<{
    type: 'success' | 'error';
    msg: string;
  } | null>(null);

  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const currentPreset = presets.find((p) => p.id === selectedPresetId);
  const isCustomBoat = Boolean(currentPreset?.isCustom);

  // Auto-hide status notification after 4s
  useEffect(() => {
    if (statusNotification) {
      const timer = setTimeout(() => setStatusNotification(null), 4000);
      return () => clearTimeout(timer);
    }
  }, [statusNotification]);

  // 1. Fetch Boat Specification and Polar Matrix on preset change
  useEffect(() => {
    let isMounted = true;
    setLoading(true);

    const presetObj = presets.find((p) => p.id === selectedPresetId);
    if (presetObj?.customBoat && presetObj.polarData) {
      // Fast path for in-memory / uploaded custom boat with polar data
      setBoatDetail(presetObj.customBoat);
      setPolarData(presetObj.polarData);
      setLoading(false);
      return;
    }

    // Otherwise query backend
    Promise.all([
      fetchBoatDetail(selectedPresetId, presets),
      fetchPolarMatrix(presetObj?.customBoat || selectedPresetId),
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
  }, [selectedPresetId, presets]);

  // 2. Fetch Active Plot Image
  useEffect(() => {
    let isMounted = true;
    setLoadingPlot(true);

    const targetBoatOrPreset = currentPreset?.customBoat || selectedPresetId;

    fetchPlotImageBlob(activePlotTab, targetBoatOrPreset, heelAngle)
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
  }, [activePlotTab, selectedPresetId, heelAngle, currentPreset]);

  // Handlers for Export
  const handleExportORC = async () => {
    try {
      setExporting(true);
      const target = currentPreset?.customBoat || selectedPresetId;
      const text = await exportORCPolFile(target);
      const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${(currentPreset?.name || 'polar').replace(/\s+/g, '_')}.pol`;
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
      const target = currentPreset?.customBoat || selectedPresetId;
      const text = await exportCSVPolFile(target);
      const blob = new Blob([text], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${(currentPreset?.name || 'polar').replace(/\s+/g, '_')}_polars.csv`;
      link.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      alert(`Export CSV failed: ${err}`);
    } finally {
      setExporting(false);
    }
  };

  // Download complete Boat Specs + Polar JSON
  const handleDownloadJSON = () => {
    if (!boatDetail) return;
    exportCustomBoatJSON(boatDetail, polarData || undefined);
    setStatusNotification({
      type: 'success',
      msg: `Saved "${boatDetail.name}" and polars to JSON file!`,
    });
  };

  // File Upload (.json)
  const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    try {
      setLoading(true);
      const text = await file.text();
      const parsedFile = parseAndValidateBoatJSON(text);

      let matrix = parsedFile.polars;
      if (!matrix) {
        // Compute matrix if not present in the file
        matrix = await fetchPolarMatrix(parsedFile.boat);
      }

      const customId = `custom-${Date.now()}`;
      const preset: BoatPreset = {
        id: customId,
        name: parsedFile.boat.name,
        loa_m: parsedFile.boat.hull.loa,
        beam_m: parsedFile.boat.hull.b_max,
        draft_m: parsedFile.boat.hull.draft_total,
        displacement_kg: parsedFile.boat.hull.displacement_mass,
        rig_type: parsedFile.boat.rig.rig_type,
        isCustom: true,
        customBoat: parsedFile.boat,
        polarData: matrix,
      };

      onAddCustomBoat(preset);
      onSelectPreset(customId);
      setStatusNotification({
        type: 'success',
        msg: `Successfully imported "${parsedFile.boat.name}"!`,
      });
    } catch (err: any) {
      console.error('File import failed:', err);
      setStatusNotification({
        type: 'error',
        msg: `Import failed: ${err.message}`,
      });
    } finally {
      setLoading(false);
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  };

  const handleOpenCreateModal = () => {
    setEditingBoat(undefined);
    setIsBuilderOpen(true);
  };

  const handleOpenEditModal = () => {
    if (boatDetail) {
      setEditingBoat(boatDetail);
      setIsBuilderOpen(true);
    }
  };

  const handleDeleteCurrentCustomBoat = () => {
    if (!isCustomBoat) return;
    if (confirm(`Are you sure you want to delete "${currentPreset?.name}"?`)) {
      onDeleteCustomBoat?.(selectedPresetId);
      onSelectPreset('36ft-ketch');
      setStatusNotification({
        type: 'success',
        msg: `Deleted "${currentPreset?.name}".`,
      });
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

  const builtinList = presets.filter((p) => !p.isCustom);
  const customList = presets.filter((p) => p.isCustom);

  return (
    <div className="vpp-inspector-container">
      {/* Hidden File Input for .json upload */}
      <input
        type="file"
        ref={fileInputRef}
        accept=".json"
        style={{ display: 'none' }}
        onChange={handleFileUpload}
      />

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
              <optgroup label="Built-in Standard Presets">
                {builtinList.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} ({p.rig_type.toUpperCase()})
                  </option>
                ))}
              </optgroup>
              {customList.length > 0 && (
                <optgroup label="Custom User Yachts">
                  {customList.map((p) => (
                    <option key={p.id} value={p.id}>
                      🛠️ {p.name} ({p.rig_type.toUpperCase()})
                    </option>
                  ))}
                </optgroup>
              )}
            </select>
          </div>

          {/* Create Custom Boat Button */}
          <button
            className="vpp-btn-action btn-create-boat"
            onClick={handleOpenCreateModal}
            title="Create and solve a new custom yacht"
          >
            <Plus size={15} />
            <span>Create Boat</span>
          </button>

          {/* Upload Boat JSON Button */}
          <button
            className="vpp-btn-action btn-upload-boat"
            onClick={() => fileInputRef.current?.click()}
            title="Upload previously exported .json boat & polar file"
          >
            <Upload size={15} />
            <span>Upload JSON</span>
          </button>

          {/* Download JSON Button (Always available to save current boat + calculations) */}
          <button
            className="vpp-btn-action btn-download-json"
            onClick={handleDownloadJSON}
            disabled={!boatDetail || loading}
            title="Download complete vessel specifications & polar matrix as .json"
          >
            <FileJson size={15} />
            <span>Download JSON</span>
          </button>

          {/* Custom boat Edit / Delete actions */}
          {isCustomBoat && (
            <>
              <button
                className="vpp-btn-icon-action"
                onClick={handleOpenEditModal}
                title="Edit vessel dimensions & recalculate polars"
              >
                <Edit size={15} />
              </button>
              <button
                className="vpp-btn-icon-action text-danger"
                onClick={handleDeleteCurrentCustomBoat}
                title="Delete this custom boat"
              >
                <Trash2 size={15} />
              </button>
            </>
          )}

          {/* Export Buttons */}
          <button
            className="vpp-btn-export"
            onClick={handleExportORC}
            disabled={exporting || loading}
            title="Download standard ORC/Expedition .pol file"
          >
            <Download size={15} />
            <span>Export .POL</span>
          </button>

          <button
            className="vpp-btn-export"
            onClick={handleExportCSV}
            disabled={exporting || loading}
            title="Download point-by-point CSV dataset"
          >
            <FileSpreadsheet size={15} />
            <span>Export CSV</span>
          </button>
        </div>
      </div>

      {/* Floating Status Notification */}
      {statusNotification && (
        <div
          className={`vpp-notification ${
            statusNotification.type === 'success' ? 'notif-success' : 'notif-error'
          }`}
        >
          {statusNotification.type === 'success' ? (
            <CheckCircle2 size={16} />
          ) : (
            <AlertTriangle size={16} />
          )}
          <span>{statusNotification.msg}</span>
        </div>
      )}

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
                  <h3>
                    {isCustomBoat ? `🛠️ ${boatDetail.name} (Custom)` : boatDetail.name}
                  </h3>
                  {isCustomBoat && (
                    <span className="custom-boat-badge">Custom Model</span>
                  )}
                </div>
                <div className="vpp-specs-grid">
                  <div className="vpp-spec-item">
                    <span className="spec-label">LOA / LWL</span>
                    <span className="spec-value">
                      {boatDetail.hull.loa.toFixed(2)}m / {boatDetail.hull.lwl.toFixed(2)}m
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Beam (Max / WL)</span>
                    <span className="spec-value">
                      {boatDetail.hull.b_max.toFixed(2)}m / {boatDetail.hull.b_wl.toFixed(2)}m
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Draft (Total / Canoe)</span>
                    <span className="spec-value">
                      {boatDetail.hull.draft_total.toFixed(2)}m /{' '}
                      {boatDetail.hull.draft_canoe.toFixed(2)}m
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Displacement</span>
                    <span className="spec-value font-bold" style={{ color: '#38bdf8' }}>
                      {boatDetail.hull.displacement_mass.toLocaleString()} kg
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Rig Type</span>
                    <span className="spec-value font-bold">
                      {boatDetail.rig.rig_type.toUpperCase()}
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Mainsail (P x E)</span>
                    <span className="spec-value">
                      {boatDetail.rig.main_p.toFixed(1)}m x {boatDetail.rig.main_e.toFixed(1)}m
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Foretriangle (I x J)</span>
                    <span className="spec-value">
                      {boatDetail.rig.fore_i.toFixed(1)}m x {boatDetail.rig.fore_j.toFixed(1)}m
                    </span>
                  </div>
                  {boatDetail.rig.mizzen_p && boatDetail.rig.mizzen_e && (
                    <div className="vpp-spec-item">
                      <span className="spec-label">Mizzen (P x E)</span>
                      <span className="spec-value">
                        {boatDetail.rig.mizzen_p.toFixed(1)}m x{' '}
                        {boatDetail.rig.mizzen_e.toFixed(1)}m
                      </span>
                    </div>
                  )}
                  <div className="vpp-spec-item">
                    <span className="spec-label">Keel Foil Type</span>
                    <span className="spec-value font-mono">
                      {boatDetail.appendages.keel_type}
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Initial GM_T</span>
                    <span className="spec-value">{boatDetail.stability.gmt.toFixed(2)} m</span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Crew Mass</span>
                    <span className="spec-value">{boatDetail.stability.crew_mass} kg</span>
                  </div>
                </div>
              </div>
            )}

            {/* 2. Optimal Upwind / Downwind VMG Targets */}
            {polarData && (
              <div className="vpp-card">
                <div className="vpp-card-header">
                  <Compass size={16} color="#38bdf8" />
                  <h3>Polar VMG Target Angles & Speeds</h3>
                </div>
                <div className="vpp-table-container">
                  <table className="vpp-vmg-table">
                    <thead>
                      <tr>
                        <th>TWS</th>
                        <th className="upwind-header">Beat TWA</th>
                        <th className="upwind-header">Boat Speed</th>
                        <th className="upwind-header">Opt VMG</th>
                        <th className="downwind-header">Run TWA</th>
                        <th className="downwind-header">Boat Speed</th>
                        <th className="downwind-header">Opt VMG</th>
                      </tr>
                    </thead>
                    <tbody>
                      {polarData.tws_list.map((tws) => {
                        const up =
                          polarData.upwind_vmg_targets[tws.toFixed(1)] ||
                          polarData.upwind_vmg_targets[String(tws)];
                        const dn =
                          polarData.downwind_vmg_targets[tws.toFixed(1)] ||
                          polarData.downwind_vmg_targets[String(tws)];
                        return (
                          <tr key={tws}>
                            <td className="font-bold">{tws} kts</td>
                            <td className="upwind-val">
                              {up ? `${up.target_twa_deg.toFixed(1)}°` : '-'}
                            </td>
                            <td className="upwind-val">
                              {up ? `${up.target_v_boat_kts.toFixed(2)}k` : '-'}
                            </td>
                            <td className="upwind-val font-bold">
                              {up ? `${up.target_vmg_kts.toFixed(2)}k` : '-'}
                            </td>
                            <td className="downwind-val">
                              {dn ? `${dn.target_twa_deg.toFixed(1)}°` : '-'}
                            </td>
                            <td className="downwind-val">
                              {dn ? `${dn.target_v_boat_kts.toFixed(2)}k` : '-'}
                            </td>
                            <td className="downwind-val font-bold">
                              {dn ? `${dn.target_vmg_kts.toFixed(2)}k` : '-'}
                            </td>
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
                                  backgroundColor:
                                    twa < 28
                                      ? 'rgba(239, 68, 68, 0.05)'
                                      : getSpeedHeatmapColor(speed),
                                  color: twa < 28 ? '#64748b' : undefined,
                                }}
                                className={`speed-cell ${twa < 28 ? 'nogo-cell' : ''}`}
                                title={
                                  twa < 28
                                    ? `TWA: ${twa}° (In-Irons No-Go Zone) -> 0.00 kts`
                                    : `TWA: ${twa}°, TWS: ${tws} kts -> Boat Speed: ${speed.toFixed(2)} kts`
                                }
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

      {/* Custom Boat Builder Modal */}
      <CustomBoatBuilderModal
        isOpen={isBuilderOpen}
        onClose={() => setIsBuilderOpen(false)}
        onSaveBoat={(newBoatPreset) => {
          onAddCustomBoat(newBoatPreset);
          onSelectPreset(newBoatPreset.id);
          setStatusNotification({
            type: 'success',
            msg: `Successfully created and saved "${newBoatPreset.name}"!`,
          });
        }}
        initialBoat={editingBoat}
        builtinPresets={builtinList}
      />
    </div>
  );
};

export default VPPInspector;
