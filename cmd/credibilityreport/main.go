// Command credibilityreport prints data-vs-model diagnostics for one pilot LA:
// log PPD vs log HPI correlations (all sales and per D/S/T/F), permissions→completions lag scan,
// and optional temporal hold-out calibration (same grid flags as calibratespine).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/umbralcalc/homark/pkg/housing"
	"github.com/umbralcalc/homark/pkg/ladata"
	"github.com/umbralcalc/homark/pkg/spine"
)

func main() {
	root := flag.String("root", ".", "repository root (directory containing go.mod)")
	spinePath := flag.String("spine", "dat/processed/spine_monthly.csv", "path under -root to spine_monthly.csv")
	area := flag.String("area", "", "ONS GSS area code")
	laName := flag.String("la", "", "pilot LA name (alternative to -area)")
	maxSteps := flag.Int("max-steps", 0, "cap months (0 = all)")
	lagMax := flag.Int("lag-max", 24, "max lag (months) for permissions vs completions correlation")
	list := flag.Bool("list", false, "list pilot LAs and exit")
	noCred := flag.Bool("no-credibility", false, "skip the data credibility block (requires -validate-months > 0)")

	validateMonths := flag.Int("validate-months", 0, "if >0, run grid calibration on train and report train/test stats (same grid as calibratespine)")
	wLogE := flag.Float64("w-log-earnings", 0, "joint log-earnings RMSE weight when validating")

	bankLo := flag.Float64("bank-beta-lo", -0.1, "grid low for bank_beta")
	bankHi := flag.Float64("bank-beta-hi", 0.1, "grid high for bank_beta")
	bankSteps := flag.Int("bank-steps", 21, "grid points for bank_beta")

	driftLo := flag.Float64("price-drift-lo", 0, "grid low for price drift")
	driftHi := flag.Float64("price-drift-hi", 0, "grid high for price drift")
	driftSteps := flag.Int("price-drift-steps", 0, "grid points (0 = hold base)")

	supLo := flag.Float64("supply-beta-lo", 0, "grid low for supply_beta")
	supHi := flag.Float64("supply-beta-hi", 0, "grid high for supply_beta")
	supSteps := flag.Int("supply-steps", 0, "grid points (0 = hold base)")

	dspLo := flag.Float64("demand-supply-beta-lo", 0, "grid low for DemandSupplyPressureBeta")
	dspHi := flag.Float64("demand-supply-beta-hi", 0, "grid high")
	dspSteps := flag.Int("demand-supply-steps", 0, "grid points (0 = hold base)")

	cfLo := flag.Float64("completion-frac-lo", 0, "grid low for completion_frac")
	cfHi := flag.Float64("completion-frac-hi", 0, "grid high")
	cfSteps := flag.Int("completion-frac-steps", 0, "grid points (0 = hold base)")

	edLo := flag.Float64("earnings-drift-lo", 0, "grid low for earnings_drift")
	edHi := flag.Float64("earnings-drift-hi", 0, "grid high")
	edSteps := flag.Int("earnings-drift-steps", 0, "grid points (0 = hold base)")

	base := housing.DefaultForwardOptions()
	flag.Float64Var(&base.EarningsDrift, "earnings-drift", base.EarningsDrift, "base log-earnings drift")
	flag.Float64Var(&base.PriceDrift, "price-drift", base.PriceDrift, "base price drift")
	flag.Float64Var(&base.SupplyBeta, "supply-beta", base.SupplyBeta, "base supply_beta")
	flag.Float64Var(&base.DemandSupplyPressureBeta, "demand-supply-beta", base.DemandSupplyPressureBeta, "base demand_supply beta")
	flag.Float64Var(&base.CompletionFrac, "completion-frac", base.CompletionFrac, "base completion_frac")
	flag.Float64Var(&base.InitMedianRatioFallback, "init-median-ratio-fallback", 7, "calibration target P/E fallback (0 → 7)")
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
		fmt.Fprintf(os.Stderr, "no rows for area %s\n", ac)
		os.Exit(1)
	}
	if *maxSteps > 0 && *maxSteps < len(obs) {
		obs = obs[:*maxSteps]
	}

	if *noCred && *validateMonths <= 0 {
		fmt.Fprintln(os.Stderr, "-no-credibility requires -validate-months > 0")
		os.Exit(1)
	}

	if !*noCred {
		sum := housing.SummarizeCredibilitySpine(obs)
		fmt.Println("=== Data credibility (spine vs HPI, supply series) ===")
		fmt.Printf("spine coverage: rows=%d  AveragePrice>0=%d  ppd_median_price>0=%d  any_typed_ppd>0=%d  permissions>0=%d  completions>0=%d  same_month_perm+comp>0=%d\n",
			sum.Rows, sum.AveragePricePositive, sum.PPDAllPositive, sum.PPDAnyTypeMedianPositive,
			sum.PermissionsPositive, sum.CompletionsPositive, sum.SameMonthPermAndCompletion)
		if sum.PPDAllPositive == 0 && sum.PPDAnyTypeMedianPositive == 0 {
			fmt.Println("note: no PPD medians on this spine — correlations need Land Registry Price Paid + NSPL (e.g. dat/raw/price_paid.csv, dat/raw/nspl.csv) then go run ./cmd/fetchspine -skip-download")
		}
		if sum.PermissionsPositive == 0 || sum.CompletionsPositive == 0 {
			fmt.Println("note: lag scan needs both permissions_approx_monthly and completions_approx_monthly (dat/raw/permissions_annual.csv, dat/raw/completions_annual.csv or -permissions / -completions on fetchspine)")
		}

		noPPD := sum.PPDAllPositive == 0 && sum.PPDAnyTypeMedianPositive == 0
		if noPPD {
			fmt.Println("PPD–HPI correlations: skipped (no PPD medians on spine)")
		} else {
			if r, n, err := housing.LogPPDVsHPICorrelation(obs); err != nil {
				fmt.Printf("log_ppd_all_vs_log_hpi_price: — (%v)\n", err)
			} else {
				fmt.Printf("log_ppd_all_vs_log_hpi_price: r=%.4f n=%d\n", r, n)
			}
			for _, typ := range []rune{'D', 'S', 'T', 'F'} {
				if r, n, err := housing.LogPPDTypeVsHPICorrelation(obs, typ); err != nil {
					fmt.Printf("log_ppd_type_%c_vs_log_hpi: — (%v)\n", typ, err)
				} else {
					fmt.Printf("log_ppd_type_%c_vs_log_hpi: r=%.4f n=%d\n", typ, r, n)
				}
			}
		}

		if sum.PermissionsPositive == 0 || sum.CompletionsPositive == 0 {
			fmt.Println("permissions↔completions lag: skipped (missing permissions or completions on spine)")
		} else if lag, r, n, err := housing.PermissionCompletionLagScan(obs, *lagMax); err != nil {
			fmt.Printf("permissions_vs_completions_lag: — (%v)\n", err)
		} else {
			fmt.Printf("permissions_vs_completions_log: best_lag_months=%d r=%.4f overlapping_pairs=%d\n", lag, r, n)
			fmt.Printf("pipeline_lag_diagnostic: permissions→completions peak correlation at lag=%d months (informative; not a fitted structural delay)\n", lag)
		}
	}

	if *validateMonths <= 0 {
		fmt.Println("\n(skip calibration: set -validate-months N for train/test RMSE)")
		return
	}

	trainN := len(obs) - *validateMonths
	if trainN < 2 {
		fmt.Fprintf(os.Stderr, "-validate-months %d leaves only %d training rows\n", *validateMonths, trainN)
		os.Exit(1)
	}

	grid := housing.CalibrateGrid{
		BankBetaLo: *bankLo, BankBetaHi: *bankHi, BankSteps: *bankSteps,
		PriceDriftLo: *driftLo, PriceDriftHi: *driftHi, PriceDriftSteps: *driftSteps,
		SupplyBetaLo: *supLo, SupplyBetaHi: *supHi, SupplySteps: *supSteps,
		DemandSupplyPressureBetaLo: *dspLo, DemandSupplyPressureBetaHi: *dspHi, DemandSupplySteps: *dspSteps,
		CompletionFracLo: *cfLo, CompletionFracHi: *cfHi, CompletionFracSteps: *cfSteps,
		EarningsDriftLo: *edLo, EarningsDriftHi: *edHi, EarningsDriftSteps: *edSteps,
	}

	if *noCred {
		fmt.Println("=== Temporal hold-out calibration (grid) ===")
	} else {
		fmt.Println("\n=== Temporal hold-out calibration (grid) ===")
	}
	trainStats, testStats, best, err := housing.ValidateCalibration(obs, base, grid, *wLogE, trainN)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("train_months=%d test_months=%d\n", trainN, *validateMonths)
	fmt.Printf("best bank_beta=%g price_drift=%g supply_beta=%g demand_supply_beta=%g completion_frac=%g earnings_drift=%g\n",
		best.BankBeta, best.PriceDrift, best.SupplyBeta, best.DemandSupplyPressureBeta, best.CompletionFrac, best.EarningsDrift)
	printStats("train", trainStats)
	printStats("test", testStats)
}

func printStats(label string, s housing.CalibrationStats) {
	fmt.Printf("%s: rmse_log_price=%g rmse_log_earnings=%g log_like_price=%g sigma_price=%g AIC=%g\n",
		label, s.RMSE_P, s.RMSE_E, s.LogLikeP, s.SigmaP, s.AIC)
}

func resolveArea(areaFlag, nameFlag string) (string, error) {
	if areaFlag != "" && nameFlag != "" {
		return "", fmt.Errorf("use only one of -area or -la")
	}
	if areaFlag != "" {
		return strings.TrimSpace(areaFlag), nil
	}
	if nameFlag == "" {
		return "", fmt.Errorf("required: -area or -la (or -list)")
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
	return "", fmt.Errorf("unknown LA %q", nameFlag)
}
