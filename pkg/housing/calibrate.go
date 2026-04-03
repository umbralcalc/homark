package housing

import (
	"fmt"
	"math"

	"github.com/umbralcalc/homark/pkg/spine"
)

// CalibrateGrid defines independent 1D grids for a deterministic least-squares / MLE search.
// A dimension with Steps <= 0 is held at the base ForwardOptions value.
type CalibrateGrid struct {
	BankBetaLo, BankBetaHi                                 float64
	BankSteps                                              int
	PriceDriftLo, PriceDriftHi                             float64
	PriceDriftSteps                                        int
	SupplyBetaLo, SupplyBetaHi                             float64
	SupplySteps                                            int
	DemandSupplyPressureBetaLo, DemandSupplyPressureBetaHi float64
	DemandSupplySteps                                      int
	// CompletionFrac controls the fraction of pipeline stock completing per month (deterministic path).
	CompletionFracLo, CompletionFracHi float64
	CompletionFracSteps                int
	// EarningsDrift is the base log-earnings drift per month.
	EarningsDriftLo, EarningsDriftHi float64
	EarningsDriftSteps               int
}

// DefaultCalibrateGrid searches bank_beta on [-0.1, 0.1] with 21 points; holds all other dims at base.
func DefaultCalibrateGrid() CalibrateGrid {
	return CalibrateGrid{
		BankBetaLo: -0.1, BankBetaHi: 0.1, BankSteps: 21,
	}
}

// effectiveMedianRatioFallback matches forward spine init: zero or negative → 7.
func effectiveMedianRatioFallback(init float64) float64 {
	if init <= 0 {
		return 7.0
	}
	return init
}

// TargetLogSeries builds historical log earnings and log price from spine rows after affordability forward-fill.
// initMedianRatioFallback is applied to any month still missing pay and ONS ratio (typical when enrichment CSVs are absent).
func TargetLogSeries(obs []spine.MonthlyObservation, initMedianRatioFallback float64) (logP, logE []float64, err error) {
	if len(obs) == 0 {
		return nil, nil, fmt.Errorf("calibrate: no observations")
	}
	fb := effectiveMedianRatioFallback(initMedianRatioFallback)
	filled := ForwardFillAffordableFields(obs, fb)
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

// GaussianLogLikelihood computes the Gaussian log-likelihood of obs given pred under the MLE noise variance.
// sigmaHat is the MLE standard deviation of the residuals.
// Returns (logLike, sigmaHat). When len(pred) != len(obs) or either is empty, returns (-Inf, 0).
func GaussianLogLikelihood(pred, obs []float64) (logLike, sigmaHat float64) {
	n := len(pred)
	if n == 0 || len(obs) != n {
		return math.Inf(-1), 0
	}
	var mse float64
	for i := range pred {
		d := pred[i] - obs[i]
		mse += d * d
	}
	mse /= float64(n)
	if mse == 0 {
		// Perfect fit — log-likelihood is +Inf; return a large finite value.
		return 0, 0
	}
	sigmaHat = math.Sqrt(mse)
	// LL = -n/2 * (1 + log(2π) + log(σ²))
	logLike = -float64(n) / 2.0 * (1 + math.Log(2*math.Pi*mse))
	return logLike, sigmaHat
}

// CalibrationStats summarises the fit quality of a single calibration result.
type CalibrationStats struct {
	RMSE_P        float64 // RMSE of log price
	RMSE_E        float64 // RMSE of log earnings
	LogLikeP      float64 // Gaussian log-likelihood for log price (MLE σ²)
	LogLikeE      float64 // Gaussian log-likelihood for log earnings (MLE σ²)
	SigmaP        float64 // MLE noise std-dev for log price
	SigmaE        float64 // MLE noise std-dev for log earnings
	AIC           float64 // Akaike IC based on log-price log-likelihood: 2K − 2·LL_P
	NumFreeParams int     // number of free parameters used for AIC
}

// ComputeCalibrationStats evaluates fit quality for a given ForwardOptions against the spine.
// numFreeParams is used for AIC calculation (count the parameters you searched over).
func ComputeCalibrationStats(
	obs []spine.MonthlyObservation,
	opt ForwardOptions,
	numFreeParams int,
) (CalibrationStats, error) {
	targetP, targetE, err := TargetLogSeries(obs, opt.InitMedianRatioFallback)
	if err != nil {
		return CalibrationStats{}, err
	}
	do := DeterministicForwardOptions(opt)
	_, series, err := RunForwardLogSeries(obs, do)
	if err != nil {
		return CalibrationStats{}, err
	}
	fp, ok1 := series["log_price"]
	fe, ok2 := series["log_earnings"]
	if !ok1 || !ok2 {
		return CalibrationStats{}, fmt.Errorf("calibrate stats: missing series")
	}
	predP := flattenFirst(fp)
	predE := flattenFirst(fe)
	llP, sigP := GaussianLogLikelihood(predP, targetP)
	llE, sigE := GaussianLogLikelihood(predE, targetE)
	rmseP := RMSELogPrice(predP, targetP)
	rmseE := RMSELogPrice(predE, targetE)
	aic := 2*float64(numFreeParams) - 2*llP
	return CalibrationStats{
		RMSE_P: rmseP, RMSE_E: rmseE,
		LogLikeP: llP, LogLikeE: llE,
		SigmaP: sigP, SigmaE: sigE,
		AIC: aic, NumFreeParams: numFreeParams,
	}, nil
}

// flattenFirst extracts the first value from each step of a partition series.
func flattenFirst(s [][]float64) []float64 {
	out := make([]float64, len(s))
	for i, v := range s {
		out[i] = v[0]
	}
	return out
}

// GridCalibrateDeterministic searches grid dimensions to minimize RMSE of log price vs TargetLogSeries.
// When wLogE > 0, adds wLogE × RMSE(log earnings) to the score (same units as log-price RMSE; earnings
// series are often smoother—try wLogE in ~0.1–1 unless you want earnings to dominate).
func GridCalibrateDeterministic(
	obs []spine.MonthlyObservation,
	base ForwardOptions,
	grid CalibrateGrid,
	wLogE float64,
) (best ForwardOptions, rmseP, rmseE float64, err error) {
	targetP, targetE, err := TargetLogSeries(obs, base.InitMedianRatioFallback)
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
	dspVals := linspaceGrid(
		grid.DemandSupplyPressureBetaLo, grid.DemandSupplyPressureBetaHi, grid.DemandSupplySteps,
		base.DemandSupplyPressureBeta,
	)
	cfVals := linspaceGrid(grid.CompletionFracLo, grid.CompletionFracHi, grid.CompletionFracSteps, base.CompletionFrac)
	edVals := linspaceGrid(grid.EarningsDriftLo, grid.EarningsDriftHi, grid.EarningsDriftSteps, base.EarningsDrift)

	for _, bb := range bankVals {
		for _, pd := range driftVals {
			for _, sb := range supplyVals {
				for _, dsp := range dspVals {
					for _, cf := range cfVals {
						for _, ed := range edVals {
							o := base
							o.BankBeta = bb
							o.PriceDrift = pd
							o.SupplyBeta = sb
							o.DemandSupplyPressureBeta = dsp
							o.CompletionFrac = cf
							o.EarningsDrift = ed
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
							predP := flattenFirst(fp)
							predE := flattenFirst(fe)
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
			}
		}
	}
	return best, rmseP, rmseE, nil
}

// SplitObsForValidation splits observations into train (first trainN rows) and test (remaining rows).
// trainN must be in [1, len(obs)-1].
func SplitObsForValidation(obs []spine.MonthlyObservation, trainN int) (train, test []spine.MonthlyObservation, err error) {
	if trainN < 1 || trainN >= len(obs) {
		return nil, nil, fmt.Errorf("calibrate: trainN %d out of range [1, %d)", trainN, len(obs))
	}
	return obs[:trainN], obs[trainN:], nil
}

// ValidateCalibration calibrates on obs[:trainN] using the given grid, then reports CalibrationStats
// for both the training window and the held-out test window obs[trainN:].
// numFreeParams is used for AIC in the train stats.
func ValidateCalibration(
	obs []spine.MonthlyObservation,
	base ForwardOptions,
	grid CalibrateGrid,
	wLogE float64,
	trainN int,
) (trainStats, testStats CalibrationStats, best ForwardOptions, err error) {
	train, test, err := SplitObsForValidation(obs, trainN)
	if err != nil {
		return CalibrationStats{}, CalibrationStats{}, ForwardOptions{}, err
	}
	best, _, _, err = GridCalibrateDeterministic(train, base, grid, wLogE)
	if err != nil {
		return CalibrationStats{}, CalibrationStats{}, ForwardOptions{}, err
	}
	nFree := countFreeParams(grid)
	trainStats, err = ComputeCalibrationStats(train, best, nFree)
	if err != nil {
		return CalibrationStats{}, CalibrationStats{}, ForwardOptions{}, err
	}
	testStats, err = ComputeCalibrationStats(test, best, nFree)
	if err != nil {
		return CalibrationStats{}, CalibrationStats{}, ForwardOptions{}, err
	}
	return trainStats, testStats, best, nil
}

// countFreeParams returns the number of grid dimensions with Steps > 1 (active search dims).
func countFreeParams(g CalibrateGrid) int {
	n := 0
	for _, s := range []int{g.BankSteps, g.PriceDriftSteps, g.SupplySteps, g.DemandSupplySteps, g.CompletionFracSteps, g.EarningsDriftSteps} {
		if s > 1 {
			n++
		}
	}
	return n
}

// perturbOpt applies a named scalar perturbation to a ForwardOptions copy.
// Supported names: "bank_beta", "price_drift", "supply_beta", "demand_supply_beta",
// "completion_frac", "earnings_drift".
func perturbOpt(opt ForwardOptions, name string, delta float64) (ForwardOptions, error) {
	o := opt
	switch name {
	case "bank_beta":
		o.BankBeta += delta
	case "price_drift":
		o.PriceDrift += delta
	case "supply_beta":
		o.SupplyBeta += delta
	case "demand_supply_beta":
		o.DemandSupplyPressureBeta += delta
	case "completion_frac":
		o.CompletionFrac += delta
	case "earnings_drift":
		o.EarningsDrift += delta
	default:
		return ForwardOptions{}, fmt.Errorf("perturbOpt: unknown param %q", name)
	}
	return o, nil
}

// forwardLogLike runs the deterministic forward pass and returns the Gaussian log-likelihood of log price.
func forwardLogLike(obs []spine.MonthlyObservation, opt ForwardOptions) (float64, error) {
	targetP, _, err := TargetLogSeries(obs, opt.InitMedianRatioFallback)
	if err != nil {
		return 0, err
	}
	do := DeterministicForwardOptions(opt)
	_, series, err := RunForwardLogSeries(obs, do)
	if err != nil {
		return 0, err
	}
	fp, ok := series["log_price"]
	if !ok {
		return 0, fmt.Errorf("forwardLogLike: log_price series missing")
	}
	predP := flattenFirst(fp)
	ll, _ := GaussianLogLikelihood(predP, targetP)
	return ll, nil
}

// LaplaceLogLikeVariances estimates the marginal posterior variance of each named parameter via the
// diagonal of the numerical Hessian of the Gaussian log-likelihood of log price, evaluated at best.
// perturbs maps parameter name → finite-difference step size ε.
// The variance estimate for parameter θ_i is  −1 / H_ii  where H_ii is the second derivative.
// A non-finite or non-negative H_ii (flat or ascending curvature) yields +Inf variance for that param.
//
// Supported parameter names: "bank_beta", "price_drift", "supply_beta", "demand_supply_beta",
// "completion_frac", "earnings_drift".
func LaplaceLogLikeVariances(
	obs []spine.MonthlyObservation,
	best ForwardOptions,
	perturbs map[string]float64,
) (map[string]float64, error) {
	ll0, err := forwardLogLike(obs, best)
	if err != nil {
		return nil, err
	}
	variances := make(map[string]float64, len(perturbs))
	for name, eps := range perturbs {
		optPlus, err := perturbOpt(best, name, eps)
		if err != nil {
			return nil, err
		}
		optMinus, err := perturbOpt(best, name, -eps)
		if err != nil {
			return nil, err
		}
		llPlus, err := forwardLogLike(obs, optPlus)
		if err != nil {
			return nil, err
		}
		llMinus, err := forwardLogLike(obs, optMinus)
		if err != nil {
			return nil, err
		}
		hii := (llPlus - 2*ll0 + llMinus) / (eps * eps)
		if hii >= 0 || math.IsNaN(hii) || math.IsInf(hii, 0) {
			variances[name] = math.Inf(1)
		} else {
			variances[name] = -1.0 / hii
		}
	}
	return variances, nil
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
