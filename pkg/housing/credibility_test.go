package housing

import (
	"math"
	"testing"

	"github.com/umbralcalc/homark/pkg/spine"
	"gonum.org/v1/gonum/floats"
)

func TestPearsonCorrelation_perfectLine(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{2, 4, 6, 8, 10}
	r, n, err := PearsonCorrelation(x, y)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("n=%d want 5", n)
	}
	if !floats.EqualApprox([]float64{r}, []float64{1}, 1e-9) {
		t.Fatalf("r=%g want 1", r)
	}
}

func TestPearsonCorrelation_withNaNSkipped(t *testing.T) {
	x := []float64{1, math.NaN(), 3, 4}
	y := []float64{1, 2, 3, 4}
	r, n, err := PearsonCorrelation(x, y)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("n=%d want 3", n)
	}
	if !floats.EqualApprox([]float64{r}, []float64{1}, 1e-9) {
		t.Fatalf("r=%g want 1", r)
	}
}

func TestSummarizeCredibilitySpine(t *testing.T) {
	obs := []spine.MonthlyObservation{
		{AveragePrice: 100, PPDMedianPrice: 90, PermissionsMonthly: 1, CompletionsMonthly: 2},
		{AveragePrice: 0, PPDMedianPrice: 0, PermissionsMonthly: 0, CompletionsMonthly: 0},
	}
	s := SummarizeCredibilitySpine(obs)
	if s.Rows != 2 || s.AveragePricePositive != 1 || s.PPDAllPositive != 1 ||
		s.PermissionsPositive != 1 || s.CompletionsPositive != 1 || s.SameMonthPermAndCompletion != 1 {
		t.Fatalf("%+v", s)
	}
}

func TestPermissionCompletionLagScan_synthetic(t *testing.T) {
	// completions track permissions one month later (lag 1).
	n := 20
	obs := make([]spine.MonthlyObservation, n)
	for i := range obs {
		obs[i].PermissionsMonthly = float64(10 + i)
		if i+1 < n {
			obs[i+1].CompletionsMonthly = float64(10 + i)
		}
	}
	lag, r, pairs, err := PermissionCompletionLagScan(obs, 6)
	if err != nil {
		t.Fatal(err)
	}
	if lag != 1 {
		t.Fatalf("best lag=%d want 1", lag)
	}
	if pairs < 3 {
		t.Fatalf("pairs=%d", pairs)
	}
	if math.Abs(r) < 0.9 {
		t.Fatalf("|r|=%g want strong correlation", math.Abs(r))
	}
}
