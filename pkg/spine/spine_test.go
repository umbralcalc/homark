package spine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBOEMonthlyMeans(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "boe.csv")
	if err := os.WriteFile(path, []byte("DATE,IUDBEDR\n02 Jan 1975,11.5\n03 Jan 1975,12.0\n01 Feb 1975,10.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := BOEMonthlyMeans(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 {
		t.Fatalf("expected 2 months, got %d", len(m))
	}
	// Jan: (11.5+12)/2 = 11.75
	if v := m[MonthKey("1975-01")]; v < 11.74 || v > 11.76 {
		t.Fatalf("Jan mean: got %v", v)
	}
	if v := m[MonthKey("1975-02")]; v != 10.0 {
		t.Fatalf("Feb mean: got %v", v)
	}
}

func TestParseHPIDate(t *testing.T) {
	d, ok := parseHPIDate("01/01/2004")
	if !ok {
		t.Fatal("expected ok")
	}
	if d.Year() != 2004 || d.Month() != 1 || d.Day() != 1 {
		t.Fatalf("got %v", d)
	}
}

func TestBuildSpine(t *testing.T) {
	dir := t.TempDir()
	uk := filepath.Join(dir, "ukhpi.csv")
	body := "Date,RegionName,AreaCode,AveragePrice,Index\n" +
		"01/01/2004,TestLA,E09000030,100000,50.0\n" +
		"01/02/2004,TestLA,E09000030,101000,51.0\n" +
		"01/01/2004,Other,E99999999,1,1\n"
	if err := os.WriteFile(uk, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	boe := filepath.Join(dir, "boe.csv")
	if err := os.WriteFile(boe, []byte("DATE,IUDBEDR\n01 Jan 2004,5.0\n15 Jan 2004,7.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bank, err := BOEMonthlyMeans(boe)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]struct{}{"E09000030": {}}
	out := filepath.Join(dir, "out.csv")
	n, err := BuildSpine(uk, codes, bank, &SpineEnrichment{}, out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("rows=%d", n)
	}
	raw, _ := os.ReadFile(out)
	if len(raw) == 0 {
		t.Fatal("empty out")
	}
}
