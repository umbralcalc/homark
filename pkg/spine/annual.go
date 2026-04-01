package spine

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// AnnualString maps area_code -> calendar year -> value string (for earnings, ratios, etc.).
type AnnualString map[string]map[int]string

// LoadEarningsAnnual reads an annual-by-LA pay CSV. Headers are case-insensitive; recognised aliases include
// area_code / geography_code / lad_code; year / calendar_year; median_gross_annual_pay / obs_value / value.
func LoadEarningsAnnual(path string) (AnnualString, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	hdr, err := r.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range hdr {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	acI, err := resolveAreaCodeColumn(col)
	if err != nil {
		return nil, fmt.Errorf("earnings csv: %w", err)
	}
	yI, ok2 := headerIndex(col, []string{"year", "calendar_year", "time"})
	if !ok2 {
		return nil, fmt.Errorf("earnings csv: need column year (or calendar_year, time)")
	}
	payI, ok3 := headerIndex(col, []string{
		"median_gross_annual_pay", "median_annual_pay", "gross_annual_pay", "pay", "value", "obs_value",
	})
	if !ok3 {
		return nil, fmt.Errorf("earnings csv: need median_gross_annual_pay (or median_annual_pay, obs_value, …)")
	}
	out := make(AnnualString)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) <= acI || len(rec) <= yI || len(rec) <= payI {
			continue
		}
		ac := strings.TrimSpace(rec[acI])
		yr, err := strconv.Atoi(strings.TrimSpace(rec[yI]))
		if err != nil {
			continue
		}
		pay := strings.TrimSpace(rec[payI])
		if out[ac] == nil {
			out[ac] = make(map[int]string)
		}
		out[ac][yr] = pay
	}
	return out, nil
}
