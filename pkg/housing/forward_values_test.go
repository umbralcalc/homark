package housing

import (
	"math"
	"testing"

	"github.com/umbralcalc/homark/pkg/spine"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// TestPriceDriftComposeHarness wires ConstantValues covariates → scalar price_drift (ValuesFunctionIteration)
// → DriftDiffusionIteration with zero diffusion, matching the forward spine’s log_price composition.
func TestPriceDriftComposeHarness(t *testing.T) {
	g := simulator.NewConfigGenerator()
	g.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 20},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	g.SetPartition(&simulator.PartitionConfig{
		Name:              "bank",
		Iteration:         &general.ConstantValuesIteration{},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{5},
		Seed:              0,
		StateHistoryDepth: 2,
	})
	g.SetPartition(&simulator.PartitionConfig{
		Name:              "supply",
		Iteration:         &general.ConstantValuesIteration{},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{1000},
		Seed:              0,
		StateHistoryDepth: 2,
	})
	g.SetPartition(&simulator.PartitionConfig{
		Name:              "pipe",
		Iteration:         &general.ConstantValuesIteration{},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{200},
		Seed:              0,
		StateHistoryDepth: 2,
	})
	const initLE = 3.0
	g.SetPartition(&simulator.PartitionConfig{
		Name:              "log_earnings",
		Iteration:         &general.ConstantValuesIteration{},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{initLE},
		Seed:              0,
		StateHistoryDepth: 2,
	})
	opt := ForwardOptions{
		PriceDrift: 0.001, BankBeta: 0.1, SupplyBeta: 0, PipelineBeta: 0.05,
		SupplyScale: 1000, PipelineRef: 500,
	}
	drift0 := opt.PriceDrift + opt.BankBeta*(5.0/100.0) - opt.PipelineBeta*(200.0/500.0)
	g.SetPartition(&simulator.PartitionConfig{
		Name: "price_drift",
		Iteration: &general.ValuesFunctionIteration{
			Function: PriceDriftValuesFunction(0, 1, 2, 3, initLE, opt, opt.SupplyScale, opt.PipelineRef),
		},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{drift0},
		Seed:              0,
		StateHistoryDepth: 2,
	})
	g.SetPartition(&simulator.PartitionConfig{
		Name:      "log_price",
		Iteration: &continuous.DriftDiffusionIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"diffusion_coefficients": {0},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"drift_coefficients": {Upstream: "price_drift"},
		},
		InitStateValues:   []float64{3.0},
		Seed:              999,
		StateHistoryDepth: 2,
	})
	settings, impl := g.GenerateConfigs()
	if err := simulator.RunWithHarnesses(settings, impl); err != nil {
		t.Fatal(err)
	}
}

func TestInitialPriceDriftScalar_compositionMix(t *testing.T) {
	opt := ForwardOptions{
		PriceDrift:           0.01,
		CompositionDriftBeta: 0.002,
		CompositionFlatShare: 1.0,
		SupplyScale:          1000,
		PipelineRef:          500,
	}
	z := spine.MonthlyObservation{BankRatePct: 0, NetAddFY: 0}
	d := initialPriceDriftScalar(z, opt, 1000, 500)
	want := 0.01 + 0.002*(1.0-0.5)
	if math.Abs(d-want) > 1e-12 {
		t.Fatalf("drift %g want %g", d, want)
	}
}
