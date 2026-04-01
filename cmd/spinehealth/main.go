// Command spinehealth reports median_gross_annual_pay and median_ratio coverage on
// dat/processed/spine_monthly.csv for pilot LAs (pkg/ladata/targets.yaml).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/umbralcalc/homark/pkg/ladata"
	"github.com/umbralcalc/homark/pkg/spine"
)

func main() {
	root := flag.String("root", ".", "repository root (directory containing go.mod)")
	spinePath := flag.String("spine", "dat/processed/spine_monthly.csv", "path under -root to spine_monthly.csv")
	minPayPct := flag.Float64("min-pay-pct", 0, "if >0, exit 1 when any pilot LA has pay coverage below this percent (0–100)")
	minRatioPct := flag.Float64("min-ratio-pct", 0, "if >0, exit 1 when any pilot LA has median_ratio coverage below this percent")
	flag.Parse()

	repo, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := os.Stat(filepath.Join(repo, "go.mod")); err != nil {
		fmt.Fprintf(os.Stderr, "-root %q must contain go.mod\n", repo)
		os.Exit(1)
	}

	full := *spinePath
	if !filepath.IsAbs(full) {
		full = filepath.Join(repo, *spinePath)
	}
	if _, err := os.Stat(full); err != nil {
		fmt.Fprintf(os.Stderr, "spine file %q: %v\n", full, err)
		os.Exit(1)
	}

	targets, err := ladata.LoadTargets()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	codes := make([]string, len(targets))
	for i := range targets {
		codes[i] = targets[i].AreaCode
	}

	pay, ratio, err := spine.PilotSpineCoverage(full, codes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if hp, hr, err := spine.EnrichmentColumnsPresent(full); err == nil && !hp && !hr {
		fmt.Fprintln(os.Stderr, "hint: spine CSV has no median_gross_annual_pay or median_ratio columns; run fetchspine to regenerate the spine (empty columns are written even without raw enrichment files).")
	}

	fmt.Println("Pilot LA spine coverage (non-zero / total rows):")
	var fail bool
	for _, a := range targets {
		p := pay[a.AreaCode]
		r := ratio[a.AreaCode]
		pp := 100 * p.Fraction()
		rp := 100 * r.Fraction()
		fmt.Printf("  %s\t%s\tpay %5.1f%% (%d/%d)\tratio %5.1f%% (%d/%d)\n",
			a.AreaCode, a.Name, pp, p.NonZero, p.TotalRows, rp, r.NonZero, r.TotalRows)
		if *minPayPct > 0 && pp+1e-6 < *minPayPct {
			fmt.Fprintf(os.Stderr, "  below -min-pay-pct=%g%%\n", *minPayPct)
			fail = true
		}
		if *minRatioPct > 0 && rp+1e-6 < *minRatioPct {
			fmt.Fprintf(os.Stderr, "  below -min-ratio-pct=%g%%\n", *minRatioPct)
			fail = true
		}
	}
	if fail {
		os.Exit(1)
	}
}
