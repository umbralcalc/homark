// Command runfromspine replays one local authority's monthly spine through a
// stochadex run: log earnings, log price, and affordability (P/E) are streamed
// from spine_monthly.csv via FromStorageIteration (see pkg/housing).
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/umbralcalc/homark/pkg/housing"
	"github.com/umbralcalc/homark/pkg/ladata"
	"github.com/umbralcalc/homark/pkg/spine"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func main() {
	root := flag.String("root", ".", "repository root (directory containing go.mod)")
	spinePath := flag.String("spine", "dat/processed/spine_monthly.csv", "path under -root to spine_monthly.csv")
	area := flag.String("area", "", "ONS GSS area code (e.g. E09000030)")
	laName := flag.String("la", "", "pilot LA name from pkg/ladata/targets.yaml (alternative to -area)")
	maxSteps := flag.Int("max-steps", 0, "cap number of months (0 = all rows for that LA)")
	validate := flag.Bool("validate", false, "compare replay affordability to spine median_ratio where both present")
	quiet := flag.Bool("q", false, "no CSV to stdout (use with -validate)")
	list := flag.Bool("list", false, "print pilot LAs from targets.yaml and exit")
	flag.Parse()

	if *list {
		targets, err := ladata.LoadTargets()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, t := range targets {
			fmt.Printf("%s\t%s\n", t.AreaCode, t.Name)
		}
		return
	}

	repo, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := os.Stat(filepath.Join(repo, "go.mod")); err != nil {
		fmt.Fprintf(os.Stderr, "-root %q must contain go.mod\n", repo)
		os.Exit(1)
	}

	ac, err := resolveArea(*area, *laName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fullSpine := *spinePath
	if !filepath.IsAbs(fullSpine) {
		fullSpine = filepath.Join(repo, *spinePath)
	}
	obs, err := spine.LoadSpineMonthlyForArea(fullSpine, ac)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(obs) == 0 {
		fmt.Fprintf(os.Stderr, "no rows for area %s in %s\n", ac, fullSpine)
		os.Exit(1)
	}
	if *maxSteps > 0 && *maxSteps < len(obs) {
		obs = obs[:*maxSteps]
	}

	logE, logP, afford, err := housing.MonthlyLogSeries(obs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	settings, impl, err := housing.ReplayImplementations(logE, logP, afford)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	store := simulator.NewStateTimeStorage()
	impl.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	housing.ConfigureReplayIterations(settings, impl.Iterations)
	coord := simulator.NewPartitionCoordinator(settings, impl)
	coord.Run()

	if err := checkStoreAligned(store); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var out io.Writer = os.Stdout
	if *quiet {
		out = io.Discard
	}
	w := csv.NewWriter(out)
	_ = w.Write([]string{
		"step", "time", "year_month", "log_earnings", "log_price", "affordability",
		"bank_rate_pct", "median_ratio_spine",
	})
	times := store.GetTimes()
	vE := store.GetValues("log_earnings")
	vP := store.GetValues("log_price")
	vA := store.GetValues("affordability")
	for i := range times {
		ym := ""
		var bank, ratio string
		if i < len(obs) {
			ym = obs[i].YearMonth
			bank = fmtFloat(obs[i].BankRatePct)
			if obs[i].MedianRatio > 0 {
				ratio = fmtFloat(obs[i].MedianRatio)
			}
		}
		_ = w.Write([]string{
			fmt.Sprintf("%d", i+1),
			fmtFloat(times[i]),
			ym,
			fmtFloat(vE[i][0]),
			fmtFloat(vP[i][0]),
			fmtFloat(vA[i][0]),
			bank,
			ratio,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *validate {
		var maxDiff float64
		var n int
		for i := 0; i < len(obs) && i < len(vA); i++ {
			if obs[i].MedianRatio <= 0 {
				continue
			}
			d := math.Abs(vA[i][0] - obs[i].MedianRatio)
			n++
			if d > maxDiff {
				maxDiff = d
			}
		}
		if n == 0 {
			fmt.Fprintln(os.Stderr, "validate: no rows with median_ratio in spine")
		} else {
			fmt.Fprintf(os.Stderr, "validate: compared %d rows, max |replay−median_ratio| = %.6g\n", n, maxDiff)
		}
	}
}

func resolveArea(areaFlag, nameFlag string) (string, error) {
	if areaFlag != "" && nameFlag != "" {
		return "", fmt.Errorf("use only one of -area or -la")
	}
	if areaFlag != "" {
		return strings.TrimSpace(areaFlag), nil
	}
	if nameFlag == "" {
		return "", fmt.Errorf("required: -area CODE or -la NAME (or -list)")
	}
	targets, err := ladata.LoadTargets()
	if err != nil {
		return "", err
	}
	want := strings.ToLower(strings.TrimSpace(nameFlag))
	for _, t := range targets {
		if strings.ToLower(t.Name) == want {
			return t.AreaCode, nil
		}
	}
	return "", fmt.Errorf("unknown LA name %q (see -list)", nameFlag)
}

func fmtFloat(v float64) string {
	if v == 0 {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.12g", v), "0"), ".")
}

func checkStoreAligned(store *simulator.StateTimeStorage) error {
	times := store.GetTimes()
	for _, name := range store.GetNames() {
		v := store.GetValues(name)
		if len(v) != len(times) {
			return fmt.Errorf("internal: partition %q len %d != times %d", name, len(v), len(times))
		}
	}
	return nil
}
