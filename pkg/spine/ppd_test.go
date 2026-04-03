package spine

import (
	"path/filepath"
	"testing"
)

func TestAggregatePricePaid(t *testing.T) {
	dir := filepath.Join("testdata")
	ppd := filepath.Join(dir, "minimal_ppd.csv")
	nspl := filepath.Join(dir, "minimal_nspl.csv")
	codes := map[string]struct{}{
		"E09000030": {},
		"E08000035": {},
	}
	b, err := AggregatePricePaid(ppd, nspl, codes)
	if err != nil {
		t.Fatal(err)
	}
	agg := PPDBucketsToAgg(b)
	if agg["E09000030"][MonthKey("2004-01")].MedianPrice == "" {
		t.Fatalf("expected Tower Hamlets Jan median, got %+v", agg["E09000030"])
	}
	if agg["E08000035"][MonthKey("2004-02")].SalesCount != "1" {
		t.Fatalf("Leeds count: %+v", agg["E08000035"])
	}
}

func TestAggregatePricePaidByType_detachedFlat(t *testing.T) {
	dir := filepath.Join("testdata")
	ppd := filepath.Join(dir, "minimal_ppd_typed.csv")
	nspl := filepath.Join(dir, "minimal_nspl.csv")
	codes := map[string]struct{}{"E09000030": {}, "E08000035": {}}
	full, err := AggregatePricePaidByType(ppd, nspl, codes)
	if err != nil {
		t.Fatal(err)
	}
	typed := PPDBucketsByTypeToAgg(full)
	th := typed["E09000030"][MonthKey("2004-01")]
	if th.All.MedianPrice == "" || th.Detached.MedianPrice == "" || th.Flat.MedianPrice == "" {
		t.Fatalf("expected All+Detached+Flat medians, got %+v", th)
	}
	if th.Detached.SalesCount != "2" || th.Flat.SalesCount != "1" {
		t.Fatalf("type counts %+v", th)
	}
}
