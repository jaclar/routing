#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
ROUTING_DIR="$ROOT_DIR/services/routing"
OUTPUT_DIR="$ROOT_DIR/output"

mkdir -p "$OUTPUT_DIR"

echo "================================================================="
echo "   RUNNING MULTI-STRATEGY ISOCHRONE BENCHMARK & WAVEFRONT SUITE  "
echo "================================================================="

# 1. Build the benchmark executable
echo "[1/5] Building Go benchmark runner..."
cd "$ROUTING_DIR"
go build -o "$OUTPUT_DIR/routing_bench" ./cmd/benchmark

# 2. Run benchmarks across all 5 strategies and all 5 passage presets
echo "[2/5] Executing 5 Pruning Strategies across 5 Presets (25 benchmarks)..."
"$OUTPUT_DIR/routing_bench" \
  -preset=all \
  -strategy=all \
  -iterations=3 \
  -warmup=1 \
  -cpuprofile="$OUTPUT_DIR/cpu.prof" \
  -memprofile="$OUTPUT_DIR/mem.prof" \
  -json="$OUTPUT_DIR/pruning_benchmark_results.json" \
  -wavefronts="$OUTPUT_DIR/wavefront_data.json"

# 3. Generate Flame Graph SVG & Interactive HTML
echo "[3/5] Generating Flame Graph (SVG + Interactive HTML)..."
python3 "$ROOT_DIR/scripts/generate_flamegraph.py" \
  "$OUTPUT_DIR/cpu.prof" \
  "$OUTPUT_DIR/flamegraph.svg" \
  "$OUTPUT_DIR/pruning_benchmark_results.json" \
  "$OUTPUT_DIR/flamegraph.html"

# 4. Generate Wavefront Comparison Studio
echo "[4/5] Generating Interactive Wavefront Visualizer Studio..."
python3 "$ROOT_DIR/scripts/generate_wavefront_visualizer.py" \
  "$OUTPUT_DIR/wavefront_data.json" \
  "$OUTPUT_DIR/pruning_benchmark_results.json" \
  "$OUTPUT_DIR/wavefront_comparison.html"

# 5. Copy to conversation artifact directory if available
ARTIFACT_DIR="/Users/jaclar/.gemini/antigravity/brain/bf9f25d1-e28e-4580-8319-af7cc51b6d69"
if [ -d "$ARTIFACT_DIR" ]; then
  echo "[5/5] Copying reports to conversation artifacts..."
  cp "$OUTPUT_DIR/flamegraph.svg" "$ARTIFACT_DIR/flamegraph.svg" 2>/dev/null || true
  cp "$OUTPUT_DIR/flamegraph.html" "$ARTIFACT_DIR/flamegraph.html" 2>/dev/null || true
  cp "$OUTPUT_DIR/wavefront_comparison.html" "$ARTIFACT_DIR/wavefront_comparison.html" 2>/dev/null || true
  cp "$OUTPUT_DIR/pruning_benchmark_results.json" "$ARTIFACT_DIR/pruning_benchmark_results.json" 2>/dev/null || true
  cp "$OUTPUT_DIR/wavefront_data.json" "$ARTIFACT_DIR/wavefront_data.json" 2>/dev/null || true
fi

echo ""
echo "================================================================="
echo "                 BENCHMARK COMPLETED SUCCESSFULLY                 "
echo "================================================================="
echo "Artifacts generated:"
echo "  - Wavefront Studio: $OUTPUT_DIR/wavefront_comparison.html"
echo "  - Flame Graph SVG:  $OUTPUT_DIR/flamegraph.svg"
echo "  - Flame Graph HTML: $OUTPUT_DIR/flamegraph.html"
echo "  - JSON Metrics:     $OUTPUT_DIR/pruning_benchmark_results.json"
echo "  - Wavefront Data:   $OUTPUT_DIR/wavefront_data.json"
echo "  - CPU Profile:      $OUTPUT_DIR/cpu.prof"
echo "  - Memory Profile:   $OUTPUT_DIR/mem.prof"
echo "================================================================="
