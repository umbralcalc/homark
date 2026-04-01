// Command fetchspine downloads UK HPI + BoE Official Bank Rate (+ optional DLUHC Table 122 ODS),
// then writes dat/processed/spine_monthly.csv for pilot LAs (pkg/ladata/targets.yaml).
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/umbralcalc/homark/pkg/ladata"
	"github.com/umbralcalc/homark/pkg/spine"
)

func main() {
	root := flag.String("root", ".", "repository root (directory containing go.mod)")
	rawDir := flag.String("raw", "dat/raw", "directory for downloaded files (under -root)")
	procDir := flag.String("processed", "dat/processed", "output directory (under -root)")
	skipDownload := flag.Bool("skip-download", false, "only use existing raw files for core inputs (still parses Table 122 if present)")
	skipSupplyODS := flag.Bool("skip-supply-download", false, "do not download Live Table 122 ODS")
	ukhpiURL := flag.String("ukhpi-url", envOr("UKHPI_URL", spine.DefaultUKHPIURL), "UK HPI full-file CSV URL")
	boeURL := flag.String("boe-url", envOr("BOE_URL", spine.DefaultBOEURL), "BoE IUDBEDR CSV URL")
	table122URL := flag.String("table122-url", envOr("TABLE122_URL", spine.DefaultLiveTable122URL), "DLUHC Live Table 122 ODS URL")
	onsPath := flag.String("ons", "", "optional path to ons_affordability.csv (area_code,year,median_ratio); default dat/raw/ons_affordability.csv if present")
	earningsPath := flag.String("earnings", "", "optional path to earnings_annual.csv (area_code,year,median_gross_annual_pay); default dat/raw/earnings_annual.csv if present")
	ppdPath := flag.String("ppd", "", "optional Land Registry Price Paid CSV; default dat/raw/price_paid.csv if present")
	nsplPath := flag.String("nspl", "", "optional NSPL-style CSV (pcds + lad*cd); default dat/raw/nspl.csv if present")
	fetchPPD := flag.Bool("fetch-ppd", false, "download full Price Paid CSV (very large; uses -ppd-url)")
	ppdURL := flag.String("ppd-url", envOr("PPD_CSV_URL", spine.DefaultPricePaidCSVURL), "Price Paid bulk CSV URL")
	onsCSVURL := flag.String("ons-csv-url", envOr("ONS_CSV_URL", ""), "if set, download CSV to dat/raw/ons_affordability.csv (you supply a direct export URL)")
	earningsCSVURL := flag.String("earnings-csv-url", envOr("EARNINGS_CSV_URL", ""), "if set, download to dat/raw/earnings_annual.csv")
	nsplZipURL := flag.String("nspl-zip-url", envOr("NSPL_ZIP_URL", ""), "if set, download zip and extract largest .csv to dat/raw/nspl.csv")
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

	raw := filepath.Join(repo, *rawDir)
	proc := filepath.Join(repo, *procDir)
	ukPath := filepath.Join(raw, "ukhpi_full.csv")
	boePath := filepath.Join(raw, "boe_bank_rate.csv")
	t122Path := filepath.Join(raw, "Live_Table_122.ods")
	outPath := filepath.Join(proc, "spine_monthly.csv")

	client := &http.Client{Timeout: 60 * time.Minute}

	if !*skipDownload {
		fmt.Println("Downloading BoE Official Bank Rate …")
		if err := spine.Download(client, *boeURL, boePath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("Downloading UK HPI …")
		if err := spine.Download(client, *ukhpiURL, ukPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if !*skipSupplyODS {
			fmt.Println("Downloading DLUHC Live Table 122 (net additional dwellings) …")
			if err := spine.Download(client, *table122URL, t122Path); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	} else {
		for _, p := range []string{ukPath, boePath} {
			if _, err := os.Stat(p); err != nil {
				fmt.Fprintf(os.Stderr, "missing %s (run without -skip-download)\n", p)
				os.Exit(1)
			}
		}
	}

	if *fetchPPD {
		dest := filepath.Join(raw, "price_paid.csv")
		fmt.Println("Downloading Land Registry Price Paid (full CSV, may be multi-GB) …")
		if err := spine.Download(client, *ppdURL, dest); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *onsCSVURL != "" {
		dest := filepath.Join(raw, "ons_affordability.csv")
		fmt.Println("Downloading ONS affordability CSV …")
		if err := spine.Download(client, *onsCSVURL, dest); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *earningsCSVURL != "" {
		dest := filepath.Join(raw, "earnings_annual.csv")
		fmt.Println("Downloading earnings CSV …")
		if err := spine.Download(client, *earningsCSVURL, dest); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *nsplZipURL != "" {
		zpath := filepath.Join(raw, "nspl_download.zip")
		fmt.Println("Downloading NSPL zip …")
		if err := spine.Download(client, *nsplZipURL, zpath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		nsplOut := filepath.Join(raw, "nspl.csv")
		fmt.Println("Extracting largest CSV from NSPL zip …")
		if err := spine.ExtractLargestCSVFromZip(zpath, nsplOut); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	targets, err := ladata.LoadTargets()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	codes := make(map[string]struct{}, len(targets))
	for _, a := range targets {
		codes[a.AreaCode] = struct{}{}
	}

	bank, err := spine.BOEMonthlyMeans(boePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	en := &spine.SpineEnrichment{}

	onsFile := *onsPath
	if onsFile == "" {
		def := filepath.Join(raw, "ons_affordability.csv")
		if _, err := os.Stat(def); err == nil {
			onsFile = def
		}
	}
	if onsFile != "" {
		en.MedianRatio, err = spine.LoadONSAnnual(onsFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	if _, err := os.Stat(t122Path); err == nil {
		en.SupplyNetFY, err = spine.ParseNetAdditionalTable122(t122Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Table 122 parse: %v (continuing without supply column)\n", err)
			en.SupplyNetFY = nil
		}
	} else if !*skipDownload && !*skipSupplyODS {
		fmt.Fprintf(os.Stderr, "warning: %s missing after download\n", t122Path)
	}

	earnFile := *earningsPath
	if earnFile == "" {
		def := filepath.Join(raw, "earnings_annual.csv")
		if _, err := os.Stat(def); err == nil {
			earnFile = def
		}
	}
	if earnFile != "" {
		en.EarningsAnnual, err = spine.LoadEarningsAnnual(earnFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	ppd := *ppdPath
	if ppd == "" {
		def := filepath.Join(raw, "price_paid.csv")
		if _, err := os.Stat(def); err == nil {
			ppd = def
		}
	}
	nspl := *nsplPath
	if nspl == "" {
		def := filepath.Join(raw, "nspl.csv")
		if _, err := os.Stat(def); err == nil {
			nspl = def
		}
	}
	if ppd != "" && nspl != "" {
		buckets, err := spine.AggregatePricePaid(ppd, nspl, codes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PPD aggregate: %v (continuing without PPD columns)\n", err)
		} else {
			en.PPDMonthly = spine.PPDBucketsToAgg(buckets)
		}
	}

	n, err := spine.BuildSpine(ukPath, codes, bank, en, outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %s rows=%d las=%d\n", outPath, n, len(codes))

	codesList := make([]string, 0, len(targets))
	for _, a := range targets {
		codesList = append(codesList, a.AreaCode)
	}
	cov, err := spine.MedianPayCoverageByArea(outPath, codesList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pay coverage: %v\n", err)
	} else {
		fmt.Println("median_gross_annual_pay coverage (non-zero / rows) by area:")
		for _, a := range targets {
			p := cov[a.AreaCode]
			pct := 100.0 * p.Fraction()
			fmt.Printf("  %s %s: %.1f%% (%d/%d)\n", a.AreaCode, a.Name, pct, p.WithPay, p.TotalRows)
		}
		if earnFile == "" && *earningsCSVURL == "" {
			fmt.Println("  (add dat/raw/earnings_annual.csv or -earnings-csv-url to populate pay; see README data section)")
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
