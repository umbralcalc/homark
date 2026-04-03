#!/usr/bin/env bash
# Run policyscenario for every pilot LA in targets.yaml (via policyscenario -list) and write CSV + HTML heatmaps under artifacts/scenarios/.
# Usage from repo root: POSTERIOR=posteriors/leeds.json ./scripts/run_pilot_scenarios.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
OUT="${OUT:-$ROOT/artifacts/scenarios}"
mkdir -p "$OUT"
POSTERIOR="${POSTERIOR:-}"
ARGS=( -approvals "0,40,80,120" -bank-scales "1,1.05,1.1" -market-fractions "1,0.85" -flat-shares "0.35,0.55" -composition-drift-beta 0.0002 )
if [[ -n "$POSTERIOR" && -f "$POSTERIOR" ]]; then
  ARGS+=( -posterior "$POSTERIOR" )
fi
while read -r code name; do
  [[ -z "$code" ]] && continue
  safe="${name// /_}"
  csv="$OUT/${safe}_${code}.csv"
  html="$OUT/${safe}_${code}.html"
  echo "# $name ($code)"
  go run ./cmd/policyscenario -area "$code" "${ARGS[@]}" >"$csv"
  go run ./cmd/scenarioplot -in "$csv" -out "$html" -title "Affordability scenarios: $name" -sample-idx 0
  echo "  wrote $csv and $html"
done < <(go run ./cmd/policyscenario -list)
