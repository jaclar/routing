#!/usr/bin/env python3
"""
Flame Graph Generator for Go pprof Profiles
Parses `go tool pprof -traces` output and renders an interactive SVG and HTML Flame Graph.
"""

import sys
import os
import re
import html
import subprocess
import json

def parse_time_to_ms(t_str):
    t_str = t_str.strip()
    if t_str.endswith('ms'):
        return float(t_str[:-2])
    elif t_str.endswith('s') and not t_str.endswith('us') and not t_str.endswith('ns') and not t_str.endswith('ms'):
        return float(t_str[:-1]) * 1000.0
    elif t_str.endswith('us') or t_str.endswith('µs'):
        return float(t_str[:-2]) / 1000.0
    elif t_str.endswith('ns'):
        return float(t_str[:-2]) / 1000000.0
    try:
        return float(t_str)
    except ValueError:
        return 1.0

class FlameNode:
    def __init__(self, name):
        self.name = name
        self.value = 0.0
        self.children = {}

    def add_trace(self, stack, value):
        self.value += value
        if not stack:
            return
        head = stack[0]
        tail = stack[1:]
        if head not in self.children:
            self.children[head] = FlameNode(head)
        self.children[head].add_trace(tail, value)

def get_component_color(name):
    # Determine color palette based on component area
    if 'landmask' in name:
        return '#059669', '#10b981' # Emerald (Land collision)
    elif 'weather' in name:
        return '#0891b2', '#06b6d4' # Cyan (Weather model & interpolation)
    elif 'polar' in name:
        return '#7c3aed', '#8b5cf6' # Violet (Polar speed lookup)
    elif 'geo' in name or 'math' in name:
        return '#2563eb', '#3b82f6' # Blue (Geodesics & trigonometry)
    elif 'isochrone' in name:
        return '#d97706', '#f59e0b' # Amber/Orange (Isochrone core)
    elif 'runtime' in name or 'sync' in name:
        return '#475569', '#64748b' # Slate (Go runtime / GC / allocator)
    elif 'testing' in name or 'main' in name:
        return '#4f46e5', '#6366f1' # Indigo (Test / Bench runner)
    else:
        return '#ea580c', '#f97316' # Warm flame fallback

def simplify_name(raw_name):
    # Remove package prefixes for clean display
    cleaned = raw_name.replace('github.com/jaclar/routing-service/', '')
    return cleaned

def parse_traces(traces_text):
    root = FlameNode('root')
    blocks = re.split(r'-----------+\+-------------------------------------------------------', traces_text)
    
    for block in blocks:
        lines = [l.rstrip() for l in block.strip().split('\n') if l.strip()]
        if not lines:
            continue
        
        # Check first line for time/sample value
        match = re.match(r'^\s*([0-9\.]+(?:ms|s|us|µs|ns)?)\s+(.+)$', lines[0])
        if not match:
            continue
        
        sample_str, leaf_func = match.groups()
        sample_val = parse_time_to_ms(sample_str)
        
        frames = [leaf_func.strip()]
        for l in lines[1:]:
            frame = l.strip()
            if frame:
                frames.append(frame)
        
        # In pprof -traces: lines[0] is the leaf, last line is root.
        # Reverse to get root -> leaf call order
        frames.reverse()
        root.add_trace(frames, sample_val)
        
    return root

def layout_flame(node, x, y, width, total_root_val, depth=0, max_depth=30):
    boxes = []
    if depth > max_depth or width < 0.2:
        return boxes
    
    if node.name != 'root':
        boxes.append({
            'name': node.name,
            'simple_name': simplify_name(node.name),
            'x': x,
            'y': y,
            'w': width,
            'h': 20,
            'depth': depth,
            'val': node.value,
            'pct': (node.value / total_root_val) * 100.0 if total_root_val > 0 else 0
        })
    
    # Layout children
    cur_x = x
    child_y = y - 22 if node.name != 'root' else y
    # Sort children deterministically by value descending
    sorted_children = sorted(node.children.values(), key=lambda c: c.value, reverse=True)
    
    for child in sorted_children:
        child_w = (child.value / total_root_val) * 1100.0 if total_root_val > 0 else 0
        if child_w >= 0.2:
            boxes.extend(layout_flame(child, cur_x, child_y, child_w, total_root_val, depth + 1, max_depth))
            cur_x += child_w
            
    return boxes

def generate_svg(root, output_path):
    total_val = root.value
    if total_val <= 0:
        total_val = 1.0

    # Calculate max depth to size canvas height
    def get_max_depth(n, d=0):
        if not n.children:
            return d
        return max(get_max_depth(c, d + 1) for c in n.children.values())

    max_d = get_max_depth(root)
    svg_w = 1140
    svg_h = max(350, (max_d + 3) * 23 + 80)
    base_y = svg_h - 60

    boxes = layout_flame(root, 20, base_y, 1100, total_val)

    svg = []
    svg.append(f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {svg_w} {svg_h}" width="100%" height="{svg_h}" style="font-family: -apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif; background: #0f172a; border-radius: 12px; border: 1px solid #334155;">')
    svg.append('<style>')
    svg.append('  .frame { cursor: pointer; transition: opacity 0.15s ease; }')
    svg.append('  .frame:hover { opacity: 0.85; filter: brightness(1.2); }')
    svg.append('  .frame-text { pointer-events: none; fill: #ffffff; font-size: 11px; font-weight: 500; }')
    svg.append('  .header-text { fill: #f8fafc; font-size: 14px; font-weight: 700; }')
    svg.append('  .sub-text { fill: #94a3b8; font-size: 11px; }')
    svg.append('</style>')

    # Header
    svg.append(f'<text x="20" y="30" class="header-text">ISOCHRONE ROUTER CPU FLAME GRAPH</text>')
    svg.append(f'<text x="20" y="48" class="sub-text">Total Sampled CPU Time: {total_val:.2f} ms | Deepest Call Depth: {max_d} frames</text>')

    # Legend in header
    legend_items = [
        ('Isochrone Core', '#d97706'),
        ('Landmask & Polygons', '#059669'),
        ('Weather Model', '#0891b2'),
        ('Geodesics & Math', '#2563eb'),
        ('Polar Table', '#7c3aed'),
        ('Go Runtime / Alloc', '#475569'),
    ]
    lx = 540
    for label, col in legend_items:
        svg.append(f'<rect x="{lx}" y="22" width="10" height="10" rx="2" fill="{col}"/>')
        svg.append(f'<text x="{lx+14}" y="31" fill="#cbd5e1" font-size="10">{label}</text>')
        lx += 100

    # Render Boxes
    for b in boxes:
        col_base, col_stroke = get_component_color(b['name'])
        rect_id = f"rect_{b['x']}_{b['y']}"
        svg.append(f'<g class="frame" id="{rect_id}">')
        svg.append(f'  <title>{html.escape(b["name"])}&#10;Time: {b["val"]:.2f} ms ({b["pct"]:.1f}%)</title>')
        svg.append(f'  <rect x="{b["x"]:.1f}" y="{b["y"]:.1f}" width="{max(1.0, b["w"]-1):.1f}" height="20" rx="3" fill="{col_base}" stroke="{col_stroke}" stroke-width="0.5"/>')
        
        # Only render text if box is wide enough
        if b['w'] > 40:
            display_label = b['simple_name']
            max_chars = int(b['w'] / 6.8)
            if len(display_label) > max_chars:
                display_label = display_label[:max(0, max_chars-2)] + '..'
            svg.append(f'  <text x="{b["x"]+4:.1f}" y="{b["y"]+14:.1f}" class="frame-text">{html.escape(display_label)}</text>')
        svg.append('</g>')

    svg.append('</svg>')

    with open(output_path, 'w', encoding='utf-8') as f:
        f.write('\n'.join(svg))
    print(f"[+] Flame Graph SVG generated -> {output_path}")

def generate_interactive_html(svg_path, benchmark_json_path, output_html_path):
    with open(svg_path, 'r', encoding='utf-8') as f:
        svg_content = f.read()

    benchmarks = []
    if os.path.exists(benchmark_json_path):
        with open(benchmark_json_path, 'r', encoding='utf-8') as f:
            benchmarks = json.load(f)

    bench_rows = ""
    for b in benchmarks:
        bench_rows += f"""
        <tr class="border-b border-slate-800 hover:bg-slate-800/50 transition">
          <td class="py-2.5 px-3 font-medium text-slate-100">{b.get('preset_name', '')}</td>
          <td class="py-2.5 px-3 font-mono text-cyan-400 text-xs">{b.get('time_step', '')}</td>
          <td class="py-2.5 px-3 font-mono text-right text-slate-300">{b.get('direct_distance_nm', 0):.1f} NM</td>
          <td class="py-2.5 px-3 font-mono text-right text-slate-300">{b.get('route_distance_nm', 0):.1f} NM</td>
          <td class="py-2.5 px-3 font-mono text-right text-slate-300">{b.get('duration_hours', 0):.1f} h</td>
          <td class="py-2.5 px-3 font-mono text-right font-bold text-amber-400">{b.get('mean_time_ms', 0):.2f} ms</td>
          <td class="py-2.5 px-3 font-mono text-right text-emerald-400">{b.get('throughput_per_sec', 0):.1f}/s</td>
          <td class="py-2.5 px-3 font-mono text-right text-slate-300">{(b.get('alloc_bytes_per_op', 0)/(1024*1024)):.2f} MB</td>
          <td class="py-2.5 px-3 text-center">
            <span class="inline-block px-2 py-0.5 rounded text-[10px] font-bold {'bg-emerald-950 text-emerald-400 border border-emerald-800' if b.get('success') else 'bg-rose-950 text-rose-400'}">
              {'PASS' if b.get('success') else 'FAIL'}
            </span>
          </td>
        </tr>
        """

    html_content = f"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Isochrone Routing Performance & Flame Graph</title>
  <script src="https://www.gstatic.com/antigravity/web/dev/tailwindcss.min.js"></script>
</head>
<body class="bg-slate-950 text-slate-100 antialiased p-6">
  <div class="max-w-7xl mx-auto space-y-6">
    
    <!-- Title Banner -->
    <div class="flex flex-wrap items-center justify-between gap-4 border-b border-slate-800 pb-4">
      <div>
        <h1 class="text-xl font-extrabold flex items-center gap-2 text-white">
          <span class="p-1.5 bg-amber-500/20 text-amber-400 rounded-lg border border-amber-500/30">🔥</span>
          Weather Routing Performance & Flame Graph Analysis
        </h1>
        <p class="text-xs text-slate-400 mt-1">Benchmarking <code class="font-mono text-cyan-400">services/routing/isochrone</code> across all passage presets</p>
      </div>
      <div class="flex items-center gap-3">
        <span class="text-xs bg-slate-900 border border-slate-700 px-3 py-1.5 rounded-lg text-slate-300 font-mono">
          Engine: Go 1.22+ Isochrone Wavefront
        </span>
      </div>
    </div>

    <!-- Preset Benchmarks Table -->
    <div class="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-xl">
      <div class="flex items-center justify-between mb-3">
        <h2 class="text-sm font-bold text-slate-200 uppercase tracking-wider flex items-center gap-2">
          <span>📊</span> Benchmark Timing & Throughput Metrics (All Presets)
        </h2>
        <span class="text-xs text-slate-400">5 runs/preset with warm cache</span>
      </div>
      <div class="overflow-x-auto">
        <table class="w-full text-xs text-left border-collapse">
          <thead>
            <tr class="border-b border-slate-700 text-slate-400 font-semibold bg-slate-950/40">
              <th class="py-2.5 px-3">Scenario / Preset</th>
              <th class="py-2.5 px-3">Time Step</th>
              <th class="py-2.5 px-3 text-right">Direct Dist</th>
              <th class="py-2.5 px-3 text-right">Sailed Dist</th>
              <th class="py-2.5 px-3 text-right">Passage Time</th>
              <th class="py-2.5 px-3 text-right">Mean Latency</th>
              <th class="py-2.5 px-3 text-right">Throughput</th>
              <th class="py-2.5 px-3 text-right">Memory/Op</th>
              <th class="py-2.5 px-3 text-center">Status</th>
            </tr>
          </thead>
          <tbody>
            {bench_rows}
          </tbody>
        </table>
      </div>
    </div>

    <!-- Interactive Flame Graph -->
    <div class="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-xl space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 class="text-sm font-bold text-slate-200 uppercase tracking-wider flex items-center gap-2">
            <span>🔥</span> CPU Flame Graph Profile
          </h2>
          <p class="text-xs text-slate-400">Hover over any block to inspect function call time, call stack depth, and percentage breakdown.</p>
        </div>
        <div class="flex items-center gap-2 text-xs">
          <span class="text-slate-400">Click & hover on blocks for deep call stack inspection</span>
        </div>
      </div>

      <!-- SVG Container -->
      <div class="w-full overflow-x-auto rounded-xl border border-slate-800 bg-slate-950 p-2">
        {svg_content}
      </div>
    </div>

  </div>
</body>
</html>
"""
    with open(output_html_path, 'w', encoding='utf-8') as f:
        f.write(html_content)
    print(f"[+] Interactive Flame Graph HTML report generated -> {output_html_path}")

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 generate_flamegraph.py <cpu.prof> [output_svg] [benchmark_json] [output_html]")
        sys.exit(1)

    prof_file = sys.argv[1]
    svg_out = sys.argv[2] if len(sys.argv) > 2 else "output/flamegraph.svg"
    json_in = sys.argv[3] if len(sys.argv) > 3 else "output/benchmark_results.json"
    html_out = sys.argv[4] if len(sys.argv) > 4 else "output/flamegraph.html"

    os.makedirs(os.path.dirname(os.path.abspath(svg_out)), exist_ok=True)

    print(f"[*] Extracting call traces from {prof_file} using go tool pprof...")
    cmd = ["go", "tool", "pprof", "-traces", prof_file]
    proc = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    if proc.returncode != 0:
        print(f"Error running pprof: {proc.stderr}")
        sys.exit(1)

    print("[*] Building flame tree and calculating layout coordinates...")
    root = parse_traces(proc.stdout)
    generate_svg(root, svg_out)
    generate_interactive_html(svg_out, json_in, html_out)

if __name__ == '__main__':
    main()
