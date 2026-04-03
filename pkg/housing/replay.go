package housing

import (
	"fmt"
	"math"

	"github.com/umbralcalc/homark/pkg/spine"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// MonthlyLogSeries builds per-month log earnings, log price, and affordability (P/E) from spine observations.
// Earnings use median_gross_annual_pay when present; otherwise log earnings = log P − log(median_ratio) when ratio > 0.
// Price uses log(AveragePrice) when > 0, else log(Index) when > 0.
func MonthlyLogSeries(obs []spine.MonthlyObservation) (logE, logP, afford [][]float64, err error) {
	if len(obs) == 0 {
		return nil, nil, nil, fmt.Errorf("housing replay: no observations")
	}
	logE = make([][]float64, len(obs))
	logP = make([][]float64, len(obs))
	afford = make([][]float64, len(obs))
	for i, o := range obs {
		lp, err := logPriceFromObs(o)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("row %s: %w", o.YearMonth, err)
		}
		le, err := logEarningsFromObs(o, lp)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("row %s: %w", o.YearMonth, err)
		}
		logP[i] = []float64{lp}
		logE[i] = []float64{le}
		afford[i] = []float64{math.Exp(lp - le)}
	}
	return logE, logP, afford, nil
}

func logPriceFromObs(o spine.MonthlyObservation) (float64, error) {
	// Prefer PPD median when present — it is a direct local transaction median rather than
	// the mix-adjusted HPI average, giving a more LA-specific price signal.
	if o.PPDMedianPrice > 0 {
		return math.Log(o.PPDMedianPrice), nil
	}
	if o.AveragePrice > 0 {
		return math.Log(o.AveragePrice), nil
	}
	if o.Index > 0 {
		return math.Log(o.Index), nil
	}
	return 0, fmt.Errorf("need PPDMedianPrice, AveragePrice, or Index")
}

func logEarningsFromObs(o spine.MonthlyObservation, logP float64) (float64, error) {
	if o.EarningsAnnual > 0 {
		return math.Log(o.EarningsAnnual), nil
	}
	if o.MedianRatio > 0 {
		return logP - math.Log(o.MedianRatio), nil
	}
	return 0, fmt.Errorf("need median_gross_annual_pay or median_ratio (with price/index)")
}

// InitLogLevelsForForward returns starting log-earnings and log-price for a stochastic forward run
// from the first spine month. Early UK HPI rows often lack ONS pay/ratio; if both are missing,
// implied log earnings uses defaultMedianRatio (price/earnings, e.g. 7) so logE = logP − log(ratio).
func InitLogLevelsForForward(o spine.MonthlyObservation, defaultMedianRatio float64) (logE, logP float64, err error) {
	logP, err = logPriceFromObs(o)
	if err != nil {
		return 0, 0, err
	}
	if o.EarningsAnnual > 0 {
		return math.Log(o.EarningsAnnual), logP, nil
	}
	if o.MedianRatio > 0 {
		return logP - math.Log(o.MedianRatio), logP, nil
	}
	if defaultMedianRatio <= 0 {
		return 0, 0, fmt.Errorf("defaultMedianRatio must be > 0")
	}
	return logP - math.Log(defaultMedianRatio), logP, nil
}

const fromStorageTimeOffset = -1 // align FromStorageIteration index with CurrentStepNumber (see stochadex from_storage.go)

// SkipInitTimestepOutputCondition suppresses NewStateIterator's one-off output at init_time_value (usually 0).
// Otherwise EveryStepOutputCondition emits an extra row before the first Step, which desynchronises replay from spine rows.
type SkipInitTimestepOutputCondition struct{}

func (SkipInitTimestepOutputCondition) IsOutputStep(_ string, _ []float64, timestepsHistory *simulator.CumulativeTimestepsHistory) bool {
	return timestepsHistory.CurrentStepNumber > 0
}

// ReplayImplementations returns storage-backed partitions: log_earnings, log_price, affordability (precomputed),
// plus simulator settings. Caller must call iteration.Configure(i, settings) for each implementation.Iterations[i]
// before NewPartitionCoordinator (same pattern as stochadex coordinator tests).
func ReplayImplementations(logE, logP, afford [][]float64) (*simulator.Settings, *simulator.Implementations, error) {
	n := len(logE)
	if len(logP) != n || len(afford) != n {
		return nil, nil, fmt.Errorf("replay: length mismatch logE=%d logP=%d afford=%d", len(logE), len(logP), len(afford))
	}
	if n == 0 {
		return nil, nil, fmt.Errorf("replay: empty series")
	}
	settings := &simulator.Settings{
		Iterations: []simulator.IterationSettings{
			{
				Name:              "log_earnings",
				Params:            simulator.NewParams(map[string][]float64{}),
				InitStateValues:   append([]float64(nil), logE[0]...),
				Seed:              0,
				StateWidth:        1,
				StateHistoryDepth: 2,
			},
			{
				Name:              "log_price",
				Params:            simulator.NewParams(map[string][]float64{}),
				InitStateValues:   append([]float64(nil), logP[0]...),
				Seed:              0,
				StateWidth:        1,
				StateHistoryDepth: 2,
			},
			{
				Name:              "affordability",
				Params:            simulator.NewParams(map[string][]float64{}),
				InitStateValues:   append([]float64(nil), afford[0]...),
				Seed:              0,
				StateWidth:        1,
				StateHistoryDepth: 2,
			},
		},
		InitTimeValue:         0,
		TimestepsHistoryDepth: 2,
	}
	settings.Init()

	iterations := []simulator.Iteration{
		&general.FromStorageIteration{Data: logE, InitStepsTaken: fromStorageTimeOffset},
		&general.FromStorageIteration{Data: logP, InitStepsTaken: fromStorageTimeOffset},
		&general.FromStorageIteration{Data: afford, InitStepsTaken: fromStorageTimeOffset},
	}
	impl := &simulator.Implementations{
		Iterations:           iterations,
		OutputCondition:      &SkipInitTimestepOutputCondition{},
		OutputFunction:       nil, // set by caller
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: n},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	}
	return settings, impl, nil
}

// ConfigureReplayIterations calls Configure on each concrete iteration (required before running the coordinator).
func ConfigureReplayIterations(settings *simulator.Settings, iterations []simulator.Iteration) {
	for i, it := range iterations {
		it.Configure(i, settings)
	}
}
