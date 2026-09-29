package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"antigravity-seo/internal/audit"
	"antigravity-seo/internal/crawler"
)

// crawlAffectedShown is how many affected URLs each issue prints in human mode
const crawlAffectedShown = 5

func runCrawlCmd(args []string) {
	fs := flag.NewFlagSet("crawl", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	maxPages := fs.Int("max-pages", 100, "Maximum pages to crawl")
	concurrency := fs.Int("concurrency", 4, "Parallel fetches")
	sitemapURL := fs.String("sitemap", "", "Sitemap URL (default: auto-discover from the site root)")
	noSitemap := fs.Bool("no-sitemap", false, "Skip sitemap checks")
	maxSitemaps := fs.Int("max-sitemaps", 50, "Child sitemaps to fetch when following an index")
	timeoutSec := fs.Int("timeout", 15, "Timeout in seconds per request")
	failOn := addFailOnFlag(fs)
	positional := parseFlags(fs, args)
	checkFailOn(*failOn)

	if len(positional) < 1 {
		fatal("Start URL required (e.g. crawl https://example.com)")
	}
	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: time.Duration(*timeoutSec) * time.Second,
	})
	report, err := audit.AuditSite(context.Background(), client, normalizeURL(positional[0]), audit.SiteAuditOptions{
		MaxPages:    *maxPages,
		Concurrency: *concurrency,
		SitemapURL:  *sitemapURL,
		NoSitemap:   *noSitemap,
		MaxSitemaps: *maxSitemaps,
	})
	if err != nil {
		fatal("crawl error: %v", err)
	}

	if *asJSON {
		outputJSON(report)
	} else {
		printSiteReport(report)
	}

	severities := make([]string, 0, len(report.Issues))
	for _, is := range report.Issues {
		severities = append(severities, string(is.Severity))
	}
	exitIfFindings(*failOn, severities)
}

func printSiteReport(r *audit.SiteAuditReport) {
	fmt.Printf("\n=== SITE CRAWL AUDIT: %s ===\n", r.StartURL)
	fmt.Printf("Pages Crawled: %d\n", r.PagesCrawled)
	fmt.Printf("Truncated:     %s\n", yesNo(r.Truncated))
	fmt.Printf("Duration:      %dms\n", r.DurationMS)
	if sm := r.Sitemap; sm != nil {
		if sm.Error != "" {
			fmt.Printf("Sitemap:       %s (error: %s)\n", sm.URL, sm.Error)
		} else {
			line := fmt.Sprintf("Sitemap:       %s", sm.URL)
			if sm.DiscoveredFrom != "" {
				line += fmt.Sprintf(" (discovered from %s)", sm.DiscoveredFrom)
			}
			fmt.Printf("%s, %d URLs, complete: %s\n", line, sm.TotalURLs, yesNo(sm.Complete))
		}
	}
	fmt.Printf("\nSummary: %d critical, %d warning, %d info\n", r.Summary.Critical, r.Summary.Warning, r.Summary.Info)

	if len(r.Issues) == 0 {
		fmt.Println("\nNo site-wide issues found.")
	}
	for _, is := range r.Issues {
		fmt.Printf("\n[%s] %s (%d)\n", is.Severity, is.Message, is.Count)
		fmt.Printf("  Fix: %s\n", is.Fix)
		for i, a := range is.Affected {
			if i == crawlAffectedShown {
				break
			}
			fmt.Printf("  - %s\n", a.URL)
			if a.Detail != "" {
				fmt.Printf("      %s\n", a.Detail)
			}
			for _, from := range a.From {
				fmt.Printf("      ← %s\n", from)
			}
		}
		if is.Count > crawlAffectedShown {
			fmt.Printf("  … and %d more\n", is.Count-crawlAffectedShown)
		}
	}

	if len(r.Notes) > 0 {
		fmt.Printf("\nNOTES:\n")
		for _, n := range r.Notes {
			fmt.Printf("  * %s\n", n)
		}
	}
	fmt.Println()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
