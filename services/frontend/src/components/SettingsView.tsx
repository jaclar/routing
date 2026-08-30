import React, { useState } from 'react';
import { BoatPreset, RouteResult } from '../types';
import { CustomBoatBuilderModal } from './CustomBoatBuilderModal';
import {
  Anchor,
  Compass,
  Gauge,
  Sliders,
  Sparkles,
  Plus,
  Shield,
  ChevronRight,
  Trash2,
} from 'lucide-react';

interface SettingsViewProps {
  presets: BoatPreset[];
  selectedPresetId: string;
  onSelectPreset: (id: string) => void;
  onAddCustomBoat: (preset: BoatPreset) => void;
  onDeleteCustomBoat?: (presetId: string) => void;
  tackPenaltyMinutes: number;
  onTackPenaltyChange: (mins: number) => void;
  gybePenaltyMinutes: number;
  onGybePenaltyChange: (mins: number) => void;
  onOpenVPP: () => void;
  onBackToRouting: () => void;
  routeResult: RouteResult | null;
}

export const SettingsView: React.FC<SettingsViewProps> = ({
  presets,
  selectedPresetId,
  onSelectPreset,
  onAddCustomBoat,
  onDeleteCustomBoat,
  tackPenaltyMinutes,
  onTackPenaltyChange,
  gybePenaltyMinutes,
  onGybePenaltyChange,
  onOpenVPP,
  onBackToRouting,
}) => {
  const [showCustomModal, setShowCustomModal] = useState<boolean>(false);

  const currentBoat = presets.find((p) => p.id === selectedPresetId);

  const setCruisingMode = () => {
    onTackPenaltyChange(5.0);
    onGybePenaltyChange(8.0);
  };

  const setRacingMode = () => {
    onTackPenaltyChange(1.5);
    onGybePenaltyChange(2.0);
  };

  return (
    <div className="settings-page-container">
      {/* Top Header */}
      <div className="settings-header">
        <div className="settings-header-title-group">
          <div className="settings-header-icon">
            <Sliders size={22} color="#38bdf8" />
          </div>
          <div>
            <h1 className="settings-title">Settings &amp; Configuration</h1>
            <p className="settings-subtitle">
              Manage sailboat hull models, polar speed tables, and maneuver penalties
            </p>
          </div>
        </div>

        <div className="settings-header-actions">
          <button
            type="button"
            className="btn-primary"
            onClick={onBackToRouting}
          >
            <Compass size={16} />
            <span>Return to Map View</span>
          </button>
        </div>
      </div>

      {/* Main Settings Cards Grid */}
      <div className="settings-content-grid">
        
        {/* Card 1: Boat Configuration & Model Selection */}
        <div className="settings-card">
          <div className="settings-card-header">
            <div className="settings-card-title">
              <Anchor size={16} color="#38bdf8" />
              <span>Boat Configuration &amp; Polar Model</span>
            </div>

            <button
              type="button"
              className="btn-add-custom-boat"
              onClick={() => setShowCustomModal(true)}
            >
              <Plus size={13} />
              <span>Custom Yacht Builder</span>
            </button>
          </div>

          <div className="settings-card-body">
            <div className="input-group">
              <div className="settings-label-row">
                <label>Select Active Yacht Model</label>
                {currentBoat?.isCustom && (
                  <span className="custom-boat-badge">
                    {currentBoat?.isPolFileOnly ? '📁 Uploaded POL File' : '🛠️ Custom Hull VPP'}
                  </span>
                )}
              </div>

              <select
                className="select-field"
                value={selectedPresetId}
                onChange={(e) => onSelectPreset(e.target.value)}
              >
                <optgroup label="Built-in Standard Presets">
                  {presets
                    .filter((p) => !p.isCustom)
                    .map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name} ({p.rig_type.toUpperCase()})
                      </option>
                    ))}
                </optgroup>
                {presets.some((p) => p.isCustom) && (
                  <optgroup label="Custom User Yachts">
                    {presets
                      .filter((p) => p.isCustom)
                      .map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.isPolFileOnly ? '📁' : '🛠️'} {p.name} ({p.isPolFileOnly ? 'POL' : p.rig_type.toUpperCase()})
                        </option>
                      ))}
                  </optgroup>
                )}
              </select>
            </div>

            {/* Boat Dimensions & Displacement Specs */}
            {currentBoat && (
              <div className="boat-specs-quad-grid">
                <div className="spec-item">
                  <span className="spec-label">Length (LOA)</span>
                  <strong className="spec-val">{currentBoat.loa_m} m</strong>
                </div>
                <div className="spec-item">
                  <span className="spec-label">Beam</span>
                  <strong className="spec-val">{currentBoat.beam_m} m</strong>
                </div>
                <div className="spec-item">
                  <span className="spec-label">Draft</span>
                  <strong className="spec-val">{currentBoat.draft_m} m</strong>
                </div>
                <div className="spec-item">
                  <span className="spec-label">Displacement</span>
                  <strong className="spec-val">{currentBoat.displacement_kg.toLocaleString()} kg</strong>
                </div>
              </div>
            )}

            {/* VPP Inspector Deep Dive Launcher Banner */}
            <div
              className="vpp-launcher-banner"
              onClick={onOpenVPP}
            >
              <div className="vpp-launcher-info">
                <div className="vpp-launcher-icon">
                  <Gauge size={22} color="#38bdf8" />
                </div>
                <div className="vpp-launcher-texts">
                  <div className="vpp-launcher-title-row">
                    <span className="vpp-launcher-title">VPP Polar Diagram &amp; Performance Inspector</span>
                  </div>
                  <p className="vpp-launcher-desc">
                    Explore polar curves, velocity prediction tables, sail trimming, and ORC/CSV exports
                  </p>
                </div>
              </div>

              <button
                type="button"
                className="btn-open-vpp"
                onClick={(e) => {
                  e.stopPropagation();
                  onOpenVPP();
                }}
              >
                <span>Open VPP</span>
                <ChevronRight size={14} />
              </button>
            </div>

            {/* Custom Boat Actions */}
            {currentBoat?.isCustom && onDeleteCustomBoat && (
              <div className="delete-custom-boat-row">
                <button
                  type="button"
                  className="btn-delete-custom-boat"
                  onClick={() => {
                    if (confirm(`Delete custom boat "${currentBoat.name}"?`)) {
                      onDeleteCustomBoat(currentBoat.id);
                    }
                  }}
                >
                  <Trash2 size={13} />
                  <span>Delete "{currentBoat.name}"</span>
                </button>
              </div>
            )}
          </div>
        </div>

        {/* Card 2: Maneuver Penalties (Cruising vs Racing) */}
        <div className="settings-card">
          <div className="settings-card-header">
            <div className="settings-card-title">
              <Shield size={16} color="#38bdf8" />
              <span>Maneuver &amp; Tack Penalties</span>
            </div>

            <div className="preset-toggle-group">
              <button
                type="button"
                onClick={setCruisingMode}
                className={`btn-mode-toggle ${tackPenaltyMinutes >= 4.0 ? 'active' : ''}`}
              >
                Cruising Preset
              </button>
              <button
                type="button"
                onClick={setRacingMode}
                className={`btn-mode-toggle ${tackPenaltyMinutes < 4.0 && tackPenaltyMinutes > 0 ? 'active' : ''}`}
              >
                Racing Preset
              </button>
            </div>
          </div>

          <div className="settings-card-body">
            <p className="settings-card-desc">
              Maneuvers incur real-world time and boat speed penalties when crossing the wind. Setting appropriate penalties prevents the isochrone algorithm from making unrealistic micro-tacks.
            </p>

            <div className="penalties-sliders-grid">
              {/* Tack Slider */}
              <div className="penalty-slider-card">
                <div className="penalty-slider-header">
                  <span className="penalty-name">Tack Penalty (Windward)</span>
                  <span className="penalty-val">{tackPenaltyMinutes.toFixed(1)} min</span>
                </div>
                <input
                  type="range"
                  step="0.5"
                  min="0"
                  max="20"
                  className="custom-range-slider"
                  value={tackPenaltyMinutes}
                  onChange={(e) => onTackPenaltyChange(parseFloat(e.target.value) || 0)}
                />
                <span className="penalty-hint">Time lost turning bow through true wind</span>
              </div>

              {/* Gybe Slider */}
              <div className="penalty-slider-card">
                <div className="penalty-slider-header">
                  <span className="penalty-name">Gybe Penalty (Leeward)</span>
                  <span className="penalty-val">{gybePenaltyMinutes.toFixed(1)} min</span>
                </div>
                <input
                  type="range"
                  step="0.5"
                  min="0"
                  max="25"
                  className="custom-range-slider"
                  value={gybePenaltyMinutes}
                  onChange={(e) => onGybePenaltyChange(parseFloat(e.target.value) || 0)}
                />
                <span className="penalty-hint">Time lost handling spinnaker / boom crossing</span>
              </div>
            </div>

            {/* Active Mode Callout */}
            <div className="active-mode-callout">
              <div className="active-mode-left">
                <Sparkles size={14} color="#f59e0b" />
                <span>
                  Active Mode: <strong>{tackPenaltyMinutes >= 4.0 ? 'Cruiser Mode (5m Tack / 8m Gybe)' : 'Racer Mode (1.5m Tack / 2m Gybe)'}</strong>
                </span>
              </div>
              <span className="active-mode-right">Applied in isochrone node cost</span>
            </div>
          </div>
        </div>

      </div>

      {/* Custom Boat Builder Modal */}
      {showCustomModal && (
        <CustomBoatBuilderModal
          isOpen={showCustomModal}
          onClose={() => setShowCustomModal(false)}
          onSaveBoat={(newPreset: BoatPreset) => {
            onAddCustomBoat(newPreset);
            setShowCustomModal(false);
          }}
          builtinPresets={presets}
        />
      )}
    </div>
  );
};

export default SettingsView;
