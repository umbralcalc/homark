package spine

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// LoadPermissionsAnnual reads an annual-by-LA planning permissions CSV.
// Headers are case-insensitive; recognised aliases:
//   - area column: area_code, geography_code, lad_code, gss_code, ons_code, …
//   - year column: year, calendar_year, time
//   - value column: permissions_granted, planning_permissions, units_permitted, permissions, value, obs_value
//
// Returns AnnualString keyed by area code and calendar year. Values are the raw
// string from the CSV; BuildSpine divides by 12 to get permissions_approx_monthly.
func LoadPermissionsAnnual(path string) (AnnualString, error) {
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
		return nil, fmt.Errorf("permissions csv: %w", err)
	}
	yI, ok2 := headerIndex(col, []string{"year", "calendar_year", "time"})
	if !ok2 {
		return nil, fmt.Errorf("permissions csv: need column year (or calendar_year, time)")
	}
	pI, ok3 := headerIndex(col, []string{
		"permissions_granted", "planning_permissions", "units_permitted",
		"permissions", "value", "obs_value",
	})
	if !ok3 {
		return nil, fmt.Errorf("permissions csv: need permissions_granted (or planning_permissions, units_permitted, …)")
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
		if len(rec) <= acI || len(rec) <= yI || len(rec) <= pI {
			continue
		}
		ac := strings.TrimSpace(rec[acI])
		yr, err := strconv.Atoi(strings.TrimSpace(rec[yI]))
		if err != nil {
			continue
		}
		val := strings.TrimSpace(rec[pI])
		if out[ac] == nil {
			out[ac] = make(map[int]string)
		}
		out[ac][yr] = val
	}
	return out, nil
}
