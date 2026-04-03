// Command policyscenario runs a Cartesian grid of planning-adjacent scenarios on the forward spine:
// approvals × bank-rate scaling × optional completion_frac, market-fraction, and flat-share (density-mix) grids,
// with optional posterior samples of ES theta.
// Write ES JSON with: go run ./cmd/calibratespine -la "Leeds" -es-steps 400 -es-json-out posteriors/leeds.json
// Then: go run ./cmd/policyscenario -la "Leeds" -posterior posteriors/leeds.json -approvals 0,80,160 -bank-scales 1,1.1 -posterior-samples 50
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
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

	approvalsStr := flag.String("approvals", "0", "comma-separated approval rates (dwellings/month into deterministic pipeline when spine has no permissions column)")
	bankScalesStr := flag.String("bank-scales", "1", "comma-separated multipliers applied to historical bank_rate_pct on the spine")
	completionFracsStr := flag.String("completion-fracs", "", "optional comma-separated completion_frac per scenario (omit = use -completion-frac or theta from -posterior)")
	marketFracsStr := flag.String("market-fractions", "1", "comma-separated market delivery fraction 0–1 (scales inflow: tenure/affordable stylisation)")
	flatSharesStr := flag.String("flat-shares", "0.5", "comma-separated composition flat-share 0–1 (neutral 0.5; used with -composition-drift-beta)")
	posteriorPath := flag.String("posterior", "", "JSON from calibratespine -es-json-out (theta_mean, theta_cov); omit to use -bank-beta … flags only")
	posteriorSamples := flag.Int("posterior-samples", 0, "if >0 and -posterior has theta_cov, draw this many theta samples per scenario cell; 0 = one run using theta_mean (or flags)")

	seed := flag.Uint64("seed", 42, "RNG seed for posterior sampling")

	base := housing.DefaultForwardOptions()
	flag.Float64Var(&base.SupplyScale, "supply-scale", base.SupplyScale, "supply scale for net additions term")
	flag.Float64Var(&base.PipelineRef, "pipeline-ref", base.PipelineRef, "pipeline reference stock")
	flag.Float64Var(&base.PipelineBeta, "pipeline-beta", base.PipelineBeta, "pipeline dampening on log-price drift")
	flag.Float64Var(&base.PipelineInit, "pipeline-init", base.PipelineInit, "initial pipeline stock")
	flag.Float64Var(&base.InitMedianRatioFallback, "init-median-ratio-fallback", 7, "first-month P/E fallback")
	// Theta components when -posterior is omitted (same six as ES / grid calibration).
	flag.Float64Var(&base.BankBeta, "bank-beta", base.BankBeta, "bank_beta")
	flag.Float64Var(&base.PriceDrift, "price-drift", base.PriceDrift, "price drift base")
	flag.Float64Var(&base.SupplyBeta, "supply-beta", base.SupplyBeta, "supply_beta")
	flag.Float64Var(&base.DemandSupplyPressureBeta, "demand-supply-beta", base.DemandSupplyPressureBeta, "demand_supply_pressure beta")
	flag.Float64Var(&base.CompletionFrac, "completion-frac", base.CompletionFrac, "completion_frac (overridden by theta when using -posterior)")
	flag.Float64Var(&base.EarningsDrift, "earnings-drift", base.EarningsDrift, "earnings drift")
	flag.Float64Var(&base.CompositionDriftBeta, "composition-drift-beta", base.CompositionDriftBeta, "density-mix lever: adds beta×(flat_share−0.5) to log-price drift; flat_share from -flat-shares grid")
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

	approvals, err := parseCommaFloats(*approvalsStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "-approvals:", err)
		os.Exit(1)
	}
	bankScales, err := parseCommaFloats(*bankScalesStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "-bank-scales:", err)
		os.Exit(1)
	}
	var completionFracs []float64
	if strings.TrimSpace(*completionFracsStr) != "" {
		completionFracs, err = parseCommaFloats(*completionFracsStr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "-completion-fracs:", err)
			os.Exit(1)
		}
	} else {
		completionFracs = []float64{math.NaN()}
	}
	marketFracs, err := parseCommaFloats(*marketFracsStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "-market-fractions:", err)
		os.Exit(1)
	}
	flatShares, err := parseCommaFloats(*flatSharesStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "-flat-shares:", err)
		os.Exit(1)
	}

	var post housing.PosteriorCalibrationJSON
	if strings.TrimSpace(*posteriorPath) != "" {
		post, err = housing.ReadPosteriorCalibrationJSON(*posteriorPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "-posterior:", err)
			os.Exit(1)
		}
	}

	nDraws := 1
	useSample := false
	if *posteriorSamples > 0 && len(post.ThetaCov) == housing.ThetaDim*housing.ThetaDim {
		nDraws = *posteriorSamples
		useSample = true
	} else if *posteriorSamples > 0 && len(post.ThetaMean) > 0 && len(post.ThetaCov) == 0 {
		fmt.Fprintln(os.Stderr, "policyscenario: -posterior-samples ignored (no theta_cov in JSON); using theta_mean once per cell")
	}

	rng := rand.New(rand.NewSource(int64(*seed)))

	w := csv.NewWriter(os.Stdout)
	_ = w.Write([]string{"sample_idx", "approval_rate", "bank_scale", "completion_frac", "market_fraction", "flat_share", "mean_afford", "last_afford", "n_steps"})
	for draw := 0; draw < nDraws; draw++ {
		var theta []float64
		switch {
		case len(post.ThetaMean) == housing.ThetaDim && useSample:
			theta = housing.SampleThetaGaussian(post.ThetaMean, post.ThetaCov, rng)
		case len(post.ThetaMean) == housing.ThetaDim:
			theta = append([]float64(nil), post.ThetaMean...)
		default:
			theta = housing.ThetaFromOptions(base)
		}
		optBase := housing.OptionsFromTheta(base, theta)
		for _, appr := range approvals {
			for _, bscale := range bankScales {
				for _, cfp := range completionFracs {
					for _, mf := range marketFracs {
						for _, fs := range flatShares {
							opt := optBase
							opt.ApprovalRate = appr
							if !math.IsNaN(cfp) {
								opt.CompletionFrac = cfp
							}
							opt.MarketDeliveryFraction = mf
							opt.CompositionFlatShare = fs
							do := housing.DeterministicForwardOptions(opt)
							scen := housing.CloneMonthlyObservations(obs)
							housing.ScaleBankRatePct(scen, bscale)
							_, series, err := housing.RunForwardLogSeries(scen, do)
							if err != nil {
								fmt.Fprintln(os.Stderr, err)
								os.Exit(1)
							}
							aff, ok := series["affordability"]
							if !ok {
								fmt.Fprintln(os.Stderr, "missing affordability series")
								os.Exit(1)
							}
							meanA, lastA, n := housing.AffordabilityPathStats(aff)
							_ = w.Write([]string{
								strconv.Itoa(draw),
								strconv.FormatFloat(appr, 'g', -1, 64),
								strconv.FormatFloat(bscale, 'g', -1, 64),
								strconv.FormatFloat(opt.CompletionFrac, 'g', -1, 64),
								strconv.FormatFloat(mf, 'g', -1, 64),
								strconv.FormatFloat(fs, 'g', -1, 64),
								strconv.FormatFloat(meanA, 'g', -1, 64),
								strconv.FormatFloat(lastA, 'g', -1, 64),
								strconv.Itoa(n),
							})
						}
					}
				}
			}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseCommaFloats(s string) ([]float64, error) {
	parts := strings.Split(s, ",")
	var out []float64
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", p, err)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no values")
	}
	return out, nil
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
