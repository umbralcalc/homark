package housing

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// DriftDiffusionBankChannelIteration is a scalar drift–diffusion step where each
// iteration’s drift is drift_base + bank_drift_beta * (bank_rate_pct / 100).
// The current bank_rate_pct must be supplied each step via params_from_upstream
// (e.g. from a FromStorageIteration partition holding spine bank_rate_pct).
//
// Params (Configure): drift_base, diffusion_coefficients (length 1 each);
// optional bank_drift_beta (default 0). Params (Iterate): bank_rate_pct (length 1)
// from upstream — typically the BoE rate as a percentage (e.g. 5.0 for 5%).
type DriftDiffusionBankChannelIteration struct {
	unitNormalDist       *distuv.Normal
	driftBase, diffusion float64
	bankBeta             float64
}

func (d *DriftDiffusionBankChannelIteration) Configure(partitionIndex int, settings *simulator.Settings) {
	p := settings.Iterations[partitionIndex].Params.Map
	d.driftBase = p["drift_base"][0]
	d.diffusion = p["diffusion_coefficients"][0]
	d.bankBeta = 0
	if v, ok := p["bank_drift_beta"]; ok && len(v) > 0 {
		d.bankBeta = v[0]
	}
	seed := settings.Iterations[partitionIndex].Seed
	d.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src:   rand.NewPCG(seed, seed),
	}
}

func (d *DriftDiffusionBankChannelIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	bank := params.Get("bank_rate_pct")[0]
	drift := d.driftBase + d.bankBeta*(bank/100.0)
	x := stateHistory.Values.At(0, 0)
	dt := timestepsHistory.NextIncrement
	noise := d.diffusion * math.Sqrt(dt) * d.unitNormalDist.Rand()
	return []float64{x + drift*dt + noise}
}
