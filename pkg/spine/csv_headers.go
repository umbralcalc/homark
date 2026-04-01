package spine

import "fmt"

func headerIndex(col map[string]int, candidates []string) (int, bool) {
	for _, c := range candidates {
		if i, ok := col[c]; ok {
			return i, true
		}
	}
	return 0, false
}

// resolveAreaCodeColumn picks a local-authority / geography code column from common ONS and NOMIS CSV exports.
func resolveAreaCodeColumn(col map[string]int) (int, error) {
	if i, ok := headerIndex(col, []string{
		"area_code", "areacode",
		"geography_code", "geo_code", "geocode",
		"lad_code", "lad20cd", "lad21cd", "lad22cd",
		"ons_code", "gss_code",
	}); ok {
		return i, nil
	}
	return 0, fmt.Errorf("csv: need an area column (e.g. area_code, geography_code, lad_code)")
}
