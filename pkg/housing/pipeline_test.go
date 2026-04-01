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
