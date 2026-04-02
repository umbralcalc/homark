#!/usr/bin/env sh
# Run spinehealth with 95% pay + ratio coverage thresholds for every pilot LA.
# Default: dat/processed/spine_monthly.csv under the repo root.
# Example: ./scripts/spinehealth_gate.sh
# Example: ./scripts/spinehealth_gate.sh pkg/spine/testdata/spine_pilot_enrichment_fixture.csv
set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SPINE="${1:-dat/processed/spine_monthly.csv}"
cd "$ROOT"
go run ./cmd/spinehealth -root "$ROOT" -spine "$SPINE" \
	-min-pay-pct 95 \
	-min-ratio-pct 95
