package spine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEarningsAnnualAlternateHeaders(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "earn.csv")
	content := "geography_code,year,obs_value\nE09000030,2004,35000\nE09000030,2005,36000\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadEarningsAnnual(p)
	if err != nil {
		t.Fatal(err)
	}
	if m["E09000030"][2004] != "35000" || m["E09000030"][2005] != "36000" {
		t.Fatalf("got %+v", m)
	}
}
