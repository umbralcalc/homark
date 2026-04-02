package housing

import (
	"path/filepath"
	"testing"

	"github.com/umbralcalc/homark/pkg/spine"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestForwardSpineHarness(t *testing.T) {
	sample := filepath.Join("..", "spine", "testdata", "spine_replay_sample.csv")
	obs, err := spine.LoadSpineMonthlyForArea(sample, "E09000030")
	if err != nil {
		t.Fatal(err)
	}
	opt := DefaultForwardOptions()
	settings, impl, err := ForwardSpineConfigs(obs, opt)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulator.RunWithHarnesses(settings, impl); err != nil {
		t.Fatal(err)
	}
}

func TestForwardSpineWithDemandSupplyPressureHarness(t *testing.T) {
	sample := filepath.Join("..", "spine", "testdata", "spine_replay_sample.csv")
	obs, err := spine.LoadSpineMonthlyForArea(sample, "E09000030")
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) > 24 {
		obs = obs[:24]
	}
	opt := DefaultForwardOptions()
	opt.DemandSupplyPressureBeta = 0.02
	opt.PriceDiff, opt.EarningsDiff = 0, 0
	settings, impl, err := ForwardSpineConfigs(obs, opt)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulator.RunWithHarnesses(settings, impl); err != nil {
		t.Fatal(err)
	}
}

func TestStochasticForwardSpineWithPermissionsHarness(t *testing.T) {
	// Stochastic pipeline with permissions data wired via approvals partition.
	obs := []spine.MonthlyObservation{
		{YearMonth: "2010-01", AveragePrice: 150e3, MedianRatio: 7.0, BankRatePct: 0.5, PermissionsMonthly: 25},
		{YearMonth: "2010-02", AveragePrice: 151e3, MedianRatio: 7.0, BankRatePct: 0.5, PermissionsMonthly: 28},
		{YearMonth: "2010-03", AveragePrice: 152e3, MedianRatio: 7.0, BankRatePct: 0.5, PermissionsMonthly: 22},
	}
	opt := DefaultForwardOptions()
	opt.SeedPipeline = 7001
	opt.PipelineInit = 100
	opt.CompletionFrac = 0.15
	opt.AttritionRate = 0.02
	settings, impl, err := ForwardSpineConfigs(obs, opt)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulator.RunWithHarnesses(settings, impl); err != nil {
		t.Fatal(err)
	}
}

func TestStochasticForwardSpineConstantApprovalFallback(t *testing.T) {
	// When no permissions data is present, approvals partition uses constant opt.ApprovalRate.
	obs := []spine.MonthlyObservation{
		{YearMonth: "2010-01", AveragePrice: 150e3, MedianRatio: 7.0, BankRatePct: 0.5},
		{YearMonth: "2010-02", AveragePrice: 151e3, MedianRatio: 7.0, BankRatePct: 0.5},
		{YearMonth: "2010-03", AveragePrice: 152e3, MedianRatio: 7.0, BankRatePct: 0.5},
	}
	opt := DefaultForwardOptions()
	opt.SeedPipeline = 7002
	opt.ApprovalRate = 30
	opt.PipelineInit = 80
	settings, impl, err := ForwardSpineConfigs(obs, opt)
	if err != nil {
		t.Fatal(err)
	}
	if err := simulator.RunWithHarnesses(settings, impl); err != nil {
		t.Fatal(err)
	}
}

func TestDeterministicForwardLogSeriesShape(t *testing.T) {
	// Constant bank rate: drift should equal drift_base when bank_drift_beta = 0.
	obs := []spine.MonthlyObservation{
		{YearMonth: "2004-01", AveragePrice: 100e3, MedianRatio: 8.0, BankRatePct: 5.0},
		{YearMonth: "2004-02", AveragePrice: 100e3, MedianRatio: 8.0, BankRatePct: 5.0},
	}
	opt := DefaultForwardOptions()
	opt.BankBeta = 0
	opt.SeedEarnings = 42
	opt.SeedPrice = 42
	settings, impl, err := ForwardSpineConfigs(obs, opt)
	if err != nil {
		t.Fatal(err)
	}
	store := simulator.NewStateTimeStorage()
	impl.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coord := simulator.NewPartitionCoordinator(settings, impl)
	coord.Run()
	vP := store.GetValues("log_price")
	if len(vP) != len(obs) {
		t.Fatalf("got %d price rows want %d", len(vP), len(obs))
	}
}
