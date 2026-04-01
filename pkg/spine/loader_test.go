package spine

import (
	"math"
	"path/filepath"
	"testing"
)

func TestLoadSpineMonthlyForArea(t *testing.T) {
	path := filepath.Join("testdata", "spine_replay_sample.csv")
	obs, err := LoadSpineMonthlyForArea(path, "E09000030")
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 3 {
		t.Fatalf("rows=%d", len(obs))
	}
	if obs[0].YearMonth != "2004-01" {
		t.Fatalf("first ym %q", obs[0].YearMonth)
	}
	if math.Abs(obs[0].AveragePrice-100000) > 1 {
		t.Fatalf("price %v", obs[0].AveragePrice)
	}
	if math.Abs(obs[0].MedianRatio-8.5) > 1e-9 {
		t.Fatalf("ratio %v", obs[0].MedianRatio)
	}
	if obs[0].EarningsAnnual != 0 {
		t.Fatalf("earnings should be empty in fixture")
	}
	if math.Abs(obs[0].NetAddFY-1200) > 1e-9 {
		t.Fatalf("NetAddFY %v", obs[0].NetAddFY)
	}
}
