import React, { useMemo, useState } from 'react';
import { RouteResult } from '../types';
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler,
  ChartOptions,
} from 'chart.js';
import { Line } from 'react-chartjs-2';
import {
  Clock,
  Wind,
  Gauge,
  Compass,
  Activity,
  Calendar,
  Layers,
  TrendingUp,
  BarChart3,
  Shield,
  Navigation,
} from 'lucide-react';

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
);

interface PassageStatisticsProps {
  routeResult: RouteResult | null;
}

export const PassageStatistics: React.FC<PassageStatisticsProps> = ({ routeResult }) => {
  const [chartViewMode, setChartViewMode] = useState<'grid' | 'twa' | 'tws' | 'sog' | 'heel' | 'combined'>('grid');

  // Compute passage statistics
  const stats = useMemo(() => {
    if (!routeResult || !routeResult.waypoints || routeResult.waypoints.length === 0) {
      return null;
    }

    const wps = routeResult.waypoints;
    const n = wps.length;

    // 1. Time & Duration calculations
    const startDate = new Date(routeResult.start_time);
    const arrivalDate = new Date(routeResult.arrival_time);
    const durHours = routeResult.total_duration_hours;
    const durDays = Math.floor(durHours / 24);
    const durRemHours = Math.floor(durHours % 24);
    const durMinutes = Math.round((durHours * 60) % 60);

    const formattedDuration =
      durDays > 0
        ? `${durDays}d ${durRemHours}h ${durMinutes}m (${durHours.toFixed(1)} hrs)`
        : `${durRemHours}h ${durMinutes}m (${durHours.toFixed(1)} hrs)`;

    // 2. Wind breakdown & Point of Sail percentages
    let upwindCount = 0;
    let reachingCount = 0;
    let downwindCount = 0;

    let upwindDist = 0;
    let reachingDist = 0;
    let downwindDist = 0;

    let minWind = Infinity;
    let maxWind = -Infinity;
    let sumWind = 0;

    let maxSOG = -Infinity;
    let sumSOG = 0;

    let minHeel = Infinity;
    let maxHeel = -Infinity;
    let sumHeel = 0;

    for (let i = 0; i < n; i++) {
      const wp = wps[i];
      const prevWp = i > 0 ? wps[i - 1] : wp;
      const stepDist = Math.max(0, wp.distance_nm - prevWp.distance_nm);

      // Point of Sail by TWA:
      // Upwind: TWA < 60° (Close-hauled / Beating)
      // Reaching: 60° <= TWA < 120° (Close reach / Beam reach / Broad reach)
      // Downwind: TWA >= 120° (Running / Deep downwind)
      if (wp.twa_deg < 60.0) {
        upwindCount++;
        upwindDist += stepDist;
      } else if (wp.twa_deg < 120.0) {
        reachingCount++;
        reachingDist += stepDist;
      } else {
        downwindCount++;
        downwindDist += stepDist;
      }

      // Wind stats
      if (wp.tws_kts < minWind) minWind = wp.tws_kts;
      if (wp.tws_kts > maxWind) maxWind = wp.tws_kts;
      sumWind += wp.tws_kts;

      // Speed stats
      if (wp.boat_speed_kts > maxSOG) maxSOG = wp.boat_speed_kts;
      sumSOG += wp.boat_speed_kts;

      // Heel stats
      if (wp.estimated_heel_deg < minHeel) minHeel = wp.estimated_heel_deg;
      if (wp.estimated_heel_deg > maxHeel) maxHeel = wp.estimated_heel_deg;
      sumHeel += wp.estimated_heel_deg;
    }

    const pctUpwind = (upwindCount / n) * 100;
    const pctReaching = (reachingCount / n) * 100;
    const pctDownwind = (downwindCount / n) * 100;

    const avgWind = sumWind / n;
    const avgSOG = sumSOG / n;
    const avgHeel = sumHeel / n;

    return {
      startDate,
      arrivalDate,
      formattedDuration,
      durHours,
      pctUpwind,
      pctReaching,
      pctDownwind,
      upwindDist,
      reachingDist,
      downwindDist,
      minWind: minWind === Infinity ? 0 : minWind,
      maxWind: maxWind === -Infinity ? 0 : maxWind,
      avgWind,
      maxSOG: maxSOG === -Infinity ? 0 : maxSOG,
      avgSOG,
      minHeel: minHeel === Infinity ? 0 : minHeel,
      maxHeel: maxHeel === -Infinity ? 0 : maxHeel,
      avgHeel,
    };
  }, [routeResult]);

  // Generate chart data labels and dataset values
  const chartData = useMemo(() => {
    if (!routeResult || !routeResult.waypoints || routeResult.waypoints.length === 0) {
      return null;
    }

    const wps = routeResult.waypoints;
    const startTime = new Date(routeResult.start_time).getTime();

    // Time-based labels (e.g. "+0h", "+2.5h", "+5h")
    const labels = wps.map((wp) => {
      const t = new Date(wp.time).getTime();
      const elapsedHours = (t - startTime) / (1000 * 60 * 60);
      if (elapsedHours < 24) {
        return `+${elapsedHours.toFixed(1)}h`;
      }
      const days = Math.floor(elapsedHours / 24);
      const rem = (elapsedHours % 24).toFixed(0);
      return `d${days}+${rem}h`;
    });

    const twaData = wps.map((wp) => wp.twa_deg);
    const twsData = wps.map((wp) => wp.tws_kts);
    const sogData = wps.map((wp) => wp.boat_speed_kts);
    const heelData = wps.map((wp) => wp.estimated_heel_deg);

    return {
      labels,
      twaData,
      twsData,
      sogData,
      heelData,
      waypoints: wps,
    };
  }, [routeResult]);

  if (!routeResult || !stats || !chartData) {
    return (
      <div className="statistics-empty-state">
        <div className="empty-state-card">
          <Activity size={48} className="text-accent mb-4" />
          <h2>No Passage Calculated Yet</h2>
          <p>
            Configure your start and destination points in the sidebar, then click{' '}
            <strong style={{ color: 'var(--accent)' }}>Calculate Optimal Route</strong> to generate comprehensive
            passage statistics and time series plots.
          </p>
        </div>
      </div>
    );
  }

  // Common chart styling options
  const baseChartOptions: ChartOptions<'line'> = {
    responsive: true,
    maintainAspectRatio: false,
    interaction: {
      mode: 'index',
      intersect: false,
    },
    plugins: {
      legend: {
        position: 'top',
        labels: {
          color: '#cbd5e1',
          font: { family: 'Inter', size: 12, weight: 'bold' },
          boxWidth: 14,
          padding: 12,
        },
      },
      tooltip: {
        backgroundColor: 'rgba(15, 23, 42, 0.95)',
        titleColor: '#38bdf8',
        bodyColor: '#f8fafc',
        borderColor: 'rgba(56, 189, 248, 0.3)',
        borderWidth: 1,
        padding: 12,
        boxPadding: 6,
        bodyFont: { family: 'JetBrains Mono', size: 12 },
        callbacks: {
          title: (items) => {
            if (!items.length) return '';
            const idx = items[0].dataIndex;
            const wp = chartData.waypoints[idx];
            const date = new Date(wp.time);
            return `${date.toUTCString().slice(0, 22)} (Step ${idx})`;
          },
          afterBody: (items) => {
            if (!items.length) return [];
            const idx = items[0].dataIndex;
            const wp = chartData.waypoints[idx];
            const lines = [
              `Position: ${wp.lat.toFixed(3)}°N, ${Math.abs(wp.lon).toFixed(3)}°W`,
              `Distance: ${wp.distance_nm.toFixed(1)} NM (${wp.distance_to_dest_nm.toFixed(1)} NM to dest)`,
              `Heading: ${wp.heading_deg.toFixed(1)}° (TWD: ${wp.twd_deg.toFixed(1)}°)`,
            ];
            if (wp.maneuver && wp.maneuver !== 'none') {
              lines.push(`Maneuver: ${wp.maneuver.toUpperCase()}`);
            }
            return lines;
          },
        },
      },
    },
    scales: {
      x: {
        grid: { color: 'rgba(255, 255, 255, 0.06)' },
        ticks: {
          color: '#94a3b8',
          font: { family: 'JetBrains Mono', size: 10 },
          maxRotation: 0,
          autoSkip: true,
          maxTicksLimit: 12,
        },
      },
      y: {
        grid: { color: 'rgba(255, 255, 255, 0.06)' },
        ticks: {
          color: '#94a3b8',
          font: { family: 'JetBrains Mono', size: 11 },
        },
      },
    },
  };

  // 1. TWA Plot Data
  const twaChartData = {
    labels: chartData.labels,
    datasets: [
      {
        label: 'True Wind Angle — TWA (°)',
        data: chartData.twaData,
        borderColor: '#a855f7',
        backgroundColor: 'rgba(168, 85, 247, 0.15)',
        fill: true,
        borderWidth: 2,
        pointRadius: 0,
        pointHoverRadius: 5,
        tension: 0.25,
      },
    ],
  };

  const twaOptions: ChartOptions<'line'> = {
    ...baseChartOptions,
    scales: {
      ...baseChartOptions.scales,
      y: {
        ...baseChartOptions.scales?.y,
        min: 0,
        max: 180,
        title: { display: true, text: 'TWA [°]', color: '#a855f7', font: { weight: 'bold' } },
        ticks: {
          stepSize: 30,
          callback: (val) => `${val}°`,
          color: '#cbd5e1',
        },
      },
    },
  };

  // 2. TWS Plot Data
  const twsChartData = {
    labels: chartData.labels,
    datasets: [
      {
        label: 'True Wind Speed — TWS (kts)',
        data: chartData.twsData,
        borderColor: '#10b981',
        backgroundColor: 'rgba(16, 185, 129, 0.15)',
        fill: true,
        borderWidth: 2,
        pointRadius: 0,
        pointHoverRadius: 5,
        tension: 0.25,
      },
    ],
  };

  const twsOptions: ChartOptions<'line'> = {
    ...baseChartOptions,
    scales: {
      ...baseChartOptions.scales,
      y: {
        ...baseChartOptions.scales?.y,
        min: 0,
        title: { display: true, text: 'Wind Speed [kts]', color: '#10b981', font: { weight: 'bold' } },
        ticks: {
          callback: (val) => `${val} kts`,
          color: '#cbd5e1',
        },
      },
    },
  };

  // 3. SOG Plot Data
  const sogChartData = {
    labels: chartData.labels,
    datasets: [
      {
        label: 'Speed Over Ground — SOG (kts)',
        data: chartData.sogData,
        borderColor: '#38bdf8',
        backgroundColor: 'rgba(56, 189, 248, 0.18)',
        fill: true,
        borderWidth: 2.2,
        pointRadius: 0,
        pointHoverRadius: 5,
        tension: 0.25,
      },
    ],
  };

  const sogOptions: ChartOptions<'line'> = {
    ...baseChartOptions,
    scales: {
      ...baseChartOptions.scales,
      y: {
        ...baseChartOptions.scales?.y,
        min: 0,
        title: { display: true, text: 'Boat Speed [kts]', color: '#38bdf8', font: { weight: 'bold' } },
        ticks: {
          callback: (val) => `${val} kts`,
          color: '#cbd5e1',
        },
      },
    },
  };

  // 4. Heel Plot Data
  const heelChartData = {
    labels: chartData.labels,
    datasets: [
      {
        label: 'Estimated Heel Angle (°)',
        data: chartData.heelData,
        borderColor: '#f59e0b',
        backgroundColor: 'rgba(245, 158, 11, 0.15)',
        fill: true,
        borderWidth: 2,
        pointRadius: 0,
        pointHoverRadius: 5,
        tension: 0.25,
      },
    ],
  };

  const heelOptions: ChartOptions<'line'> = {
    ...baseChartOptions,
    scales: {
      ...baseChartOptions.scales,
      y: {
        ...baseChartOptions.scales?.y,
        min: 0,
        max: Math.max(30, Math.ceil(stats.maxHeel / 5) * 5 + 5),
        title: { display: true, text: 'Heel Angle [°]', color: '#f59e0b', font: { weight: 'bold' } },
        ticks: {
          stepSize: 5,
          callback: (val) => `${val}°`,
          color: '#cbd5e1',
        },
      },
    },
  };

  // 5. Combined Multi-Axis Chart Data
  const combinedChartData = {
    labels: chartData.labels,
    datasets: [
      {
        label: 'SOG (kts)',
        data: chartData.sogData,
        borderColor: '#38bdf8',
        backgroundColor: 'rgba(56, 189, 248, 0.08)',
        fill: true,
        borderWidth: 2,
        pointRadius: 0,
        yAxisID: 'y_speed',
      },
      {
        label: 'TWS (kts)',
        data: chartData.twsData,
        borderColor: '#10b981',
        borderDash: [5, 4],
        borderWidth: 2,
        pointRadius: 0,
        yAxisID: 'y_speed',
      },
      {
        label: 'TWA (°)',
        data: chartData.twaData,
        borderColor: '#a855f7',
        borderWidth: 1.5,
        pointRadius: 0,
        yAxisID: 'y_angle',
      },
      {
        label: 'Heel (°)',
        data: chartData.heelData,
        borderColor: '#f59e0b',
        borderWidth: 1.5,
        pointRadius: 0,
        yAxisID: 'y_angle',
      },
    ],
  };

  const combinedOptions: ChartOptions<'line'> = {
    ...baseChartOptions,
    scales: {
      ...baseChartOptions.scales,
      y_speed: {
        type: 'linear',
        display: true,
        position: 'left',
        title: { display: true, text: 'Speed [kts]', color: '#38bdf8', font: { weight: 'bold' } },
        grid: { color: 'rgba(255, 255, 255, 0.06)' },
        ticks: { color: '#38bdf8' },
      },
      y_angle: {
        type: 'linear',
        display: true,
        position: 'right',
        min: 0,
        max: 180,
        title: { display: true, text: 'Angle [°]', color: '#f59e0b', font: { weight: 'bold' } },
        grid: { drawOnChartArea: false },
        ticks: { color: '#f59e0b', stepSize: 30, callback: (v) => `${v}°` },
      },
    },
  };

  return (
    <div className="passage-statistics-view">
      {/* Header Banner */}
      <div className="stats-header-banner">
        <div>
          <div className="stats-badge">
            <BarChart3 size={14} />
            <span>Passage Performance & Weather Telemetry</span>
          </div>
          <h1>{routeResult.boat_name} — Passage Statistics</h1>
          <p className="stats-subtitle">
            From <span className="highlight">({routeResult.start_point.lat.toFixed(3)}°, {routeResult.start_point.lon.toFixed(3)}°)</span> to{' '}
            <span className="highlight">({routeResult.dest_point.lat.toFixed(3)}°, {routeResult.dest_point.lon.toFixed(3)}°)</span> • Total Distance:{' '}
            <strong>{routeResult.total_distance_nm.toFixed(1)} NM</strong>
          </p>
        </div>

        <div className="stats-status-badge">
          {routeResult.destination_reached ? (
            <span className="badge-reached">
              <Shield size={14} /> Destination Reached
            </span>
          ) : (
            <span className="badge-partial">
              <Navigation size={14} /> Max Time Frontier
            </span>
          )}
        </div>
      </div>

      {/* Main Required Statistics Table */}
      <div className="stats-table-section">
        <div className="table-card">
          <div className="table-card-header">
            <Calendar size={18} className="text-accent" />
            <h3>Comprehensive Passage Summary Table</h3>
          </div>

          <div className="responsive-table-wrapper">
            <table className="passage-summary-table">
              <thead>
                <tr>
                  <th>Category</th>
                  <th>Metric</th>
                  <th>Value</th>
                </tr>
              </thead>
              <tbody>
                {/* 1. Start Time */}
                <tr title={`Departure timestamp (ISO: ${routeResult.start_time})`}>
                  <td className="row-category" rowSpan={3}>
                    <Clock size={16} /> Time &amp; Schedule
                  </td>
                  <td className="metric-label">Start Time</td>
                  <td className="metric-value font-mono">
                    {stats.startDate.toUTCString().replace(' GMT', ' UTC')}
                  </td>
                </tr>

                {/* 2. Duration */}
                <tr title={`Total elapsed sailing time across ${routeResult.waypoints.length} isochrone steps`}>
                  <td className="metric-label">Passage Duration</td>
                  <td className="metric-value font-mono highlight-duration">
                    {stats.formattedDuration}
                  </td>
                </tr>

                {/* 3. Arrival Time */}
                <tr title={`Estimated arrival at destination (Remaining dist: ${routeResult.waypoints[routeResult.waypoints.length - 1].distance_to_dest_nm.toFixed(2)} NM)`}>
                  <td className="metric-label">Arrival Time</td>
                  <td className="metric-value font-mono">
                    {stats.arrivalDate.toUTCString().replace(' GMT', ' UTC')}
                  </td>
                </tr>

                {/* 4. Points of Sail Breakdowns */}
                <tr title={`Beating / Close-hauled (TWA < 60°) • ${stats.upwindDist.toFixed(1)} NM sailed`}>
                  <td className="row-category" rowSpan={3}>
                    <Compass size={16} /> Points of Sail
                  </td>
                  <td className="metric-label">
                    <span className="pos-indicator pos-upwind"></span> Percentage Upwind
                  </td>
                  <td className="metric-value font-mono text-cyan">
                    {stats.pctUpwind.toFixed(1)}%
                  </td>
                </tr>

                <tr title={`Beam & Broad Reach (60° ≤ TWA < 120°) • ${stats.reachingDist.toFixed(1)} NM sailed`}>
                  <td className="metric-label">
                    <span className="pos-indicator pos-reaching"></span> Percentage Reaching
                  </td>
                  <td className="metric-value font-mono text-emerald">
                    {stats.pctReaching.toFixed(1)}%
                  </td>
                </tr>

                <tr title={`Running (TWA ≥ 120°) • ${stats.downwindDist.toFixed(1)} NM sailed`}>
                  <td className="metric-label">
                    <span className="pos-indicator pos-downwind"></span> Percentage Downwind
                  </td>
                  <td className="metric-value font-mono text-purple">
                    {stats.pctDownwind.toFixed(1)}%
                  </td>
                </tr>

                {/* 5. Wind Speed Telemetry */}
                <tr title="Minimum True Wind Speed encountered on passage">
                  <td className="row-category" rowSpan={3}>
                    <Wind size={16} /> Wind Speed (TWS)
                  </td>
                  <td className="metric-label">Lowest Wind</td>
                  <td className="metric-value font-mono text-emerald">
                    {stats.minWind.toFixed(1)} kts
                  </td>
                </tr>

                <tr title="Peak gust/breeze condition encountered">
                  <td className="metric-label">Highest Wind</td>
                  <td className="metric-value font-mono text-warning">
                    {stats.maxWind.toFixed(1)} kts
                  </td>
                </tr>

                <tr title="Mean True Wind Speed over full route">
                  <td className="metric-label">Average Wind</td>
                  <td className="metric-value font-mono text-accent">
                    {stats.avgWind.toFixed(1)} kts
                  </td>
                </tr>

                {/* 6. Boat Performance Metrics */}
                <tr title={`Mean speed over ground (Max: ${stats.maxSOG.toFixed(2)} kts)`}>
                  <td className="row-category" rowSpan={3}>
                    <Gauge size={16} /> Boat Performance
                  </td>
                  <td className="metric-label">Average SOG (Boat Speed)</td>
                  <td className="metric-value font-mono text-accent">
                    {routeResult.average_speed_kts.toFixed(2)} kts
                  </td>
                </tr>

                <tr title="Hydrodynamic heel stability estimate from VPP matrix">
                  <td className="metric-label">Estimated Heel Angle</td>
                  <td className="metric-value font-mono text-warning">
                    Avg: {stats.avgHeel.toFixed(1)}° / Max: {stats.maxHeel.toFixed(1)}°
                  </td>
                </tr>

                <tr title={`Tack penalty: ${routeResult.tack_penalty_minutes}m, Gybe penalty: ${routeResult.gybe_penalty_minutes}m`}>
                  <td className="metric-label">Maneuvers &amp; Penalties</td>
                  <td className="metric-value font-mono">
                    {routeResult.total_tacks} Tacks, {routeResult.total_gybes} Gybes
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* Point of Sail Visual Bar */}
      <div className="pos-visual-bar-card">
        <div className="pos-bar-header">
          <span>Point of Sail Distribution (% Time)</span>
          <div className="pos-legend">
            <span className="legend-item"><span className="dot dot-cyan"></span> Upwind ({stats.pctUpwind.toFixed(1)}%)</span>
            <span className="legend-item"><span className="dot dot-emerald"></span> Reaching ({stats.pctReaching.toFixed(1)}%)</span>
            <span className="legend-item"><span className="dot dot-purple"></span> Downwind ({stats.pctDownwind.toFixed(1)}%)</span>
          </div>
        </div>
        <div className="pos-progress-track">
          <div className="pos-seg pos-seg-upwind" style={{ width: `${stats.pctUpwind}%` }} title={`Upwind: ${stats.pctUpwind.toFixed(1)}%`} />
          <div className="pos-seg pos-seg-reaching" style={{ width: `${stats.pctReaching}%` }} title={`Reaching: ${stats.pctReaching.toFixed(1)}%`} />
          <div className="pos-seg pos-seg-downwind" style={{ width: `${stats.pctDownwind}%` }} title={`Downwind: ${stats.pctDownwind.toFixed(1)}%`} />
        </div>
      </div>

      {/* Time Plots Section */}
      <div className="stats-plots-section">
        <div className="plots-header">
          <div>
            <h2>Passage Time Telemetry Plots</h2>
            <p className="text-muted text-sm">Synchronized time series plots across passage duration</p>
          </div>

          <div className="plot-mode-tabs">
            <button
              className={`plot-tab-btn ${chartViewMode === 'grid' ? 'active' : ''}`}
              onClick={() => setChartViewMode('grid')}
            >
              <Layers size={14} /> 2x2 Grid
            </button>
            <button
              className={`plot-tab-btn ${chartViewMode === 'twa' ? 'active' : ''}`}
              onClick={() => setChartViewMode('twa')}
            >
              TWA
            </button>
            <button
              className={`plot-tab-btn ${chartViewMode === 'tws' ? 'active' : ''}`}
              onClick={() => setChartViewMode('tws')}
            >
              TWS
            </button>
            <button
              className={`plot-tab-btn ${chartViewMode === 'sog' ? 'active' : ''}`}
              onClick={() => setChartViewMode('sog')}
            >
              SOG
            </button>
            <button
              className={`plot-tab-btn ${chartViewMode === 'heel' ? 'active' : ''}`}
              onClick={() => setChartViewMode('heel')}
            >
              Heel
            </button>
            <button
              className={`plot-tab-btn ${chartViewMode === 'combined' ? 'active' : ''}`}
              onClick={() => setChartViewMode('combined')}
            >
              <TrendingUp size={14} /> Combined
            </button>
          </div>
        </div>

        {/* 2x2 Grid View */}
        {chartViewMode === 'grid' && (
          <div className="plots-grid-2x2">
            {/* Chart 1: TWA */}
            <div className="plot-card">
              <div className="plot-card-header">
                <div className="plot-card-title">
                  <Compass size={16} className="text-purple" />
                  <span>True Wind Angle (TWA)</span>
                </div>
                <span className="plot-card-badge">0° — 180°</span>
              </div>
              <div className="plot-canvas-container">
                <Line data={twaChartData} options={twaOptions} />
              </div>
            </div>

            {/* Chart 2: TWS */}
            <div className="plot-card">
              <div className="plot-card-header">
                <div className="plot-card-title">
                  <Wind size={16} className="text-emerald" />
                  <span>True Wind Speed (TWS)</span>
                </div>
                <span className="plot-card-badge">Min: {stats.minWind.toFixed(1)}k • Max: {stats.maxWind.toFixed(1)}k</span>
              </div>
              <div className="plot-canvas-container">
                <Line data={twsChartData} options={twsOptions} />
              </div>
            </div>

            {/* Chart 3: SOG */}
            <div className="plot-card">
              <div className="plot-card-header">
                <div className="plot-card-title">
                  <Gauge size={16} className="text-accent" />
                  <span>Speed Over Ground (SOG)</span>
                </div>
                <span className="plot-card-badge">Avg: {stats.avgSOG.toFixed(2)}k • Max: {stats.maxSOG.toFixed(2)}k</span>
              </div>
              <div className="plot-canvas-container">
                <Line data={sogChartData} options={sogOptions} />
              </div>
            </div>

            {/* Chart 4: Heel */}
            <div className="plot-card">
              <div className="plot-card-header">
                <div className="plot-card-title">
                  <Activity size={16} className="text-warning" />
                  <span>Estimated Heel Angle</span>
                </div>
                <span className="plot-card-badge">Avg: {stats.avgHeel.toFixed(1)}° • Max: {stats.maxHeel.toFixed(1)}°</span>
              </div>
              <div className="plot-canvas-container">
                <Line data={heelChartData} options={heelOptions} />
              </div>
            </div>
          </div>
        )}

        {/* Single Full-Width Views */}
        {chartViewMode === 'twa' && (
          <div className="plot-card plot-card-full">
            <div className="plot-card-header">
              <div className="plot-card-title">
                <Compass size={18} className="text-purple" />
                <span>True Wind Angle (TWA) vs Passage Time</span>
              </div>
              <span className="plot-card-badge">0° = Headwind | 90° = Beam Reach | 180° = Dead Run</span>
            </div>
            <div className="plot-canvas-container-large">
              <Line data={twaChartData} options={twaOptions} />
            </div>
          </div>
        )}

        {chartViewMode === 'tws' && (
          <div className="plot-card plot-card-full">
            <div className="plot-card-header">
              <div className="plot-card-title">
                <Wind size={18} className="text-emerald" />
                <span>True Wind Speed (TWS) vs Passage Time</span>
              </div>
              <span className="plot-card-badge">Lowest: {stats.minWind.toFixed(1)} kts • Highest: {stats.maxWind.toFixed(1)} kts • Mean: {stats.avgWind.toFixed(1)} kts</span>
            </div>
            <div className="plot-canvas-container-large">
              <Line data={twsChartData} options={twsOptions} />
            </div>
          </div>
        )}

        {chartViewMode === 'sog' && (
          <div className="plot-card plot-card-full">
            <div className="plot-card-header">
              <div className="plot-card-title">
                <Gauge size={18} className="text-accent" />
                <span>Speed Over Ground (SOG) vs Passage Time</span>
              </div>
              <span className="plot-card-badge">Average SOG: {stats.avgSOG.toFixed(2)} kts • Peak SOG: {stats.maxSOG.toFixed(2)} kts</span>
            </div>
            <div className="plot-canvas-container-large">
              <Line data={sogChartData} options={sogOptions} />
            </div>
          </div>
        )}

        {chartViewMode === 'heel' && (
          <div className="plot-card plot-card-full">
            <div className="plot-card-header">
              <div className="plot-card-title">
                <Activity size={18} className="text-warning" />
                <span>Estimated Heel Angle vs Passage Time</span>
              </div>
              <span className="plot-card-badge">Mean: {stats.avgHeel.toFixed(1)}° • Peak: {stats.maxHeel.toFixed(1)}°</span>
            </div>
            <div className="plot-canvas-container-large">
              <Line data={heelChartData} options={heelOptions} />
            </div>
          </div>
        )}

        {chartViewMode === 'combined' && (
          <div className="plot-card plot-card-full">
            <div className="plot-card-header">
              <div className="plot-card-title">
                <TrendingUp size={18} className="text-accent" />
                <span>Multi-Variable Passage Telemetry (SOG, TWS, TWA, Heel)</span>
              </div>
              <span className="plot-card-badge">Dual-Axis Multi-Series Overlay</span>
            </div>
            <div className="plot-canvas-container-large">
              <Line data={combinedChartData} options={combinedOptions} />
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
