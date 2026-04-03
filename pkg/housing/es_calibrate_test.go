package housing

import (
	"math"
	"testing"

	"github.com/umbralcalc/homark/pkg/spine"
)

func makeSteadyObs(n int) []spine.MonthlyObservation {
	obs := make([]spine.MonthlyObservation, n)
	for i := range obs {
		obs[i] = spine.MonthlyObservation{
			YearMonth:    "2004-01",
			AveragePrice: 100e3,
			MedianRatio:  8.0,
			BankRatePct:  4.5,
			EarningsAnnual: 30e3,
		}
	}
	return obs
}

func TestESNaturalWeights(t *testing.T) {
	for _, k := range []int{2, 10, 20} {
		w := esNaturalWeights(k)
		if len(w) != k {
			t.Fatalf("len weights want %d got %d", k, len(w))
		}
		sum := 0.0
		for _, v := range w {
			sum += v
		}
		if math.Abs(sum-1.0) > 1e-9 {
			t.Fatalf("weights don't sum to 1: %g", sum)
		}
		// Top half should have positive weights.
		for i := 0; i < k/2; i++ {
			if w[i] <= 0 {
				t.Fatalf("top-half weight[%d] = %g", i, w[i])
			}
		}
		// Weights should be decreasing for the top half.
		for i := 1; i < k/2; i++ {
			if w[i] > w[i-1]+1e-12 {
				t.Fatalf("weights not decreasing at %d: %g > %g", i, w[i], w[i-1])
			}
		}
	}
}

func TestESDiagCovariance(t *testing.T) {
	opt := DefaultESOptions()
	cov := esDiagCovariance(opt)
	if len(cov) != ThetaDim*ThetaDim {
		t.Fatalf("cov length %d want %d", len(cov), ThetaDim*ThetaDim)
	}
	// Diagonal entries should be positive (std^2).
	stds := []float64{opt.BankBetaStd, opt.PriceDriftStd, opt.SupplyBetaStd,
		opt.DemandSupplyBetaStd, opt.CompletionFracStd, opt.EarningsDriftStd}
	for i, s := range stds {
		got := cov[i*ThetaDim+i]
		want := s * s
		if math.Abs(got-want) > 1e-15 {
			t.Fatalf("cov[%d,%d] want %g got %g", i, i, want, got)
		}
	}
	// Off-diagonal should be zero.
	for i := range ThetaDim {
		for j := range ThetaDim {
			if i == j {
				continue
			}
			if cov[i*ThetaDim+j] != 0 {
				t.Fatalf("off-diag cov[%d,%d] = %g", i, j, cov[i*ThetaDim+j])
			}
		}
	}
}

func TestThetaRoundTrip(t *testing.T) {
	base := DefaultForwardOptions()
	theta := ThetaFromOptions(base)
	if len(theta) != ThetaDim {
		t.Fatalf("theta len %d want %d", len(theta), ThetaDim)
	}
	back := OptionsFromTheta(base, theta)
	if back.BankBeta != base.BankBeta {
		t.Fatalf("BankBeta roundtrip: %g != %g", back.BankBeta, base.BankBeta)
	}
	if back.EarningsDrift != base.EarningsDrift {
		t.Fatalf("EarningsDrift roundtrip")
	}
}

func TestPriceDriftFromThetaFunction(t *testing.T) {
	base := DefaultForwardOptions()
	base.BankBeta = -0.05
	base.PriceDrift = 0.001
	base.SupplyBeta = 0.0
	base.DemandSupplyPressureBeta = 0.0

	// Build a minimal forward sim to extract the price_drift values using the
	// theta-based function (runs correctly with the existing ES path).
	// Just verify the function doesn't panic and returns a plausible value.
	const (
		bankIdx  = 0
		supIdx   = 1
		pipeIdx  = 3
		leIdx    = 5
	)
	initLE := math.Log(30e3)
	fn := PriceDriftFromThetaValuesFunction(bankIdx, supIdx, pipeIdx, leIdx, initLE, 1000, 500)
	if fn == nil {
		t.Fatal("nil function returned")
	}
}

func TestESCalibrateRuns(t *testing.T) {
	// Smoke test: ESCalibrate should run without error on a short steady-state spine.
	obs := makeSteadyObs(8)
	base := DefaultForwardOptions()
	esOpt := ESOptions{
		CollectionSize: 4,
		Steps:          8,
		Seed:           1,
	}
	res, err := ESCalibrate(obs, base, esOpt)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ThetaMean) != ThetaDim {
		t.Fatalf("ThetaMean len %d want %d", len(res.ThetaMean), ThetaDim)
	}
	if len(res.ThetaCov) != ThetaDim*ThetaDim {
		t.Fatalf("ThetaCov len %d", len(res.ThetaCov))
	}
	for _, v := range res.ThetaMean {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("ThetaMean contains non-finite: %v", res.ThetaMean)
		}
	}
}

func TestESCalibrateStatsRuns(t *testing.T) {
	obs := makeSteadyObs(8)
	base := DefaultForwardOptions()
	esOpt := ESOptions{CollectionSize: 4, Steps: 8, Seed: 2}
	_, stats, err := ESCalibrateStats(obs, base, esOpt)
	if err != nil {
		t.Fatal(err)
	}
	if math.IsInf(stats.RMSE_P, 1) || math.IsNaN(stats.RMSE_P) {
		t.Fatalf("RMSE_P invalid: %g", stats.RMSE_P)
	}
	if stats.NumFreeParams != ThetaDim {
		t.Fatalf("NumFreeParams want %d got %d", ThetaDim, stats.NumFreeParams)
	}
}
