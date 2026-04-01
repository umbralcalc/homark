package housing

import (
	"fmt"
	"math"

	"github.com/umbralcalc/homark/pkg/spine"
)

// CalibrateGrid defines independent 1D grids for a small deterministic least-squares search.
// A dimension with Steps <= 0 is held at the base ForwardOptions value.
type CalibrateGrid struct {
	BankBetaLo, BankBetaHi     float64
	BankSteps                  int
	PriceDriftLo, PriceDriftHi float64
	PriceDriftSteps            int
	SupplyBetaLo, SupplyBetaHi float64
	SupplySteps                int
}

// DefaultCalibrateGrid searches bank_beta on [-0.1, 0.1] with 21 points; holds price drift and supply beta at base.
func DefaultCalibrateGrid() CalibrateGrid {
	return CalibrateGrid{
		BankBetaLo: -0.1, BankBetaHi: 0.1, BankSteps: 21,
		PriceDriftLo: 0, PriceDriftHi: 0, PriceDriftSteps: 0,
		SupplyBetaLo: 0, SupplyBetaHi: 0, SupplySteps: 0,
	}
}

// TargetLogSeries builds historical log earnings and log price from spine rows after affordability forward-fill.
func TargetLogSeries(obs []spine.MonthlyObservation) (logP, logE []float64, err error) {
	if len(obs) == 0 {
		return nil, nil, fmt.Errorf("calibrate: no observations")
	}
	filled := ForwardFillAffordableFields(obs)
	logE2, logP2, _, err := MonthlyLogSeries(filled)
	if err != nil {
		return nil, nil, err
	}
	logP = make([]float64, len(logP2))
	logE = make([]float64, len(logE2))
	for i := range logP2 {
		logP[i] = logP2[i][0]
		logE[i] = logE2[i][0]
	}
	return logP, logE, nil
}

// DeterministicForwardOptions returns a copy of opt with price and earnings diffusion set to zero (ODE path).
func DeterministicForwardOptions(opt ForwardOptions) ForwardOptions {
	o := opt
	o.PriceDiff = 0
	o.EarningsDiff = 0
	return o
}

// RMSELogPrice compares two same-length log-price paths.
func RMSELogPrice(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return math.Inf(1)
	}
	var s float64
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return math.Sqrt(s / float64(len(a)))
}

// GridCalibrateDeterministic searches grid dimensions to minimize RMSE of log price vs TargetLogSeries.
// Weights log-earnings RMSE with wLogE (0 = ignore).
func GridCalibrateDeterministic(
	obs []spine.MonthlyObservation,
	base ForwardOptions,
	grid CalibrateGrid,
	wLogE float64,
) (best ForwardOptions, rmseP, rmseE float64, err error) {
	targetP, targetE, err := TargetLogSeries(obs)
	if err != nil {
		return ForwardOptions{}, 0, 0, err
	}
	if len(targetP) != len(obs) {
		return ForwardOptions{}, 0, 0, fmt.Errorf("calibrate: target length mismatch")
	}

	best = base
	bestRMSE := math.Inf(1)
	rmseP = math.Inf(1)
	rmseE = math.Inf(1)

	bankVals := linspaceGrid(grid.BankBetaLo, grid.BankBetaHi, grid.BankSteps, base.BankBeta)
	driftVals := linspaceGrid(grid.PriceDriftLo, grid.PriceDriftHi, grid.PriceDriftSteps, base.PriceDrift)
	supplyVals := linspaceGrid(grid.SupplyBetaLo, grid.SupplyBetaHi, grid.SupplySteps, base.SupplyBeta)

	for _, bb := range bankVals {
		for _, pd := range driftVals {
			for _, sb := range supplyVals {
				o := base
				o.BankBeta = bb
				o.PriceDrift = pd
				o.SupplyBeta = sb
				do := DeterministicForwardOptions(o)
				_, series, runErr := RunForwardLogSeries(obs, do)
				if runErr != nil {
					return ForwardOptions{}, 0, 0, runErr
				}
				fp, ok1 := series["log_price"]
				fe, ok2 := series["log_earnings"]
				if !ok1 || !ok2 || len(fp) != len(targetP) {
					return ForwardOptions{}, 0, 0, fmt.Errorf("calibrate: forward series length")
				}
				predP := make([]float64, len(fp))
				predE := make([]float64, len(fe))
				for i := range fp {
					predP[i] = fp[i][0]
					predE[i] = fe[i][0]
				}
				rp := RMSELogPrice(predP, targetP)
				re := RMSELogPrice(predE, targetE)
				score := rp
				if wLogE > 0 {
					score += wLogE * re
				}
				if score < bestRMSE {
					bestRMSE = score
					best = o
					rmseP = rp
					rmseE = re
				}
			}
		}
	}
	return best, rmseP, rmseE, nil
}

func linspaceGrid(lo, hi float64, steps int, base float64) []float64 {
	if steps <= 0 {
		return []float64{base}
	}
	if lo == hi {
		return []float64{lo}
	}
	if steps == 1 {
		return []float64{(lo + hi) * 0.5}
	}
	out := make([]float64, steps)
	for i := 0; i < steps; i++ {
		t := float64(i) / float64(steps-1)
		out[i] = lo + t*(hi-lo)
	}
	return out
}
