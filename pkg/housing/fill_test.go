package housing

import (
	"math"
	"testing"

	"github.com/umbralcalc/homark/pkg/spine"
)

func TestForwardFillAffordableFields(t *testing.T) {
	obs := []spine.MonthlyObservation{
		{YearMonth: "1995-01", AveragePrice: 100e3},
		{YearMonth: "1995-02", AveragePrice: 101e3, MedianRatio: 8.0},
		{YearMonth: "1995-03", AveragePrice: 102e3},
	}
	out := ForwardFillAffordableFields(obs)
	if out[0].MedianRatio != 8.0 || out[2].MedianRatio != 8.0 {
		t.Fatalf("ratios %+v %+v", out[0].MedianRatio, out[2].MedianRatio)
	}
	_, _, _, err := MonthlyLogSeries(out)
	if err != nil {
		t.Fatal(err)
	}
}

func TestForwardFillEarningsCarry(t *testing.T) {
	obs := []spine.MonthlyObservation{
		{YearMonth: "2000-01", AveragePrice: 100e3},
		{YearMonth: "2000-02", AveragePrice: 101e3, MedianRatio: 7.0, EarningsAnnual: 32000},
		{YearMonth: "2000-03", AveragePrice: 102e3},
	}
	out := ForwardFillAffordableFields(obs)
	if out[0].EarningsAnnual != 32000 || out[2].EarningsAnnual != 32000 {
		t.Fatalf("earnings fill/carry %+v %+v", out[0].EarningsAnnual, out[2].EarningsAnnual)
	}
	_, _, _, err := MonthlyLogSeries(out)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGridCalibrateDeterministicRuns(t *testing.T) {
	sample := spine.MonthlyObservation{
		YearMonth: "2004-01", AveragePrice: 100e3, MedianRatio: 8.0, BankRatePct: 4.5,
	}
	obs := []spine.MonthlyObservation{sample, sample, sample}
	base := DefaultForwardOptions()
	grid := CalibrateGrid{BankBetaLo: 0, BankBetaHi: 0, BankSteps: 1}
	best, rmseP, _, err := GridCalibrateDeterministic(obs, base, grid, 0)
	if err != nil {
		t.Fatal(err)
	}
	if math.IsInf(rmseP, 1) {
		t.Fatal("rmse")
	}
	_ = best
}

func TestGridCalibrateDemandSupplyPressureGrid(t *testing.T) {
	sample := spine.MonthlyObservation{
		YearMonth: "2004-01", AveragePrice: 100e3, MedianRatio: 8.0, BankRatePct: 4.5, EarningsAnnual: 30e3,
	}
	obs := []spine.MonthlyObservation{sample, sample, sample}
	base := DefaultForwardOptions()
	grid := CalibrateGrid{
		BankBetaLo: 0, BankBetaHi: 0, BankSteps: 1,
		DemandSupplyPressureBetaLo: -0.01, DemandSupplyPressureBetaHi: 0.01, DemandSupplySteps: 3,
	}
	best, _, _, err := GridCalibrateDeterministic(obs, base, grid, 0)
	if err != nil {
		t.Fatal(err)
	}
	if best.DemandSupplyPressureBeta < -0.01-1e-9 || best.DemandSupplyPressureBeta > 0.01+1e-9 {
		t.Fatalf("best dsp out of grid: %g", best.DemandSupplyPressureBeta)
	}
}
