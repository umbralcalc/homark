package housing

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestPipelineStockHarness(t *testing.T) {
	g := simulator.NewConfigGenerator()
	g.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 12},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	g.SetPartition(&simulator.PartitionConfig{
		Name: "pipeline",
		Iteration: &general.ValuesFunctionIteration{
			Function: PipelineStockValuesFunction(0, 20, 0.2),
		},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{100},
		Seed:              0,
		StateHistoryDepth: 2,
	})
	settings, impl := g.GenerateConfigs()
	if err := simulator.RunWithHarnesses(settings, impl); err != nil {
		t.Fatal(err)
	}
}

func TestStochasticPipelineHarness(t *testing.T) {
	t.Run("stochastic pipeline with completion and attrition", func(t *testing.T) {
		g := simulator.NewConfigGenerator()
		g.SetSimulation(&simulator.SimulationConfig{
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 24},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			InitTimeValue:        0,
		})
		g.SetPartition(&simulator.PartitionConfig{
			Name:      "pipeline",
			Iteration: &StochasticPipelineIteration{},
			Params: simulator.NewParams(map[string][]float64{
				"completion_rate": {0.15},
				"attrition_rate":  {0.02},
				"approval_rate":   {20},
			}),
			InitStateValues:   []float64{100},
			Seed:              42,
			StateHistoryDepth: 2,
		})
		settings, impl := g.GenerateConfigs()
		if err := simulator.RunWithHarnesses(settings, impl); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("zero attrition all-complete drains stock to zero", func(t *testing.T) {
		// With completion_rate=1, no attrition, no inflow: every unit completes each step,
		// so stock → 0 after the first step.
		g := simulator.NewConfigGenerator()
		g.SetSimulation(&simulator.SimulationConfig{
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			InitTimeValue:        0,
		})
		g.SetPartition(&simulator.PartitionConfig{
			Name:      "pipeline",
			Iteration: &StochasticPipelineIteration{},
			Params: simulator.NewParams(map[string][]float64{
				"completion_rate": {1.0},
				"attrition_rate":  {0},
				"approval_rate":   {0},
			}),
			InitStateValues:   []float64{50},
			Seed:              1,
			StateHistoryDepth: 2,
		})
		settings, impl := g.GenerateConfigs()
		store := simulator.NewStateTimeStorage()
		impl.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
		coord := simulator.NewPartitionCoordinator(settings, impl)
		coord.Run()
		vals := store.GetValues("pipeline")
		if len(vals) < 2 {
			t.Fatalf("expected at least 2 output rows, got %d", len(vals))
		}
		// vals[0] is the initial state (t=0 = 50); from step 1 onward all units complete.
		for i, v := range vals[1:] {
			if v[0] != 0 {
				t.Errorf("step %d: expected 0 stock after first iterate, got %g", i+1, v[0])
			}
		}
	})
}
