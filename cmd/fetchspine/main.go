// Command fetchspine downloads UK HPI + BoE Official Bank Rate and writes dat/processed/spine_monthly.csv
// for pilot local authorities (pkg/ladata/targets.yaml).
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
	rawDir := flag.String("raw", "dat/raw", "directory for downloaded CSVs (under -root)")
	procDir := flag.String("processed", "dat/processed", "output directory (under -root)")
	skipDownload := flag.Bool("skip-download", false, "only build spine from existing raw files")
	ukhpiURL := flag.String("ukhpi-url", envOr("UKHPI_URL", spine.DefaultUKHPIURL), "UK HPI full-file CSV URL")
	boeURL := flag.String("boe-url", envOr("BOE_URL", spine.DefaultBOEURL), "BoE IUDBEDR CSV URL")
	onsPath := flag.String("ons", "", "optional path to ons_affordability.csv (area_code,year,median_ratio); default dat/raw/ons_affordability.csv if that file exists")
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
	} else {
		if _, err := os.Stat(ukPath); err != nil {
			fmt.Fprintf(os.Stderr, "missing %s (run without -skip-download)\n", ukPath)
			os.Exit(1)
		}
		if _, err := os.Stat(boePath); err != nil {
			fmt.Fprintf(os.Stderr, "missing %s (run without -skip-download)\n", boePath)
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

	onsFile := *onsPath
	if onsFile == "" {
		def := filepath.Join(raw, "ons_affordability.csv")
		if _, err := os.Stat(def); err == nil {
			onsFile = def
		}
	}

	var ons spine.ONSAnnual
	if onsFile != "" {
		ons, err = spine.LoadONSAnnual(onsFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	n, err := spine.BuildSpine(ukPath, codes, bank, ons, outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %s rows=%d las=%d\n", outPath, n, len(codes))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
