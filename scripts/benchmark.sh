#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
ROUTING_DIR="$ROOT_DIR/services/routing"
OUTPUT_DIR="$ROOT_DIR/output"

mkdir -p "$OUTPUT_DIR"

echo "================================================================="
echo "        RUNNING ISOCHRONE ROUTING BENCHMARK SUITE                "
echo "================================================================="

# 1. Build the benchmark executable
echo "[1/4] Building Go benchmark runner..."
cd "$ROUTING_DIR"
go build -o "$OUTPUT_DIR/routing_bench" ./cmd/benchmark

# 2. Run benchmarks across all passage presets with CPU profiling
echo "[2/4] Executing benchmarks across all presets & capturing CPU profile..."
"$OUTPUT_DIR/routing_bench" \
  -preset=all \
  -iterations=5 \
  -warmup=1 \
  -cpuprofile="$OUTPUT_DIR/cpu.prof" \
  -memprofile="$OUTPUT_DIR/mem.prof" \
  -json="$OUTPUT_DIR/benchmark_results.json"

# 3. Generate Flame Graph SVG & Interactive HTML
echo "[3/4] Generating Flame Graph (SVG + Interactive HTML)..."
python3 "$ROOT_DIR/scripts/generate_flamegraph.py" \
  "$OUTPUT_DIR/cpu.prof" \
  "$OUTPUT_DIR/flamegraph.svg" \
  "$OUTPUT_DIR/benchmark_results.json" \
  "$OUTPUT_DIR/flamegraph.html"

# 4. Copy to conversation artifact directory if available
ARTIFACT_DIR="/Users/jaclar/.gemini/antigravity/brain/bf9f25d1-e28e-4580-8319-af7cc51b6d69"
if [ -d "$ARTIFACT_DIR" ]; then
  echo "[4/4] Copying report to conversation artifacts..."
  cp "$OUTPUT_DIR/flamegraph.svg" "$ARTIFACT_DIR/flamegraph.svg" 2>/dev/null || true
  cp "$OUTPUT_DIR/flamegraph.html" "$ARTIFACT_DIR/flamegraph.html" 2>/dev/null || true
  cp "$OUTPUT_DIR/benchmark_results.json" "$ARTIFACT_DIR/benchmark_results.json" 2>/dev/null || true
fi

echo ""
echo "================================================================="
echo "                 BENCHMARK COMPLETED SUCCESSFULLY                 "
echo "================================================================="
echo "Artifacts generated:"
echo "  - Flame Graph SVG:  $OUTPUT_DIR/flamegraph.svg"
echo "  - Interactive HTML: $OUTPUT_DIR/flamegraph.html"
echo "  - JSON Metrics:     $OUTPUT_DIR/benchmark_results.json"
echo "  - CPU Profile:      $OUTPUT_DIR/cpu.prof"
echo "  - Memory Profile:   $OUTPUT_DIR/mem.prof"
echo "================================================================="
