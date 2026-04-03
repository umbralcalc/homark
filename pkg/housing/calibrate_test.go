package housing

import (
	"math"
	"testing"

	"github.com/umbralcalc/homark/pkg/spine"
)

func TestGaussianLogLikelihood(t *testing.T) {
	t.Run("perfect_fit", func(t *testing.T) {
		pred := []float64{1.0, 2.0, 3.0}
		ll, sigma := GaussianLogLikelihood(pred, pred)
		if ll != 0 {
			t.Fatalf("perfect fit should return 0, got %g", ll)
		}
		if sigma != 0 {
			t.Fatalf("perfect fit sigma should be 0, got %g", sigma)
		}
	})

	t.Run("known_residuals", func(t *testing.T) {
		// residuals = [1, -1] → mse = 1 → sigma = 1
		pred := []float64{1.0, 3.0}
		obs := []float64{0.0, 4.0}
		ll, sigma := GaussianLogLikelihood(pred, obs)
		if math.Abs(sigma-1.0) > 1e-9 {
			t.Fatalf("sigma want 1 got %g", sigma)
		}
		expected := -float64(len(pred)) / 2.0 * (1 + math.Log(2*math.Pi))
		if math.Abs(ll-expected) > 1e-9 {
			t.Fatalf("ll want %g got %g", expected, ll)
		}
	})

	t.Run("length_mismatch", func(t *testing.T) {
		ll, _ := GaussianLogLikelihood([]float64{1}, []float64{1, 2})
		if !math.IsInf(ll, -1) {
			t.Fatal("length mismatch should return -Inf")
		}
	})

	t.Run("empty", func(t *testing.T) {
		ll, _ := GaussianLogLikelihood(nil, nil)
		if !math.IsInf(ll, -1) {
			t.Fatal("empty should return -Inf")
		}
	})
}

func TestComputeCalibrationStats(t *testing.T) {
	sample := spine.MonthlyObservation{
		YearMonth: "2004-01", AveragePrice: 100e3, MedianRatio: 8.0, BankRatePct: 4.5,
	}
	obs := []spine.MonthlyObservation{sample, sample, sample}
	opt := DefaultForwardOptions()
	stats, err := ComputeCalibrationStats(obs, opt, 1)
	if err != nil {
		t.Fatal(err)
	}
	if math.IsInf(stats.RMSE_P, 1) || math.IsNaN(stats.RMSE_P) {
		t.Fatalf("RMSE_P invalid: %g", stats.RMSE_P)
	}
	if math.IsInf(stats.LogLikeP, 1) || math.IsNaN(stats.LogLikeP) {
		t.Fatalf("LogLikeP invalid: %g", stats.LogLikeP)
	}
	if stats.NumFreeParams != 1 {
		t.Fatalf("NumFreeParams want 1 got %d", stats.NumFreeParams)
	}
}

func TestSplitObsForValidation(t *testing.T) {
	obs := make([]spine.MonthlyObservation, 10)
	for i := range obs {
		obs[i].YearMonth = "2000-01"
		obs[i].AveragePrice = float64(100e3 + i*1000)
	}

	train, test, err := SplitObsForValidation(obs, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(train) != 7 || len(test) != 3 {
		t.Fatalf("split: train=%d test=%d", len(train), len(test))
	}

	_, _, err = SplitObsForValidation(obs, 0)
	if err == nil {
		t.Fatal("expected error for trainN=0")
	}
	_, _, err = SplitObsForValidation(obs, 10)
	if err == nil {
		t.Fatal("expected error for trainN=len(obs)")
	}
}

func TestValidateCalibration(t *testing.T) {
	sample := spine.MonthlyObservation{
		YearMonth: "2004-01", AveragePrice: 100e3, MedianRatio: 8.0, BankRatePct: 4.5,
	}
	obs := make([]spine.MonthlyObservation, 8)
	for i := range obs {
		obs[i] = sample
	}
	base := DefaultForwardOptions()
	grid := CalibrateGrid{BankBetaLo: 0, BankBetaHi: 0, BankSteps: 1}
	trainStats, testStats, best, err := ValidateCalibration(obs, base, grid, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if math.IsInf(trainStats.RMSE_P, 1) || math.IsInf(testStats.RMSE_P, 1) {
		t.Fatal("RMSE infinite")
	}
	_ = best
}

func TestGridCalibrateCompletionFracGrid(t *testing.T) {
	sample := spine.MonthlyObservation{
		YearMonth: "2004-01", AveragePrice: 100e3, MedianRatio: 8.0, BankRatePct: 4.5,
	}
	obs := []spine.MonthlyObservation{sample, sample, sample}
	base := DefaultForwardOptions()
	grid := CalibrateGrid{
		BankBetaLo: 0, BankBetaHi: 0, BankSteps: 1,
		CompletionFracLo: 0.05, CompletionFracHi: 0.3, CompletionFracSteps: 3,
	}
	best, _, _, err := GridCalibrateDeterministic(obs, base, grid, 0)
	if err != nil {
		t.Fatal(err)
	}
	if best.CompletionFrac < 0.05-1e-9 || best.CompletionFrac > 0.3+1e-9 {
		t.Fatalf("best completion_frac %g out of grid [0.05, 0.3]", best.CompletionFrac)
	}
}

func TestGridCalibrateEarningsDriftGrid(t *testing.T) {
	sample := spine.MonthlyObservation{
		YearMonth: "2004-01", AveragePrice: 100e3, MedianRatio: 8.0, BankRatePct: 4.5, EarningsAnnual: 30e3,
	}
	obs := []spine.MonthlyObservation{sample, sample, sample}
	base := DefaultForwardOptions()
	grid := CalibrateGrid{
		BankBetaLo: 0, BankBetaHi: 0, BankSteps: 1,
		EarningsDriftLo: 0.0001, EarningsDriftHi: 0.001, EarningsDriftSteps: 3,
	}
	best, _, _, err := GridCalibrateDeterministic(obs, base, grid, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if best.EarningsDrift < 0.0001-1e-9 || best.EarningsDrift > 0.001+1e-9 {
		t.Fatalf("best earnings_drift %g out of grid", best.EarningsDrift)
	}
}

func TestLaplaceLogLikeVariances(t *testing.T) {
	sample := spine.MonthlyObservation{
		YearMonth: "2004-01", AveragePrice: 100e3, MedianRatio: 8.0, BankRatePct: 4.5,
	}
	obs := make([]spine.MonthlyObservation, 12)
	for i := range obs {
		obs[i] = sample
	}
	best := DefaultForwardOptions()
	perturbs := map[string]float64{"bank_beta": 0.001}
	variances, err := LaplaceLogLikeVariances(obs, best, perturbs)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := variances["bank_beta"]; !ok {
		t.Fatal("missing bank_beta in variances")
	}
}

func TestPPDPricePreferenceInLogPriceFromObs(t *testing.T) {
	// When PPDMedianPrice is set, it should be preferred over AveragePrice.
	o := spine.MonthlyObservation{
		AveragePrice:  200e3,
		PPDMedianPrice: 180e3,
	}
	lp, err := logPriceFromObs(o)
	if err != nil {
		t.Fatal(err)
	}
	want := math.Log(180e3)
	if math.Abs(lp-want) > 1e-9 {
		t.Fatalf("expected PPD price log %g got %g", want, lp)
	}

	// Without PPD, fall back to AveragePrice.
	o2 := spine.MonthlyObservation{AveragePrice: 200e3}
	lp2, err := logPriceFromObs(o2)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(lp2-math.Log(200e3)) > 1e-9 {
		t.Fatalf("expected AveragePrice fallback")
	}
}
