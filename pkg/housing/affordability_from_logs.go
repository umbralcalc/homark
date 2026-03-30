package housing

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// AffordabilityFromLogsIteration maps log(price) and log(earnings) from two upstream
// partitions to price/earnings: exp(logP - logE). Upstream partition indices are read
// from params at configure time.
type AffordabilityFromLogsIteration struct {
	logPriceIx    int
	logEarningsIx int
}

func (a *AffordabilityFromLogsIteration) Configure(partitionIndex int, settings *simulator.Settings) {
	p := settings.Iterations[partitionIndex].Params.Map
	a.logPriceIx = int(p["log_price_partition"][0])
	a.logEarningsIx = int(p["log_earnings_partition"][0])
}

func (a *AffordabilityFromLogsIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	logP := stateHistories[a.logPriceIx].Values.At(0, 0)
	logE := stateHistories[a.logEarningsIx].Values.At(0, 0)
	return []float64{math.Exp(logP - logE)}
}
