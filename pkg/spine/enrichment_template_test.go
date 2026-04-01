package spine

import (
	"path/filepath"
	"testing"
)

// Pilot template CSVs under testdata/enrichment are illustrative (not official ONS statistics):
// copy to dat/raw/earnings_annual.csv and dat/raw/ons_affordability.csv to bootstrap local builds,
// then replace with real exports from NOMIS / ONS.
func TestPilotEnrichmentTemplatesLoad(t *testing.T) {
	earn := filepath.Join("testdata", "enrichment", "earnings_annual_pilot_template.csv")
	m, err := LoadEarningsAnnual(earn)
	if err != nil {
		t.Fatal(err)
	}
	if m["E09000030"][2020] == "" {
		t.Fatal("expected Tower Hamlets 2020 pay")
	}
	ons := filepath.Join("testdata", "enrichment", "ons_affordability_pilot_template.csv")
	o, err := LoadONSAnnual(ons)
	if err != nil {
		t.Fatal(err)
	}
	if o["E08000035"][2015] == "" {
		t.Fatal("expected Leeds 2015 ratio")
	}
}
