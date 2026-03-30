package housing

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestSingleLAHousingHarness(t *testing.T) {
	t.Run("log earnings, log price, affordability runs with harnesses", func(t *testing.T) {
		settings := simulator.LoadSettingsFromYaml("./housing_settings.yaml")
		iterations := []simulator.Iteration{
			&continuous.DriftDiffusionIteration{},
			&continuous.DriftDiffusionIteration{},
			&AffordabilityFromLogsIteration{},
		}
		for i := range iterations {
			iterations[i].Configure(i, settings)
		}
		store := simulator.NewStateTimeStorage()
		implementations := &simulator.Implementations{
			Iterations:      iterations,
			OutputCondition: &simulator.EveryStepOutputCondition{},
			OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
				MaxNumberOfSteps: 120,
			},
			TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatalf("harness: %v", err)
		}
	})
}
