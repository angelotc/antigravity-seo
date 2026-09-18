package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"antigravity-seo/internal/audit"
	"antigravity-seo/internal/drift"
	"antigravity-seo/internal/runtime"
)

func runSetupCmd(args []string) {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	parseFlags(fs, args)

	report, err := runtime.RunSetup(Version, runtime.PluginRoot())
	if err != nil {
		fatal("setup failed: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Println("=== ANTIGRAVITY SEO SETUP ===")
	fmt.Printf("Data dir:       %s\n", report.DataDir)
	fmt.Printf("Runtime state:  %s\n", report.StatePath)
	fmt.Printf("Drift store:    %s\n", report.DriftDir)
	fmt.Printf("Adapters:       %s\n", strings.Join(report.Adapters, ", "))
	if report.Integrations.GoogleAPIKey {
		fmt.Println("Integrations:   GOOGLE_API_KEY detected (psi/crux enabled)")
	} else {
		fmt.Println("Integrations:   none (set GOOGLE_API_KEY / INDEXNOW_KEY to enable psi/crux/indexnow)")
	}
	for _, w := range report.Warnings {
		fmt.Printf("WARNING: %s\n", w)
	}
	fmt.Println("\nSetup complete. Verify with: seo-engine doctor")
}

func runDoctorCmd(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	offline := fs.Bool("offline", false, "Skip network reachability probe")
	parseFlags(fs, args)

	report := runtime.RunDoctor(Version, *offline)

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Println("=== ANTIGRAVITY SEO DOCTOR ===")
	fmt.Printf("Engine version: %s\n", report.EngineVersion)
	fmt.Printf("Platform:       %s\n", report.Platform)
	fmt.Printf("Data dir:       %s\n", report.DataDir)
	fmt.Println()
	for _, c := range report.Checks {
		fmt.Printf("  [%-5s] %-14s %s\n", c.Status, c.Name, c.Detail)
	}
	fmt.Println()
	if report.Ready {
		fmt.Println("STATUS: READY")
		return
	}
	fmt.Println("STATUS: NOT READY — run `seo-engine setup` to initialize")
	os.Exit(1)
}

func runLintSchemaFileCmd(args []string) {
	fs := flag.NewFlagSet("lint-schema-file", flag.ExitOnError)
	positional := parseFlags(fs, args)

	var report *audit.LintReport
	var err error

	if len(positional) >= 1 {
		report, err = audit.LintSchemaFile(positional[0])
	} else {
		// Hook mode: receive the tool payload on stdin and lint the written file
		payload, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			fatal("cannot read hook payload: %v", readErr)
		}
		var hook struct {
			ToolName string `json:"tool_name"`
			ToolInput map[string]interface{} `json:"tool_input"`
			ToolResponse map[string]interface{} `json:"tool_response"`
		}
		if err := json.Unmarshal(payload, &hook); err != nil {
			// Not JSON — treat stdin as raw file content
			report = audit.LintSchemaSource("<stdin>", payload)
			err = nil
		} else {
			path := extractHookFilePath(hook.ToolInput, hook.ToolResponse)
			if path == "" {
				fmt.Println("{}")
				return
			}
			report, err = audit.LintSchemaFile(path)
		}
	}

	if err != nil {
		// Unreadable file must not block the edit
		fmt.Fprintln(os.Stderr, "schema-lint: "+err.Error())
		fmt.Println("{}")
		return
	}

	if !report.Checked {
		fmt.Println("{}")
		return
	}

	if len(report.Findings) == 0 {
		fmt.Println("{}")
		return
	}

	// Findings: report on stderr for the harness to surface, JSON summary on stdout
	for _, f := range report.Findings {
		level := "WARN"
		if f.Severity == "block" {
			level = "ERROR"
		}
		fmt.Fprintf(os.Stderr, "schema-lint [%s] %s block #%d: %s\n", level, report.File, f.Block, f.Message)
	}

	verdict := "warned"
	exitCode := 0
	if report.Blocked {
		verdict = "blocked"
		exitCode = 2
	}

	out, _ := json.Marshal(map[string]interface{}{
		"schema_lint": verdict,
		"file":        report.File,
		"blocks":      report.Blocks,
		"findings":    report.Findings,
	})
	fmt.Println(string(out))
	os.Exit(exitCode)
}

// extractHookFilePath finds the file path inside a PostToolUse payload
func extractHookFilePath(toolInput, toolResponse map[string]interface{}) string {
	candidates := []string{
		"TargetFile", "target_file", "targetFile", "file_path", "filePath",
		"AbsolutePath", "absolutePath", "path", "filename", "file",
	}
	for _, source := range []map[string]interface{}{toolInput, toolResponse} {
		if source == nil {
			continue
		}
		for _, key := range candidates {
			if v, ok := source[key].(string); ok && v != "" {
				return v
			}
		}
		for k, v := range source {
			lower := strings.ToLower(k)
			if lower == "targetfile" || lower == "target_file" || lower == "filepath" || lower == "file_path" || lower == "absolutepath" || lower == "absolute_path" {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func runDriftCmd(args []string) {
	if len(args) < 2 {
		fatal("usage: drift baseline|compare|history <url>")
	}
	action := args[0]
	rest := args[1:]

	if action == "history" {
		runDriftHistoryCmd(rest)
		return
	}

	fs := flag.NewFlagSet("drift "+action, flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	timeoutSec := fs.Int("timeout", 20, "Timeout in seconds")
	positional := parseFlags(fs, rest)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	targetURL := normalizeURL(positional[0])
	res, _ := fetchTarget(*timeoutSec, targetURL)

	switch action {
	case "baseline":
		snap, path, err := drift.Baseline(targetURL, res)
		if err != nil {
			fatal("baseline failed: %v", err)
		}
		if *asJSON {
			outputJSON(map[string]interface{}{
				"snapshot": snap,
				"path":     path,
			})
			return
		}
		fmt.Printf("Baseline captured: %s\n", targetURL)
		fmt.Printf("  Title:     %s\n", snap.Title)
		fmt.Printf("  Status:    %d | TTFB %dms\n", snap.StatusCode, snap.TTFBMS)
		fmt.Printf("  Words:     %d | Images: %d | Schema: %s\n",
			snap.WordCount, snap.ImageCount, strings.Join(snap.SchemaTypes, ", "))
		fmt.Printf("  Stored at: %s\n", path)

	case "compare":
		report, err := drift.Compare(targetURL, res)
		if err != nil {
			fatal("compare failed: %v", err)
		}
		if *asJSON {
			outputJSON(report)
			return
		}
		fmt.Printf("\n=== DRIFT COMPARE: %s ===\n", targetURL)
		fmt.Printf("Baseline: %s\n", report.BaselineTime.Format(time.RFC3339))
		fmt.Printf("Current:  %s\n", report.CurrentTime.Format(time.RFC3339))
		if !report.Changed {
			fmt.Println("No changes detected.")
			return
		}
		fmt.Printf("\nCHANGES (%d):\n", len(report.Changes))
		for _, c := range report.Changes {
			fmt.Printf("  %-18s %s -> %s\n", c.Field, truncateForCLI(c.Before, 60), truncateForCLI(c.After, 60))
		}

	default:
		fatal("unknown drift action %q (expected baseline|compare|history)", action)
	}
}

func runDriftHistoryCmd(args []string) {
	fs := flag.NewFlagSet("drift history", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	targetURL := normalizeURL(positional[0])

	snaps, err := drift.History(targetURL)
	if err != nil {
		fatal("history failed: %v", err)
	}

	if *asJSON {
		outputJSON(map[string]interface{}{"url": targetURL, "snapshots": snaps})
		return
	}

	fmt.Printf("\n=== DRIFT HISTORY: %s ===\n", targetURL)
	if len(snaps) == 0 {
		fmt.Println("No snapshots. Run: seo-engine drift baseline " + targetURL)
		return
	}
	for _, s := range snaps {
		fmt.Printf("  %s  %d  %dms  %q\n", s.Timestamp.Format(time.RFC3339), s.StatusCode, s.TTFBMS, truncateForCLI(s.Title, 50))
	}
	fmt.Println()
}
