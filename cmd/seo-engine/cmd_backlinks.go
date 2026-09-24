package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"antigravity-seo/internal/api"
)

func runBacklinksCmd(args []string) {
	fs := flag.NewFlagSet("backlinks", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	limit := fs.Int("limit", 50, "Maximum number of records to return (1-500)")
	crawlID := fs.String("crawl", "", "Specific Common Crawl collection ID (e.g. CC-MAIN-2026-34)")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("domain or URL argument required\n  Usage: seo-engine backlinks <domain> [--limit 50] [--json]\n  Note: returns Common Crawl's capture index for this domain (archived URLs on this domain) — not inbound links from other sites.")
	}
	targetDomain := positional[0]

	report, err := api.QueryCommonCrawlBacklinks(context.Background(), targetDomain, *limit, *crawlID)
	if err != nil {
		fatal("backlinks query failed: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== COMMON CRAWL CAPTURE INDEX: %s ===\n", report.Domain)
	fmt.Println("(archived URLs on this domain — not inbound links from other sites)")
	fmt.Printf("Crawl Archive: %s\n", report.CrawlIndex)
	fmt.Printf("Total Records: %d (Unique URLs: %d)\n", report.TotalFound, report.UniqueURLs)

	if len(report.MimeBreakdown) > 0 {
		var mimeParts []string
		for m, count := range report.MimeBreakdown {
			mimeParts = append(mimeParts, fmt.Sprintf("%s: %d", m, count))
		}
		fmt.Printf("Content Types: %s\n", strings.Join(mimeParts, ", "))
	}
	if len(report.Languages) > 0 {
		var langParts []string
		for l, count := range report.Languages {
			langParts = append(langParts, fmt.Sprintf("%s: %d", l, count))
		}
		fmt.Printf("Languages:     %s\n", strings.Join(langParts, ", "))
	}
	fmt.Println()

	if report.TotalFound == 0 {
		fmt.Println("No crawl records found in this Common Crawl index for this domain.")
		return
	}

	fmt.Printf("%-16s %-6s %-20s %s\n", "TIMESTAMP", "STATUS", "MIME", "URL")
	fmt.Println(strings.Repeat("-", 80))
	for _, r := range report.Records {
		mimeShort := r.Mime
		if len(mimeShort) > 20 {
			mimeShort = mimeShort[:17] + "..."
		}
		fmt.Printf("%-16s %-6s %-20s %s\n", r.Timestamp, r.Status, mimeShort, r.URL)
	}
	fmt.Println()
}
