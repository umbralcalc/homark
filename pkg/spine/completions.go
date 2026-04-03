package spine

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// LoadCompletionsAnnual reads an annual-by-LA dwelling completions CSV (MHCLG/DLUHC-style).
// Headers are case-insensitive; area and year columns match LoadPermissionsAnnual;
// value column aliases: dwelling_completions, completions, housing_completions, value, obs_value.
func LoadCompletionsAnnual(path string) (AnnualString, error) {
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
		return nil, fmt.Errorf("completions csv: %w", err)
	}
	yI, ok2 := headerIndex(col, []string{"year", "calendar_year", "time"})
	if !ok2 {
		return nil, fmt.Errorf("completions csv: need column year (or calendar_year, time)")
	}
	vI, ok3 := headerIndex(col, []string{
		"dwelling_completions", "completions", "housing_completions",
		"completions_dwellings", "value", "obs_value",
	})
	if !ok3 {
		return nil, fmt.Errorf("completions csv: need dwelling_completions (or completions, housing_completions, value, …)")
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
		if len(rec) <= acI || len(rec) <= yI || len(rec) <= vI {
			continue
		}
		ac := strings.TrimSpace(rec[acI])
		yr, err := strconv.Atoi(strings.TrimSpace(rec[yI]))
		if err != nil {
			continue
		}
		val := strings.TrimSpace(rec[vI])
		if out[ac] == nil {
			out[ac] = make(map[int]string)
		}
		out[ac][yr] = val
	}
	return out, nil
}
