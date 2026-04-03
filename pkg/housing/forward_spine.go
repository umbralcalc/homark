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
	CompletionFrac              float64 // fraction/probability of pipeline stock completing per month; 0 → 0.15
	AttritionRate               float64 // probability each remaining unit lapses per month (stochastic path only)
	PipelineInit                float64 // initial pipeline stock
	SeedPipeline                uint64  // >0 uses StochasticPipelineIteration; 0 = deterministic ValuesFunctionIteration
	// DemandSupplyPressureBeta scales a composite imbalance in log-price drift:
	//   (log_earnings - init_log_earnings) - supply_net/supply_scale - pipeline_stock/pipeline_ref
	// Positive earnings growth vs t0 raises drift; higher net additions or pipeline stock lowers it. Zero disables.
	DemandSupplyPressureBeta float64
	SeedEarnings, SeedPrice  uint64
	// InitMedianRatioFallback is used only when the first spine row has no pay or ONS ratio (typical before ~1997).
	// Implied log earnings = logP − log(fallback). Zero means use 7.0.
	InitMedianRatioFallback float64
	// MarketDeliveryFraction scales planning inflow into the pipeline (0–1). 1 = all approved units enter;
	// <1 stylises tenure/affordable requirements that reduce market-facing supply. Applied to permissions_approx_monthly
	// when present, else to constant ApprovalRate.
	MarketDeliveryFraction float64
	// CompositionFlatShare is a stylised share of new supply that is “flats” (0–1); neutral mix at 0.5.
	// When CompositionDriftBeta != 0, log-price drift adds beta×(CompositionFlatShare−0.5) for density-mix scenarios.
	CompositionFlatShare float64
	CompositionDriftBeta float64
}

// DefaultForwardOptions matches the illustrative coefficients in cfg/single_la_housing.yaml.
func DefaultForwardOptions() ForwardOptions {
	return ForwardOptions{
		EarningsDrift: 0.0005, EarningsDiff: 0.004,
		PriceDrift: 0.0008, PriceDiff: 0.012,
		BankBeta: 0, SupplyBeta: 0, PipelineBeta: 0, DemandSupplyPressureBeta: 0,
		SupplyScale: 1000, PipelineRef: 500,
		ApprovalRate: 0, CompletionFrac: 0.15, AttritionRate: 0,
		PipelineInit: 0, SeedPipeline: 0,
		SeedEarnings: 9101,
		SeedPrice:    9102,
		MarketDeliveryFraction: 1,
		CompositionFlatShare:   0.5,
		CompositionDriftBeta:   0,
	}
}

// ForwardSpineConfigs builds a monthly simulation: bank_rate_pct, net_additional_dwellings_fy (scaled),
// and pipeline stock feed a scalar price_drift partition (ValuesFunctionIteration); optional
// DemandSupplyPressureBeta couples log earnings (vs initial) to supply and pipeline in that drift.
// log_price uses continuous.DriftDiffusionIteration with drift_coefficients wired from price_drift.
// Log earnings use DriftDiffusionIteration; affordability is exp(logP − logE).
//
// Partition order: bank_rate, supply_net, pipeline, price_drift, log_earnings, log_price, affordability.
// price_drift reads bank (0), supply (1), pipeline (2), and log_earnings (4) for the demand–supply pressure term.
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
	approvalsData := make([][]float64, len(obs))
	hasPermissions := false
	mf := opt.MarketDeliveryFraction
	if mf <= 0 || mf > 1 {
		mf = 1
	}
	for i := range obs {
		bankData[i] = []float64{obs[i].BankRatePct}
		supplyData[i] = []float64{obs[i].NetAddFY}
		approvalsData[i] = []float64{obs[i].PermissionsMonthly * mf}
		if obs[i].PermissionsMonthly > 0 {
			hasPermissions = true
		}
	}
	// Fall back to constant approval rate when no permissions data in spine.
	if !hasPermissions {
		ar := opt.ApprovalRate * mf
		for i := range approvalsData {
			approvalsData[i] = []float64{ar}
		}
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
	if opt.SeedPipeline > 0 {
		g.SetPartition(&simulator.PartitionConfig{
			Name:      "pipeline",
			Iteration: &StochasticPipelineIteration{},
			Params: simulator.NewParams(map[string][]float64{
				"completion_rate": {compFrac},
				"attrition_rate":  {opt.AttritionRate},
			}),
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"approval_rate": {Upstream: "approvals"},
			},
			InitStateValues:   []float64{opt.PipelineInit},
			Seed:              opt.SeedPipeline,
			StateHistoryDepth: 2,
		})
	} else {
		// Deterministic pipeline uses a constant monthly inflow (ApprovalRate×marketFraction), not the
		// time-varying permissions column; use SeedPipeline>0 (stochastic path + approvals partition) for spine-driven inflow.
		g.SetPartition(&simulator.PartitionConfig{
			Name: "pipeline",
			Iteration: &general.ValuesFunctionIteration{
				Function: PipelineStockValuesFunction(2, opt.ApprovalRate*mf, compFrac),
			},
			Params:            simulator.NewParams(map[string][]float64{}),
			InitStateValues:   []float64{opt.PipelineInit},
			Seed:              0,
			StateHistoryDepth: 2,
		})
	}
	const idxLogEarnings = 4
	g.SetPartition(&simulator.PartitionConfig{
		Name: "price_drift",
		Iteration: &general.ValuesFunctionIteration{
			Function: PriceDriftValuesFunction(0, 1, 2, idxLogEarnings, initLE, opt, supplyScale, pipeRef),
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

	// approvals partition: supplies approval_rate to the stochastic pipeline each step.
	// Uses permissions_approx_monthly from spine when available; falls back to constant opt.ApprovalRate.
	// Only added when the stochastic pipeline is active (SeedPipeline > 0).
	if opt.SeedPipeline > 0 {
		g.SetPartition(&simulator.PartitionConfig{
			Name:              "approvals",
			Iteration:         &general.FromStorageIteration{Data: approvalsData, InitStepsTaken: fromStorageTimeOffset},
			Params:            simulator.NewParams(map[string][]float64{}),
			InitStateValues:   []float64{approvalsData[0][0]},
			Seed:              0,
			StateHistoryDepth: 2,
		})
	}

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
