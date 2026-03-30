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

// PPDAgg holds monthly median price paid and sale count for one LA.
type PPDAgg struct {
	MedianPrice string
	SalesCount  string
}

// NormalizeUKPostcode removes spaces and uppercases for lookup keys.
func NormalizeUKPostcode(p string) string {
	s := strings.ToUpper(strings.TrimSpace(p))
	return strings.ReplaceAll(s, " ", "")
}

// LoadNSPLPostcodeToLAD reads National Statistics Postcode Lookup (or similar) CSV:
// must contain a postcode column (pcds, Postcode, postcode) and an LA code column
// (lad23cd, lad22cd, lad19cd, or header containing "lad" and ending with "cd").
func LoadNSPLPostcodeToLAD(path string) (map[string]string, error) {
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
	low := make([]string, len(hdr))
	for i, h := range hdr {
		low[i] = strings.ToLower(strings.TrimSpace(h))
	}
	pcdI := -1
	for _, name := range []string{"pcds", "pcd", "postcode"} {
		for i, h := range low {
			if h == name {
				pcdI = i
				break
			}
		}
		if pcdI >= 0 {
			break
		}
	}
	if pcdI < 0 {
		for i, h := range low {
			if strings.Contains(h, "postcode") {
				pcdI = i
				break
			}
		}
	}
	ladI := -1
	for i, h := range low {
		if h == "lad23cd" || h == "lad22cd" || h == "lad19cd" || h == "lad18cd" {
			ladI = i
			break
		}
	}
	if ladI < 0 {
		for i, h := range low {
			if strings.HasPrefix(h, "lad") && strings.HasSuffix(h, "cd") {
				ladI = i
				break
			}
		}
	}
	if pcdI < 0 || ladI < 0 {
		return nil, fmt.Errorf("nspl: could not detect postcode and LAD code columns in %s", path)
	}
	out := make(map[string]string)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) <= pcdI || len(rec) <= ladI {
			continue
		}
		p := NormalizeUKPostcode(rec[pcdI])
		lad := strings.TrimSpace(rec[ladI])
		if p == "" || lad == "" || !strings.HasPrefix(lad, "E") {
			continue
		}
		if _, ok := out[p]; !ok {
			out[p] = lad
		}
	}
	return out, nil
}

// AggregatePricePaid streams Land Registry Price Paid CSV, maps postcodes to LAD via nspl,
// keeps rows in codes, buckets by calendar month, computes median price and count.
// Expected headers include price, date, postcode (case-insensitive).
func AggregatePricePaid(ppdPath, nsplPath string, codes map[string]struct{}) (map[string]map[MonthKey][]float64, error) {
	pcToLAD, err := LoadNSPLPostcodeToLAD(nsplPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(ppdPath)
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
	idx := map[string]int{}
	for i, h := range hdr {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	priceI, ok1 := idx["price"]
	dateI, ok2 := idx["date of transfer"]
	if !ok2 {
		dateI, ok2 = idx["date"]
	}
	pcdI, ok3 := idx["postcode"]
	if !ok1 || !ok2 || !ok3 {
		return nil, fmt.Errorf("ppd: need columns price, date/date of transfer, postcode")
	}
	// buckets[lad][month] = prices
	buckets := make(map[string]map[MonthKey][]float64)
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) <= priceI || len(rec) <= dateI || len(rec) <= pcdI {
			continue
		}
		pcd := NormalizeUKPostcode(rec[pcdI])
		lad, ok := pcToLAD[pcd]
		if !ok {
			continue
		}
		if _, ok := codes[lad]; !ok {
			continue
		}
		price, err := strconv.ParseFloat(strings.TrimSpace(rec[priceI]), 64)
		if err != nil || price <= 0 {
			continue
		}
		ds := strings.TrimSpace(rec[dateI])
		dt, ok := parseHPIDate(ds)
		if !ok {
			// try ISO
			if t2, e2 := parsePPDDateAlt(ds); e2 == nil {
				dt = t2
				ok = true
			}
		}
		if !ok {
			continue
		}
		mk := monthKeyFromTime(dt)
		if buckets[lad] == nil {
			buckets[lad] = make(map[MonthKey][]float64)
		}
		buckets[lad][mk] = append(buckets[lad][mk], price)
	}
	return buckets, nil
}

func parsePPDDateAlt(s string) (t time.Time, err error) {
	layouts := []string{"2006-01-02", "02/01/2006", time.RFC3339}
	for _, layout := range layouts {
		if tt, e := time.Parse(layout, s); e == nil {
			return tt.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("bad date")
}

// PPDBucketsToAgg converts raw price slices to median + count strings per month.
func PPDBucketsToAgg(b map[string]map[MonthKey][]float64) map[string]map[MonthKey]PPDAgg {
	out := make(map[string]map[MonthKey]PPDAgg)
	for lad, byM := range b {
		out[lad] = make(map[MonthKey]PPDAgg)
		for mk, prices := range byM {
			n := len(prices)
			if n == 0 {
				continue
			}
			sort.Float64s(prices)
			med := prices[n/2]
			if n%2 == 0 {
				med = (prices[n/2-1] + prices[n/2]) / 2
			}
			out[lad][mk] = PPDAgg{
				MedianPrice: formatFloat(med),
				SalesCount:  strconv.Itoa(n),
			}
		}
	}
	return out
}
