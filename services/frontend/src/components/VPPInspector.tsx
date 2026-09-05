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
  parsePolFile,
} from '../services/api';
import { usePersistedState } from '../services/persistence';
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
  FileText,
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
  const [activePlotTab, setActivePlotTab] = usePersistedState<'polar' | 'curves' | 'resistance'>(
    'vpp.activePlotTab',
    'polar'
  );
  const [heelAngle, setHeelAngle] = usePersistedState<number>('vpp.heelAngle', 15);
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
  const polFileInputRef = useRef<HTMLInputElement | null>(null);

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
    if (presetObj?.polarData) {
      // In-memory / uploaded custom boat or .pol file with polar data (No VPP solve needed!)
      setBoatDetail(presetObj.customBoat || null);
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

    const presetObj = presets.find((p) => p.id === selectedPresetId);

    // Resistance breakdown is not available for pure .pol input files (needs 3D hull geometry)
    if (activePlotTab === 'resistance' && presetObj?.isPolFileOnly) {
      setLoadingPlot(false);
      return;
    }

    const targetBoatOrPreset = presetObj?.polarData || presetObj?.customBoat || selectedPresetId;

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
  }, [activePlotTab, selectedPresetId, heelAngle, currentPreset, presets]);

  // Handlers for Export
  const handleExportORC = async () => {
    try {
      setExporting(true);
      const target = currentPreset?.polarData || currentPreset?.customBoat || selectedPresetId;
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
      const target = currentPreset?.polarData || currentPreset?.customBoat || selectedPresetId;
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
    if (!polarData) return;
    const fallbackBoat: BoatDetail = boatDetail || {
      name: currentPreset?.name || 'Custom Polar Yacht',
      hull: {
        loa: currentPreset?.loa_m || 12.0,
        lwl: (currentPreset?.loa_m || 12.0) * 0.85,
        b_max: currentPreset?.beam_m || 3.8,
        b_wl: (currentPreset?.beam_m || 3.8) * 0.88,
        draft_canoe: (currentPreset?.draft_m || 2.2) * 0.35,
        draft_total: currentPreset?.draft_m || 2.2,
        displacement_mass: currentPreset?.displacement_kg || 7500,
        prismatic_coef: 0.56,
        form_factor_k: 0.12,
        lcb_fraction: 0.52,
      },
      appendages: {
        keel_type: 'fin_bulb',
        keel_area: 2.0,
        keel_span: 1.6,
        rudder_area: 0.9,
        rudder_span: 1.3,
      },
      rig: {
        rig_type: currentPreset?.rig_type || 'sloop',
        main_p: 14.5,
        main_e: 4.8,
        fore_i: 15.2,
        fore_j: 4.5,
        mast_height_above_water: 18.0,
        boom_height_above_water: 1.9,
      },
      stability: {
        gmt: 1.2,
        crew_mass: 400,
        crew_hiking_distance: 1.6,
        crew_hiking_fraction: 0.8,
      },
    };

    exportCustomBoatJSON(fallbackBoat, polarData || undefined);
    setStatusNotification({
      type: 'success',
      msg: `Saved "${fallbackBoat.name}" and polars to JSON file!`,
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
      console.error('JSON file import failed:', err);
      setStatusNotification({
        type: 'error',
        msg: `Import failed: ${err.message}`,
      });
    } finally {
      setLoading(false);
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  };

  // Direct .POL File Upload (No VPP solving!)
  const handlePolFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    try {
      setLoading(true);
      const text = await file.text();
      const parsedPolar = parsePolFile(text, file.name);

      const customId = `pol-${Date.now()}`;
      const preset: BoatPreset = {
        id: customId,
        name: parsedPolar.boat_name,
        loa_m: 12.0,
        beam_m: 3.8,
        draft_m: 2.2,
        displacement_kg: 7500,
        rig_type: 'pol',
        isCustom: true,
        isPolFileOnly: true,
        polarData: parsedPolar,
      };

      onAddCustomBoat(preset);
      onSelectPreset(customId);
      setStatusNotification({
        type: 'success',
        msg: `Successfully imported "${parsedPolar.boat_name}" (.pol data: ${parsedPolar.tws_list.length} TWS x ${parsedPolar.twa_list.length} TWA)!`,
      });
    } catch (err: any) {
      console.error('POL upload failed:', err);
      setStatusNotification({
        type: 'error',
        msg: `POL parse failed: ${err.message}`,
      });
    } finally {
      setLoading(false);
      if (polFileInputRef.current) polFileInputRef.current.value = '';
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

  // Matplotlib 'plasma' colormap reference RGB samples for t in [0, 1]
  const PLASMA_STOPS: [number, number, number][] = [
    [13, 8, 135],    // 0.00 #0d0887
    [65, 3, 157],    // 0.10 #41039d
    [106, 0, 168],   // 0.20 #6a00a8
    [143, 13, 161],  // 0.30 #8f0da1
    [177, 42, 144],  // 0.40 #b12a90
    [204, 71, 120],  // 0.50 #cc4778
    [225, 100, 98],  // 0.60 #e16462
    [241, 130, 77],  // 0.70 #f1824d
    [251, 162, 56],  // 0.80 #fba238
    [253, 203, 42],  // 0.90 #fdcb2a
    [240, 249, 33],  // 1.00 #f0f921
  ];

  const getTWSColumnColor = (twsIndex: number, totalTws: number) => {
    // Exactly matches np.linspace(0.1, 0.9, len(tws_list)) used in polar_diagram plotter
    const norm = totalTws > 1 ? 0.1 + (twsIndex / (totalTws - 1)) * 0.8 : 0.5;
    const pos = norm * (PLASMA_STOPS.length - 1);
    const idx = Math.floor(pos);
    const frac = pos - idx;
    const c0 = PLASMA_STOPS[idx];
    const c1 = PLASMA_STOPS[Math.min(idx + 1, PLASMA_STOPS.length - 1)];
    const r = Math.round(c0[0] + (c1[0] - c0[0]) * frac);
    const g = Math.round(c0[1] + (c1[1] - c0[1]) * frac);
    const b = Math.round(c0[2] + (c1[2] - c0[2]) * frac);
    const hex = `#${r.toString(16).padStart(2, '0')}${g.toString(16).padStart(2, '0')}${b.toString(16).padStart(2, '0')}`;
    return {
      r,
      g,
      b,
      hex,
      rgba: (alpha: number) => `rgba(${r}, ${g}, ${b}, ${alpha})`,
    };
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

      {/* Hidden File Input for standard .pol polar table upload */}
      <input
        type="file"
        ref={polFileInputRef}
        accept=".pol,.txt"
        style={{ display: 'none' }}
        onChange={handlePolFileUpload}
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
                      {p.isPolFileOnly ? '📁' : '🛠️'} {p.name} ({p.isPolFileOnly ? 'POL' : p.rig_type.toUpperCase()})
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
            title="Create and solve a new custom yacht from parameters"
          >
            <Plus size={15} />
            <span>Create Boat</span>
          </button>

          {/* Upload .POL File Button */}
          <button
            className="vpp-btn-action btn-upload-pol"
            onClick={() => polFileInputRef.current?.click()}
            title="Upload standard ORC / OpenCPN / Expedition *.pol polar table (direct data input, no VPP calculation)"
          >
            <FileText size={15} />
            <span>Upload .POL</span>
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

          {/* Download JSON Button */}
          <button
            className="vpp-btn-action btn-download-json"
            onClick={handleDownloadJSON}
            disabled={!polarData || loading}
            title="Download vessel specifications & polar matrix as .json"
          >
            <FileJson size={15} />
            <span>Download JSON</span>
          </button>

          {/* Custom boat Edit / Delete actions */}
          {isCustomBoat && (
            <>
              {!currentPreset?.isPolFileOnly && (
                <button
                  className="vpp-btn-icon-action"
                  onClick={handleOpenEditModal}
                  title="Edit vessel dimensions & recalculate polars"
                >
                  <Edit size={15} />
                </button>
              )}
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
          <span>Loading Polar Performance Curves & Matrix...</span>
        </div>
      ) : (
        <div className="vpp-content-layout">
          {/* Left Column: Boat Specs & VMG Targets */}
          <div className="vpp-left-pane">
            {/* 1. Geometry or .POL Summary Card */}
            {currentPreset?.isPolFileOnly ? (
              <div className="vpp-card">
                <div className="vpp-card-header">
                  <FileText size={16} color="#c084fc" />
                  <h3>📁 {currentPreset.name}</h3>
                  <span
                    className="custom-boat-badge"
                    style={{
                      backgroundColor: 'rgba(168, 85, 247, 0.15)',
                      borderColor: 'rgba(168, 85, 247, 0.35)',
                      color: '#c084fc',
                    }}
                  >
                    .POL File Input
                  </span>
                </div>
                <div className="vpp-specs-grid">
                  <div className="vpp-spec-item">
                    <span className="spec-label">Input Source</span>
                    <span className="spec-value font-bold" style={{ color: '#c084fc' }}>
                      Imported .POL Table
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">VPP Physics Solver</span>
                    <span className="spec-value" style={{ color: '#94a3b8' }}>
                      Direct Polar Input
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Wind Speeds (TWS)</span>
                    <span className="spec-value font-bold" style={{ color: '#38bdf8' }}>
                      {polarData?.tws_list.length || 0} speeds ({polarData?.tws_list.join(', ')} kts)
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Wind Angles (TWA)</span>
                    <span className="spec-value">
                      {polarData?.twa_list.length || 0} angles ({polarData?.twa_list[0]}° to{' '}
                      {polarData?.twa_list[polarData.twa_list.length - 1]}°)
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">Total Matrix Cells</span>
                    <span className="spec-value font-bold">
                      {polarData ? polarData.tws_list.length * polarData.twa_list.length : 0} points
                    </span>
                  </div>
                  <div className="vpp-spec-item">
                    <span className="spec-label">VMG Targets Computed</span>
                    <span className="spec-value font-bold" style={{ color: '#10b981' }}>
                      {polarData ? Object.keys(polarData.upwind_vmg_targets).length * 2 : 0} targets
                    </span>
                  </div>
                </div>
              </div>
            ) : boatDetail ? (
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
            ) : null}

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
                  {!currentPreset?.isPolFileOnly && (
                    <button
                      className={`vpp-tab-btn ${activePlotTab === 'resistance' ? 'active' : ''}`}
                      onClick={() => setActivePlotTab('resistance')}
                    >
                      <Wind size={15} />
                      <span>Hydro Resistance</span>
                    </button>
                  )}
                </div>

                {activePlotTab === 'resistance' && !currentPreset?.isPolFileOnly && (
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
                {currentPreset?.isPolFileOnly && activePlotTab === 'resistance' ? (
                  <div
                    className="vpp-plot-empty"
                    style={{
                      padding: '30px',
                      textAlign: 'center',
                      color: '#94a3b8',
                      maxWidth: '440px',
                    }}
                  >
                    <Layers size={36} color="#64748b" style={{ marginBottom: '12px' }} />
                    <h4 style={{ color: '#f8fafc', marginBottom: '8px' }}>
                      Hydrodynamic Resistance Unavailable
                    </h4>
                    <p style={{ fontSize: '0.82rem', lineHeight: '1.5' }}>
                      Decomposing hydrodynamic resistance requires 3D hull geometry parameters.
                    </p>
                    <p style={{ fontSize: '0.82rem', marginTop: '6px', color: '#38bdf8' }}>
                      For imported <b>.POL</b> files, sailing performance is rendered directly in the{' '}
                      <b>Polar Diagram</b> and <b>Performance Curves</b> tabs.
                    </p>
                  </div>
                ) : loadingPlot ? (
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
                        {polarData.tws_list.map((tws, twsIdx) => {
                          const colColor = getTWSColumnColor(twsIdx, polarData.tws_list.length);
                          return (
                            <th
                              key={tws}
                              style={{
                                borderBottom: `2px solid ${colColor.hex}`,
                              }}
                            >
                              <div
                                className="tws-header-pill"
                                style={{
                                  backgroundColor: colColor.rgba(0.18),
                                  borderColor: colColor.rgba(0.4),
                                }}
                              >
                                <span
                                  className="tws-color-dot"
                                  style={{ backgroundColor: colColor.hex }}
                                />
                                <span style={{ color: colColor.hex, fontWeight: 700 }}>
                                  {tws} kt
                                </span>
                              </div>
                            </th>
                          );
                        })}
                      </tr>
                    </thead>
                    <tbody>
                      {polarData.twa_list
                        .map((twa, twaIdx) => ({ twa, twaIdx }))
                        .filter(({ twa }) => twa >= 28)
                        .map(({ twa, twaIdx }) => (
                          <tr key={twa}>
                            <td className="sticky-col font-bold">
                              {twa}°
                            </td>
                            {polarData.tws_list.map((tws, twsIdx) => {
                              const colColor = getTWSColumnColor(
                                twsIdx,
                                polarData.tws_list.length
                              );
                              const speed = polarData.speed_matrix[twsIdx]?.[twaIdx] || 0;
                              const speedFrac = Math.min(Math.max(speed / 9.5, 0.05), 1.0);

                              return (
                                <td
                                  key={`${twa}_${tws}`}
                                  style={{
                                    backgroundColor: colColor.rgba(0.08 + speedFrac * 0.35),
                                    color: '#f8fafc',
                                  }}
                                  className="speed-cell"
                                  title={`TWA: ${twa}°, TWS: ${tws} kts -> Boat Speed: ${speed.toFixed(2)} kts`}
                                >
                                  {speed.toFixed(2)}
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
