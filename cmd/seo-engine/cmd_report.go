package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"antigravity-seo/internal/crawler"
	"antigravity-seo/internal/report"
)

func runReportCmd(args []string) {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON summary instead of HTML/PDF")
	asPDF := fs.Bool("pdf", false, "Render report directly to PDF (pure Go native A4 engine)")
	outFile := fs.String("out", "", "Output file path (default: report-<host>-<date>.[html|pdf])")
	timeoutSec := fs.Int("timeout", 30, "Timeout in seconds")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required\n  Usage: seo-engine report <url> [--pdf] [--out report.pdf]")
	}
	targetURL := normalizeURL(positional[0])

	// Auto-enable PDF if --out ends in .pdf
	if outFile != nil && strings.HasSuffix(strings.ToLower(*outFile), ".pdf") {
		*asPDF = true
	}

	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: time.Duration(*timeoutSec) * time.Second,
	})

	fmt.Printf("Generating comprehensive SEO & GEO audit report for %s...\n", targetURL)
	data, err := report.BuildAuditData(context.Background(), client, targetURL)
	if err != nil {
		fatal("report generation failed: %v", err)
	}

	if *asJSON {
		outputJSON(data)
		return
	}

	htmlContent, err := report.RenderHTML(data)
	if err != nil {
		fatal("failed rendering HTML report: %v", err)
	}

	nowStr := time.Now().Format("20060102-150405")
	hostClean := strings.ReplaceAll(data.Host, ":", "_")

	if *asPDF {
		targetPath := *outFile
		if targetPath == "" {
			targetPath = fmt.Sprintf("seo-report-%s-%s.pdf", hostClean, nowStr)
		}

		review, err := report.RenderNativePDF(data, targetPath)
		if err != nil {
			// Fallback: save HTML so the user doesn't lose the report
			htmlFallback := strings.TrimSuffix(targetPath, filepath.Ext(targetPath)) + ".html"
			_ = os.WriteFile(htmlFallback, []byte(htmlContent), 0o644)
			fatal("PDF rendering failed (%v).\n  Saved HTML report to %s\n  You can open the HTML file in any browser and choose Print -> Save as PDF.", err, htmlFallback)
		}

		fmt.Printf("\n=== AUDIT COMPLETE ===\n")
		fmt.Printf("Target:        %s\n", data.URL)
		fmt.Printf("Overall Score: %d/100 (Tech: %d, Schema: %d, Content: %d, Media: %d)\n",
			data.Scores.Overall, data.Scores.Technical, data.Scores.Schema, data.Scores.Content, data.Scores.Media)
		fmt.Printf("Issues:        %d total findings\n", len(data.Issues))
		fmt.Printf("Engine:        Pure Go A4 PDF (github.com/go-pdf/fpdf)\n")
		fmt.Printf("Quality Check: %s (Size: %d KB)\n", review.Status, review.SizeBytes/1024)
		fmt.Printf("PDF Saved:     %s\n\n", targetPath)
		return
	}

	targetPath := *outFile
	if targetPath == "" {
		targetPath = fmt.Sprintf("seo-report-%s-%s.html", hostClean, nowStr)
	}

	if err := os.WriteFile(targetPath, []byte(htmlContent), 0o644); err != nil {
		fatal("failed writing HTML report: %v", err)
	}

	fmt.Printf("\n=== AUDIT COMPLETE ===\n")
	fmt.Printf("Target:        %s\n", data.URL)
	fmt.Printf("Overall Score: %d/100 (Tech: %d, Schema: %d, Content: %d, Media: %d)\n",
		data.Scores.Overall, data.Scores.Technical, data.Scores.Schema, data.Scores.Content, data.Scores.Media)
	fmt.Printf("Issues:        %d total findings\n", len(data.Issues))
	fmt.Printf("HTML Saved:    %s\n", targetPath)
	fmt.Printf("Tip: Run with `--pdf` to render a native PDF.\n\n")
}
