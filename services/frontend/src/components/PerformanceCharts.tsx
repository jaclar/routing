import React from 'react';
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
} from 'chart.js';
import { Line } from 'react-chartjs-2';

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

interface PerformanceChartsProps {
  routeResult: RouteResult;
}

export const PerformanceCharts: React.FC<PerformanceChartsProps> = ({ routeResult }) => {
  const waypoints = routeResult.waypoints;
  const labels = waypoints.map((_, idx) => `${idx * 2}h`);

  const data = {
    labels,
    datasets: [
      {
        label: 'Boat Speed (kts)',
        data: waypoints.map((wp) => wp.boat_speed_kts),
        borderColor: '#38bdf8',
        backgroundColor: 'rgba(56, 189, 248, 0.15)',
        fill: true,
        tension: 0.3,
        yAxisID: 'y',
      },
      {
        label: 'True Wind Speed (kts)',
        data: waypoints.map((wp) => wp.tws_kts),
        borderColor: '#10b981',
        backgroundColor: 'transparent',
        borderDash: [4, 4],
        tension: 0.3,
        yAxisID: 'y',
      },
      {
        label: 'Heel Angle (deg)',
        data: waypoints.map((wp) => wp.estimated_heel_deg),
        borderColor: '#f59e0b',
        backgroundColor: 'transparent',
        tension: 0.3,
        yAxisID: 'y1',
      },
    ],
  };

  const options = {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: {
        position: 'top' as const,
        labels: {
          color: '#94a3b8',
          font: { size: 11 },
        },
      },
      tooltip: {
        mode: 'index' as const,
        intersect: false,
      },
    },
    scales: {
      x: {
        grid: { color: 'rgba(255, 255, 255, 0.05)' },
        ticks: { color: '#94a3b8' },
      },
      y: {
        type: 'linear' as const,
        display: true,
        position: 'left' as const,
        title: { display: true, text: 'Speed [kts]', color: '#94a3b8' },
        grid: { color: 'rgba(255, 255, 255, 0.05)' },
        ticks: { color: '#94a3b8' },
      },
      y1: {
        type: 'linear' as const,
        display: true,
        position: 'right' as const,
        title: { display: true, text: 'Heel [deg]', color: '#f59e0b' },
        grid: { drawOnChartArea: false },
        ticks: { color: '#f59e0b' },
      },
    },
  };

  return (
    <div style={{ height: '180px', width: '100%' }}>
      <Line data={data} options={options} />
    </div>
  );
};
