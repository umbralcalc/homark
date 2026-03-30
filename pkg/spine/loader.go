package spine

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MonthlyObservation is one row of spine_monthly.csv for replay / calibration.
type MonthlyObservation struct {
	Date           time.Time
	YearMonth      string
	AreaCode       string
	RegionName     string
	AveragePrice   float64 // 0 = missing
	Index          float64 // 0 = missing
	BankRatePct    float64
	MedianRatio    float64 // 0 = missing (ONS affordability)
	EarningsAnnual float64 // 0 = missing (ASHE-style gross pay)
}

func parseFloatOK(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// LoadSpineMonthlyForArea reads spine_monthly.csv (BuildSpine output) and returns rows for areaCode, oldest first.
func LoadSpineMonthlyForArea(path, areaCode string) ([]MonthlyObservation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseSpineMonthlyCSV(f, areaCode)
}

func parseSpineMonthlyCSV(r io.Reader, wantArea string) ([]MonthlyObservation, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	hdr, err := cr.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range hdr {
		col[strings.TrimSpace(h)] = i
	}
	req := []string{"Date", "AreaCode"}
	for _, k := range req {
		if _, ok := col[k]; !ok {
			return nil, fmt.Errorf("spine csv: missing column %q", k)
		}
	}
	get := func(rec []string, name string) string {
		j := col[name]
		if j < 0 || j >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[j])
	}
	var out []MonthlyObservation
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		ac := get(rec, "AreaCode")
		if ac != wantArea {
			continue
		}
		ds := get(rec, "Date")
		dt, ok := parseHPIDate(ds)
		if !ok {
			if t2, err := time.Parse("2006-01-02", ds); err == nil {
				dt = t2.UTC()
				ok = true
			}
		}
		if !ok {
			continue
		}
		o := MonthlyObservation{
			Date:       dt,
			YearMonth:  get(rec, "year_month"),
			AreaCode:   ac,
			RegionName: get(rec, "RegionName"),
		}
		if v, ok := parseFloatOK(get(rec, "AveragePrice")); ok {
			o.AveragePrice = v
		}
		if v, ok := parseFloatOK(get(rec, "Index")); ok {
			o.Index = v
		}
		if v, ok := parseFloatOK(get(rec, "bank_rate_pct")); ok {
			o.BankRatePct = v
		}
		if v, ok := parseFloatOK(get(rec, "median_ratio")); ok {
			o.MedianRatio = v
		}
		if v, ok := parseFloatOK(get(rec, "median_gross_annual_pay")); ok {
			o.EarningsAnnual = v
		}
		if o.YearMonth == "" {
			o.YearMonth = string(monthKeyFromTime(dt))
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}
