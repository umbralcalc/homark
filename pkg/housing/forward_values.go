package housing

import (
	"github.com/umbralcalc/homark/pkg/spine"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PipelineStockValuesFunction is the body for a scalar pipeline-stock partition using
// general.ValuesFunctionIteration. It reads the pipeline partition's latest state at pipePartitionIndex.
func PipelineStockValuesFunction(pipePartitionIndex int, approvalRate, completionFrac float64) func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	cf := completionFrac
	if cf <= 0 {
		cf = 0.15
	}
	if cf > 1 {
		cf = 1
	}
	return func(
		_ *simulator.Params,
		_ int,
		stateHistories []*simulator.StateHistory,
		_ *simulator.CumulativeTimestepsHistory,
	) []float64 {
		W := stateHistories[pipePartitionIndex].Values.At(0, 0)
		complete := cf * W
		if complete > W {
			complete = W
		}
		wNew := W - complete + approvalRate
		if wNew < 0 {
			wNew = 0
		}
		return []float64{wNew}
	}
}

// PriceDriftValuesFunction returns drift_base + bank + supply/scale − pipeline×stock/ref, matching the
// former monolithic log-price iteration (optional terms omitted when the corresponding beta is zero).
func PriceDriftValuesFunction(bankIdx, supplyIdx, pipeIdx int, opt ForwardOptions, supplyScale, pipeRef float64) func(
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
		bank := stateHistories[bankIdx].Values.At(0, 0)
		supply := stateHistories[supplyIdx].Values.At(0, 0)
		pipe := stateHistories[pipeIdx].Values.At(0, 0)
		d := opt.PriceDrift + opt.BankBeta*(bank/100.0)
		if opt.SupplyBeta != 0 {
			d += opt.SupplyBeta * (supply / supplyScale)
		}
		if opt.PipelineBeta != 0 {
			d -= opt.PipelineBeta * (pipe / pipeRef)
		}
		return []float64{d}
	}
}

// initialPriceDriftScalar is the drift at t=0 using the first spine row and initial pipeline stock.
func initialPriceDriftScalar(obs0 spine.MonthlyObservation, opt ForwardOptions, supplyScale, pipeRef float64) float64 {
	bank := obs0.BankRatePct
	supply := obs0.NetAddFY
	pipe := opt.PipelineInit
	d := opt.PriceDrift + opt.BankBeta*(bank/100.0)
	if opt.SupplyBeta != 0 {
		d += opt.SupplyBeta * (supply / supplyScale)
	}
	if opt.PipelineBeta != 0 {
		d -= opt.PipelineBeta * (pipe / pipeRef)
	}
	return d
}
