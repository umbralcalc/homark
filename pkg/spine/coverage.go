package spine

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// FieldCoverage holds row counts for a numeric spine column (e.g. pay or median_ratio).
type FieldCoverage struct {
	TotalRows int
	NonZero   int
}

// Fraction returns NonZero/TotalRows, or 0 if TotalRows==0.
func (f FieldCoverage) Fraction() float64 {
	if f.TotalRows == 0 {
		return 0
	}
	return float64(f.NonZero) / float64(f.TotalRows)
}

// MedianPayCoverageByArea scans spine_monthly.csv and, for each code in areaCodes,
// counts rows and rows with parseable median_gross_annual_pay > 0.
func MedianPayCoverageByArea(path string, areaCodes []string) (map[string]FieldCoverage, error) {
	return numericColumnCoverageByArea(path, areaCodes, "median_gross_annual_pay")
}

// MedianRatioCoverageByArea counts rows with parseable median_ratio > 0.
func MedianRatioCoverageByArea(path string, areaCodes []string) (map[string]FieldCoverage, error) {
	return numericColumnCoverageByArea(path, areaCodes, "median_ratio")
}

// EnrichmentColumnsPresent returns whether spine_monthly.csv headers include the optional enrichment columns.
func EnrichmentColumnsPresent(path string) (hasPay, hasRatio bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, false, err
	}
	defer f.Close()
	cr := csv.NewReader(f)
	cr.FieldsPerRecord = -1
	hdr, err := cr.Read()
	if err != nil {
		return false, false, err
	}
	col := map[string]struct{}{}
	for _, h := range hdr {
		col[strings.TrimSpace(h)] = struct{}{}
	}
	_, hasPay = col["median_gross_annual_pay"]
	_, hasRatio = col["median_ratio"]
	return hasPay, hasRatio, nil
}

// PilotSpineCoverage returns pay and median_ratio coverage in one pass.
func PilotSpineCoverage(path string, areaCodes []string) (pay, ratio map[string]FieldCoverage, err error) {
	want := make(map[string]struct{}, len(areaCodes))
	for _, c := range areaCodes {
		want[strings.TrimSpace(c)] = struct{}{}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	return pilotSpineCoverageFromReader(f, want)
}

func numericColumnCoverageByArea(path string, areaCodes []string, column string) (map[string]FieldCoverage, error) {
	want := make(map[string]struct{}, len(areaCodes))
	for _, c := range areaCodes {
		want[strings.TrimSpace(c)] = struct{}{}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cr := csv.NewReader(f)
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
	valI, ok := col[column]
	if !ok {
		return nil, fmt.Errorf("spine csv: missing %s column", column)
	}
	out := make(map[string]FieldCoverage)
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
		fc := out[ac]
		fc.TotalRows++
		if len(rec) > valI {
			s := strings.TrimSpace(rec[valI])
			if s != "" {
				if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
					fc.NonZero++
				}
			}
		}
		out[ac] = fc
	}
	return out, nil
}

func pilotSpineCoverageFromReader(r io.Reader, want map[string]struct{}) (pay, ratio map[string]FieldCoverage, err error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	hdr, err := cr.Read()
	if err != nil {
		return nil, nil, err
	}
	col := map[string]int{}
	for i, h := range hdr {
		col[strings.TrimSpace(h)] = i
	}
	acI, ok := col["AreaCode"]
	if !ok {
		return nil, nil, fmt.Errorf("spine csv: missing AreaCode column")
	}
	payI, okPay := col["median_gross_annual_pay"]
	ratioI, okRatio := col["median_ratio"]
	pay = make(map[string]FieldCoverage)
	ratio = make(map[string]FieldCoverage)
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if len(rec) <= acI {
			continue
		}
		ac := strings.TrimSpace(rec[acI])
		if _, ok := want[ac]; !ok {
			continue
		}
		// Count every pilot row per field: missing column ⇒ TotalRows still increases, NonZero stays 0
		// (same as an empty enrichment column on a current spine).
		pc := pay[ac]
		pc.TotalRows++
		if okPay && len(rec) > payI {
			s := strings.TrimSpace(rec[payI])
			if s != "" {
				if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
					pc.NonZero++
				}
			}
		}
		pay[ac] = pc

		rc := ratio[ac]
		rc.TotalRows++
		if okRatio && len(rec) > ratioI {
			s := strings.TrimSpace(rec[ratioI])
			if s != "" {
				if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
					rc.NonZero++
				}
			}
		}
		ratio[ac] = rc
	}
	return pay, ratio, nil
}

func medianPayCoverageFromReader(r io.Reader, want map[string]struct{}) (map[string]FieldCoverage, error) {
	pay, _, err := pilotSpineCoverageFromReader(r, want)
	return pay, err
}
