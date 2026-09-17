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

func runGSCCmd(args []string) {
	if len(args) < 1 {
		printGSCUsage()
		os.Exit(1)
	}

	sub := args[0]
	switch sub {
	case "query":
		runGSCQuery(args[1:])
	case "inspect":
		runGSCInspect(args[1:])
	case "help", "-h", "--help":
		printGSCUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown gsc subcommand: %s\n\n", sub)
		printGSCUsage()
		os.Exit(1)
	}
}

func printGSCUsage() {
	fmt.Println(`Google Search Console (GSC) Commands:
  query      Query Search Analytics (clicks, impressions, CTR, position)
  inspect    Inspect URL indexation, crawl status, and canonical verdict

AUTHENTICATION:
  Set GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json
  Or set GSC_ACCESS_TOKEN=<oauth-access-token>

USAGE:
  seo-engine gsc query <site-url> [flags]
  seo-engine gsc inspect <site-url> <page-url> [flags]

FLAGS:
  --start-date   Start date (YYYY-MM-DD, default: 28 days ago)
  --end-date     End date (YYYY-MM-DD, default: 2 days ago)
  --dimensions   Comma-separated: query, page, device, country, date (default: query)
  --limit        Row limit (default: 25, max: 25000)
  --json         Output JSON format

EXAMPLES:
  seo-engine gsc query sc-domain:example.com --dimensions query,page --limit 20
  seo-engine gsc inspect https://example.com/ https://example.com/pricing`)
}

func runGSCQuery(args []string) {
	fs := flag.NewFlagSet("gsc query", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	startDate := fs.String("start-date", "", "Start date (YYYY-MM-DD)")
	endDate := fs.String("end-date", "", "End date (YYYY-MM-DD)")
	dimsStr := fs.String("dimensions", "query", "Comma-separated dimensions")
	limit := fs.Int("limit", 25, "Row limit")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("site-url required (e.g. sc-domain:example.com or https://example.com/)")
	}
	siteURL := positional[0]

	token, err := api.ResolveGSCToken(context.Background())
	if err != nil {
		if errors.Is(err, api.ErrNoGSCAuth) {
			fatal("GSC authentication not found.\n  1. Set GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json\n  2. Or set GSC_ACCESS_TOKEN=<token>\n  3. Re-run: seo-engine gsc query %s", siteURL)
		}
		fatal("GSC auth error: %v", err)
	}

	var dims []string
	for _, d := range strings.Split(*dimsStr, ",") {
		if s := strings.TrimSpace(d); s != "" {
			dims = append(dims, s)
		}
	}

	report, err := api.QuerySearchAnalytics(context.Background(), token, api.GSCQueryOptions{
		SiteURL:    siteURL,
		StartDate:  *startDate,
		EndDate:    *endDate,
		Dimensions: dims,
		RowLimit:   *limit,
	})
	if err != nil {
		fatal("GSC query failed: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== GSC SEARCH ANALYTICS: %s ===\n", report.SiteURL)
	fmt.Printf("Range:      %s to %s\n", report.StartDate, report.EndDate)
	fmt.Printf("Dimensions: %s\n", strings.Join(report.Dimensions, ", "))
	fmt.Printf("Total rows: %d\n\n", report.TotalRows)

	if report.TotalRows == 0 {
		fmt.Println("No Search Analytics data returned for this query and time range.")
		return
	}

	fmt.Printf("%-35s %8s %12s %8s %8s\n", "KEYS", "CLICKS", "IMPRESSIONS", "CTR", "POSITION")
	fmt.Println(strings.Repeat("-", 75))
	for _, r := range report.Rows {
		keyStr := strings.Join(r.Keys, " | ")
		if len(keyStr) > 35 {
			keyStr = keyStr[:32] + "..."
		}
		fmt.Printf("%-35s %8.0f %12.0f %7.2f%% %8.1f\n", keyStr, r.Clicks, r.Impressions, r.CTR*100, r.Position)
	}
	fmt.Println()
}

func runGSCInspect(args []string) {
	fs := flag.NewFlagSet("gsc inspect", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	positional := parseFlags(fs, args)

	if len(positional) < 2 {
		fatal("both <site-url> and <page-url> required\n  Usage: seo-engine gsc inspect <site-url> <page-url>")
	}
	siteURL := positional[0]
	pageURL := normalizeURL(positional[1])

	token, err := api.ResolveGSCToken(context.Background())
	if err != nil {
		if errors.Is(err, api.ErrNoGSCAuth) {
			fatal("GSC authentication not found.\n  1. Set GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json\n  2. Or set GSC_ACCESS_TOKEN=<token>\n  3. Re-run: seo-engine gsc inspect %s %s", siteURL, pageURL)
		}
		fatal("GSC auth error: %v", err)
	}

	report, err := api.InspectURL(context.Background(), token, api.GSCInspectOptions{
		SiteURL:       siteURL,
		InspectionURL: pageURL,
	})
	if err != nil {
		fatal("GSC inspection failed: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== GSC URL INSPECTION ===\n")
	fmt.Printf("URL:              %s\n", report.InspectionURL)
	fmt.Printf("Verdict:          %s\n", report.Verdict)
	fmt.Printf("Coverage State:   %s\n", report.CoverageState)
	fmt.Printf("Robots.txt:       %s\n", report.RobotsTxtState)
	fmt.Printf("Indexing State:   %s\n", report.IndexingState)
	fmt.Printf("Last Crawl:       %s\n", report.LastCrawlTime)
	fmt.Printf("Page Fetch:       %s\n", report.PageFetchState)
	fmt.Printf("Google Canonical: %s\n", report.GoogleCanonical)
	fmt.Printf("User Canonical:   %s\n", report.UserCanonical)
	if len(report.ReferringURLs) > 0 {
		fmt.Printf("Referring URLs:   %s\n", strings.Join(report.ReferringURLs, ", "))
	}
	if report.MobileUsabilityVerdict != "" {
		fmt.Printf("Mobile Usability: %s\n", report.MobileUsabilityVerdict)
	}
	if report.RichResultsVerdict != "" {
		fmt.Printf("Rich Results:     %s\n", report.RichResultsVerdict)
	}
	fmt.Println()
}
