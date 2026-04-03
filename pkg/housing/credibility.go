package housing

import (
	"fmt"
	"math"

	"github.com/umbralcalc/homark/pkg/spine"
)

// PearsonCorrelation returns the Pearson r for paired (x[i],y[i]) where both are finite; requires variance in both.
func PearsonCorrelation(x, y []float64) (r float64, n int, err error) {
	if len(x) != len(y) {
		return 0, 0, fmt.Errorf("credibility: length mismatch %d vs %d", len(x), len(y))
	}
	var xs, ys []float64
	for i := range x {
		if !math.IsNaN(x[i]) && !math.IsNaN(y[i]) && math.IsInf(x[i], 0) == false && math.IsInf(y[i], 0) == false {
			xs = append(xs, x[i])
			ys = append(ys, y[i])
		}
	}
	n = len(xs)
	if n < 3 {
		return 0, n, fmt.Errorf("credibility: need at least 3 pairs, got %d", n)
	}
	mx, my := mean(xs), mean(ys)
	var sxx, syy, sxy float64
	for i := range xs {
		dx := xs[i] - mx
		dy := ys[i] - my
		sxx += dx * dx
		syy += dy * dy
		sxy += dx * dy
	}
	if sxx < 1e-18 || syy < 1e-18 {
		return 0, n, fmt.Errorf("credibility: zero variance in x or y")
	}
	return sxy / math.Sqrt(sxx*syy), n, nil
}

func mean(a []float64) float64 {
	var s float64
	for _, v := range a {
		s += v
	}
	return s / float64(len(a))
}

// LogPPDVsHPICorrelation correlates log(PPD all-type median) with log(UK HPI AveragePrice) where both > 0.
func LogPPDVsHPICorrelation(obs []spine.MonthlyObservation) (r float64, n int, err error) {
	x := make([]float64, len(obs))
	y := make([]float64, len(obs))
	for i, o := range obs {
		if o.PPDMedianPrice > 0 && o.AveragePrice > 0 {
			x[i] = math.Log(o.PPDMedianPrice)
			y[i] = math.Log(o.AveragePrice)
		} else {
			x[i] = math.NaN()
			y[i] = math.NaN()
		}
	}
	r, n, err = PearsonCorrelation(x, y)
	return r, n, err
}

// LogPPDTypeVsHPICorrelation correlates log(PPD median for property type D|S|T|F) with log(AveragePrice).
func LogPPDTypeVsHPICorrelation(obs []spine.MonthlyObservation, propertyType rune) (r float64, n int, err error) {
	var med func(spine.MonthlyObservation) float64
	switch propertyType {
	case 'D', 'd':
		med = func(o spine.MonthlyObservation) float64 { return o.PPDMedianDetached }
	case 'S', 's':
		med = func(o spine.MonthlyObservation) float64 { return o.PPDMedianSemi }
	case 'T', 't':
		med = func(o spine.MonthlyObservation) float64 { return o.PPDMedianTerraced }
	case 'F', 'f':
		med = func(o spine.MonthlyObservation) float64 { return o.PPDMedianFlat }
	default:
		return 0, 0, fmt.Errorf("credibility: propertyType must be D, S, T, or F")
	}
	x := make([]float64, len(obs))
	y := make([]float64, len(obs))
	for i, o := range obs {
		p := med(o)
		if p > 0 && o.AveragePrice > 0 {
			x[i] = math.Log(p)
			y[i] = math.Log(o.AveragePrice)
		} else {
			x[i] = math.NaN()
			y[i] = math.NaN()
		}
	}
	return PearsonCorrelation(x, y)
}

// CredibilitySpineSummary counts how many rows have the fields used by credibility diagnostics
// (helps interpret "n=0" or failed lag scans when enrichments were not merged into the spine).
type CredibilitySpineSummary struct {
	Rows                       int
	AveragePricePositive       int
	PPDAllPositive             int
	PPDAnyTypeMedianPositive   int // any of D/S/T/F median > 0
	PermissionsPositive        int
	CompletionsPositive        int
	SameMonthPermAndCompletion int // both permissions and completions > 0 on the same row
}

// SummarizeCredibilitySpine scans obs for nonnegative spine enrichments relevant to credibilityreport.
func SummarizeCredibilitySpine(obs []spine.MonthlyObservation) CredibilitySpineSummary {
	var s CredibilitySpineSummary
	s.Rows = len(obs)
	for _, o := range obs {
		if o.AveragePrice > 0 {
			s.AveragePricePositive++
		}
		if o.PPDMedianPrice > 0 {
			s.PPDAllPositive++
		}
		if o.PPDMedianDetached > 0 || o.PPDMedianSemi > 0 || o.PPDMedianTerraced > 0 || o.PPDMedianFlat > 0 {
			s.PPDAnyTypeMedianPositive++
		}
		if o.PermissionsMonthly > 0 {
			s.PermissionsPositive++
		}
		if o.CompletionsMonthly > 0 {
			s.CompletionsPositive++
		}
		if o.PermissionsMonthly > 0 && o.CompletionsMonthly > 0 {
			s.SameMonthPermAndCompletion++
		}
	}
	return s
}

// PermissionCompletionLagScan correlates permissions_approx_monthly[t] with completions_approx_monthly[t+lag]
// for lag in [0, maxLag]. Returns the lag with highest |r| and that r (may be negative).
func PermissionCompletionLagScan(obs []spine.MonthlyObservation, maxLag int) (bestLag int, bestR float64, pairs int, err error) {
	if maxLag < 0 {
		return 0, 0, 0, fmt.Errorf("credibility: maxLag must be >= 0")
	}
	p := make([]float64, len(obs))
	c := make([]float64, len(obs))
	for i, o := range obs {
		p[i] = o.PermissionsMonthly
		c[i] = o.CompletionsMonthly
	}
	bestR = 0.0
	bestLag = 0
	bestN := 0
	for lag := 0; lag <= maxLag; lag++ {
		var xs, ys []float64
		for t := 0; t+lag < len(obs); t++ {
			if p[t] > 0 && c[t+lag] > 0 {
				xs = append(xs, math.Log(p[t]))
				ys = append(ys, math.Log(c[t+lag]))
			}
		}
		if len(xs) < 3 {
			continue
		}
		r, _, e := PearsonCorrelation(xs, ys)
		if e != nil {
			continue
		}
		if math.Abs(r) > math.Abs(bestR) || (bestN == 0) {
			bestR = r
			bestLag = lag
			bestN = len(xs)
		}
	}
	if bestN == 0 {
		return 0, 0, 0, fmt.Errorf("credibility: insufficient overlapping permissions/completions for lag scan")
	}
	return bestLag, bestR, bestN, nil
}
