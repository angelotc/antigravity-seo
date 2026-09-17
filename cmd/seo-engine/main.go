package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"antigravity-seo/internal/mcp"
)

const Version = "2.0.0"

func printUsage() {
	fmt.Printf(`Antigravity SEO Engine v%s
High-speed, zero-dependency SEO & GEO audit engine and MCP server.

USAGE:
  seo-engine <command> [options] <url>

AUDIT COMMANDS:
  headers     Inspect HTTP status, redirect chains, X-Robots-Tag, and canonical headers
  audit       On-page technical SEO & Schema.org audit (headers + technical + schema)
  page        Deep single-page audit (audit + images + content + hreflang)
  report      Generate executive HTML or PDF audit report (weasyprint / chromium)
  schema      Extract and validate JSON-LD structured data against Google Rich Results
  images      Image optimization audit (alt coverage, dimensions/CLS, formats, lazy-load)
  content     Content quality & E-E-A-T audit (word count, headings, byline, answer blocks)
  hreflang    International hreflang annotation audit (BCP47, x-default, self-reference)
  llms        Audit /llms.txt and /llms-full.txt availability for AI discovery

SITE COMMANDS:
  sitemap     Analyze a sitemap URL — or generate one: sitemap generate <url>
  robots      Inspect robots.txt directives and AI crawler access policies
  drift       Page-change monitoring: drift baseline|compare|history <url>
  backlinks   Explore open Common Crawl link graph and domain captures (keyless)

INTEGRATION COMMANDS (optional API keys / credentials):
  gsc         Google Search Console (query search analytics, inspect URL indexation)
  psi         Google PageSpeed Insights (lab + CrUX field CWV). Needs GOOGLE_API_KEY
  crux        Chrome UX Report p75 field data (--history for 25 weeks). Needs GOOGLE_API_KEY
  indexnow    Submit URLs to IndexNow (Bing/Yandex/Seznam/Naver). Needs INDEXNOW_KEY

OPS COMMANDS:
  setup       Create the data dir and runtime-state manifest
  doctor      Readiness check (runtime state, drift store, integrations, network)
  lint-schema-file  JSON-LD quality gate for a file (used by the PostToolUse hook)
  serve-mcp   Launch the Model Context Protocol (MCP) server over stdio
  version     Print version information

OPTIONS:
  --json      Output results in machine-readable JSON format (default: human-readable)
  --pdf       (report) Render native PDF directly via WeasyPrint or Chromium
  --out       (report, sitemap) Output file path
  --limit     Number of URLs to check in sitemaps (default: 10, max: 100)
  --timeout   Request timeout in seconds (default: 15)
  --offline   (doctor) skip the network reachability probe

EXAMPLES:
  seo-engine headers https://example.com
  seo-engine page https://example.com --json
  seo-engine report https://example.com --pdf --out audit.pdf
  seo-engine backlinks example.com --limit 25
  seo-engine gsc query sc-domain:example.com
  seo-engine doctor
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

	case "page":
		runPageCmd(os.Args[2:])

	case "report":
		runReportCmd(os.Args[2:])

	case "schema":
		runSchemaCmd(os.Args[2:])

	case "images":
		runImagesCmd(os.Args[2:])

	case "content":
		runContentCmd(os.Args[2:])

	case "hreflang":
		runHreflangCmd(os.Args[2:])

	case "llms":
		runLLMSCmd(os.Args[2:])

	case "sitemap":
		runSitemapCmd(os.Args[2:])

	case "robots":
		runRobotsCmd(os.Args[2:])

	case "drift":
		runDriftCmd(os.Args[2:])

	case "backlinks":
		runBacklinksCmd(os.Args[2:])

	case "gsc":
		runGSCCmd(os.Args[2:])

	case "psi":
		runPSICmd(os.Args[2:])

	case "crux":
		runCruxCmd(os.Args[2:])

	case "indexnow":
		runIndexNowCmd(os.Args[2:])

	case "setup":
		runSetupCmd(os.Args[2:])

	case "doctor":
		runDoctorCmd(os.Args[2:])

	case "lint-schema-file":
		runLintSchemaFileCmd(os.Args[2:])

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func normalizeURL(raw string) string {
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return "https://" + raw
	}
	return raw
}

// parseFlags parses flags that may appear before or after positional
// arguments (the documented convention is `seo-engine <cmd> <url> --json`).
// Returns the positional arguments.
func parseFlags(fs *flag.FlagSet, args []string) []string {
	var positional []string
	rest := args
	for {
		_ = fs.Parse(rest)
		remaining := fs.Args()
		if len(remaining) == 0 {
			return positional
		}
		positional = append(positional, remaining[0])
		rest = remaining[1:]
		if len(rest) == 0 {
			return positional
		}
	}
}

func outputJSON(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
