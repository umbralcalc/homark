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
	BankBeta                    float64 // on (bank_pct / 100)
	SupplyBeta                  float64 // on (net_add_FY / supply_scale)
	SupplyScale                 float64 // dwellings scale; 0 → 1000 inside iteration
	PipelineBeta                float64 // on (pipeline_stock / pipeline_ref); higher → more dampening when stock high
	PipelineRef                 float64 // reference stock; 0 → 500 inside iteration
	ApprovalRate                float64 // mean units/month entering pipeline (0 = no inflow)
	CompletionFrac              float64 // fraction of stock completing per month; 0 → 0.15 in iteration
	PipelineInit                float64 // initial pipeline stock
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
		BankBeta: 0, SupplyBeta: 0, PipelineBeta: 0,
		SupplyScale: 1000, PipelineRef: 500,
		ApprovalRate: 0, CompletionFrac: 0.15,
		PipelineInit: 0,
		SeedEarnings: 9101,
		SeedPrice:    9102,
	}
}

// ForwardSpineConfigs builds a monthly simulation: bank_rate_pct, net_additional_dwellings_fy (scaled),
// and pipeline stock feed a scalar price_drift partition (ValuesFunctionIteration); log_price uses
// continuous.DriftDiffusionIteration with drift_coefficients wired from price_drift. Log earnings use
// DriftDiffusionIteration; affordability is exp(logP − logE).
//
// Partition order: bank_rate, supply_net, pipeline, price_drift, log_earnings, log_price, affordability.
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
	supplyData := make([][]float64, len(obs))
	for i := range obs {
		bankData[i] = []float64{obs[i].BankRatePct}
		supplyData[i] = []float64{obs[i].NetAddFY}
	}

	supplyScale := opt.SupplyScale
	if supplyScale <= 0 {
		supplyScale = 1000
	}
	pipeRef := opt.PipelineRef
	if pipeRef <= 0 {
		pipeRef = 500
	}
	compFrac := opt.CompletionFrac
	if compFrac <= 0 {
		compFrac = 0.15
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
		Name:              "supply_net",
		Iteration:         &general.FromStorageIteration{Data: supplyData, InitStepsTaken: fromStorageTimeOffset},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{supplyData[0][0]},
		Seed:              0,
		StateHistoryDepth: 2,
	})
	g.SetPartition(&simulator.PartitionConfig{
		Name: "pipeline",
		Iteration: &general.ValuesFunctionIteration{
			Function: PipelineStockValuesFunction(2, opt.ApprovalRate, compFrac),
		},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{opt.PipelineInit},
		Seed:              0,
		StateHistoryDepth: 2,
	})
	g.SetPartition(&simulator.PartitionConfig{
		Name: "price_drift",
		Iteration: &general.ValuesFunctionIteration{
			Function: PriceDriftValuesFunction(0, 1, 2, opt, supplyScale, pipeRef),
		},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{initialPriceDriftScalar(obs[0], opt, supplyScale, pipeRef)},
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
		Iteration: &continuous.DriftDiffusionIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"diffusion_coefficients": {opt.PriceDiff},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"drift_coefficients": {Upstream: "price_drift"},
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

// RunForwardLogSeries runs the forward spine configs, collects one value per partition per step from StateTimeStorage,
// and returns time series keyed by partition name (same length as obs). Typical use: deterministic calibration (set PriceDiff and EarningsDiff to 0 in opt).
func RunForwardLogSeries(obs []spine.MonthlyObservation, opt ForwardOptions) (times []float64, series map[string][][]float64, err error) {
	settings, impl, err := ForwardSpineConfigs(obs, opt)
	if err != nil {
		return nil, nil, err
	}
	store := simulator.NewStateTimeStorage()
	impl.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coord := simulator.NewPartitionCoordinator(settings, impl)
	coord.Run()
	times = append([]float64(nil), store.GetTimes()...)
	series = make(map[string][][]float64)
	for _, name := range store.GetNames() {
		series[name] = store.GetValues(name)
	}
	return times, series, nil
}
