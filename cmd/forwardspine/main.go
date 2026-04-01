// Command forwardspine runs a stochastic single-LA monthly model with historical
// bank_rate_pct from spine_monthly.csv fed into log-price drift (see pkg/housing.ForwardSpineConfigs).
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
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
	list := flag.Bool("list", false, "print pilot LAs and exit")

	earningsDrift := flag.Float64("earnings-drift", 0.0005, "log-earnings drift per month")
	earningsDiff := flag.Float64("earnings-diff", 0.004, "log-earnings diffusion σ")
	priceDrift := flag.Float64("price-drift", 0.0008, "log-price base drift per month (before bank term)")
	priceDiff := flag.Float64("price-diff", 0.012, "log-price diffusion σ")
	bankBeta := flag.Float64("bank-beta", 0, "log-price drift term: beta × (bank_rate_pct/100)")
	supplyBeta := flag.Float64("supply-beta", 0, "log-price drift term: beta × (net_add_FY / supply-scale)")
	supplyScale := flag.Float64("supply-scale", 1000, "scale for net_additional_dwellings_fy in drift")
	pipelineBeta := flag.Float64("pipeline-beta", 0, "log-price drift dampening: beta × (pipeline_stock / pipeline-ref)")
	pipelineRef := flag.Float64("pipeline-ref", 500, "reference pipeline stock for pipeline-beta term")
	demandSupplyBeta := flag.Float64("demand-supply-beta", 0, "log-price drift: beta × ((log_earnings−init) − net_add/scale − pipeline/ref)")
	approvalRate := flag.Float64("approval-rate", 0, "mean dwellings/month entering pipeline (0 keeps pipeline at 0 without init)")
	completionFrac := flag.Float64("completion-frac", 0.15, "fraction of pipeline stock completing per month")
	pipelineInit := flag.Float64("pipeline-init", 0, "initial pipeline stock (dwellings)")
	seedE := flag.Uint64("seed-earnings", 9101, "RNG seed for log earnings")
	seedP := flag.Uint64("seed-price", 9102, "RNG seed for log price")
	initRatio := flag.Float64("init-median-ratio-fallback", 7, "if first month has no pay/ONS ratio, use this P/E to set initial log earnings (logE = logP − log(ratio))")
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

	opt := housing.ForwardOptions{
		EarningsDrift: *earningsDrift, EarningsDiff: *earningsDiff,
		PriceDrift: *priceDrift, PriceDiff: *priceDiff,
		BankBeta: *bankBeta, SupplyBeta: *supplyBeta, SupplyScale: *supplyScale,
		PipelineBeta: *pipelineBeta, PipelineRef: *pipelineRef,
		DemandSupplyPressureBeta: *demandSupplyBeta,
		ApprovalRate:             *approvalRate, CompletionFrac: *completionFrac, PipelineInit: *pipelineInit,
		SeedEarnings: *seedE, SeedPrice: *seedP,
		InitMedianRatioFallback: *initRatio,
	}
	settings, impl, err := housing.ForwardSpineConfigs(obs, opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	store := simulator.NewStateTimeStorage()
	impl.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coord := simulator.NewPartitionCoordinator(settings, impl)
	coord.Run()

	if err := checkStoreAligned(store); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	w := csv.NewWriter(os.Stdout)
	_ = w.Write([]string{
		"step", "time", "year_month", "bank_rate_pct", "net_add_fy", "pipeline_stock",
		"log_earnings", "log_price", "affordability",
	})
	times := store.GetTimes()
	vB := store.GetValues("bank_rate")
	vS := store.GetValues("supply_net")
	vPl := store.GetValues("pipeline")
	vE := store.GetValues("log_earnings")
	vP := store.GetValues("log_price")
	vA := store.GetValues("affordability")
	for i := range times {
		ym := ""
		if i < len(obs) {
			ym = obs[i].YearMonth
		}
		_ = w.Write([]string{
			fmt.Sprintf("%d", i+1),
			fmtFloat(times[i]),
			ym,
			fmtFloat(vB[i][0]),
			fmtFloat(vS[i][0]),
			fmtFloat(vPl[i][0]),
			fmtFloat(vE[i][0]),
			fmtFloat(vP[i][0]),
			fmtFloat(vA[i][0]),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
