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

// PriceDriftValuesFunction returns drift for the log-price SDE: base + bank/supply/pipeline terms
// (optional betas) plus DemandSupplyPressureBeta × imbalance when non-zero.
// logEarningsIdx is the partition index of log_earnings (same coordinator step sees its latest committed state).
func PriceDriftValuesFunction(bankIdx, supplyIdx, pipeIdx, logEarningsIdx int, initLogEarnings float64, opt ForwardOptions, supplyScale, pipeRef float64) func(
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
		if opt.DemandSupplyPressureBeta != 0 {
			le := stateHistories[logEarningsIdx].Values.At(0, 0)
			imb := (le - initLogEarnings) - (supply / supplyScale) - (pipe / pipeRef)
			d += opt.DemandSupplyPressureBeta * imb
		}
		if opt.CompositionDriftBeta != 0 {
			fs := opt.CompositionFlatShare
			if fs < 0 {
				fs = 0
			}
			if fs > 1 {
				fs = 1
			}
			d += opt.CompositionDriftBeta * (fs - 0.5)
		}
		return []float64{d}
	}
}

// ThetaParamName is the params key used to pass the ES parameter vector into inner-simulation partitions.
const ThetaParamName = "theta"

// ThetaLayout defines the fixed ordering of parameters in the theta vector used by ES calibration.
// Index meanings: 0=bank_beta, 1=price_drift_base, 2=supply_beta, 3=demand_supply_beta,
// 4=completion_frac, 5=earnings_drift.
const (
	ThetaBankBeta          = 0
	ThetaPriceDriftBase    = 1
	ThetaSupplyBeta        = 2
	ThetaDemandSupplyBeta  = 3
	ThetaCompletionFrac    = 4
	ThetaEarningsDrift     = 5
	ThetaDim               = 6
)

// ThetaFromOptions extracts the theta vector from a ForwardOptions value.
func ThetaFromOptions(opt ForwardOptions) []float64 {
	return []float64{
		opt.BankBeta, opt.PriceDrift, opt.SupplyBeta,
		opt.DemandSupplyPressureBeta, opt.CompletionFrac, opt.EarningsDrift,
	}
}

// OptionsFromTheta writes the theta vector fields back into a ForwardOptions copy.
func OptionsFromTheta(base ForwardOptions, theta []float64) ForwardOptions {
	o := base
	o.BankBeta = theta[ThetaBankBeta]
	o.PriceDrift = theta[ThetaPriceDriftBase]
	o.SupplyBeta = theta[ThetaSupplyBeta]
	o.DemandSupplyPressureBeta = theta[ThetaDemandSupplyBeta]
	o.CompletionFrac = theta[ThetaCompletionFrac]
	o.EarningsDrift = theta[ThetaEarningsDrift]
	return o
}

// PriceDriftFromThetaValuesFunction is a variant of PriceDriftValuesFunction where the
// model coefficients are read from params.Get(ThetaParamName) at runtime rather than from
// a closure. This enables the ES sampler to wire in different parameter values each outer step.
// theta layout: [bank_beta, price_drift_base, supply_beta, demand_supply_beta, completion_frac, earnings_drift].
// The function still captures partition indices, initial log-earnings, and scaling constants in the closure.
func PriceDriftFromThetaValuesFunction(bankIdx, supplyIdx, pipeIdx, logEarningsIdx int, initLogEarnings float64, supplyScale, pipeRef float64) func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return func(
		params *simulator.Params,
		_ int,
		stateHistories []*simulator.StateHistory,
		_ *simulator.CumulativeTimestepsHistory,
	) []float64 {
		theta := params.Get(ThetaParamName)
		bank := stateHistories[bankIdx].Values.At(0, 0)
		supply := stateHistories[supplyIdx].Values.At(0, 0)
		pipe := stateHistories[pipeIdx].Values.At(0, 0)
		d := theta[ThetaPriceDriftBase] + theta[ThetaBankBeta]*(bank/100.0)
		if theta[ThetaSupplyBeta] != 0 {
			d += theta[ThetaSupplyBeta] * (supply / supplyScale)
		}
		if theta[ThetaDemandSupplyBeta] != 0 {
			le := stateHistories[logEarningsIdx].Values.At(0, 0)
			imb := (le - initLogEarnings) - (supply / supplyScale) - (pipe / pipeRef)
			d += theta[ThetaDemandSupplyBeta] * imb
		}
		return []float64{d}
	}
}

// PipelineFromThetaValuesFunction is a variant of PipelineStockValuesFunction where
// completion_frac is read from params.Get(ThetaParamName)[ThetaCompletionFrac] at runtime.
func PipelineFromThetaValuesFunction(pipePartitionIndex int, approvalRate float64) func(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return func(
		params *simulator.Params,
		_ int,
		stateHistories []*simulator.StateHistory,
		_ *simulator.CumulativeTimestepsHistory,
	) []float64 {
		theta := params.Get(ThetaParamName)
		cf := theta[ThetaCompletionFrac]
		if cf <= 0 {
			cf = 0.15
		}
		if cf > 1 {
			cf = 1
		}
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
	if opt.DemandSupplyPressureBeta != 0 {
		imb := -(supply / supplyScale) - (pipe / pipeRef)
		d += opt.DemandSupplyPressureBeta * imb
	}
	if opt.CompositionDriftBeta != 0 {
		fs := opt.CompositionFlatShare
		if fs < 0 {
			fs = 0
		}
		if fs > 1 {
			fs = 1
		}
		d += opt.CompositionDriftBeta * (fs - 0.5)
	}
	return d
}
