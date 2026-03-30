package spine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/knieriem/odf/ods"
)

// DefaultLiveTable122URL is DLUHC Live Table 122 (net additional dwellings by LA, England), ODS format.
const DefaultLiveTable122URL = "https://assets.publishing.service.gov.uk/media/691f395e9c8e8f345bf985d3/Live_Table_122.ods"

var fyCol = regexp.MustCompile(`^(\d{4})-\d{2}`)

// ParseNetAdditionalTable122 reads DLUHC Live Table 122 ODS. It returns
// net additional dwellings by current ONS LA code and UK financial year start
// (e.g. April 2024–March 2025 → key 2024).
func ParseNetAdditionalTable122(odsPath string) (map[string]map[int]float64, error) {
	f, err := ods.Open(odsPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var doc ods.Doc
	if err := f.ParseContent(&doc); err != nil {
		return nil, err
	}
	var sheet *ods.Table
	for i := range doc.Table {
		if doc.Table[i].Name == "LT_122" {
			sheet = &doc.Table[i]
			break
		}
	}
	if sheet == nil {
		return nil, fmt.Errorf("table122: sheet LT_122 not found")
	}
	rows := sheet.Strings()
	hdrRow := -1
	for i, row := range rows {
		if len(row) > 3 && strings.TrimSpace(row[2]) == "Current ONS code" {
			hdrRow = i
			break
		}
	}
	if hdrRow < 0 {
		return nil, fmt.Errorf("table122: header row not found")
	}
	header := rows[hdrRow]
	yearCol := make(map[int]int) // FY start -> column index
	for j, h := range header {
		h = strings.TrimSpace(h)
		if m := fyCol.FindStringSubmatch(h); m != nil {
			yr, err := strconv.Atoi(m[1])
			if err == nil {
				yearCol[yr] = j
			}
		}
	}
	if len(yearCol) == 0 {
		return nil, fmt.Errorf("table122: no year columns")
	}
	out := make(map[string]map[int]float64)
	for _, row := range rows[hdrRow+1:] {
		if len(row) <= 3 {
			continue
		}
		code := strings.TrimSpace(row[2])
		if !strings.HasPrefix(code, "E") || len(code) != 9 {
			continue
		}
		// Skip England / region totals (e.g. E92000001); keep LAD/UA codes (E06…, E07…, E08…, E09…).
		if strings.HasPrefix(code, "E92") || strings.HasPrefix(code, "E12") {
			continue
		}
		if out[code] == nil {
			out[code] = make(map[int]float64)
		}
		for yr, col := range yearCol {
			if col >= len(row) {
				continue
			}
			v, ok := parseONSNumber(row[col])
			if ok {
				out[code][yr] = v
			}
		}
	}
	return out, nil
}

func parseONSNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "[x]" || strings.HasPrefix(s, "[") {
		return 0, false
	}
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// FYStartForCalendar returns the UK financial year start calendar year for t (April–March).
func FYStartForCalendar(t time.Time) int {
	y, m, _ := t.Date()
	if m >= time.April {
		return y
	}
	return y - 1
}
