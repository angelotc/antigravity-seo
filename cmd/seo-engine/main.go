package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"antigravity-seo/internal/audit"
)

// Version is the engine's release version. It defaults to the value below
// for `go run`/plain `go build`, and is overridden at release-build time via
// `-ldflags "-X main.Version=..."` (see .github/workflows/release.yml and
// install.sh/install.ps1) so a single source of truth drives the binary.
var Version = "2.1.0"

func printUsage() { printUsageTo(os.Stdout) }

func printUsageTo(w io.Writer) {
	fmt.Fprintf(w, `Antigravity SEO Engine v%s
High-speed, small pure-Go SEO & GEO audit engine.

USAGE:
  seo-engine <command> [options] <url>

AUDIT COMMANDS:
  headers     Inspect HTTP status, redirect chains, X-Robots-Tag, and canonical headers
  audit       On-page technical SEO & Schema.org audit (headers + technical + schema)
  page        Deep single-page audit (audit + images + content + hreflang)
  report      Generate executive HTML or native A4 PDF audit report (--pdf)
  schema      Extract and validate JSON-LD structured data against Google Rich Results
  images      Image optimization audit (alt coverage, dimensions/CLS, formats, lazy-load)
  content     Content quality & E-E-A-T audit (word count, headings, byline, answer blocks)
  hreflang    International hreflang annotation audit (BCP47, x-default, self-reference)
  llms        Audit /llms.txt and /llms-full.txt availability for AI discovery

SITE COMMANDS:
  crawl       Site-wide crawl audit: broken links, duplicates, canonicals, hreflang, sitemap/orphans
  sitemap     Analyze a sitemap (follows indexes; auto-discovers from a site root) — or generate one: sitemap generate <url>
  robots      Inspect robots.txt directives and AI crawler access policies
  drift       Page-change monitoring: drift baseline|compare|history <url>
  backlinks   Explore the domain's own Common Crawl capture index (keyless)

INTEGRATION COMMANDS (optional API keys / credentials):
  gsc         Google Search Console (query search analytics, inspect URL indexation)
  psi         Google PageSpeed Insights (lab + CrUX field CWV). Needs GOOGLE_API_KEY
  crux        Chrome UX Report p75 field data (--history for 25 weeks). Needs GOOGLE_API_KEY
  indexnow    Submit URLs to IndexNow (Bing/Yandex/Seznam/Naver). Needs INDEXNOW_KEY

OPS COMMANDS:
  setup       Create the data dir and runtime-state manifest
  doctor      Readiness check (runtime state, drift store, integrations, network)
  lint-schema-file  JSON-LD quality gate for a file (used by the PostToolUse hook)
  version     Print version information

OPTIONS:
  --json      Output results in machine-readable JSON format (default: human-readable)
  --pdf       (report) Render a native A4 PDF directly (pure Go, embedded font
              covering Latin + Japanese; no WeasyPrint/Chromium dependency)
  --out       (report, sitemap) Output file path
  --limit     Number of URLs to check in sitemaps (default: 10, max: 100)
  --max-sitemaps
              (sitemap, crawl) Child sitemaps to fetch when following an index (default: 50)
  --max-pages (crawl, sitemap generate) Pages to crawl (crawl default: 100)
  --concurrency
              (crawl) Parallel fetches (default: 4)
  --sitemap   (crawl) Sitemap URL; default is auto-discovery from the site root
  --no-sitemap
              (crawl) Skip sitemap checks
  --timeout   Request timeout in seconds (default: 15)
  --offline   (doctor) skip the network reachability probe
  --fail-on critical|warning
              (headers, audit, page, report, schema, images, content, hreflang, crawl)
              Exit 3 after normal output if any finding is at or above the level.
              For drift compare use --fail-on change.

EXIT CODES:
  0  success
  1  error (fetch, auth, usage)
  2  invalid flag (Go flag parser)
  3  --fail-on threshold met

EXAMPLES:
  seo-engine headers https://example.com
  seo-engine page https://example.com --json
  seo-engine crawl https://example.com --max-pages 100 --json
  seo-engine sitemap https://example.com --limit 20
  seo-engine report https://example.com --pdf --out audit.pdf
  seo-engine backlinks example.com --limit 25
  seo-engine gsc query sc-domain:example.com
  seo-engine doctor
`, Version)
}

func main() {
	if len(os.Args) < 2 {
		printUsageTo(os.Stderr)
		os.Exit(1)
	}

	command := os.Args[1]
	jsonMode = hasJSONFlag(os.Args[2:])

	switch command {
	case "version", "-v", "--version":
		runVersionCmd(os.Args[2:])
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

	case "crawl":
		runCrawlCmd(os.Args[2:])

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
		if jsonMode {
			fatal("unknown command: %s\n  Run `seo-engine help` for usage", command)
		}
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsageTo(os.Stderr)
		os.Exit(1)
	}
}

func runVersionCmd(args []string) {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	parseFlags(fs, args)
	if *asJSON {
		outputJSON(map[string]string{"version": Version})
		return
	}
	fmt.Printf("seo-engine version %s\n", Version)
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

// jsonMode is true when the invocation carries --json; errors then go to
// stdout as a JSON object instead of plain text on stderr.
var jsonMode bool

// hasJSONFlag reports whether args contain a true --json / -json flag.
func hasJSONFlag(args []string) bool {
	for _, a := range args {
		switch a {
		case "--json", "-json", "--json=true", "-json=true":
			return true
		}
	}
	return false
}

// formatErrorJSON renders an error as {"error":{command,message,hint}}. The
// first line of msg is the message; any remaining lines become the hint.
func formatErrorJSON(command, msg string) []byte {
	first, rest, _ := strings.Cut(strings.TrimSpace(msg), "\n")
	body := struct {
		Command string `json:"command"`
		Message string `json:"message"`
		Hint    string `json:"hint,omitempty"`
	}{command, strings.TrimSpace(first), strings.TrimSpace(rest)}
	b, _ := json.Marshal(map[string]interface{}{"error": body})
	return b
}

func fatal(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if jsonMode {
		command := ""
		if len(os.Args) > 1 {
			command = os.Args[1]
		}
		fmt.Println(string(formatErrorJSON(command, msg)))
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	os.Exit(1)
}

// Exit code returned when a --fail-on threshold is met.
const exitFindings = 3

// addFailOnFlag registers --fail-on on fs. Validate the value with
// checkFailOn right after parsing, before any network work.
func addFailOnFlag(fs *flag.FlagSet) *string {
	return fs.String("fail-on", "", "Exit 3 if findings reach this severity: critical or warning")
}

// findingsTrip reports whether any severity meets the threshold. Valid
// thresholds are "" (never trips), "critical" and "warning", case-insensitive.
func findingsTrip(threshold string, severities []string) (bool, error) {
	var tripOn map[string]bool
	switch strings.ToLower(strings.TrimSpace(threshold)) {
	case "":
		return false, nil
	case "critical":
		tripOn = map[string]bool{"CRITICAL": true}
	case "warning":
		tripOn = map[string]bool{"CRITICAL": true, "WARNING": true}
	default:
		return false, fmt.Errorf("invalid --fail-on value %q (expected critical or warning)", threshold)
	}
	for _, sev := range severities {
		if tripOn[strings.ToUpper(sev)] {
			return true, nil
		}
	}
	return false, nil
}

// checkFailOn fatals on an invalid --fail-on value.
func checkFailOn(threshold string) {
	if _, err := findingsTrip(threshold, nil); err != nil {
		fatal("%v", err)
	}
}

// exitIfFindings exits 3 when the threshold is met. Call it after all normal
// output has been written.
func exitIfFindings(threshold string, severities []string) {
	trip, err := findingsTrip(threshold, severities)
	if err != nil {
		fatal("%v", err)
	}
	if !trip {
		return
	}
	if !jsonMode {
		fmt.Fprintf(os.Stderr, "seo-engine: findings at or above --fail-on %s\n", strings.ToLower(threshold))
	}
	os.Exit(exitFindings)
}

// issueSeverities collects severities from any number of issue lists.
func issueSeverities(lists ...[]audit.AuditIssue) []string {
	var out []string
	for _, l := range lists {
		for _, i := range l {
			out = append(out, string(i.Severity))
		}
	}
	return out
}
