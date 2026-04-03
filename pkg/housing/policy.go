package housing

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/umbralcalc/homark/pkg/spine"
)

// CloneMonthlyObservations returns a shallow copy of the slice (each element is a value copy).
func CloneMonthlyObservations(obs []spine.MonthlyObservation) []spine.MonthlyObservation {
	out := make([]spine.MonthlyObservation, len(obs))
	copy(out, obs)
	return out
}

// ScaleBankRatePct multiplies BankRatePct on every row by factor (in-place).
func ScaleBankRatePct(obs []spine.MonthlyObservation, factor float64) {
	for i := range obs {
		obs[i].BankRatePct *= factor
	}
}

// AffordabilityPathStats summarises the affordability partition from RunForwardLogSeries.
func AffordabilityPathStats(aff [][]float64) (mean, last float64, n int) {
	if len(aff) == 0 {
		return 0, 0, 0
	}
	n = len(aff)
	var s float64
	for _, row := range aff {
		if len(row) > 0 {
			s += row[0]
		}
	}
	last = aff[len(aff)-1][0]
	mean = s / float64(n)
	return mean, last, n
}

// PosteriorCalibrationJSON is written by calibratespine (-es-json-out) and read by policyscenario (-posterior).
type PosteriorCalibrationJSON struct {
	ThetaMean []float64 `json:"theta_mean"`
	ThetaCov  []float64 `json:"theta_cov"`
}

// WritePosteriorCalibrationJSON writes ES theta mean and covariance for downstream policy runs.
func WritePosteriorCalibrationJSON(path string, res ESResult) error {
	if len(res.ThetaMean) != ThetaDim {
		return fmt.Errorf("write posterior: theta_mean len %d want %d", len(res.ThetaMean), ThetaDim)
	}
	if len(res.ThetaCov) != ThetaDim*ThetaDim {
		return fmt.Errorf("write posterior: theta_cov len %d want %d", len(res.ThetaCov), ThetaDim*ThetaDim)
	}
	b, err := json.MarshalIndent(PosteriorCalibrationJSON{
		ThetaMean: append([]float64(nil), res.ThetaMean...),
		ThetaCov:  append([]float64(nil), res.ThetaCov...),
	}, "", "  ")
	if err != nil {
		return err
	}
	if d := filepath.Dir(path); d != "." && d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir for posterior json: %w", err)
		}
	}
	return os.WriteFile(path, b, 0o644)
}

// ReadPosteriorCalibrationJSON loads theta mean and covariance from a JSON file.
func ReadPosteriorCalibrationJSON(path string) (PosteriorCalibrationJSON, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return PosteriorCalibrationJSON{}, err
	}
	var p PosteriorCalibrationJSON
	if err := json.Unmarshal(b, &p); err != nil {
		return PosteriorCalibrationJSON{}, err
	}
	if len(p.ThetaMean) != ThetaDim {
		return PosteriorCalibrationJSON{}, fmt.Errorf("posterior: theta_mean len %d want %d", len(p.ThetaMean), ThetaDim)
	}
	if len(p.ThetaCov) != 0 && len(p.ThetaCov) != ThetaDim*ThetaDim {
		return PosteriorCalibrationJSON{}, fmt.Errorf("posterior: theta_cov len %d want 0 or %d", len(p.ThetaCov), ThetaDim*ThetaDim)
	}
	return p, nil
}

// SampleThetaGaussian draws theta ~ N(mean, Sigma) with Sigma row-major; uses Cholesky when possible,
// otherwise falls back to independent normals with marginal std dev sqrt(diag(Sigma)).
func SampleThetaGaussian(mean []float64, cov []float64, rng *rand.Rand) []float64 {
	if len(mean) != ThetaDim {
		return append([]float64(nil), mean...)
	}
	if len(cov) != ThetaDim*ThetaDim {
		return append([]float64(nil), mean...)
	}
	L, ok := choleskyLower(cov, ThetaDim)
	if !ok {
		out := make([]float64, ThetaDim)
		for i := 0; i < ThetaDim; i++ {
			v := cov[i*ThetaDim+i]
			s := math.Sqrt(math.Max(v, 1e-18))
			out[i] = mean[i] + s*rng.NormFloat64()
		}
		return out
	}
	z := make([]float64, ThetaDim)
	for i := range z {
		z[i] = rng.NormFloat64()
	}
	// y = L * z (lower triangular)
	y := make([]float64, ThetaDim)
	for i := 0; i < ThetaDim; i++ {
		for j := 0; j <= i; j++ {
			y[i] += L[i*ThetaDim+j] * z[j]
		}
	}
	for i := 0; i < ThetaDim; i++ {
		y[i] += mean[i]
	}
	return y
}

// choleskyLower returns L with Sigma = L L^T (row-major lower triangle stored).
func choleskyLower(a []float64, n int) ([]float64, bool) {
	L := make([]float64, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sum := a[i*n+j]
			if i != j {
				for k := 0; k < j; k++ {
					sum -= L[i*n+k] * L[j*n+k]
				}
				d := L[j*n+j]
				if d == 0 {
					return nil, false
				}
				L[i*n+j] = sum / d
			} else {
				for k := 0; k < i; k++ {
					sum -= L[i*n+k] * L[i*n+k]
				}
				if sum <= 0 {
					return nil, false
				}
				L[i*n+j] = math.Sqrt(sum)
			}
		}
	}
	return L, true
}
