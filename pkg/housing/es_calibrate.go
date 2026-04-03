package housing

import (
	"fmt"
	"math"

	"github.com/umbralcalc/homark/pkg/spine"
	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ESOptions configures the Evolution Strategy calibration.
type ESOptions struct {
	// CollectionSize is the number of samples ranked per ES update.
	// Larger values give more stable updates; defaults to 20.
	CollectionSize int
	// Steps is the total number of outer coordinator steps (= CollectionSize × num_updates).
	// Defaults to 400 (20 ES updates of 20 samples each).
	Steps int
	// Seed for the sampler RNG. Defaults to 42.
	Seed uint64
	// Mean and covariance learning rates. Defaults: 0.5 and 0.2.
	MeanLearningRate float64
	CovLearningRate  float64
	// DiscountFactor for the cumulative reward inside the embedded sim. 1.0 = no discounting.
	DiscountFactor float64
	// Initial per-parameter standard deviations used to seed the ES covariance matrix.
	// Zero values fall back to their defaults (10% of the base value or a fixed small positive).
	BankBetaStd         float64
	PriceDriftStd       float64
	SupplyBetaStd       float64
	DemandSupplyBetaStd float64
	CompletionFracStd   float64
	EarningsDriftStd    float64
}

// DefaultESOptions returns sensible defaults for ESOptions.
func DefaultESOptions() ESOptions {
	return ESOptions{
		CollectionSize:      20,
		Steps:               400,
		Seed:                42,
		MeanLearningRate:    0.5,
		CovLearningRate:     0.2,
		DiscountFactor:      1.0,
		BankBetaStd:         0.05,
		PriceDriftStd:       0.001,
		SupplyBetaStd:       0.001,
		DemandSupplyBetaStd: 0.01,
		CompletionFracStd:   0.05,
		EarningsDriftStd:    0.0003,
	}
}

// ESResult holds the output of ESCalibrate.
type ESResult struct {
	Best      ForwardOptions // best parameter set (from converged theta mean)
	ThetaMean []float64      // converged parameter mean vector (len 6)
	ThetaCov  []float64      // converged parameter covariance matrix (6×6, row-major)
}

// ESCalibrate uses analysis.NewEvolutionStrategyOptimisationPartitions to optimise
// the forward-spine model parameters against the observed spine, replacing the grid search.
//
// The embedded simulation runs the full spine window deterministically for each sampled
// parameter vector (theta). Partition indices inside the embedded sim are fixed and
// document in innerPartitionOrder.
//
// The ES converges to the MLE of the 6D parameter vector
// [bank_beta, price_drift_base, supply_beta, demand_supply_beta, completion_frac, earnings_drift].
// The covariance of the converged distribution approximates parameter uncertainty.
func ESCalibrate(
	obs []spine.MonthlyObservation,
	base ForwardOptions,
	esOpt ESOptions,
) (ESResult, error) {
	if len(obs) < 2 {
		return ESResult{}, fmt.Errorf("es calibrate: need at least 2 observations")
	}
	esOpt = fillESDefaults(esOpt)

	// Compute target log series (forward-filled) for the reward comparison.
	targetP, _, err := TargetLogSeries(obs)
	if err != nil {
		return ESResult{}, fmt.Errorf("es calibrate target series: %w", err)
	}
	if len(targetP) != len(obs) {
		return ESResult{}, fmt.Errorf("es calibrate: target length mismatch")
	}

	// Extract initial log levels using the base forward options.
	fallback := base.InitMedianRatioFallback
	if fallback <= 0 {
		fallback = 7.0
	}
	initLE, initLP, err := InitLogLevelsForForward(obs[0], fallback)
	if err != nil {
		return ESResult{}, fmt.Errorf("es calibrate init levels: %w", err)
	}

	supplyScale := base.SupplyScale
	if supplyScale <= 0 {
		supplyScale = 1000
	}
	pipeRef := base.PipelineRef
	if pipeRef <= 0 {
		pipeRef = 500
	}

	// Build per-step data slices for the inner FromStorageIteration partitions.
	bankData := make([][]float64, len(obs))
	supplyData := make([][]float64, len(obs))
	obsLogPData := make([][]float64, len(obs))
	for i, o := range obs {
		bankData[i] = []float64{o.BankRatePct}
		supplyData[i] = []float64{o.NetAddFY}
		obsLogPData[i] = []float64{targetP[i]}
	}

	// Inner partition indices (fixed by order in Window.Partitions below).
	const (
		innerBankRate    = 0
		innerSupplyNet   = 1
		innerObsLogPrice = 2
		innerPipeline    = 3
		innerPriceDrift  = 4
		innerLogEarnings = 5
		innerLogPrice    = 6
	)

	sampler := "es_sampler"

	// Initial price drift at t=0 from base options (used as init state for price_drift partition).
	initDrift := initialPriceDriftScalar(obs[0], base, supplyScale, pipeRef)

	// Window partitions — all baked-in, sampled params wired via OutsideUpstreams.
	windowPartitions := []analysis.WindowedPartition{
		{
			Partition: &simulator.PartitionConfig{
				Name:              "bank_rate",
				Iteration:         &general.FromStorageIteration{Data: bankData, InitStepsTaken: fromStorageTimeOffset},
				Params:            simulator.NewParams(map[string][]float64{}),
				InitStateValues:   []float64{bankData[0][0]},
				Seed:              0,
				StateHistoryDepth: 2,
			},
		},
		{
			Partition: &simulator.PartitionConfig{
				Name:              "supply_net",
				Iteration:         &general.FromStorageIteration{Data: supplyData, InitStepsTaken: fromStorageTimeOffset},
				Params:            simulator.NewParams(map[string][]float64{}),
				InitStateValues:   []float64{supplyData[0][0]},
				Seed:              0,
				StateHistoryDepth: 2,
			},
		},
		{
			Partition: &simulator.PartitionConfig{
				Name:              "obs_log_price",
				Iteration:         &general.FromStorageIteration{Data: obsLogPData, InitStepsTaken: fromStorageTimeOffset},
				Params:            simulator.NewParams(map[string][]float64{}),
				InitStateValues:   []float64{obsLogPData[0][0]},
				Seed:              0,
				StateHistoryDepth: 2,
			},
		},
		{
			// Pipeline reads completion_frac from theta[4] via OutsideUpstreams.
			Partition: &simulator.PartitionConfig{
				Name: "pipeline",
				Iteration: &general.ValuesFunctionIteration{
					Function: PipelineFromThetaValuesFunction(innerPipeline, base.ApprovalRate),
				},
				Params:            simulator.NewParams(map[string][]float64{ThetaParamName: ThetaFromOptions(base)}),
				InitStateValues:   []float64{base.PipelineInit},
				Seed:              0,
				StateHistoryDepth: 2,
			},
			OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{
				ThetaParamName: {Upstream: sampler},
			},
		},
		{
			// Price drift reads bank_beta, price_drift_base, supply_beta, demand_supply_beta
			// from theta[0..3] via OutsideUpstreams.
			Partition: &simulator.PartitionConfig{
				Name: "price_drift",
				Iteration: &general.ValuesFunctionIteration{
					Function: PriceDriftFromThetaValuesFunction(
						innerBankRate, innerSupplyNet, innerPipeline, innerLogEarnings,
						initLE, supplyScale, pipeRef,
					),
				},
				Params:            simulator.NewParams(map[string][]float64{ThetaParamName: ThetaFromOptions(base)}),
				InitStateValues:   []float64{initDrift},
				Seed:              0,
				StateHistoryDepth: 2,
			},
			OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{
				ThetaParamName: {Upstream: sampler},
			},
		},
		{
			// Log earnings: drift_coefficients wired from theta[5] = earnings_drift.
			Partition: &simulator.PartitionConfig{
				Name:      "log_earnings",
				Iteration: &continuous.DriftDiffusionIteration{},
				Params: simulator.NewParams(map[string][]float64{
					"drift_coefficients":     {base.EarningsDrift},
					"diffusion_coefficients": {0.0},
				}),
				InitStateValues:   []float64{initLE},
				Seed:              0,
				StateHistoryDepth: 2,
			},
			OutsideUpstreams: map[string]simulator.NamedUpstreamConfig{
				"drift_coefficients": {Upstream: sampler, Indices: []int{ThetaEarningsDrift}},
			},
		},
		{
			// Log price: deterministic drift-diffusion; drift from price_drift.
			Partition: &simulator.PartitionConfig{
				Name:      "log_price",
				Iteration: &continuous.DriftDiffusionIteration{},
				Params: simulator.NewParams(map[string][]float64{
					"diffusion_coefficients": {0.0},
				}),
				ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
					"drift_coefficients": {Upstream: "price_drift"},
				},
				InitStateValues:   []float64{initLP},
				Seed:              0,
				StateHistoryDepth: 2,
			},
		},
	}

	// Reward: negative squared error between simulated and observed log price.
	rewardPartition := analysis.WindowedPartition{
		Partition: &simulator.PartitionConfig{
			Name: "neg_sq_err",
			Iteration: &general.ValuesFunctionIteration{
				Function: negSqErrFunction(innerLogPrice, innerObsLogPrice),
			},
			Params:            simulator.NewParams(map[string][]float64{}),
			InitStateValues:   []float64{0.0},
			Seed:              0,
			StateHistoryDepth: 1,
		},
	}

	// ES weights: natural log weights over top half of collection, then zeros.
	weights := esNaturalWeights(esOpt.CollectionSize)

	// Initial theta mean (from base options) and diagonal covariance.
	initMean := ThetaFromOptions(base)
	initCov := esDiagCovariance(esOpt)

	applied := analysis.AppliedEvolutionStrategyOptimisation{
		Sampler: analysis.EvolutionStrategySampler{
			Name:    sampler,
			Default: initMean,
		},
		Sorting: analysis.EvolutionStrategySorting{
			Name:           "es_sorting",
			CollectionSize: esOpt.CollectionSize,
			EmptyValue:     math.Inf(-1),
		},
		Mean: analysis.EvolutionStrategyMean{
			Name:         "es_theta_mean",
			Default:      initMean,
			Weights:      weights,
			LearningRate: esOpt.MeanLearningRate,
		},
		Covariance: analysis.EvolutionStrategyCovariance{
			Name:         "es_theta_cov",
			Default:      initCov,
			LearningRate: esOpt.CovLearningRate,
		},
		Reward: analysis.EvolutionStrategyReward{
			Partition:      rewardPartition,
			DiscountFactor: esOpt.DiscountFactor,
		},
		Window: analysis.WindowedPartitions{
			Partitions: windowPartitions,
			Depth:      len(obs),
		},
		Seed: esOpt.Seed,
	}

	esPartitions := analysis.NewEvolutionStrategyOptimisationPartitions(applied, nil)

	// Pre-compute the base-theta reward and seed ALL sorting collection slots with it.
	// This prevents math.Inf(-1) EmptyValue entries from propagating NaN into the mean
	// iteration during the first CollectionSize outer steps before the collection fills.
	baseReward, err := computeESBaseReward(obs, base, targetP)
	if err != nil {
		return ESResult{}, fmt.Errorf("es calibrate base reward: %w", err)
	}
	entryWidth := ThetaDim + 1
	sortInit := make([]float64, esOpt.CollectionSize*entryWidth)
	for i := 0; i < esOpt.CollectionSize; i++ {
		sortInit[i*entryWidth] = baseReward
		copy(sortInit[i*entryWidth+1:i*entryWidth+1+ThetaDim], initMean)
	}
	// The sorting partition is the third partition returned (index 2).
	esPartitions[2].InitStateValues = sortInit

	// Build and run the outer coordinator.
	g := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()
	g.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: esOpt.Steps},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	for _, p := range esPartitions {
		g.SetPartition(p)
	}
	settings, impl := g.GenerateConfigs()
	coord := simulator.NewPartitionCoordinator(settings, impl)
	coord.Run()

	// Extract converged theta mean and covariance from final stored values.
	meanSeries := store.GetValues("es_theta_mean")
	covSeries := store.GetValues("es_theta_cov")
	if len(meanSeries) == 0 {
		return ESResult{}, fmt.Errorf("es calibrate: no mean values stored")
	}
	thetaMean := meanSeries[len(meanSeries)-1]
	thetaCov := covSeries[len(covSeries)-1]

	best := OptionsFromTheta(base, thetaMean)

	return ESResult{
		Best:      best,
		ThetaMean: thetaMean,
		ThetaCov:  thetaCov,
	}, nil
}

// computeESBaseReward runs one deterministic forward pass with base options and returns
// the cumulative negative squared error of log_price vs targetP — the same quantity
// the ES reward accumulates inside the embedded simulation.
func computeESBaseReward(obs []spine.MonthlyObservation, base ForwardOptions, targetP []float64) (float64, error) {
	do := DeterministicForwardOptions(base)
	_, series, err := RunForwardLogSeries(obs, do)
	if err != nil {
		return 0, err
	}
	fp, ok := series["log_price"]
	if !ok {
		return 0, fmt.Errorf("computeESBaseReward: log_price series missing")
	}
	var reward float64
	for i, v := range fp {
		if i >= len(targetP) {
			break
		}
		d := v[0] - targetP[i]
		reward -= d * d
	}
	return reward, nil
}

// negSqErrFunction returns a ValuesFunctionIteration body computing
// -(simulated_log_price - observed_log_price)^2.
func negSqErrFunction(logPriceIdx, obsLogPriceIdx int) func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return func(
		_ *simulator.Params,
		_ int,
		stateHistories []*simulator.StateHistory,
		_ *simulator.CumulativeTimestepsHistory,
	) []float64 {
		sim := stateHistories[logPriceIdx].Values.At(0, 0)
		obs := stateHistories[obsLogPriceIdx].Values.At(0, 0)
		d := sim - obs
		return []float64{-d * d}
	}
}

// esNaturalWeights computes natural evolution-strategy weights over the top half of
// the collection, with zero weight for the bottom half. Weights sum to 1.
func esNaturalWeights(collectionSize int) []float64 {
	topK := collectionSize / 2
	if topK < 1 {
		topK = 1
	}
	raw := make([]float64, topK)
	sum := 0.0
	for i := range topK {
		raw[i] = math.Log(float64(topK)+1) - math.Log(float64(i)+1)
		sum += raw[i]
	}
	weights := make([]float64, collectionSize)
	for i := range topK {
		weights[i] = raw[i] / sum
	}
	return weights
}

// esDiagCovariance builds the initial diagonal covariance matrix (ThetaDim×ThetaDim, row-major).
func esDiagCovariance(opt ESOptions) []float64 {
	stds := [ThetaDim]float64{
		opt.BankBetaStd, opt.PriceDriftStd, opt.SupplyBetaStd,
		opt.DemandSupplyBetaStd, opt.CompletionFracStd, opt.EarningsDriftStd,
	}
	cov := make([]float64, ThetaDim*ThetaDim)
	for i, s := range stds {
		cov[i*ThetaDim+i] = s * s
	}
	return cov
}

// fillESDefaults ensures zero-valued ESOptions fields are set to sensible defaults.
func fillESDefaults(opt ESOptions) ESOptions {
	d := DefaultESOptions()
	if opt.CollectionSize <= 0 {
		opt.CollectionSize = d.CollectionSize
	}
	if opt.Steps <= 0 {
		opt.Steps = d.Steps
	}
	if opt.Seed == 0 {
		opt.Seed = d.Seed
	}
	if opt.MeanLearningRate <= 0 {
		opt.MeanLearningRate = d.MeanLearningRate
	}
	if opt.CovLearningRate <= 0 {
		opt.CovLearningRate = d.CovLearningRate
	}
	if opt.DiscountFactor <= 0 {
		opt.DiscountFactor = d.DiscountFactor
	}
	if opt.BankBetaStd <= 0 {
		opt.BankBetaStd = d.BankBetaStd
	}
	if opt.PriceDriftStd <= 0 {
		opt.PriceDriftStd = d.PriceDriftStd
	}
	if opt.SupplyBetaStd <= 0 {
		opt.SupplyBetaStd = d.SupplyBetaStd
	}
	if opt.DemandSupplyBetaStd <= 0 {
		opt.DemandSupplyBetaStd = d.DemandSupplyBetaStd
	}
	if opt.CompletionFracStd <= 0 {
		opt.CompletionFracStd = d.CompletionFracStd
	}
	if opt.EarningsDriftStd <= 0 {
		opt.EarningsDriftStd = d.EarningsDriftStd
	}
	return opt
}

// ESCalibrateStats runs ESCalibrate and then calls ComputeCalibrationStats on the
// resulting best options, returning both the ESResult and full fit statistics.
func ESCalibrateStats(
	obs []spine.MonthlyObservation,
	base ForwardOptions,
	esOpt ESOptions,
) (ESResult, CalibrationStats, error) {
	res, err := ESCalibrate(obs, base, esOpt)
	if err != nil {
		return ESResult{}, CalibrationStats{}, err
	}
	stats, err := ComputeCalibrationStats(obs, res.Best, ThetaDim)
	if err != nil {
		return ESResult{}, CalibrationStats{}, err
	}
	return res, stats, nil
}

// NormalLikelihoodDistributionForLogPrice wraps inference.NormalLikelihoodDistribution
// so it can be used as the data-comparison model in analysis.NewLikelihoodComparisonPartition
// for the log-price partition.
//
// This exists purely to export the stochadex NormalLikelihoodDistribution under a
// descriptive housing-domain alias — the underlying type is unchanged.
type NormalLikelihoodDistributionForLogPrice = inference.NormalLikelihoodDistribution
