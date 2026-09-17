package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"antigravity-seo/internal/audit"
	"antigravity-seo/internal/crawler"
	"antigravity-seo/internal/mcp"
)

const Version = "1.0.0"

func printUsage() {
	fmt.Printf(`Antigravity SEO Engine v%s
High-speed, zero-dependency SEO & GEO audit engine and MCP server.

USAGE:
  seo-engine <command> [options] <url>

COMMANDS:
  headers   Inspect HTTP status, redirect chains, X-Robots-Tag, and canonical headers
  audit     Perform full on-page technical SEO & Schema.org audit
  sitemap   Stream, parse, and validate XML sitemap (detect broken links, limits)
  robots    Inspect robots.txt directives and AI crawler access policies
  schema    Extract and validate JSON-LD structured data against Google Rich Results
  serve-mcp Launch the Model Context Protocol (MCP) server over stdio
  version   Print version information

OPTIONS:
  --json    Output results in machine-readable JSON format (default: human-readable)
  --limit   Number of URLs to check in sitemaps (default: 10)
  --timeout Request timeout in seconds (default: 15)

EXAMPLES:
  seo-engine headers https://nipponhomes.com
  seo-engine audit https://nipponhomes.com --json
  seo-engine sitemap https://nipponhomes.com/sitemap.xml --limit 20
  seo-engine robots https://nipponhomes.com
  seo-engine serve-mcp
`, Version)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "version", "-v", "--version":
		fmt.Printf("seo-engine version %s\n", Version)
		return

	case "serve-mcp":
		if err := mcp.StartMCPServer(Version); err != nil {
			fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
			os.Exit(1)
		}
		return

	case "headers":
		runHeadersCmd(os.Args[2:])

	case "audit":
		runAuditCmd(os.Args[2:])

	case "sitemap":
		runSitemapCmd(os.Args[2:])

	case "robots":
		runRobotsCmd(os.Args[2:])

	case "schema":
		runSchemaCmd(os.Args[2:])

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func runHeadersCmd(args []string) {
	fs := flag.NewFlagSet("headers", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	timeoutSec := fs.Int("timeout", 15, "Timeout in seconds")
	_ = fs.Parse(args)

	targetURL := fs.Arg(0)
	if targetURL == "" {
		fmt.Fprintln(os.Stderr, "Error: URL argument required.")
		os.Exit(1)
	}
	targetURL = normalizeURL(targetURL)

	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: time.Duration(*timeoutSec) * time.Second,
	})

	res, err := client.Fetch(context.Background(), targetURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fetch error: %v\n", err)
		os.Exit(1)
	}

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
	_ = fs.Parse(args)

	targetURL := fs.Arg(0)
	if targetURL == "" {
		fmt.Fprintln(os.Stderr, "Error: URL argument required.")
		os.Exit(1)
	}
	targetURL = normalizeURL(targetURL)

	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: time.Duration(*timeoutSec) * time.Second,
	})

	res, err := client.Fetch(context.Background(), targetURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fetch error: %v\n", err)
		os.Exit(1)
	}

	hdrAudit := audit.InspectHeaders(res)
	htmlAudit, err := audit.InspectHTML(targetURL, res.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "HTML audit error: %v\n", err)
		os.Exit(1)
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

func runSitemapCmd(args []string) {
	fs := flag.NewFlagSet("sitemap", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	limit := fs.Int("limit", 10, "URLs to health check")
	timeoutSec := fs.Int("timeout", 20, "Timeout in seconds")
	_ = fs.Parse(args)

	targetURL := fs.Arg(0)
	if targetURL == "" {
		fmt.Fprintln(os.Stderr, "Error: Sitemap URL required.")
		os.Exit(1)
	}
	targetURL = normalizeURL(targetURL)

	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: time.Duration(*timeoutSec) * time.Second,
	})

	report, err := client.InspectSitemap(context.Background(), targetURL, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Sitemap error: %v\n", err)
		os.Exit(1)
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

func runRobotsCmd(args []string) {
	fs := flag.NewFlagSet("robots", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	_ = fs.Parse(args)

	targetURL := fs.Arg(0)
	if targetURL == "" {
		fmt.Fprintln(os.Stderr, "Error: robots.txt URL or domain required.")
		os.Exit(1)
	}
	targetURL = normalizeURL(targetURL)
	if !strings.HasSuffix(targetURL, "robots.txt") {
		targetURL = strings.TrimRight(targetURL, "/") + "/robots.txt"
	}

	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: 10 * time.Second,
	})

	report, err := client.InspectRobots(context.Background(), targetURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Robots error: %v\n", err)
		os.Exit(1)
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
		fmt.Printf("  %-18s: %-10s (%s)\n", bot.UserAgent, bot.Status, bot.Purpose)
	}
	fmt.Println()
}

func runSchemaCmd(args []string) {
	fs := flag.NewFlagSet("schema", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	_ = fs.Parse(args)

	targetURL := fs.Arg(0)
	if targetURL == "" {
		fmt.Fprintln(os.Stderr, "Error: URL argument required.")
		os.Exit(1)
	}
	targetURL = normalizeURL(targetURL)

	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: 15 * time.Second,
	})

	res, err := client.Fetch(context.Background(), targetURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fetch error: %v\n", err)
		os.Exit(1)
	}

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

func normalizeURL(raw string) string {
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return "https://" + raw
	}
	return raw
}

func outputJSON(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}
