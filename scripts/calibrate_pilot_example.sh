#!/usr/bin/env sh
# Example: deterministic grid calibration for one pilot LA (adjust flags as needed).
# Usage: ./scripts/calibrate_pilot_example.sh "Tower Hamlets"
set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LA="${1:-Tower Hamlets}"
cd "$ROOT"
go run ./cmd/calibratespine -root "$ROOT" -la "$LA" \
	-bank-steps 21 \
	-demand-supply-steps 5 \
	-demand-supply-beta-lo -0.02 \
	-demand-supply-beta-hi 0.02 \
	-w-log-earnings 0.25
