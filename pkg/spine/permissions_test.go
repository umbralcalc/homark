package spine

import (
	"math"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLoadPermissionsAnnual(t *testing.T) {
	path := filepath.Join("testdata", "enrichment", "permissions_annual_pilot_template.csv")
	data, err := LoadPermissionsAnnual(path)
	if err != nil {
		t.Fatal(err)
	}
	// Template covers all five pilot LAs.
	for _, ac := range []string{"E09000030", "E07000240", "E08000035", "E06000043", "E07000117"} {
		if _, ok := data[ac]; !ok {
			t.Errorf("missing area %s", ac)
		}
	}
	// Spot-check: Tower Hamlets 2017 = 3100 permissions.
	th := data["E09000030"]
	if th == nil {
		t.Fatal("no Tower Hamlets data")
	}
	if th[2017] != "3100" {
		t.Errorf("Tower Hamlets 2017: got %q want %q", th[2017], "3100")
	}
}

func TestPermissionsMonthlyDivision(t *testing.T) {
	// Verify that annual/12 gives the expected monthly approximation.
	data, err := LoadPermissionsAnnual(filepath.Join("testdata", "enrichment", "permissions_annual_pilot_template.csv"))
	if err != nil {
		t.Fatal(err)
	}
	th := data["E09000030"]
	s, ok := th[2017]
	if !ok {
		t.Fatal("Tower Hamlets 2017 missing")
	}
	annual, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatal(err)
	}
	monthly := annual / 12.0
	want := 3100.0 / 12.0
	if math.Abs(monthly-want) > 1e-9 {
		t.Errorf("monthly=%g want %g", monthly, want)
	}
}
