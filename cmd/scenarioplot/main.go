// Command scenarioplot turns policyscenario CSV into a small HTML heatmap table.
// Example: go run ./cmd/policyscenario -la Leeds -approvals 0,50,100 -bank-scales 1,1.05 | go run ./cmd/scenarioplot -out scenarios.html
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

func main() {
	inPath := flag.String("in", "", "CSV path (default: stdin)")
	outPath := flag.String("out", "", "write HTML here (default: stdout)")
	xCol := flag.String("x", "approval_rate", "CSV column for heatmap x axis")
	yCol := flag.String("y", "bank_scale", "CSV column for heatmap y axis")
	valCol := flag.String("value", "mean_afford", "CSV cell value column")
	sampleIdx := flag.String("sample-idx", "0", "keep only rows where sample_idx equals this (empty = all rows, average duplicates)")
	title := flag.String("title", "Scenario heatmap", "page title")
	flag.Parse()

	var r io.Reader = os.Stdin
	if strings.TrimSpace(*inPath) != "" {
		f, err := os.Open(*inPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		r = f
	}
	cr := csv.NewReader(r)
	rows, err := cr.ReadAll()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(rows) < 2 {
		fmt.Fprintln(os.Stderr, "scenarioplot: need header + data rows")
		os.Exit(1)
	}
	hdr := rows[0]
	ix := colIndex(hdr, *xCol)
	iy := colIndex(hdr, *yCol)
	iv := colIndex(hdr, *valCol)
	is := colIndex(hdr, "sample_idx")
	if ix < 0 || iy < 0 || iv < 0 {
		fmt.Fprintf(os.Stderr, "scenarioplot: missing columns (need %q %q %q)\n", *xCol, *yCol, *valCol)
		os.Exit(1)
	}

	type key struct{ x, y float64 }
	sum := make(map[key]float64)
	cnt := make(map[key]int)
	for _, rec := range rows[1:] {
		if len(rec) <= max3(ix, iy, iv) {
			continue
		}
		if is >= 0 && *sampleIdx != "" {
			if strings.TrimSpace(rec[is]) != *sampleIdx {
				continue
			}
		}
		xv, e1 := strconv.ParseFloat(strings.TrimSpace(rec[ix]), 64)
		yv, e2 := strconv.ParseFloat(strings.TrimSpace(rec[iy]), 64)
		vv, e3 := strconv.ParseFloat(strings.TrimSpace(rec[iv]), 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		k := key{x: roundKey(xv), y: roundKey(yv)}
		sum[k] += vv
		cnt[k]++
	}
	if len(sum) == 0 {
		fmt.Fprintln(os.Stderr, "scenarioplot: no numeric rows after filters")
		os.Exit(1)
	}

	xSet, ySet := map[float64]struct{}{}, map[float64]struct{}{}
	for k := range sum {
		xSet[k.x] = struct{}{}
		ySet[k.y] = struct{}{}
	}
	xs := sortedKeys(xSet)
	ys := sortedKeys(ySet)
	mat := make([][]float64, len(ys))
	for i := range mat {
		mat[i] = make([]float64, len(xs))
		for j := range mat[i] {
			mat[i][j] = math.NaN()
		}
	}
	xIdx := indexMap(xs)
	yIdx := indexMap(ys)
	vmin, vmax := math.Inf(1), math.Inf(-1)
	for k, s := range sum {
		v := s / float64(cnt[k])
		mat[yIdx[k.y]][xIdx[k.x]] = v
		if v < vmin {
			vmin = v
		}
		if v > vmax {
			vmax = v
		}
	}
	if math.Abs(vmax-vmin) < 1e-18 {
		vmax = vmin + 1
	}

	html := buildHTML(*title, *xCol, *yCol, *valCol, xs, ys, mat, vmin, vmax)
	if strings.TrimSpace(*outPath) != "" {
		if err := os.WriteFile(*outPath, []byte(html), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	fmt.Print(html)
}

func max3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

func colIndex(hdr []string, name string) int {
	for i, h := range hdr {
		if strings.TrimSpace(h) == name {
			return i
		}
	}
	return -1
}

func roundKey(v float64) float64 {
	return math.Round(v*1e9) / 1e9
}

func sortedKeys(m map[float64]struct{}) []float64 {
	out := make([]float64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func indexMap(xs []float64) map[float64]int {
	m := make(map[float64]int, len(xs))
	for i, v := range xs {
		m[v] = i
	}
	return m
}

func buildHTML(title, xLabel, yLabel, valLabel string, xs, ys []float64, mat [][]float64, vmin, vmax float64) string {
	var thead strings.Builder
	thead.WriteString(fmt.Sprintf(`<tr><th class="corner">%s \ %s</th>`, yLabel, xLabel))
	for _, x := range xs {
		thead.WriteString(fmt.Sprintf("<th>%.4g</th>", x))
	}
	thead.WriteString("</tr>")

	var tbody strings.Builder
	for i := range ys {
		tbody.WriteString("<tr>")
		tbody.WriteString(fmt.Sprintf(`<th class="y">%.4g</th>`, ys[i]))
		for j := range xs {
			v := mat[i][j]
			if math.IsNaN(v) {
				tbody.WriteString(`<td class="cell empty">—</td>`)
				continue
			}
			t := (v - vmin) / (vmax - vmin)
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			r := int(255 * t)
			b := int(255 * (1 - t))
			g := int(128 + 127*(1-math.Abs(t-0.5)*2))
			tbody.WriteString(fmt.Sprintf(
				`<td class="cell" style="background:rgb(%d,%d,%d);color:#111" title="%g">%.4g</td>`,
				r, g, b, v, v,
			))
		}
		tbody.WriteString("</tr>\n")
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<title>%s</title>
<style>
body { font-family: system-ui, sans-serif; margin: 24px; background: #111; color: #e8e8e8; }
h1 { font-size: 1.1rem; font-weight: 600; }
.note { font-size: 0.85rem; opacity: 0.75; margin-bottom: 16px; }
.grid-wrap { overflow: auto; }
table { border-collapse: collapse; font-size: 0.8rem; }
th, td { padding: 4px 8px; text-align: right; border: 1px solid #333; }
th.corner, th.y { background: #1a1a1a; }
th.y { white-space: nowrap; }
td.cell { min-width: 3.5rem; text-align: center; font-variant-numeric: tabular-nums; }
td.empty { background: #222; color: #666; }
</style>
</head>
<body>
<h1>%s</h1>
<p class="note">Columns: %s. Rows: %s. Colour: %s (min %.4g, max %.4g).</p>
<div class="grid-wrap">
<table>
<thead>%s</thead>
<tbody>
%s</tbody>
</table>
</div>
</body>
</html>
`, title, title, xLabel, yLabel, valLabel, vmin, vmax, thead.String(), tbody.String())
}
