// Command calibratespine runs a coarse deterministic grid search on forward-spine
// coefficients to minimise RMSE of log price vs filled historical spine (see pkg/housing).
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
	list := flag.Bool("list", false, "list pilot LAs and exit")

	bankLo := flag.Float64("bank-beta-lo", -0.1, "grid low for bank_beta")
	bankHi := flag.Float64("bank-beta-hi", 0.1, "grid high for bank_beta")
	bankSteps := flag.Int("bank-steps", 21, "grid points for bank_beta (1 = use lo only when lo==hi)")

	driftLo := flag.Float64("price-drift-lo", 0, "grid low for price drift (0 with hi 0 = hold base)")
	driftHi := flag.Float64("price-drift-hi", 0, "grid high for price drift")
	driftSteps := flag.Int("price-drift-steps", 0, "grid points for price drift (0 = hold base)")

	supLo := flag.Float64("supply-beta-lo", 0, "grid low for supply_beta")
	supHi := flag.Float64("supply-beta-hi", 0, "grid high for supply_beta")
	supSteps := flag.Int("supply-steps", 0, "grid points (0 = hold base)")

	wLogE := flag.Float64("w-log-earnings", 0, "weight on log-earnings RMSE in objective (0 = log price only)")

	base := housing.DefaultForwardOptions()
	flag.Float64Var(&base.EarningsDrift, "earnings-drift", base.EarningsDrift, "fixed log-earnings drift")
	flag.Float64Var(&base.PriceDrift, "price-drift", base.PriceDrift, "base price drift when not on grid")
	flag.Float64Var(&base.SupplyBeta, "supply-beta", base.SupplyBeta, "base supply_beta when not on grid")
	flag.Float64Var(&base.DemandSupplyPressureBeta, "demand-supply-beta", base.DemandSupplyPressureBeta, "fixed demand–supply pressure beta (not on grid)")
	flag.Float64Var(&base.InitMedianRatioFallback, "init-median-ratio-fallback", 7, "first-month P/E fallback")
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

	grid := housing.CalibrateGrid{
		BankBetaLo: *bankLo, BankBetaHi: *bankHi, BankSteps: *bankSteps,
		PriceDriftLo: *driftLo, PriceDriftHi: *driftHi, PriceDriftSteps: *driftSteps,
		SupplyBetaLo: *supLo, SupplyBetaHi: *supHi, SupplySteps: *supSteps,
	}

	best, rmseP, rmseE, err := housing.GridCalibrateDeterministic(obs, base, grid, *wLogE)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("best bank_beta=%g price_drift=%g supply_beta=%g demand_supply_beta=%g\n", best.BankBeta, best.PriceDrift, best.SupplyBeta, best.DemandSupplyPressureBeta)
	fmt.Printf("rmse_log_price=%g rmse_log_earnings=%g\n", rmseP, rmseE)
	if *laName != "" {
		fmt.Printf("hint: go run ./cmd/forwardspine -la %q -bank-beta %g -price-drift %g -supply-beta %g -demand-supply-beta %g\n", *laName, best.BankBeta, best.PriceDrift, best.SupplyBeta, best.DemandSupplyPressureBeta)
	} else {
		fmt.Printf("hint: go run ./cmd/forwardspine -area %q -bank-beta %g -price-drift %g -supply-beta %g -demand-supply-beta %g\n", ac, best.BankBeta, best.PriceDrift, best.SupplyBeta, best.DemandSupplyPressureBeta)
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
