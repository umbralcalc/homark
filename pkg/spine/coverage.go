package spine

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// PayCoverage holds row counts for median_gross_annual_pay on a built spine CSV.
type PayCoverage struct {
	TotalRows int
	WithPay   int
}

// Fraction returns WithPay/TotalRows, or 0 if TotalRows==0.
func (p PayCoverage) Fraction() float64 {
	if p.TotalRows == 0 {
		return 0
	}
	return float64(p.WithPay) / float64(p.TotalRows)
}

// MedianPayCoverageByArea scans spine_monthly.csv and, for each code in areaCodes,
// counts rows and rows with parseable median_gross_annual_pay > 0.
func MedianPayCoverageByArea(path string, areaCodes []string) (map[string]PayCoverage, error) {
	want := make(map[string]struct{}, len(areaCodes))
	for _, c := range areaCodes {
		want[strings.TrimSpace(c)] = struct{}{}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return medianPayCoverageFromReader(f, want)
}

func medianPayCoverageFromReader(r io.Reader, want map[string]struct{}) (map[string]PayCoverage, error) {
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
	acI, ok := col["AreaCode"]
	if !ok {
		return nil, fmt.Errorf("spine csv: missing AreaCode column")
	}
	payI, ok := col["median_gross_annual_pay"]
	if !ok {
		return nil, fmt.Errorf("spine csv: missing median_gross_annual_pay column")
	}
	out := make(map[string]PayCoverage)
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) <= acI {
			continue
		}
		ac := strings.TrimSpace(rec[acI])
		if _, ok := want[ac]; !ok {
			continue
		}
		pc := out[ac]
		pc.TotalRows++
		if len(rec) > payI {
			s := strings.TrimSpace(rec[payI])
			if s != "" {
				if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
					pc.WithPay++
				}
			}
		}
		out[ac] = pc
	}
	return out, nil
}
