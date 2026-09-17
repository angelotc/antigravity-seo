package main

import (
	"flag"
	"fmt"
	"strings"

	"antigravity-seo/internal/audit"
)

func runImagesCmd(args []string) {
	fs := flag.NewFlagSet("images", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	timeoutSec := fs.Int("timeout", 15, "Timeout in seconds")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	targetURL := normalizeURL(positional[0])
	res, _ := fetchTarget(*timeoutSec, targetURL)

	report, err := audit.InspectImages(targetURL, res.Body)
	if err != nil {
		fatal("images audit error: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== IMAGE SEO AUDIT: %s ===\n", targetURL)
	fmt.Printf("Score:            %d/100\n", report.Score)
	fmt.Printf("Total Images:     %d\n", report.TotalImages)
	fmt.Printf("Missing Alt:      %d\n", report.MissingAlt)
	fmt.Printf("Empty Alt (OK):   %d (decorative)\n", report.EmptyAltDecorative)
	fmt.Printf("Alt > 125 chars:  %d\n", report.LongAlt)
	fmt.Printf("No width/height:  %d (CLS risk)\n", report.MissingDimensions)
	fmt.Printf("Lazy Loaded:      %d\n", report.LazyLoaded)
	fmt.Printf("Modern Formats:   %d (webp/avif), Legacy: %d (jpg/png/gif)\n", report.ModernFormats, report.LegacyFormats)
	fmt.Printf("Insecure http://: %d\n", report.InsecureHTTP)
	fmt.Printf("Data URIs:        %d (%d oversized >10KB)\n", report.DataURI, report.OversizedDataURI)

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

func runContentCmd(args []string) {
	fs := flag.NewFlagSet("content", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	keyword := fs.String("keyword", "", "Target keyword for density/placement analysis")
	timeoutSec := fs.Int("timeout", 15, "Timeout in seconds")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	targetURL := normalizeURL(positional[0])
	res, _ := fetchTarget(*timeoutSec, targetURL)

	report, err := audit.InspectContent(targetURL, res.Body, *keyword)
	if err != nil {
		fatal("content audit error: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== CONTENT & E-E-A-T AUDIT: %s ===\n", targetURL)
	fmt.Printf("Score:           %d/100\n", report.Score)
	fmt.Printf("Word Count:      %d (~%d min read)\n", report.WordCount, report.ReadingTimeMin)
	fmt.Printf("Paragraphs:      %d (%d exceed 150 words)\n", report.ParagraphCount, report.LongParagraphs)
	fmt.Printf("Answer Blocks:   %d (40-80 word direct answers)\n", report.AnswerBlocks)
	fmt.Printf("Citation Blocks: %d (130-180 word AI-citation passages)\n", report.CitationBlocks)
	fmt.Printf("Lists / Tables:  %d / %d\n", report.ListCount, report.TableCount)
	fmt.Printf("Author Byline:   %t %s\n", report.HasAuthorByline, strings.Join(report.AuthorSignals, ", "))
	fmt.Printf("Publish Date:    %t %s\n", report.HasPublishDate, strings.Join(report.DateSignals, ", "))
	fmt.Printf("Skipped Levels:  %d\n", report.SkippedLevels)

	if report.Keyword != "" {
		fmt.Printf("\nKeyword Analysis (%q):\n", report.Keyword)
		fmt.Printf("  Occurrences: %d | Density: %.2f%% | In first 100 words: %t\n",
			report.KeywordCount, report.KeywordDensity, report.KeywordInLede)
	}

	if len(report.Headings) > 0 {
		fmt.Println("\nHeading Outline:")
		for _, h := range report.Headings {
			if h.Level <= 4 {
				fmt.Printf("  %s %s\n", strings.Repeat("#", h.Level), truncateForCLI(h.Text, 80))
			}
		}
	}

	if len(report.Issues) > 0 {
		fmt.Println("\nFindings:")
		for _, issue := range report.Issues {
			fmt.Printf("  [%-8s] %s: %s\n", issue.Severity, issue.Category, issue.Message)
			if issue.Details != "" {
				fmt.Printf("             %s\n", issue.Details)
			}
		}
	}
	fmt.Println()
}

func runHreflangCmd(args []string) {
	fs := flag.NewFlagSet("hreflang", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "Output JSON")
	timeoutSec := fs.Int("timeout", 15, "Timeout in seconds")
	positional := parseFlags(fs, args)

	if len(positional) < 1 {
		fatal("URL argument required")
	}
	targetURL := normalizeURL(positional[0])
	res, _ := fetchTarget(*timeoutSec, targetURL)

	report, err := audit.InspectHreflang(targetURL, res.Body)
	if err != nil {
		fatal("hreflang audit error: %v", err)
	}

	if *asJSON {
		outputJSON(report)
		return
	}

	fmt.Printf("\n=== HREFLANG / INTERNATIONAL AUDIT: %s ===\n", targetURL)
	fmt.Printf("Score:        %d/100\n", report.Score)
	fmt.Printf("Alternates:   %d\n", report.Count)
	fmt.Printf("Languages:    %s\n", strings.Join(report.Languages, ", "))
	fmt.Printf("x-default:    %t\n", report.HasXDefault)
	fmt.Printf("Self-ref:     %t\n", report.HasSelfRef)

	if len(report.Alternates) > 0 {
		fmt.Println("\nDeclared Alternates:")
		for _, alt := range report.Alternates {
			valid := "OK"
			if !alt.ValidCode {
				valid = "INVALID"
			}
			fmt.Printf("  %-12s %-8s %s\n", alt.Hreflang, valid, alt.Href)
		}
	}

	if len(report.Issues) > 0 {
		fmt.Println("\nFindings:")
		for _, issue := range report.Issues {
			fmt.Printf("  [%-8s] %s\n", issue.Severity, issue.Message)
		}
	}
	fmt.Println()
}

func truncateForCLI(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
