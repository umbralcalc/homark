package spine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/homark/pkg/ladata"
)

func TestSpinePilotEnrichmentFixture_meetsNinetyFivePctGate(t *testing.T) {
	targets, err := ladata.LoadTargets()
	if err != nil {
		t.Fatal(err)
	}
	codes := make([]string, len(targets))
	for i := range targets {
		codes[i] = targets[i].AreaCode
	}
	path := filepath.Join("testdata", "spine_pilot_enrichment_fixture.csv")
	pay, ratio, err := PilotSpineCoverage(path, codes)
	if err != nil {
		t.Fatal(err)
	}
	const min = 0.95
	for _, a := range targets {
		p := pay[a.AreaCode]
		r := ratio[a.AreaCode]
		if p.Fraction()+1e-9 < min {
			t.Fatalf("%s (%s) pay coverage %g below %g", a.Name, a.AreaCode, p.Fraction(), min)
		}
		if r.Fraction()+1e-9 < min {
			t.Fatalf("%s (%s) ratio coverage %g below %g", a.Name, a.AreaCode, r.Fraction(), min)
		}
	}
}

func TestPilotSpineCoverageFromReader_noEnrichmentColumns(t *testing.T) {
	csv := "Date,RegionName,AreaCode,AveragePrice,Index,year_month,bank_rate_pct\n" +
		"2004-01-01,X,E09000030,1,1,2004-01,1\n" +
		"2004-02-01,X,E09000030,1,1,2004-02,1\n"
	pay, ratio, err := pilotSpineCoverageFromReader(strings.NewReader(csv), map[string]struct{}{"E09000030": {}})
	if err != nil {
		t.Fatal(err)
	}
	p := pay["E09000030"]
	r := ratio["E09000030"]
	if p.TotalRows != 2 || p.NonZero != 0 || r.TotalRows != 2 || r.NonZero != 0 {
		t.Fatalf("pay %+v ratio %+v", p, r)
	}
}

func TestPilotSpineCoverageFromReader(t *testing.T) {
	csv := "Date,RegionName,AreaCode,AveragePrice,Index,year_month,bank_rate_pct,median_ratio,net_additional_dwellings_fy,median_gross_annual_pay,ppd_median_price,ppd_sales_count\n" +
		"2004-01-01,X,E09000030,1,1,2004-01,1,8.5,,30000,,\n" +
		"2004-02-01,X,E09000030,1,1,2004-02,1,,,,\n"
	pay, ratio, err := pilotSpineCoverageFromReader(strings.NewReader(csv), map[string]struct{}{"E09000030": {}})
	if err != nil {
		t.Fatal(err)
	}
	p := pay["E09000030"]
	r := ratio["E09000030"]
	if p.TotalRows != 2 || p.NonZero != 1 || r.TotalRows != 2 || r.NonZero != 1 {
		t.Fatalf("pay %+v ratio %+v", p, r)
	}
}

func TestMedianPayCoverageByArea(t *testing.T) {
	csv := "Date,RegionName,AreaCode,AveragePrice,Index,year_month,bank_rate_pct,median_ratio,net_additional_dwellings_fy,median_gross_annual_pay,ppd_median_price,ppd_sales_count\n" +
		"2004-01-01,X,E09000030,1,1,2004-01,1,,,30000,,\n" +
		"2004-02-01,X,E09000030,1,1,2004-02,1,,,,,\n" +
		"2004-01-01,Y,E08000001,1,1,2004-01,1,,,25000,,\n"
	m, err := medianPayCoverageFromReader(strings.NewReader(csv), map[string]struct{}{
		"E09000030": {},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := m["E09000030"]
	if p.TotalRows != 2 || p.NonZero != 1 {
		t.Fatalf("got %+v", p)
	}
	if p.Fraction() != 0.5 {
		t.Fatalf("fraction %g", p.Fraction())
	}
}
