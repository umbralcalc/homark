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

// PPDByTypeBuckets holds monthly price lists for all sales and per LR property type (D,S,T,F).
type PPDByTypeBuckets struct {
	All map[string]map[MonthKey][]float64
	// Typed[lad][month]["D"|"S"|"T"|"F"] — only standard types; other LR codes are counted in All only.
	Typed map[string]map[MonthKey]map[string][]float64
}

// PPDTypeMonthAgg is monthly PPD medians: All sales plus detached/semi/terraced/flat when sample exists.
type PPDTypeMonthAgg struct {
	All              PPDAgg
	Detached, Semi   PPDAgg
	Terraced, Flat   PPDAgg
}

// AggregatePricePaid streams Land Registry Price Paid CSV, maps postcodes to LAD via nspl,
// keeps rows in codes, buckets by calendar month, computes median price and count.
// Expected headers include price, date, postcode (case-insensitive).
func AggregatePricePaid(ppdPath, nsplPath string, codes map[string]struct{}) (map[string]map[MonthKey][]float64, error) {
	full, err := AggregatePricePaidByType(ppdPath, nsplPath, codes)
	if err != nil {
		return nil, err
	}
	return full.All, nil
}

// AggregatePricePaidByType is like AggregatePricePaid but also buckets by Property type (D,S,T,F) when present.
func AggregatePricePaidByType(ppdPath, nsplPath string, codes map[string]struct{}) (*PPDByTypeBuckets, error) {
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
	propI := ppdPropertyTypeColumn(idx)

	out := &PPDByTypeBuckets{
		All:   make(map[string]map[MonthKey][]float64),
		Typed: make(map[string]map[MonthKey]map[string][]float64),
	}
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
			if t2, e2 := parsePPDDateAlt(ds); e2 == nil {
				dt = t2
				ok = true
			}
		}
		if !ok {
			continue
		}
		mk := monthKeyFromTime(dt)
		if out.All[lad] == nil {
			out.All[lad] = make(map[MonthKey][]float64)
		}
		out.All[lad][mk] = append(out.All[lad][mk], price)

		if propI >= 0 && propI < len(rec) {
			pt := normalizeLRPropertyType(rec[propI])
			if pt == "D" || pt == "S" || pt == "T" || pt == "F" {
				if out.Typed[lad] == nil {
					out.Typed[lad] = make(map[MonthKey]map[string][]float64)
				}
				if out.Typed[lad][mk] == nil {
					out.Typed[lad][mk] = make(map[string][]float64)
				}
				out.Typed[lad][mk][pt] = append(out.Typed[lad][mk][pt], price)
			}
		}
	}
	return out, nil
}

func ppdPropertyTypeColumn(idx map[string]int) int {
	for _, name := range []string{
		"property type", "propertytype", "type",
	} {
		if j, ok := idx[name]; ok {
			return j
		}
	}
	return -1
}

// normalizeLRPropertyType maps Land Registry PPD values to D, S, T, F, or "".
func normalizeLRPropertyType(s string) string {
	u := strings.ToUpper(strings.TrimSpace(s))
	switch u {
	case "D", "DETACHED":
		return "D"
	case "S", "SEMI-DETACHED", "SEMI-DETACHED HOUSE":
		return "S"
	case "T", "TERRACED", "TERRACED HOUSE":
		return "T"
	case "F", "FLAT", "FLAT/MAISONETTE":
		return "F"
	default:
		return ""
	}
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
			out[lad][mk] = ppdPricesToAgg(prices)
		}
	}
	return out
}

func ppdPricesToAgg(prices []float64) PPDAgg {
	n := len(prices)
	if n == 0 {
		return PPDAgg{}
	}
	sort.Float64s(prices)
	med := prices[n/2]
	if n%2 == 0 {
		med = (prices[n/2-1] + prices[n/2]) / 2
	}
	return PPDAgg{
		MedianPrice: formatFloat(med),
		SalesCount:  strconv.Itoa(n),
	}
}

// PPDBucketsByTypeToAgg converts PPDByTypeBuckets to typed monthly aggregates for BuildSpine.
func PPDBucketsByTypeToAgg(b *PPDByTypeBuckets) map[string]map[MonthKey]PPDTypeMonthAgg {
	out := make(map[string]map[MonthKey]PPDTypeMonthAgg)
	merge := func(lad string, mk MonthKey, upd func(*PPDTypeMonthAgg)) {
		if out[lad] == nil {
			out[lad] = make(map[MonthKey]PPDTypeMonthAgg)
		}
		cur := out[lad][mk]
		upd(&cur)
		out[lad][mk] = cur
	}
	for lad, byM := range b.All {
		for mk, prices := range byM {
			agg := ppdPricesToAgg(prices)
			merge(lad, mk, func(p *PPDTypeMonthAgg) { p.All = agg })
		}
	}
	for lad, byM := range b.Typed {
		for mk, byT := range byM {
			for typ, prices := range byT {
				agg := ppdPricesToAgg(prices)
				merge(lad, mk, func(p *PPDTypeMonthAgg) {
					switch typ {
					case "D":
						p.Detached = agg
					case "S":
						p.Semi = agg
					case "T":
						p.Terraced = agg
					case "F":
						p.Flat = agg
					}
				})
			}
		}
	}
	return out
}
