package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"antigravity-seo/internal/api"
)

func runPSICmd(args []string) {
	fs := flag.NewFlagSet("psi", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	strategy := fs.String("strategy", "mobile", "mobile or desktop")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	targetURL := normalizeURL(positional[0])
	apiKey := os.Getenv("GOOGLE_API_KEY")

	report, err := api.RunPSI(context.Background(), targetURL, *strategy, apiKey)
	if errors.Is(err, api.ErrNoAPIKey) {
		fatal("GOOGLE_API_KEY not set.\n  1. Create a free key at https://console.cloud.google.com (PageSpeed Insights API enabled)\n  2. export GOOGLE_API_KEY=<key> (or add it to your shell profile)\n  3. Re-run: seo-engine psi %s", targetURL)
	}
	if err != nil {
		fatal("psi error: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== PAGESPEED INSIGHTS: %s (%s) ===\n", report.URL, report.Strategy)
	if report.FieldOverall != "" {
		fmt.Printf("\nField Data (CrUX, 28 days) — overall: %s\n", strings.ToUpper(report.FieldOverall))
		for _, m := range report.FieldMetrics {
			fmt.Printf("  %-18s p75=%-8d %s\n", m.Name, m.Percentile, m.Category)
		}
	} else {
		fmt.Println("\nField Data (CrUX): none (insufficient real-user traffic)")
	}

	if len(report.LabScores) > 0 {
		fmt.Println("\nLab Scores (Lighthouse):")
		for _, cat := range []string{"performance", "accessibility", "best-practices", "seo"} {
			if score, ok := report.LabScores[cat]; ok {
				fmt.Printf("  %-16s %d/100\n", cat, score)
			}
		}
	}
	if len(report.LabMetrics) > 0 {
		fmt.Println("\nLab Metrics:")
		for _, m := range report.LabMetrics {
			fmt.Printf("  %-26s %s\n", m.Name, m.DisplayValue)
		}
	}
	fmt.Println()
}

func runCruxCmd(args []string) {
	fs := flag.NewFlagSet("crux", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	history := fs.Bool("history", false, "Return the 25-week p75 history")
	formFactor := fs.String("form-factor", "", "PHONE, DESKTOP, or TABLET (default: all)")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL or origin required")
	}
	targetURL := normalizeURL(positional[0])
	apiKey := os.Getenv("GOOGLE_API_KEY")

	report, err := api.RunCrUX(context.Background(), targetURL, *formFactor, apiKey, *history)
	if errors.Is(err, api.ErrNoAPIKey) {
		fatal("GOOGLE_API_KEY not set.\n  1. Create a free key at https://console.cloud.google.com (Chrome UX Report API enabled)\n  2. export GOOGLE_API_KEY=<key>\n  3. Re-run: seo-engine crux %s", targetURL)
	}
	if err != nil {
		fatal("crux error: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== CHROME UX REPORT: %s ===\n", report.OriginOrURL)
	if *formFactor != "" {
		fmt.Printf("Form factor: %s\n", report.FormFactor)
	}

	if len(report.Metrics) == 0 && len(report.History) == 0 {
		fmt.Println("No CrUX data available for this origin/URL (insufficient traffic).")
		return
	}

	for _, m := range report.Metrics {
		fmt.Printf("  %-8s p75=%s %s\n", m.Name, m.P75, m.Unit)
	}
	if len(report.History) > 0 {
		fmt.Println("\n25-week history (p75 per week, most recent last):")
		for _, series := range report.History {
			if len(series) > 0 {
				fmt.Printf("  %s ... %s\n", series[0], series[len(series)-1])
			}
		}
	}
	fmt.Println()
}

func runIndexNowCmd(args []string) {
	fs := flag.NewFlagSet("indexnow", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	key := fs.String("key", "", "IndexNow key (default: INDEXNOW_KEY env)")
	keyLocation := fs.String("key-location", "", "URL where the key file is hosted")
	genKey := fs.Bool("gen-key", false, "Generate a key and write <key>.txt to the current directory, then exit")
	positional := parseFlags(fs, args)

	if *genKey {
		newKey, err := api.GenerateIndexNowKey()
		if err != nil {
			fatal("key generation failed: %v", err)
		}
		if err := os.WriteFile(newKey+".txt", []byte(newKey), 0o644); err != nil {
			fatal("cannot write key file: %v", err)
		}
		fmt.Printf("Generated key: %s\nWrote file:    %s.txt\n", newKey, newKey)
		fmt.Printf("\nNext steps:\n  1. Upload %s.txt to your site root so https://yoursite/%s.txt returns the key\n  2. export INDEXNOW_KEY=%s\n  3. seo-engine indexnow https://yoursite/page\n", newKey, newKey, newKey)
		return
	}

	if len(positional) < 1 {
		fatal("URL(s) required (space-separated), or --gen-key to bootstrap")
	}
	if *key == "" {
		*key = os.Getenv("INDEXNOW_KEY")
	}

	urls := make([]string, 0, len(positional))
	for i := 0; i < len(positional); i++ {
		urls = append(urls, normalizeURL(positional[i]))
	}

	report, err := api.SubmitIndexNow(context.Background(), urls, *key, *keyLocation)
	if err != nil {
		if errors.Is(err, api.ErrNoAPIKey) {
			fatal("INDEXNOW_KEY not set.\n  Run `seo-engine indexnow --gen-key` to bootstrap a key file, then export INDEXNOW_KEY=<key>")
		}
		fatal("indexnow error: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== INDEXNOW SUBMISSION ===\n")
	fmt.Printf("Host:    %s\n", report.Host)
	fmt.Printf("URLs:    %d\n", len(report.URLs))
	fmt.Printf("Status:  %d %s\n", report.Status, report.StatusText)
	if !report.Accepted {
		os.Exit(1)
	}
}
