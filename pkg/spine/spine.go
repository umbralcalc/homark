// Package spine downloads UK HPI + BoE inputs and builds the pilot-LA monthly spine CSV.
package spine

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultUKHPIURL = "https://publicdata.landregistry.gov.uk/market-trend-data/house-price-index-data/UK-HPI-full-file-2025-12.csv"
	DefaultBOEURL   = "https://www.bankofengland.co.uk/boeapps/iadb/fromshowcolumns.asp?csv.x=yes&SeriesCodes=IUDBEDR&UsingCodes=Y&VPD=Y&VFD=N&Datefrom=01/Jan/1975&Dateto=31/Dec/2026"
)

// Download saves response body to path (creating parent dirs).
func Download(client *http.Client, url, path string) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: status %s", url, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}

// MonthKey is YYYY-MM for joining HPI to BoE aggregates.
type MonthKey string

func monthKeyFromTime(t time.Time) MonthKey {
	y, m, _ := t.Date()
	return MonthKey(fmt.Sprintf("%04d-%02d", y, int(m)))
}

// BOEMonthlyMeans reads BoE IUDBEDR CSV (DATE, rate) and returns mean rate per calendar month.
func BOEMonthlyMeans(path string) (map[MonthKey]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	if len(header) < 2 {
		return nil, fmt.Errorf("boe: expected at least 2 columns")
	}
	type agg struct {
		sum   float64
		count int
	}
	buckets := make(map[MonthKey]agg)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 2 {
			continue
		}
		ds := strings.TrimSpace(rec[0])
		rs := strings.TrimSpace(rec[1])
		t, ok := parseBOEDate(ds)
		if !ok {
			continue
		}
		rate, err := strconv.ParseFloat(rs, 64)
		if err != nil {
			continue
		}
		key := monthKeyFromTime(t)
		a := buckets[key]
		a.sum += rate
		a.count++
		buckets[key] = a
	}
	out := make(map[MonthKey]float64, len(buckets))
	for k, a := range buckets {
		if a.count > 0 {
			out[k] = a.sum / float64(a.count)
		}
	}
	return out, nil
}

func parseBOEDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	layouts := []string{
		"02 Jan 2006",
		time.RFC3339,
		"2006-01-02",
		"02/01/2006",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseHPIDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	layouts := []string{
		"02/01/2006", // DD/MM/YYYY
		"2006-01-02",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// Row is one output line in spine_monthly.csv.
type Row struct {
	Date         time.Time
	RegionName   string
	AreaCode     string
	AveragePrice string
	Index        string
	BankRatePct  string // empty if no BoE match
	MedianRatio  string // empty unless ONS enrichment
}

func (r Row) yearMonth() MonthKey { return monthKeyFromTime(r.Date) }

// BuildSpine streams ukhpiPath, keeps rows whose AreaCode is in codes, joins bank rates, sorts, writes outPath.
// If ons is non-nil, adds median_ratio from annual ONS data (same as Python EDA).
func BuildSpine(ukhpiPath string, codes map[string]struct{}, bank map[MonthKey]float64, ons ONSAnnual, outPath string) (int, error) {
	f, err := os.Open(ukhpiPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	cr := csv.NewReader(f)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return 0, err
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.TrimSpace(h)] = i
	}
	req := []string{"Date", "AreaCode", "RegionName", "AveragePrice", "Index"}
	for _, k := range req {
		if _, ok := idx[k]; !ok {
			return 0, fmt.Errorf("uk hpi: missing column %q", k)
		}
	}
	var rows []Row
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		get := func(name string) string {
			j := idx[name]
			if j >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[j])
		}
		ac := get("AreaCode")
		if _, ok := codes[ac]; !ok {
			continue
		}
		ds := get("Date")
		dt, ok := parseHPIDate(ds)
		if !ok {
			continue
		}
		key := monthKeyFromTime(dt)
		br := ""
		if v, ok := bank[key]; ok {
			br = formatFloat(v)
		}
		mr := ""
		if ons != nil {
			yr := dt.Year()
			if m, ok := ons[ac]; ok {
				mr = m[yr]
			}
		}
		rows = append(rows, Row{
			Date:         dt,
			RegionName:   get("RegionName"),
			AreaCode:     ac,
			AveragePrice: get("AveragePrice"),
			Index:        get("Index"),
			BankRatePct:  br,
			MedianRatio:  mr,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AreaCode != rows[j].AreaCode {
			return rows[i].AreaCode < rows[j].AreaCode
		}
		return rows[i].Date.Before(rows[j].Date)
	})
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	w := csv.NewWriter(out)
	outHeader := []string{"Date", "RegionName", "AreaCode", "AveragePrice", "Index", "year_month", "bank_rate_pct"}
	if ons != nil {
		outHeader = append(outHeader, "median_ratio")
	}
	if err := w.Write(outHeader); err != nil {
		return 0, err
	}
	for _, row := range rows {
		line := []string{
			row.Date.Format("2006-01-02"),
			row.RegionName,
			row.AreaCode,
			row.AveragePrice,
			row.Index,
			string(row.yearMonth()),
			row.BankRatePct,
		}
		if ons != nil {
			line = append(line, row.MedianRatio)
		}
		if err := w.Write(line); err != nil {
			return 0, err
		}
	}
	w.Flush()
	return len(rows), w.Error()
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ONSAnnual maps (area code, calendar year) -> median_ratio string for optional enrichment.
type ONSAnnual map[string]map[int]string

// LoadONSAnnual reads CSV with columns area_code, year, median_ratio (header names case-insensitive).
func LoadONSAnnual(path string) (ONSAnnual, error) {
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
	mI, ok3 := col["median_ratio"]
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("ons: need columns area_code, year, median_ratio")
	}
	out := make(ONSAnnual)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) <= acI || len(rec) <= yI || len(rec) <= mI {
			continue
		}
		ac := strings.TrimSpace(rec[acI])
		yr, err := strconv.Atoi(strings.TrimSpace(rec[yI]))
		if err != nil {
			continue
		}
		mr := strings.TrimSpace(rec[mI])
		if out[ac] == nil {
			out[ac] = make(map[int]string)
		}
		out[ac][yr] = mr
	}
	return out, nil
}
