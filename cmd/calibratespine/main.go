// Command calibratespine runs a deterministic grid search or ES optimisation on forward-spine
// coefficients to minimise RMSE / log-likelihood of log price vs filled historical spine.
// Add -validate-months to hold out the most recent N months for out-of-sample scoring.
// Add -laplace to compute Gaussian posterior variance via the diagonal Hessian at the optimum.
// Add -es-steps N to run the Evolution Strategy optimiser (uses analysis.NewEvolutionStrategyOptimisationPartitions).
package main

import (
	"flag"
	"fmt"
	"math"
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

	dspLo := flag.Float64("demand-supply-beta-lo", 0, "grid low for DemandSupplyPressureBeta")
	dspHi := flag.Float64("demand-supply-beta-hi", 0, "grid high for DemandSupplyPressureBeta")
	dspSteps := flag.Int("demand-supply-steps", 0, "grid points (0 = use -demand-supply-beta base only)")

	cfLo := flag.Float64("completion-frac-lo", 0, "grid low for completion_frac (pipeline fraction completing per month)")
	cfHi := flag.Float64("completion-frac-hi", 0, "grid high for completion_frac")
	cfSteps := flag.Int("completion-frac-steps", 0, "grid points for completion_frac (0 = hold base)")

	edLo := flag.Float64("earnings-drift-lo", 0, "grid low for earnings_drift")
	edHi := flag.Float64("earnings-drift-hi", 0, "grid high for earnings_drift")
	edSteps := flag.Int("earnings-drift-steps", 0, "grid points for earnings_drift (0 = hold base)")

	wLogE := flag.Float64("w-log-earnings", 0, "weight on log-earnings RMSE in objective (0 = log price only; try ~0.1–1 for joint fit)")

	validateMonths := flag.Int("validate-months", 0, "hold out the most recent N months for out-of-sample evaluation (0 = disabled)")
	laplace := flag.Bool("laplace", false, "compute Gaussian posterior variance at optimum via numerical Hessian diagonal")

	// Evolution Strategy flags — when -es-steps > 0 the ES is used instead of grid search.
	esSteps := flag.Int("es-steps", 0, "run ES optimiser for N outer steps (0 = grid search; suggested: 400)")
	esCollection := flag.Int("es-collection-size", 0, "ES samples per update (0 = default 20)")
	esSeed := flag.Uint64("es-seed", 0, "ES sampler seed (0 = default 42)")
	esMeanLR := flag.Float64("es-mean-lr", 0, "ES mean learning rate (0 = default 0.5)")
	esCovLR := flag.Float64("es-cov-lr", 0, "ES covariance learning rate (0 = default 0.2)")

	base := housing.DefaultForwardOptions()
	flag.Float64Var(&base.EarningsDrift, "earnings-drift", base.EarningsDrift, "base log-earnings drift (when not on grid)")
	flag.Float64Var(&base.PriceDrift, "price-drift", base.PriceDrift, "base price drift when not on grid")
	flag.Float64Var(&base.SupplyBeta, "supply-beta", base.SupplyBeta, "base supply_beta when not on grid")
	flag.Float64Var(&base.DemandSupplyPressureBeta, "demand-supply-beta", base.DemandSupplyPressureBeta, "DemandSupplyPressureBeta when demand-supply-steps <= 0")
	flag.Float64Var(&base.CompletionFrac, "completion-frac", base.CompletionFrac, "base completion_frac when not on grid")
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
		DemandSupplyPressureBetaLo: *dspLo, DemandSupplyPressureBetaHi: *dspHi, DemandSupplySteps: *dspSteps,
		CompletionFracLo: *cfLo, CompletionFracHi: *cfHi, CompletionFracSteps: *cfSteps,
		EarningsDriftLo: *edLo, EarningsDriftHi: *edHi, EarningsDriftSteps: *edSteps,
	}

	// ES optimisation path — replaces grid search when -es-steps > 0.
	if *esSteps > 0 {
		esOpt := housing.ESOptions{
			Steps:          *esSteps,
			CollectionSize: *esCollection,
			Seed:           *esSeed,
			MeanLearningRate: *esMeanLR,
			CovLearningRate:  *esCovLR,
		}
		res, stats, err := housing.ESCalibrateStats(obs, base, esOpt)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		best := res.Best
		fmt.Printf("es best bank_beta=%g price_drift=%g supply_beta=%g demand_supply_beta=%g completion_frac=%g earnings_drift=%g\n",
			best.BankBeta, best.PriceDrift, best.SupplyBeta, best.DemandSupplyPressureBeta, best.CompletionFrac, best.EarningsDrift)
		fmt.Printf("es theta_mean=%v\n", res.ThetaMean)
		printStats("es", stats)
		printHint(*laName, ac, best)
		return
	}

	// Temporal validation: calibrate on train, score on test.
	if *validateMonths > 0 {
		trainN := len(obs) - *validateMonths
		if trainN < 2 {
			fmt.Fprintf(os.Stderr, "-validate-months %d leaves only %d training rows\n", *validateMonths, trainN)
			os.Exit(1)
		}
		trainStats, testStats, best, err := housing.ValidateCalibration(obs, base, grid, *wLogE, trainN)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("validation: train_months=%d test_months=%d\n", trainN, *validateMonths)
		fmt.Printf("best bank_beta=%g price_drift=%g supply_beta=%g demand_supply_beta=%g completion_frac=%g earnings_drift=%g\n",
			best.BankBeta, best.PriceDrift, best.SupplyBeta, best.DemandSupplyPressureBeta, best.CompletionFrac, best.EarningsDrift)
		printStats("train", trainStats)
		printStats("test", testStats)
		printHint(*laName, ac, best)
		if *laplace {
			printLaplace(obs[:trainN], best, grid)
		}
		return
	}

	best, rmseP, rmseE, err := housing.GridCalibrateDeterministic(obs, base, grid, *wLogE)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("best bank_beta=%g price_drift=%g supply_beta=%g demand_supply_beta=%g completion_frac=%g earnings_drift=%g\n",
		best.BankBeta, best.PriceDrift, best.SupplyBeta, best.DemandSupplyPressureBeta, best.CompletionFrac, best.EarningsDrift)
	fmt.Printf("rmse_log_price=%g rmse_log_earnings=%g\n", rmseP, rmseE)

	nFree := countActiveGridDims(grid)
	stats, err := housing.ComputeCalibrationStats(obs, best, nFree)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stats:", err)
	} else {
		fmt.Printf("log_like_price=%g sigma_price=%g log_like_earnings=%g sigma_earnings=%g AIC=%g\n",
			stats.LogLikeP, stats.SigmaP, stats.LogLikeE, stats.SigmaE, stats.AIC)
	}

	printHint(*laName, ac, best)

	if *laplace {
		printLaplace(obs, best, grid)
	}
}

func printStats(label string, s housing.CalibrationStats) {
	fmt.Printf("%s: rmse_log_price=%g rmse_log_earnings=%g log_like_price=%g sigma_price=%g AIC=%g\n",
		label, s.RMSE_P, s.RMSE_E, s.LogLikeP, s.SigmaP, s.AIC)
}

func printHint(laName, ac string, best housing.ForwardOptions) {
	if laName != "" {
		fmt.Printf("hint: go run ./cmd/forwardspine -la %q -bank-beta %g -price-drift %g -supply-beta %g -demand-supply-beta %g -completion-frac %g\n",
			laName, best.BankBeta, best.PriceDrift, best.SupplyBeta, best.DemandSupplyPressureBeta, best.CompletionFrac)
	} else {
		fmt.Printf("hint: go run ./cmd/forwardspine -area %q -bank-beta %g -price-drift %g -supply-beta %g -demand-supply-beta %g -completion-frac %g\n",
			ac, best.BankBeta, best.PriceDrift, best.SupplyBeta, best.DemandSupplyPressureBeta, best.CompletionFrac)
	}
}

// printLaplace computes and prints Gaussian posterior variance estimates for active grid dimensions.
func printLaplace(obs []spine.MonthlyObservation, best housing.ForwardOptions, grid housing.CalibrateGrid) {
	perturbs := make(map[string]float64)
	if grid.BankSteps > 1 {
		perturbs["bank_beta"] = math.Max(math.Abs(best.BankBeta)*0.01, 1e-4)
	}
	if grid.PriceDriftSteps > 1 {
		perturbs["price_drift"] = math.Max(math.Abs(best.PriceDrift)*0.01, 1e-6)
	}
	if grid.SupplySteps > 1 {
		perturbs["supply_beta"] = math.Max(math.Abs(best.SupplyBeta)*0.01, 1e-6)
	}
	if grid.DemandSupplySteps > 1 {
		perturbs["demand_supply_beta"] = math.Max(math.Abs(best.DemandSupplyPressureBeta)*0.01, 1e-5)
	}
	if grid.CompletionFracSteps > 1 {
		perturbs["completion_frac"] = math.Max(math.Abs(best.CompletionFrac)*0.01, 1e-4)
	}
	if grid.EarningsDriftSteps > 1 {
		perturbs["earnings_drift"] = math.Max(math.Abs(best.EarningsDrift)*0.01, 1e-6)
	}
	if len(perturbs) == 0 {
		fmt.Println("laplace: no active grid dimensions to perturb")
		return
	}
	variances, err := housing.LaplaceLogLikeVariances(obs, best, perturbs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "laplace:", err)
		return
	}
	for name, v := range variances {
		if math.IsInf(v, 1) {
			fmt.Printf("laplace: %s posterior_std=Inf (flat or ascending curvature at optimum)\n", name)
		} else {
			fmt.Printf("laplace: %s posterior_std=%g\n", name, math.Sqrt(v))
		}
	}
}

func countActiveGridDims(grid housing.CalibrateGrid) int {
	n := 0
	for _, s := range []int{grid.BankSteps, grid.PriceDriftSteps, grid.SupplySteps, grid.DemandSupplySteps, grid.CompletionFracSteps, grid.EarningsDriftSteps} {
		if s > 1 {
			n++
		}
	}
	return n
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
