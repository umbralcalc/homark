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

// LoadEarningsAnnual reads CSV with columns area_code, year, median_gross_annual_pay (headers case-insensitive).
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
	acI, ok1 := col["area_code"]
	yI, ok2 := col["year"]
	payI, ok3 := col["median_gross_annual_pay"]
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("earnings csv: need columns area_code, year, median_gross_annual_pay")
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
