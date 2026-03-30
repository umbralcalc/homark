package housing

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/homark/pkg/spine"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestInitLogLevelsForForwardFallback(t *testing.T) {
	o := spine.MonthlyObservation{YearMonth: "1995-01", AveragePrice: 100e3}
	logE, logP, err := InitLogLevelsForForward(o, 8.0)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(logP-math.Log(100e3)) > 1e-9 {
		t.Fatalf("logP %v", logP)
	}
	wantE := math.Log(100e3 / 8.0)
	if math.Abs(logE-wantE) > 1e-9 {
		t.Fatalf("logE %v want %v", logE, wantE)
	}
}

func TestMonthlyLogSeries(t *testing.T) {
	obs := []spine.MonthlyObservation{
		{YearMonth: "2004-01", AveragePrice: 100000, MedianRatio: 8.0},
		{YearMonth: "2004-02", AveragePrice: 0, Index: 50, MedianRatio: 8.0},
		{YearMonth: "2004-03", AveragePrice: 90000, EarningsAnnual: 30000},
	}
	logE, logP, afford, err := MonthlyLogSeries(obs)
	if err != nil {
		t.Fatal(err)
	}
	if len(logE) != 3 {
		t.Fatalf("len %d", len(logE))
	}
	wantLE0 := math.Log(100000.0 / 8.0)
	if math.Abs(logE[0][0]-wantLE0) > 1e-9 {
		t.Fatalf("logE0 %v want %v", logE[0][0], wantLE0)
	}
	if math.Abs(afford[0][0]-8.0) > 1e-9 {
		t.Fatalf("afford0 %v", afford[0][0])
	}
	if math.Abs(afford[2][0]-90000.0/30000.0) > 1e-9 {
		t.Fatalf("afford2 %v", afford[2][0])
	}
	if len(logP) != 3 {
		t.Fatalf("logP len %d", len(logP))
	}
}

func TestReplayHarnessFromSpineFixture(t *testing.T) {
	sample := filepath.Join("..", "spine", "testdata", "spine_replay_sample.csv")
	obs, err := spine.LoadSpineMonthlyForArea(sample, "E09000030")
	if err != nil {
		t.Fatal(err)
	}
	logE, logP, afford, err := MonthlyLogSeries(obs)
	if err != nil {
		t.Fatal(err)
	}
	settings, impl, err := ReplayImplementations(logE, logP, afford)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulator.RunWithHarnesses(settings, impl); err != nil {
		t.Fatal(err)
	}
}
