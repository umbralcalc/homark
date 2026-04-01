package spine

import (
	"strings"
	"testing"
)

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
	if p.TotalRows != 2 || p.WithPay != 1 {
		t.Fatalf("got %+v", p)
	}
	if p.Fraction() != 0.5 {
		t.Fatalf("fraction %g", p.Fraction())
	}
}
