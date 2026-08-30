#!/usr/bin/env python3
"""
Wavefront Comparison Visualizer Generator
Parses wavefront_data.json and pruning_benchmark_results.json to generate an interactive
HTML app allowing visual exploration and side-by-side comparison of all 5 pruning strategies.
"""

import sys
import os
import json

HTML_TEMPLATE = """<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Isochrone Wavefront Pruning Strategies Visualizer</title>
  <script src="https://www.gstatic.com/antigravity/web/dev/tailwindcss.min.js"></script>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    }
    .custom-scrollbar::-webkit-scrollbar {
      width: 6px;
      height: 6px;
    }
    .custom-scrollbar::-webkit-scrollbar-thumb {
      background: #334155;
      border-radius: 4px;
    }
  </style>
</head>
<body class="bg-slate-950 text-slate-100 antialiased p-5">
  <div class="max-w-7xl mx-auto space-y-4">
    
    <!-- Top Header -->
    <div class="flex flex-wrap items-center justify-between gap-4 border-b border-slate-800 pb-3">
      <div>
        <h1 class="text-lg font-extrabold flex items-center gap-2 text-white">
          <span class="p-1 bg-cyan-500/20 text-cyan-400 rounded-lg border border-cyan-500/30">🌊</span>
          Isochrone Wavefront Pruning Strategy Comparison
        </h1>
        <p class="text-xs text-slate-400 mt-0.5">Visualizing spatial propagation &amp; metric tradeoffs across all 5 pruning algorithms</p>
      </div>
      
      <!-- Scenario Selector -->
      <div class="flex items-center gap-2">
        <label class="text-xs text-slate-400 font-medium">Scenario:</label>
        <select id="scenarioSelect" class="bg-slate-900 border border-slate-700 text-cyan-400 text-xs rounded-lg px-3 py-1.5 focus:outline-none focus:border-cyan-500 font-semibold cursor-pointer">
          <!-- Populated dynamically -->
        </select>
      </div>
    </div>

    <!-- Main Studio Grid -->
    <div class="grid grid-cols-1 lg:grid-cols-4 gap-4">
      
      <!-- Canvas Simulation (3 cols) -->
      <div class="lg:col-span-3 bg-slate-900 border border-slate-800 rounded-2xl p-4 shadow-xl flex flex-col gap-3">
        
        <!-- Controls Bar -->
        <div class="flex flex-wrap items-center justify-between gap-3 bg-slate-950/70 border border-slate-800/80 rounded-xl p-2.5 px-3.5">
          <div class="flex items-center gap-2">
            <button id="btnPlay" class="px-3.5 py-1.5 text-xs font-semibold bg-cyan-600 hover:bg-cyan-500 text-white rounded-lg transition active:scale-95 shadow-sm">
              ▶ Play Wavefronts
            </button>
            <button id="btnReset" class="px-3 py-1.5 text-xs font-medium bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg transition active:scale-95">
              ↺ Reset
            </button>
          </div>

          <!-- Time Step Slider -->
          <div class="flex items-center gap-3 flex-1 max-w-md mx-2">
            <span class="text-xs text-slate-400 font-mono whitespace-nowrap">Step:</span>
            <input type="range" id="timeSlider" min="0" max="50" value="50" class="w-full h-1.5 bg-slate-800 rounded-lg appearance-none cursor-pointer accent-cyan-400">
            <span id="sliderLabel" class="text-xs font-mono font-bold text-cyan-400 whitespace-nowrap w-24 text-right">Step Max</span>
          </div>

          <!-- Layer Toggles -->
          <div class="flex items-center gap-3 text-xs">
            <label class="flex items-center gap-1.5 cursor-pointer text-slate-300 hover:text-white">
              <input type="checkbox" id="chkShowWaves" checked class="rounded border-slate-700 bg-slate-800 text-cyan-500">
              Wavefronts
            </label>
            <label class="flex items-center gap-1.5 cursor-pointer text-slate-300 hover:text-white">
              <input type="checkbox" id="chkShowRoutes" checked class="rounded border-slate-700 bg-slate-800 text-cyan-500">
              Optimal Paths
            </label>
          </div>
        </div>

        <!-- 2D Canvas Map -->
        <div class="relative bg-slate-950 rounded-xl overflow-hidden border border-slate-800 shadow-inner flex items-center justify-center">
          <canvas id="mapCanvas" width="850" height="480" class="w-full h-auto block cursor-crosshair"></canvas>
          
          <!-- Canvas Overlay Legend -->
          <div class="absolute bottom-2 left-2 right-2 bg-slate-900/90 backdrop-blur-sm border border-slate-800 rounded-lg p-2 flex flex-wrap items-center justify-between text-[10px] text-slate-300 gap-2">
            <div id="strategyLegend" class="flex flex-wrap items-center gap-3">
              <!-- Dynamically populated -->
            </div>
            <div class="text-[10px] text-slate-400 font-mono" id="coordIndicator">
              Hover to view map coordinates
            </div>
          </div>
        </div>

      </div>

      <!-- Strategy Selector & Live Scorecard (1 col) -->
      <div class="space-y-3 flex flex-col justify-between">
        
        <!-- Active Strategies Multi-Select -->
        <div class="bg-slate-900 border border-slate-800 rounded-2xl p-3.5 shadow-xl">
          <h3 class="text-xs font-bold uppercase tracking-wider text-slate-400 mb-2.5 flex items-center justify-between">
            <span>Compare Strategies</span>
            <button id="btnToggleAll" class="text-[10px] text-cyan-400 hover:underline">Toggle All</button>
          </h3>
          <div id="strategyCheckboxes" class="space-y-2 text-xs">
            <!-- Populated dynamically -->
          </div>
        </div>

        <!-- Strategy Benchmark Telemetry Cards -->
        <div class="bg-slate-900 border border-slate-800 rounded-2xl p-3.5 shadow-xl flex-1 flex flex-col">
          <h3 class="text-xs font-bold uppercase tracking-wider text-slate-400 mb-2 flex items-center justify-between">
            <span>Performance Scorecard</span>
            <span class="text-[10px] text-emerald-400 font-mono" id="scorecardScenarioName"></span>
          </h3>
          
          <div id="scorecardList" class="space-y-2 overflow-y-auto max-h-80 custom-scrollbar pr-1">
            <!-- Populated dynamically -->
          </div>
        </div>

      </div>

    </div>

  </div>

  <script>
    const WF_DATA = /*WF_DATA_PLACEHOLDER*/;
    const BENCH_DATA = /*BENCH_DATA_PLACEHOLDER*/;

    const STRATEGY_CONFIG = {
      'radial_sector': { name: '1. Radial Sector', color: '#f43f5e', lightColor: '#fb7185' },
      'spatial_grid': { name: '2. 2D Spatial Grid', color: '#10b981', lightColor: '#34d399' },
      'astar_beam': { name: '3. A* Beam Search', color: '#06b6d4', lightColor: '#22d3ee' },
      'pareto_envelope': { name: '4. Pareto Envelope', color: '#8b5cf6', lightColor: '#a78bfa' },
      'state_space_grid': { name: '5. State-Space Grid', color: '#f59e0b', lightColor: '#fbbf24' }
    };

    let activeScenarioId = Object.keys(WF_DATA)[0] || 'cowes_fastnet';
    let enabledStrategies = new Set(Object.keys(STRATEGY_CONFIG));
    let currentStep = 999;
    let isPlaying = false;
    let playTimer = null;

    const canvas = document.getElementById('mapCanvas');
    const ctx = canvas.getContext('2d');

    function initUI() {
      // Populate Scenario dropdown
      const select = document.getElementById('scenarioSelect');
      select.innerHTML = '';
      for (const [id, sc] of Object.entries(WF_DATA)) {
        const opt = document.createElement('option');
        opt.value = id;
        opt.textContent = sc.preset_name;
        if (id === activeScenarioId) opt.selected = true;
        select.appendChild(opt);
      }

      select.addEventListener('change', (e) => {
        activeScenarioId = e.target.value;
        stopPlay();
        updateSliderMax();
        render();
        updateScorecard();
      });

      // Populate Strategy Checkboxes
      const container = document.getElementById('strategyCheckboxes');
      container.innerHTML = '';
      for (const [id, cfg] of Object.entries(STRATEGY_CONFIG)) {
        const row = document.createElement('label');
        row.className = 'flex items-center justify-between p-2 rounded-lg bg-slate-950/60 border border-slate-800 cursor-pointer hover:border-slate-700 transition';
        row.innerHTML = `
          <div class="flex items-center gap-2">
            <span class="w-3 h-3 rounded-full" style="background: ${cfg.color}"></span>
            <span class="font-medium text-xs text-slate-200">${cfg.name}</span>
          </div>
          <input type="checkbox" value="${id}" checked class="rounded border-slate-700 bg-slate-800 text-cyan-500 strat-chk">
        `;
        const chk = row.querySelector('input');
        chk.addEventListener('change', (e) => {
          if (e.target.checked) enabledStrategies.add(id);
          else enabledStrategies.delete(id);
          render();
          updateScorecard();
        });
        container.appendChild(row);
      }

      document.getElementById('btnToggleAll').addEventListener('click', () => {
        const checkboxes = document.querySelectorAll('.strat-chk');
        if (enabledStrategies.size > 0) {
          enabledStrategies.clear();
          checkboxes.forEach(c => c.checked = false);
        } else {
          Object.keys(STRATEGY_CONFIG).forEach(id => enabledStrategies.add(id));
          checkboxes.forEach(c => c.checked = true);
        }
        render();
        updateScorecard();
      });

      document.getElementById('timeSlider').addEventListener('input', (e) => {
        currentStep = parseInt(e.target.value, 10);
        render();
      });

      document.getElementById('btnPlay').addEventListener('click', () => {
        if (isPlaying) stopPlay();
        else startPlay();
      });

      document.getElementById('btnReset').addEventListener('click', () => {
        stopPlay();
        currentStep = 0;
        document.getElementById('timeSlider').value = 0;
        render();
      });

      document.getElementById('chkShowWaves').addEventListener('change', render);
      document.getElementById('chkShowRoutes').addEventListener('change', render);

      updateSliderMax();
      render();
      updateScorecard();
    }

    function updateSliderMax() {
      const sc = WF_DATA[activeScenarioId];
      if (!sc) return;
      let maxSteps = 0;
      for (const run of Object.values(sc.runs)) {
        if (run.isochrones && run.isochrones.length > maxSteps) {
          maxSteps = run.isochrones.length;
        }
      }
      const slider = document.getElementById('timeSlider');
      slider.max = maxSteps;
      slider.value = maxSteps;
      currentStep = maxSteps;
    }

    function startPlay() {
      isPlaying = true;
      const btn = document.getElementById('btnPlay');
      btn.textContent = '⏸ Pause';
      btn.className = 'px-3.5 py-1.5 text-xs font-semibold bg-rose-600 hover:bg-rose-500 text-white rounded-lg transition active:scale-95';

      const slider = document.getElementById('timeSlider');
      if (parseInt(slider.value, 10) >= parseInt(slider.max, 10)) {
        currentStep = 0;
        slider.value = 0;
      }

      playTimer = setInterval(() => {
        if (currentStep < parseInt(slider.max, 10)) {
          currentStep++;
          slider.value = currentStep;
          render();
        } else {
          stopPlay();
        }
      }, 250);
    }

    function stopPlay() {
      isPlaying = false;
      clearInterval(playTimer);
      const btn = document.getElementById('btnPlay');
      btn.textContent = '▶ Play Wavefronts';
      btn.className = 'px-3.5 py-1.5 text-xs font-semibold bg-cyan-600 hover:bg-cyan-500 text-white rounded-lg transition active:scale-95 shadow-sm';
    }

    function geoToCanvas(lat, lon, bounds) {
      const pad = 40;
      const w = canvas.width - pad * 2;
      const h = canvas.height - pad * 2;

      const normX = (lon - bounds.minLon) / (bounds.maxLon - bounds.minLon || 1);
      const normY = (bounds.maxLat - lat) / (bounds.maxLat - bounds.minLat || 1);

      return {
        x: pad + normX * w,
        y: pad + normY * h
      };
    }

    function calculateBounds(sc) {
      let minLat = Math.min(sc.start.lat, sc.dest.lat);
      let maxLat = Math.max(sc.start.lat, sc.dest.lat);
      let minLon = Math.min(sc.start.lon, sc.dest.lon);
      let maxLon = Math.max(sc.start.lon, sc.dest.lon);

      for (const run of Object.values(sc.runs)) {
        if (run.isochrones) {
          for (const wave of run.isochrones) {
            if (wave.points) {
              for (const pt of wave.points) {
                if (pt.lat < minLat) minLat = pt.lat;
                if (pt.lat > maxLat) maxLat = pt.lat;
                if (pt.lon < minLon) minLon = pt.lon;
                if (pt.lon > maxLon) maxLon = pt.lon;
              }
            }
          }
        }
      }

      const latMargin = Math.max(0.1, (maxLat - minLat) * 0.12);
      const lonMargin = Math.max(0.1, (maxLon - minLon) * 0.12);

      return {
        minLat: minLat - latMargin,
        maxLat: maxLat + latMargin,
        minLon: minLon - lonMargin,
        maxLon: maxLon + lonMargin
      };
    }

    function render() {
      ctx.clearRect(0, 0, canvas.width, canvas.height);

      const sc = WF_DATA[activeScenarioId];
      if (!sc) return;

      const bounds = calculateBounds(sc);
      const showWaves = document.getElementById('chkShowWaves').checked;
      const showRoutes = document.getElementById('chkShowRoutes').checked;

      // 1. Draw Grid Lines
      ctx.strokeStyle = '#1e293b';
      ctx.lineWidth = 0.5;
      for (let x = 0; x < canvas.width; x += 50) {
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, canvas.height);
        ctx.stroke();
      }
      for (let y = 0; y < canvas.height; y += 50) {
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(canvas.width, y);
        ctx.stroke();
      }

      // 2. Draw Start & Destination
      const startPt = geoToCanvas(sc.start.lat, sc.start.lon, bounds);
      const destPt = geoToCanvas(sc.dest.lat, sc.dest.lon, bounds);

      // Great circle direct reference line
      ctx.strokeStyle = 'rgba(100, 116, 139, 0.3)';
      ctx.lineWidth = 1;
      ctx.setLineDash([4, 4]);
      ctx.beginPath();
      ctx.moveTo(startPt.x, startPt.y);
      ctx.lineTo(destPt.x, destPt.y);
      ctx.stroke();
      ctx.setLineDash([]);

      // 3. Render Isochrone Wavefronts for each enabled strategy
      if (showWaves) {
        for (const [stratId, run] of Object.entries(sc.runs)) {
          if (!enabledStrategies.has(stratId) || !run.isochrones) continue;
          const cfg = STRATEGY_CONFIG[stratId] || { color: '#38bdf8' };

          const stepsToDraw = Math.min(currentStep, run.isochrones.length);

          for (let s = 0; s < stepsToDraw; s++) {
            const wave = run.isochrones[s];
            if (!wave.points || wave.points.length < 2) continue;

            const isLatest = s === stepsToDraw - 1;
            ctx.strokeStyle = isLatest ? cfg.color : `${cfg.color}44`;
            ctx.lineWidth = isLatest ? 2.2 : 1.0;
            ctx.setLineDash(isLatest ? [] : [2, 2]);

            ctx.beginPath();
            const p0 = geoToCanvas(wave.points[0].lat, wave.points[0].lon, bounds);
            ctx.moveTo(p0.x, p0.y);
            for (let i = 1; i < wave.points.length; i++) {
              const p = geoToCanvas(wave.points[i].lat, wave.points[i].lon, bounds);
              ctx.lineTo(p.x, p.y);
            }
            ctx.stroke();
            ctx.setLineDash([]);

            // Draw small dots on latest wave
            if (isLatest) {
              ctx.fillStyle = cfg.color;
              for (const pt of wave.points) {
                const cp = geoToCanvas(pt.lat, pt.lon, bounds);
                ctx.beginPath();
                ctx.arc(cp.x, cp.y, 2, 0, Math.PI * 2);
                ctx.fill();
              }
            }
          }
        }
      }

      // 4. Render Backtracked Optimal Routes
      if (showRoutes) {
        for (const [stratId, run] of Object.entries(sc.runs)) {
          if (!enabledStrategies.has(stratId) || !run.waypoints || run.waypoints.length === 0) continue;
          const cfg = STRATEGY_CONFIG[stratId] || { color: '#38bdf8' };

          ctx.strokeStyle = cfg.color;
          ctx.lineWidth = 2.5;
          ctx.lineJoin = 'round';
          ctx.lineCap = 'round';

          ctx.beginPath();
          const p0 = geoToCanvas(run.waypoints[0].lat, run.waypoints[0].lon, bounds);
          ctx.moveTo(p0.x, p0.y);
          for (let i = 1; i < run.waypoints.length; i++) {
            const p = geoToCanvas(run.waypoints[i].lat, run.waypoints[i].lon, bounds);
            ctx.lineTo(p.x, p.y);
          }
          ctx.stroke();

          // Waypoint dots
          for (let i = 0; i < run.waypoints.length; i += Math.max(1, Math.floor(run.waypoints.length / 12))) {
            const p = geoToCanvas(run.waypoints[i].lat, run.waypoints[i].lon, bounds);
            ctx.fillStyle = cfg.color;
            ctx.beginPath();
            ctx.arc(p.x, p.y, 3, 0, Math.PI * 2);
            ctx.fill();
          }
        }
      }

      // 5. Draw Start and Dest Markers
      // Start Marker
      ctx.fillStyle = '#10b981';
      ctx.beginPath();
      ctx.arc(startPt.x, startPt.y, 6, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = '#a7f3d0';
      ctx.font = 'bold 11px sans-serif';
      ctx.fillText('START: ' + sc.start.lat.toFixed(2) + ', ' + sc.start.lon.toFixed(2), startPt.x + 8, startPt.y - 6);

      // Destination Marker
      ctx.fillStyle = '#f43f5e';
      ctx.beginPath();
      ctx.arc(destPt.x, destPt.y, 6, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = '#fda4af';
      ctx.font = 'bold 11px sans-serif';
      ctx.fillText('DEST: ' + sc.dest.lat.toFixed(2) + ', ' + sc.dest.lon.toFixed(2), destPt.x + 8, destPt.y - 6);

      // Slider label update
      document.getElementById('sliderLabel').textContent = 'Step ' + currentStep + ' / ' + document.getElementById('timeSlider').max;
    }

    function updateScorecard() {
      const sc = WF_DATA[activeScenarioId];
      if (!sc) return;
      document.getElementById('scorecardScenarioName').textContent = sc.preset_name + ' (' + sc.direct_distance_nm.toFixed(1) + ' NM)';

      const list = document.getElementById('scorecardList');
      list.innerHTML = '';

      for (const [stratId, cfg] of Object.entries(STRATEGY_CONFIG)) {
        const run = sc.runs[stratId];
        if (!run) continue;
        const isEnabled = enabledStrategies.has(stratId);

        const card = document.createElement('div');
        card.className = `p-2.5 rounded-xl border transition ${isEnabled ? 'bg-slate-950/80 border-slate-800' : 'opacity-40 bg-slate-950/30 border-slate-900'}`;
        card.innerHTML = `
          <div class="flex items-center justify-between mb-1.5">
            <div class="flex items-center gap-1.5">
              <span class="w-2.5 h-2.5 rounded-full" style="background: ${cfg.color}"></span>
              <span class="font-bold text-xs text-slate-100">${cfg.name}</span>
            </div>
            <span class="text-[10px] font-bold px-1.5 py-0.5 rounded ${run.destination_reached ? 'bg-emerald-950 text-emerald-400 border border-emerald-800' : 'bg-rose-950 text-rose-400'}">
              ${run.destination_reached ? 'REACHED' : 'TRAPPED'}
            </span>
          </div>
          <div class="grid grid-cols-3 gap-1.5 text-[11px] font-mono">
            <div class="bg-slate-900/60 p-1.5 rounded">
              <span class="text-[9px] text-slate-500 block">Latency</span>
              <span class="font-bold text-amber-400">${run.mean_time_ms.toFixed(1)} ms</span>
            </div>
            <div class="bg-slate-900/60 p-1.5 rounded">
              <span class="text-[9px] text-slate-500 block">Passage Time</span>
              <span class="font-bold text-slate-200">${run.total_duration_hours.toFixed(1)} h</span>
            </div>
            <div class="bg-slate-900/60 p-1.5 rounded">
              <span class="text-[9px] text-slate-500 block">Sailed Dist</span>
              <span class="font-bold text-slate-200">${run.total_distance_nm.toFixed(1)} NM</span>
            </div>
          </div>
        `;
        list.appendChild(card);
      }
    }

    // Initialize UI on load
    initUI();
  </script>
</body>
</html>
"""

def compress_wavefront_data(wf_data):
    compressed = {}
    for sc_id, sc in wf_data.items():
        compressed[sc_id] = {
            'preset_id': sc['preset_id'],
            'preset_name': sc['preset_name'],
            'start': {'lat': round(sc['start']['lat'], 4), 'lon': round(sc['start']['lon'], 4)},
            'dest': {'lat': round(sc['dest']['lat'], 4), 'lon': round(sc['dest']['lon'], 4)},
            'time_step': sc['time_step'],
            'direct_distance_nm': round(sc['direct_distance_nm'], 1),
            'runs': {}
        }
        for strat_id, run in sc.get('runs', {}).items():
            compact_wps = []
            for wp in run.get('waypoints', []):
                compact_wps.append({
                    'lat': round(wp['lat'], 4),
                    'lon': round(wp['lon'], 4),
                    'boat_speed_kts': round(wp.get('boat_speed_kts', 0), 1),
                    'heading_deg': round(wp.get('heading_deg', 0), 1)
                })
            
            compact_waves = []
            for wave in run.get('isochrones', []):
                pts = wave.get('points', [])
                if not pts:
                    continue
                stride = max(1, len(pts) // 50)
                sampled_pts = [{'lat': round(p['lat'], 4), 'lon': round(p['lon'], 4)} for i, p in enumerate(pts) if i % stride == 0 or i == len(pts) - 1]
                compact_waves.append({
                    'step_index': wave.get('step_index', 0),
                    'points': sampled_pts
                })
            
            compressed[sc_id]['runs'][strat_id] = {
                'strategy_id': run.get('strategy_id', strat_id),
                'strategy_name': run.get('strategy_name', strat_id),
                'total_distance_nm': round(run.get('total_distance', 0), 1),
                'total_duration_hours': round(run.get('total_duration', 0), 1),
                'destination_reached': run.get('reached', False),
                'mean_time_ms': round(run.get('mean_time_ms', 0), 1),
                'waypoints': compact_wps,
                'isochrones': compact_waves
            }
    return compressed

def generate_html(wavefront_json_path, benchmark_json_path, output_html_path):
    if not os.path.exists(wavefront_json_path):
        print(f"Error: {wavefront_json_path} not found")
        sys.exit(1)

    with open(wavefront_json_path, 'r', encoding='utf-8') as f:
        raw_wf_data = json.load(f)

    bench_data = []
    if os.path.exists(benchmark_json_path):
        with open(benchmark_json_path, 'r', encoding='utf-8') as f:
            bench_data = json.load(f)

    wf_data = compress_wavefront_data(raw_wf_data)

    # Replace placeholders
    wf_json_str = json.dumps(wf_data)
    bench_json_str = json.dumps(bench_data)

    content = HTML_TEMPLATE.replace('/*WF_DATA_PLACEHOLDER*/', wf_json_str).replace('/*BENCH_DATA_PLACEHOLDER*/', bench_json_str)

    with open(output_html_path, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"[+] Wavefront comparison interactive HTML generated ({len(content)/(1024*1024):.2f} MB) -> {output_html_path}")

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 generate_wavefront_visualizer.py <wavefront_json> [benchmark_json] [output_html]")
        sys.exit(1)

    wf_in = sys.argv[1]
    bench_in = sys.argv[2] if len(sys.argv) > 2 else "output/pruning_benchmark_results.json"
    html_out = sys.argv[3] if len(sys.argv) > 3 else "output/wavefront_comparison.html"

    os.makedirs(os.path.dirname(os.path.abspath(html_out)), exist_ok=True)
    generate_html(wf_in, bench_in, html_out)

if __name__ == '__main__':
    main()
