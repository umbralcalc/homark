package housing

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/umbralcalc/homark/pkg/spine"
)

func TestCloneMonthlyObservations_ScaleBankRatePct(t *testing.T) {
	obs := []spine.MonthlyObservation{{BankRatePct: 2}, {BankRatePct: 4}}
	cp := CloneMonthlyObservations(obs)
	ScaleBankRatePct(cp, 0.5)
	if cp[0].BankRatePct != 1 || cp[1].BankRatePct != 2 {
		t.Fatalf("scaled %+v", cp)
	}
	if obs[0].BankRatePct != 2 {
		t.Fatal("original mutated")
	}
}

func TestAffordabilityPathStats(t *testing.T) {
	aff := [][]float64{{5}, {7}, {6}}
	m, last, n := AffordabilityPathStats(aff)
	if n != 3 || math.Abs(m-6) > 1e-9 || last != 6 {
		t.Fatalf("mean=%g last=%g n=%d", m, last, n)
	}
}

func TestCholeskyLower_identity(t *testing.T) {
	n := ThetaDim
	a := make([]float64, n*n)
	for i := 0; i < n; i++ {
		a[i*n+i] = 1
	}
	L, ok := choleskyLower(a, n)
	if !ok {
		t.Fatal("cholesky failed")
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			want := 0.0
			if i == j {
				want = 1
			}
			got := L[i*n+j]
			if i < j && got != 0 {
				t.Fatalf("upper L[%d,%d]=%g", i, j, got)
			}
			if i >= j && math.Abs(got-want) > 1e-9 {
				t.Fatalf("L[%d,%d]=%g want %g", i, j, got, want)
			}
		}
	}
}

func TestWritePosteriorCalibrationJSON_createsParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "leeds.json")
	mean := make([]float64, ThetaDim)
	cov := make([]float64, ThetaDim*ThetaDim)
	if err := WritePosteriorCalibrationJSON(path, ESResult{ThetaMean: mean, ThetaCov: cov}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestPosteriorCalibrationJSON_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post.json")
	mean := []float64{0.01, 0.0008, 0, 0, 0.15, 0.0005}
	cov := make([]float64, ThetaDim*ThetaDim)
	for i := 0; i < ThetaDim; i++ {
		cov[i*ThetaDim+i] = 1e-6
	}
	res := ESResult{ThetaMean: mean, ThetaCov: cov}
	if err := WritePosteriorCalibrationJSON(path, res); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPosteriorCalibrationJSON(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ThetaMean) != ThetaDim || len(got.ThetaCov) != ThetaDim*ThetaDim {
		t.Fatalf("read %+v", got)
	}
}

func TestSampleThetaGaussian_matchesMeanWithZeroCov(t *testing.T) {
	mean := ThetaFromOptions(DefaultForwardOptions())
	rng := rand.New(rand.NewSource(99))
	got := SampleThetaGaussian(mean, nil, rng)
	for i := range mean {
		if math.Abs(got[i]-mean[i]) > 1e-12 {
			t.Fatalf("idx %d got %g mean %g", i, got[i], mean[i])
		}
	}
}

func TestSampleThetaGaussian_diagonalPosterior(t *testing.T) {
	mean := make([]float64, ThetaDim)
	cov := make([]float64, ThetaDim*ThetaDim)
	for i := 0; i < ThetaDim; i++ {
		cov[i*ThetaDim+i] = 0.01 * 0.01
	}
	rng := rand.New(rand.NewSource(42))
	s0 := SampleThetaGaussian(mean, cov, rng)
	s1 := SampleThetaGaussian(mean, cov, rand.New(rand.NewSource(43)))
	var diff float64
	for i := range s0 {
		diff += math.Abs(s0[i] - s1[i])
	}
	if diff < 1e-6 {
		t.Fatal("expected different samples")
	}
}

func TestWritePosteriorCalibrationJSON_badLen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	err := WritePosteriorCalibrationJSON(path, ESResult{ThetaMean: []float64{1, 2}, ThetaCov: make([]float64, 36)})
	if err == nil {
		t.Fatal("want error")
	}
	_ = os.Remove(path)
}
