package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"antigravity-seo/internal/audit"
	"antigravity-seo/internal/crawler"
)

func fetchTarget(timeoutSec int, targetURL string) (*crawler.FetchResult, *crawler.SafeClient) {
	targetURL = normalizeURL(targetURL)
	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: time.Duration(timeoutSec) * time.Second,
	})
	res, err := client.Fetch(context.Background(), targetURL)
	if err != nil {
		fatal("fetch error: %v", err)
	}
	return res, client
}

func runHeadersCmd(args []string) {
	fs := flag.NewFlagSet("headers", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	timeoutSec := fs.Int("timeout", 15, "Timeout in seconds")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	res, _ := fetchTarget(*timeoutSec, positional[0])
	result := audit.InspectHeaders(res)

	if *asJSON {
		outputJSON(result)
		return
	}

	fmt.Printf("\n=== HTTP TRANSPORT & HEADERS AUDIT: %s ===\n", result.URL)
	fmt.Printf("Status:           %d %s\n", result.StatusCode, res.StatusText)
	fmt.Printf("Final URL:        %s\n", result.FinalURL)
	fmt.Printf("Redirected:       %t (Hops: %d)\n", result.IsRedirected, result.RedirectHops)
	if result.IsRedirected {
		for i, hop := range result.RedirectChain {
			fmt.Printf("  #%d: %s -> %d\n", i+1, hop.URL, hop.StatusCode)
		}
	}
	if len(res.Redirects) > 0 {
		fmt.Println("Hop Timings:")
		for i, hop := range res.Redirects {
			fmt.Printf("  #%d: %s  dns=%dms tcp=%dms tls=%dms ttfb=%dms total=%dms\n",
				i+1, hop.URL, hop.Timings.DNSLookupMS, hop.Timings.TCPConnectMS, hop.Timings.TLSHandshakeMS, hop.Timings.TTFBMS, hop.Timings.TotalMS)
		}
		fmt.Printf("  final: %s  dns=%dms tcp=%dms tls=%dms ttfb=%dms total=%dms\n",
			res.FinalURL, res.Timings.DNSLookupMS, res.Timings.TCPConnectMS, res.Timings.TLSHandshakeMS, res.Timings.TTFBMS, res.Timings.TotalMS)
		fmt.Printf("  chain total elapsed: %dms\n", res.TotalElapsedMS)
	}
	if res.Charset != "" {
		fmt.Printf("Charset:          %s\n", res.Charset)
	}
	if res.Truncated {
		fmt.Printf("Body Truncated:   true (capped at %d bytes)\n", res.BodySize)
	}
	fmt.Printf("X-Robots-Tag:     %s (Noindex: %t, Nofollow: %t)\n", result.XRobotsTag, result.HasNoindex, result.HasNofollow)
	if result.HeaderCanonical != "" {
		fmt.Printf("Header Canonical: %s\n", result.HeaderCanonical)
	}
	fmt.Printf("Compression:      %s\n", result.ContentEncoding)
	fmt.Printf("HSTS Active:      %t\n", result.HSTS)
	fmt.Printf("TTFB:             %dms (Total: %dms)\n", result.TTFBMS, result.TotalDurationMS)

	fmt.Println("\n--- DIAGNOSTIC ISSUES ---")
	for _, issue := range result.Issues {
		badge := fmt.Sprintf("[%s]", issue.Severity)
		fmt.Printf(" %-10s %-12s: %s\n", badge, issue.Category, issue.Message)
		if issue.Details != "" {
			fmt.Printf("              %s\n", issue.Details)
		}
	}
	fmt.Println()
}

func runAuditCmd(args []string) {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	timeoutSec := fs.Int("timeout", 15, "Timeout in seconds")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	targetURL := normalizeURL(positional[0])
	res, _ := fetchTarget(*timeoutSec, targetURL)

	hdrAudit := audit.InspectHeaders(res)
	htmlAudit, err := audit.InspectHTML(targetURL, res.Body)
	if err != nil {
		fatal("HTML audit error: %v", err)
	}
	schemaAudit := audit.InspectSchema(targetURL, res.Body)

	if *asJSON {
		outputJSON(map[string]interface{}{
			"headers":   hdrAudit,
			"technical": htmlAudit,
			"schema":    schemaAudit,
		})
		return
	}

	fmt.Printf("\n=======================================================\n")
	fmt.Printf(" ANTIGRAVITY ON-PAGE TECHNICAL SEO AUDIT\n")
	fmt.Printf(" Target: %s\n", targetURL)
	fmt.Printf("=======================================================\n")

	fmt.Printf("Scores:\n")
	fmt.Printf("  Technical On-Page: %d/100\n", htmlAudit.Score)
	fmt.Printf("  Structured Data:   %d/100\n", schemaAudit.Score)
	fmt.Printf("  HTTP Transport:    %dms TTFB (%d %s)\n", hdrAudit.TTFBMS, hdrAudit.StatusCode, res.StatusText)

	fmt.Println("\nMeta Information:")
	fmt.Printf("  Title (%d chars):       %s\n", htmlAudit.TitleLength, htmlAudit.Title)
	fmt.Printf("  Description (%d chars): %s\n", htmlAudit.MetaDescLength, htmlAudit.MetaDescription)
	fmt.Printf("  Canonical:             %s (Self: %t)\n", htmlAudit.Canonical, htmlAudit.IsSelfCanonical)
	fmt.Printf("  Mobile Viewport:       %t\n", htmlAudit.HasViewport)
	fmt.Printf("  Robots Noindex:        %t\n", htmlAudit.HasMetaNoindex || hdrAudit.HasNoindex)

	fmt.Println("\nContent Structure:")
	fmt.Printf("  H1 Headings (%d):      %s\n", htmlAudit.H1Count, strings.Join(htmlAudit.H1Text, " | "))
	fmt.Printf("  H2 Subheadings:        %d\n", htmlAudit.H2Count)
	fmt.Printf("  Internal Links:        %d\n", htmlAudit.InternalLinks)
	fmt.Printf("  External Links:        %d\n", htmlAudit.ExternalLinks)
	fmt.Printf("  Images:                %d (%d missing alt)\n", htmlAudit.TotalImages, htmlAudit.ImagesMissingAlt)

	fmt.Println("\nStructured Data (JSON-LD):")
	fmt.Printf("  Blocks Found:          %d\n", schemaAudit.BlocksCount)
	fmt.Printf("  Schema Types:          %s\n", strings.Join(schemaAudit.TypesFound, ", "))

	fmt.Println("\nActionable Diagnostics:")
	allIssues := append(hdrAudit.Issues, htmlAudit.Issues...)
	for _, b := range schemaAudit.Blocks {
		for _, e := range b.Errors {
			allIssues = append(allIssues, audit.AuditIssue{
				Severity: audit.SeverityCritical,
				Category: "Schema",
				Message:  e,
			})
		}
		for _, w := range b.Warnings {
			allIssues = append(allIssues, audit.AuditIssue{
				Severity: audit.SeverityWarning,
				Category: "Schema",
				Message:  w,
			})
		}
	}

	for _, issue := range allIssues {
		badge := fmt.Sprintf("[%s]", issue.Severity)
		fmt.Printf(" %-10s %-15s: %s\n", badge, issue.Category, issue.Message)
	}
	fmt.Println()
}

func runPageCmd(args []string) {
	fs := flag.NewFlagSet("page", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	keyword := fs.String("keyword", "", "Target keyword for content analysis")
	timeoutSec := fs.Int("timeout", 20, "Timeout in seconds")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	targetURL := normalizeURL(positional[0])
	res, _ := fetchTarget(*timeoutSec, targetURL)

	hdrAudit := audit.InspectHeaders(res)
	htmlAudit, err := audit.InspectHTML(targetURL, res.Body)
	if err != nil {
		fatal("HTML audit error: %v", err)
	}
	schemaAudit := audit.InspectSchema(targetURL, res.Body)
	imagesAudit, err := audit.InspectImages(targetURL, res.Body)
	if err != nil {
		fatal("images audit error: %v", err)
	}
	contentAudit, err := audit.InspectContent(targetURL, res.Body, *keyword)
	if err != nil {
		fatal("content audit error: %v", err)
	}
	hreflangAudit, err := audit.InspectHreflang(targetURL, res.Body)
	if err != nil {
		fatal("hreflang audit error: %v", err)
	}

	pageScore := (htmlAudit.Score + schemaAudit.Score + imagesAudit.Score + contentAudit.Score) / 4

	if *asJSON {
		outputJSON(map[string]interface{}{
			"url":        targetURL,
			"page_score": pageScore,
			"headers":    hdrAudit,
			"technical":  htmlAudit,
			"schema":     schemaAudit,
			"images":     imagesAudit,
			"content":    contentAudit,
			"hreflang":   hreflangAudit,
		})
		return
	}

	fmt.Printf("\n=======================================================\n")
	fmt.Printf(" DEEP PAGE AUDIT\n")
	fmt.Printf(" Target: %s\n", targetURL)
	fmt.Printf("=======================================================\n")
	fmt.Printf("Page Score (avg technical/schema/images/content): %d/100\n\n", pageScore)

	printSection := func(name string, score int, issues []audit.AuditIssue) {
		fmt.Printf("[%s: %d/100]\n", name, score)
		if len(issues) == 0 {
			fmt.Println("  no issues")
		}
		for _, issue := range issues {
			fmt.Printf("  [%-8s] %s: %s\n", issue.Severity, issue.Category, issue.Message)
		}
		fmt.Println()
	}

	fmt.Printf("Transport: %d %s | TTFB %dms | Hops %d\n\n", hdrAudit.StatusCode, res.StatusText, hdrAudit.TTFBMS, hdrAudit.RedirectHops)
	printSection("Technical", htmlAudit.Score, htmlAudit.Issues)
	printSection("Schema", schemaAudit.Score, schemaIssues(schemaAudit))
	printSection("Images", imagesAudit.Score, imagesAudit.Issues)
	printSection("Content", contentAudit.Score, contentAudit.Issues)
	printSection("Hreflang", hreflangAudit.Score, hreflangAudit.Issues)
}

func schemaIssues(r *audit.SchemaAuditReport) []audit.AuditIssue {
	issues := []audit.AuditIssue{}
	for _, b := range r.Blocks {
		for _, e := range b.Errors {
			issues = append(issues, audit.AuditIssue{Severity: audit.SeverityCritical, Category: "Schema", Message: e})
		}
		for _, w := range b.Warnings {
			issues = append(issues, audit.AuditIssue{Severity: audit.SeverityWarning, Category: "Schema", Message: w})
		}
	}
	return issues
}

func runSchemaCmd(args []string) {
	fs := flag.NewFlagSet("schema", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	targetURL := normalizeURL(positional[0])
	res, _ := fetchTarget(15, targetURL)

	report := audit.InspectSchema(targetURL, res.Body)

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== SCHEMA.ORG (JSON-LD) AUDIT: %s ===\n", targetURL)
	fmt.Printf("Health Score: %d/100\n", report.Score)
	fmt.Printf("Summary:      %s\n", report.Summary)
	for i, b := range report.Blocks {
		fmt.Printf("\nBlock #%d (Valid: %t):\n", i+1, b.IsValid)
		fmt.Printf("  Types: %s\n", strings.Join(b.Types, ", "))
		for _, w := range b.Warnings {
			fmt.Printf("  [WARNING] %s\n", w)
		}
		for _, e := range b.Errors {
			fmt.Printf("  [ERROR]   %s\n", e)
		}
	}
	fmt.Println()
}

func runSitemapCmd(args []string) {
	if len(args) > 0 && args[0] == "generate" {
		runSitemapGenerateCmd(args[1:])
		return
	}

	fs := flag.NewFlagSet("sitemap", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	limit := fs.Int("limit", 10, "URLs to health check")
	timeoutSec := fs.Int("timeout", 20, "Timeout in seconds")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("Sitemap URL required")
	}
	targetURL := normalizeURL(positional[0])
	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: time.Duration(*timeoutSec) * time.Second,
	})

	report, err := client.InspectSitemap(context.Background(), targetURL, *limit)
	if err != nil {
		fatal("sitemap error: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== XML SITEMAP AUDIT: %s ===\n", report.SitemapURL)
	fmt.Printf("Is Index Sitemap: %t\n", report.IsSitemapIndex)
	fmt.Printf("Total URLs:       %d\n", report.TotalURLs)
	fmt.Printf("Scan Duration:    %dms\n", report.DurationMS)

	if len(report.ChildSitemaps) > 0 {
		fmt.Printf("\nChild Sitemaps (%d):\n", len(report.ChildSitemaps))
		for _, sm := range report.ChildSitemaps {
			fmt.Printf("  - %s\n", sm)
		}
	}

	if len(report.BrokenURLs) > 0 {
		fmt.Printf("\nBROKEN URLS IN SITEMAP (%d):\n", len(report.BrokenURLs))
		for _, b := range report.BrokenURLs {
			fmt.Printf("  [404/ERR] %s\n", b)
		}
	}

	if len(report.BlockedURLs) > 0 {
		fmt.Printf("\nBLOCKED URLS IN SITEMAP (%d, not counted as broken):\n", len(report.BlockedURLs))
		for _, b := range report.BlockedURLs {
			fmt.Printf("  [401/403/429] %s\n", b)
		}
	}

	if len(report.RedirectingURLs) > 0 {
		fmt.Printf("\nREDIRECTING URLS IN SITEMAP (%d):\n", len(report.RedirectingURLs))
		for _, r := range report.RedirectingURLs {
			fmt.Printf("  [REDIRECT] %s\n", r)
		}
	}

	if len(report.Errors) > 0 {
		fmt.Printf("\nERRORS:\n")
		for _, e := range report.Errors {
			fmt.Printf("  ! %s\n", e)
		}
	}
	fmt.Println()
}

func runSitemapGenerateCmd(args []string) {
	fs := flag.NewFlagSet("sitemap generate", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON summary")
	maxPages := fs.Int("max-pages", 500, "Maximum pages to crawl")
	out := fs.String("out", "", "Write XML to file (default: stdout)")
	timeoutSec := fs.Int("timeout", 15, "Timeout in seconds per request")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("Start URL required (e.g. sitemap generate https://example.com)")
	}
	targetURL := normalizeURL(positional[0])
	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: time.Duration(*timeoutSec) * time.Second,
	})

	report, err := client.CrawlSite(context.Background(), targetURL, *maxPages)
	if err != nil {
		fatal("crawl error: %v", err)
	}

	xmlOut := crawler.GenerateSitemapXML(report.Pages)

	if *out != "" {
		if err := os.WriteFile(*out, xmlOut, 0o644); err != nil {
			fatal("cannot write %s: %v", *out, err)
		}
	}

	if *asJSON {
		outputJSON(map[string]interface{}{
			"start_url":   report.StartURL,
			"pages":       len(report.Pages),
			"excluded":    len(report.Excluded),
			"errors":      len(report.Errors),
			"visited":     report.Visited,
			"duration_ms": report.DurationMS,
			"output_file": *out,
		})
		return
	}

	if *out == "" {
		os.Stdout.Write(xmlOut)
		return
	}

	fmt.Printf("Sitemap written to %s (%d URLs, %d excluded, %d errors, %d pages visited)\n",
		*out, len(report.Pages), len(report.Excluded), len(report.Errors), report.Visited)
	if len(report.Excluded) > 0 {
		fmt.Println("Excluded (noindex/non-200):")
		for _, u := range report.Excluded {
			fmt.Printf("  - %s\n", u)
		}
	}
}

func runRobotsCmd(args []string) {
	fs := flag.NewFlagSet("robots", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("robots.txt URL or domain required")
	}
	targetURL := normalizeURL(positional[0])
	if !strings.HasSuffix(targetURL, "robots.txt") {
		targetURL = strings.TrimRight(targetURL, "/") + "/robots.txt"
	}

	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: 10 * time.Second,
	})

	report, err := client.InspectRobots(context.Background(), targetURL)
	if err != nil {
		fatal("robots error: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== ROBOTS.TXT AUDIT: %s ===\n", report.URL)
	fmt.Printf("Found:             %t (HTTP %d)\n", report.Exists, report.StatusCode)
	fmt.Printf("Global Block:      %t\n", report.HasGlobalBlock)
	fmt.Printf("Sitemaps Declared: %d\n", len(report.Sitemaps))
	for _, sm := range report.Sitemaps {
		fmt.Printf("  - %s\n", sm)
	}

	fmt.Println("\nAI Search & Crawler Access Policy:")
	for _, bot := range report.AICrawlers {
		delay := ""
		if bot.CrawlDelaySec > 0 {
			delay = fmt.Sprintf(" delay=%.1fs", bot.CrawlDelaySec)
		}
		group := ""
		if bot.EffectiveGroup != "" && bot.EffectiveGroup != bot.UserAgent {
			group = fmt.Sprintf(" [via %s]", bot.EffectiveGroup)
		}
		fmt.Printf("  %-18s: %-22s (%s)%s%s\n", bot.UserAgent, bot.Status, bot.Purpose, delay, group)
		if len(bot.DisallowedPaths) > 0 && bot.Status != "Disallowed" {
			fmt.Printf("      disallow patterns: %s\n", strings.Join(bot.DisallowedPaths, ", "))
		}
	}
	fmt.Println()
}

func runLLMSCmd(args []string) {
	fs := flag.NewFlagSet("llms", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	targetURL := normalizeURL(positional[0])
	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: 10 * time.Second,
	})

	report, err := audit.InspectLLMSTxt(context.Background(), client, targetURL)
	if err != nil {
		fatal("llms.txt error: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== LLM DISCOVERY (llms.txt) AUDIT: %s ===\n", report.Domain)
	fmt.Printf("/llms.txt:      %s (%d bytes)\n", existsLabel(report.LLMSTxtExists, report.LLMSTxtStatus), report.LLMSTxtBytes)
	if report.LLMSTxtExists {
		fmt.Printf("  Sections:     %d\n", report.LLMSTxtSections)
		fmt.Printf("  MD Links:     %d\n", report.LLMSTxtLinks)
	}
	fmt.Printf("/llms-full.txt: %s\n", existsLabel(report.FullTxtExists, report.FullTxtStatus))
	fmt.Printf("Score:          %d/100\n", report.Score)

	if len(report.Issues) > 0 {
		fmt.Println("\nFindings:")
		for _, issue := range report.Issues {
			fmt.Printf("  [%-8s] %s\n", issue.Severity, issue.Message)
			if issue.Details != "" {
				fmt.Printf("             %s\n", issue.Details)
			}
		}
	}
	fmt.Println()
}

func existsLabel(exists bool, status int) string {
	if exists {
		return "FOUND"
	}
	return fmt.Sprintf("MISSING (HTTP %d)", status)
}
