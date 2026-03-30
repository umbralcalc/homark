package housing

import (
	"fmt"
	"math"

	"github.com/umbralcalc/homark/pkg/spine"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ForwardOptions holds default coefficients for ForwardSpineConfigs (single-LA monthly step).
type ForwardOptions struct {
	EarningsDrift, EarningsDiff float64
	PriceDrift, PriceDiff       float64
	BankBeta                    float64 // added to log-price drift as beta * (bank_pct / 100)
	SeedEarnings, SeedPrice     uint64
	// InitMedianRatioFallback is used only when the first spine row has no pay or ONS ratio (typical before ~1997).
	// Implied log earnings = logP − log(fallback). Zero means use 7.0.
	InitMedianRatioFallback float64
}

// DefaultForwardOptions matches the illustrative coefficients in cfg/single_la_housing.yaml.
func DefaultForwardOptions() ForwardOptions {
	return ForwardOptions{
		EarningsDrift: 0.0005, EarningsDiff: 0.004,
		PriceDrift: 0.0008, PriceDiff: 0.012,
		BankBeta:     0,
		SeedEarnings: 9101, SeedPrice: 9102,
	}
}

// ForwardSpineConfigs builds a monthly simulation: historical bank_rate_pct from storage drives
// log-price drift (via DriftDiffusionBankChannelIteration); log earnings follow a constant
// DriftDiffusionIteration; affordability is exp(logP − logE).
//
// Partition order: bank_rate, log_earnings, log_price, affordability.
// GenerateConfigs has already called Configure on each iteration.
func ForwardSpineConfigs(obs []spine.MonthlyObservation, opt ForwardOptions) (*simulator.Settings, *simulator.Implementations, error) {
	if len(obs) == 0 {
		return nil, nil, fmt.Errorf("forward spine: no observations")
	}
	fallback := opt.InitMedianRatioFallback
	if fallback <= 0 {
		fallback = 7.0
	}
	initLE, initLP, err := InitLogLevelsForForward(obs[0], fallback)
	if err != nil {
		return nil, nil, fmt.Errorf("forward spine init: %w", err)
	}

	bankData := make([][]float64, len(obs))
	for i := range obs {
		bankData[i] = []float64{obs[i].BankRatePct}
	}

	g := simulator.NewConfigGenerator()
	g.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &SkipInitTimestepOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: len(obs)},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})

	g.SetPartition(&simulator.PartitionConfig{
		Name:              "bank_rate",
		Iteration:         &general.FromStorageIteration{Data: bankData, InitStepsTaken: fromStorageTimeOffset},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{bankData[0][0]},
		Seed:              0,
		StateHistoryDepth: 2,
	})
	g.SetPartition(&simulator.PartitionConfig{
		Name:      "log_earnings",
		Iteration: &continuous.DriftDiffusionIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"drift_coefficients":     {opt.EarningsDrift},
			"diffusion_coefficients": {opt.EarningsDiff},
		}),
		InitStateValues:   []float64{initLE},
		Seed:              opt.SeedEarnings,
		StateHistoryDepth: 2,
	})
	g.SetPartition(&simulator.PartitionConfig{
		Name:      "log_price",
		Iteration: &DriftDiffusionBankChannelIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"drift_base":             {opt.PriceDrift},
			"diffusion_coefficients": {opt.PriceDiff},
			"bank_drift_beta":        {opt.BankBeta},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"bank_rate_pct": {Upstream: "bank_rate"},
		},
		InitStateValues:   []float64{initLP},
		Seed:              opt.SeedPrice,
		StateHistoryDepth: 2,
	})
	g.SetPartition(&simulator.PartitionConfig{
		Name:      "affordability",
		Iteration: &AffordabilityFromLogsIteration{},
		Params:    simulator.NewParams(map[string][]float64{}),
		ParamsAsPartitions: map[string][]string{
			"log_price_partition":    {"log_price"},
			"log_earnings_partition": {"log_earnings"},
		},
		InitStateValues:   []float64{math.Exp(initLP - initLE)},
		Seed:              0,
		StateHistoryDepth: 2,
	})

	settings, impl := g.GenerateConfigs()
	return settings, impl, nil
}
