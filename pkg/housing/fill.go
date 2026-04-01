package housing

import "github.com/umbralcalc/homark/pkg/spine"

// ForwardFillAffordableFields returns a copy of obs with median_ratio and median_gross_annual_pay
// filled along the time axis (backward from first observed month, then forward carry) so MonthlyLogSeries
// can be built for calibration targets on long UK HPI histories.
func ForwardFillAffordableFields(obs []spine.MonthlyObservation) []spine.MonthlyObservation {
	if len(obs) == 0 {
		return nil
	}
	out := make([]spine.MonthlyObservation, len(obs))
	copy(out, obs)

	first := -1
	for i := range out {
		if out[i].MedianRatio > 0 || out[i].EarningsAnnual > 0 {
			first = i
			break
		}
	}
	if first > 0 {
		for i := 0; i < first; i++ {
			if out[i].MedianRatio == 0 && out[first].MedianRatio > 0 {
				out[i].MedianRatio = out[first].MedianRatio
			}
			if out[i].EarningsAnnual == 0 && out[first].EarningsAnnual > 0 {
				out[i].EarningsAnnual = out[first].EarningsAnnual
			}
		}
	}
	var lastR, lastE float64
	for i := range out {
		if out[i].MedianRatio > 0 {
			lastR = out[i].MedianRatio
		} else if lastR > 0 {
			out[i].MedianRatio = lastR
		}
		if out[i].EarningsAnnual > 0 {
			lastE = out[i].EarningsAnnual
		} else if lastE > 0 {
			out[i].EarningsAnnual = lastE
		}
	}
	return out
}
