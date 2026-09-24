package audit

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"antigravity-seo/internal/textutil"
)

// ContentAuditReport evaluates content quality, structure, and E-E-A-T signals
type ContentAuditReport struct {
	URL             string        `json:"url"`
	WordCount       int           `json:"word_count"`  // count in LengthUnit's unit: words for space-delimited scripts, characters for CJK/NoSpace
	LengthUnit      string        `json:"length_unit"` // "words" or "characters" — what WordCount is counting
	Script          string        `json:"script"`      // detected writing system: latin, cjk, hangul, no-space
	ReadingTimeMin  int           `json:"reading_time_min"`
	ParagraphCount  int           `json:"paragraph_count"`
	LongParagraphs  int           `json:"long_paragraphs"` // > 150 words
	Headings        []HeadingInfo `json:"headings"`
	SkippedLevels   int           `json:"skipped_heading_levels"` // e.g. h2 -> h4
	ListCount       int           `json:"list_count"`
	TableCount      int           `json:"table_count"`
	HasAuthorByline bool          `json:"has_author_byline"`
	AuthorSignals   []string      `json:"author_signals,omitempty"`
	HasPublishDate  bool          `json:"has_publish_date"`
	DateSignals     []string      `json:"date_signals,omitempty"`
	AnswerBlocks    int           `json:"answer_blocks"`   // 40-80 word direct-answer paragraphs
	CitationBlocks  int           `json:"citation_blocks"` // 130-180 word AI-citation-shaped passages
	Keyword         string        `json:"keyword,omitempty"`
	KeywordCount    int           `json:"keyword_count,omitempty"`
	KeywordDensity  float64       `json:"keyword_density,omitempty"` // percentage
	KeywordInLede   bool          `json:"keyword_in_lede,omitempty"` // appears in first 100 words
	Score           int           `json:"score"`
	Issues          []AuditIssue  `json:"issues"`
}

// HeadingInfo records one heading with its level for hierarchy analysis
type HeadingInfo struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// InspectContent analyzes visible content structure and E-E-A-T signals.
// keyword is optional; when provided, density and placement are evaluated.
func InspectContent(pageURL string, rawHTML []byte, keyword string) (*ContentAuditReport, error) {
	doc, err := html.Parse(bytes.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("failed parsing HTML document: %w", err)
	}

	report := &ContentAuditReport{
		URL:      pageURL,
		Keyword:  keyword,
		Score:    100,
		Issues:   []AuditIssue{},
		Headings: []HeadingInfo{},
	}

	var paragraphs []string
	var bodyText strings.Builder
	var langHint string

	var walk func(*html.Node)
	invisible := map[string]bool{"script": true, "style": true, "noscript": true, "template": true, "svg": true}

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)

			if tag == "html" {
				for _, a := range n.Attr {
					if strings.ToLower(a.Key) == "lang" {
						langHint = a.Val
					}
				}
			}

			if tag == "p" {
				text := nodeText(n)
				if strings.TrimSpace(text) != "" {
					paragraphs = append(paragraphs, text)
				}
			}

			if lvl := headingLevel(tag); lvl > 0 {
				text := strings.TrimSpace(nodeText(n))
				if text != "" && len(report.Headings) < 100 {
					report.Headings = append(report.Headings, HeadingInfo{Level: lvl, Text: truncate(text, 120)})
				}
			}

			if tag == "ul" || tag == "ol" {
				report.ListCount++
			}
			if tag == "table" {
				report.TableCount++
			}

			// Author byline signals
			if !report.HasAuthorByline {
				for _, a := range n.Attr {
					lk := strings.ToLower(a.Key)
					lv := strings.ToLower(a.Val)
					if lk == "rel" && (strings.Contains(lv, "author")) {
						report.HasAuthorByline = true
						report.AuthorSignals = append(report.AuthorSignals, fmt.Sprintf("<%s rel=author>", tag))
					}
					if lk == "class" || lk == "itemprop" {
						if strings.Contains(lv, "byline") || strings.Contains(lv, "author") {
							report.HasAuthorByline = true
							report.AuthorSignals = append(report.AuthorSignals, fmt.Sprintf("<%s class=%q>", tag, a.Val))
						}
					}
				}
			}

			// Date signals
			if tag == "time" {
				for _, a := range n.Attr {
					if strings.ToLower(a.Key) == "datetime" && a.Val != "" {
						report.HasPublishDate = true
						report.DateSignals = append(report.DateSignals, fmt.Sprintf("<time datetime=%q>", truncate(a.Val, 40)))
					}
				}
			}
			if tag == "meta" {
				var name, property, content string
				for _, a := range n.Attr {
					switch strings.ToLower(a.Key) {
					case "name":
						name = strings.ToLower(a.Val)
					case "property":
						property = strings.ToLower(a.Val)
					case "content":
						content = a.Val
					}
				}
				if name == "author" && content != "" {
					report.HasAuthorByline = true
					report.AuthorSignals = append(report.AuthorSignals, "meta[name=author]")
				}
				if property == "article:published_time" || property == "article:modified_time" || name == "date" {
					if content != "" {
						report.HasPublishDate = true
						report.DateSignals = append(report.DateSignals, fmt.Sprintf("%s=%s", property+name, truncate(content, 40)))
					}
				}
			}

			if invisible[tag] {
				return // do not descend into non-rendered containers
			}
		}

		if n.Type == html.TextNode {
			parent := n.Parent
			skip := false
			for p := parent; p != nil; p = p.Parent {
				if p.Type == html.ElementNode && invisible[strings.ToLower(p.Data)] {
					skip = true
					break
				}
			}
			if !skip {
				bodyText.WriteString(n.Data)
				bodyText.WriteString(" ")
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Script-aware length + reading time. The script is detected from the
	// body text itself, with the <html lang> attribute (if any) breaking
	// ties on short/ambiguous text (see textutil.ClassifyWithHint).
	allText := bodyText.String()
	script := textutil.ClassifyWithHint(allText, langHint)
	report.Script = script.String()

	length, unit := textutil.Length(allText, script)
	report.WordCount = length
	report.LengthUnit = string(unit)

	thr := textutil.ThresholdsFor(script)
	report.ReadingTimeMin = length / thr.ReadingSpeed
	if thr.ReadingSpeed > 0 && length%thr.ReadingSpeed > thr.ReadingSpeed/2 {
		report.ReadingTimeMin++
	}

	// Paragraph shape and GEO answer/citation blocks
	report.ParagraphCount = len(paragraphs)
	for _, p := range paragraphs {
		pLen, _ := textutil.Length(p, script)
		if pLen > thr.LongParagraphMax {
			report.LongParagraphs++
		}
		// Featured-snippet / AI-citation passage shapes are inherently
		// word-counted (that's how Google/LLMs describe them), so these
		// stay word-based regardless of script; on CJK pages, which have no
		// inter-word spaces, this heuristic under-fires — a known
		// limitation carried over unchanged from the prior implementation.
		words := textutil.CountWords(p)
		if words >= 40 && words <= 80 {
			report.AnswerBlocks++
		}
		if words >= 130 && words <= 180 {
			report.CitationBlocks++
		}
	}

	// Heading hierarchy: detect skipped levels
	for i := 1; i < len(report.Headings); i++ {
		drop := report.Headings[i].Level - report.Headings[i-1].Level
		if drop > 1 {
			report.SkippedLevels++
		}
	}

	// Keyword analysis
	if keyword != "" {
		report.KeywordCount = textutil.CountKeyword(allText, keyword)
		if length > 0 {
			report.KeywordDensity = float64(report.KeywordCount) / float64(length) * 100
		}
		// Scale the "lede" window by the same ratio as the script's
		// thin-content threshold vs. Latin's, so e.g. a Japanese lede
		// window is ~2x wider in characters than the Latin 100-word window
		// (matching the ~2x char-vs-word density ratio used throughout
		// thresholds.go).
		latinThr := textutil.ThresholdsFor(textutil.Latin)
		ledeWindow := 100
		if latinThr.ThinContentMin > 0 {
			ledeWindow = 100 * thr.ThinContentMin / latinThr.ThinContentMin
		}
		lede := textutil.FirstN(allText, script, ledeWindow)
		report.KeywordInLede = textutil.ContainsKeyword(lede, keyword)
	}

	// Scoring & issues
	if length < thr.ThinContentMin {
		sev := SeverityWarning
		deduction := 15
		if length < thr.ThinContentMin/2 {
			sev = SeverityCritical
			deduction = 30
		}
		report.Issues = append(report.Issues, AuditIssue{
			Severity: sev, Category: "Content Depth",
			Message: fmt.Sprintf("Thin content: %d %s (%d+ recommended)", length, unit, thr.ThinContentMin),
		})
		report.Score -= deduction
	}
	if report.LongParagraphs > 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo, Category: "Readability",
			Message: fmt.Sprintf("%d paragraphs exceed %d %s — split for scannability", report.LongParagraphs, thr.LongParagraphMax, unit),
		})
		report.Score -= minInt(report.LongParagraphs, 5)
	}
	if report.SkippedLevels > 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning, Category: "Heading Hierarchy",
			Message: fmt.Sprintf("%d skipped heading levels (e.g. h2 -> h4) break the document outline", report.SkippedLevels),
		})
		report.Score -= minInt(report.SkippedLevels*3, 15)
	}
	if len(report.Headings) == 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning, Category: "Heading Hierarchy",
			Message: "No heading content found — page lacks structure for crawlers and AI passage retrieval",
		})
		report.Score -= 15
	}
	if !report.HasAuthorByline {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning, Category: "E-E-A-T",
			Message: "No author byline signals detected (rel=author, meta author, or byline element)",
			Details: "Author attribution is a first-class E-E-A-T signal under Google's quality rater guidelines.",
		})
		report.Score -= 10
	}
	if !report.HasPublishDate {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo, Category: "E-E-A-T",
			Message: "No publish/update date detected (<time datetime> or article:published_time)",
		})
		report.Score -= 5
	}
	if report.AnswerBlocks == 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo, Category: "GEO",
			Message: "No 40-80 word self-contained answer paragraphs — add direct-answer blocks for featured snippets and AI citations",
		})
		report.Score -= 5
	}
	if keyword != "" {
		if report.KeywordDensity > 3.0 {
			report.Issues = append(report.Issues, AuditIssue{
				Severity: SeverityWarning, Category: "Keyword Usage",
				Message: fmt.Sprintf("Keyword density %.1f%% exceeds 3%% — reads as keyword stuffing", report.KeywordDensity),
			})
			report.Score -= 10
		} else if report.KeywordDensity < 0.3 {
			report.Issues = append(report.Issues, AuditIssue{
				Severity: SeverityWarning, Category: "Keyword Usage",
				Message: fmt.Sprintf("Keyword density %.2f%% is very low — target keyword barely appears", report.KeywordDensity),
			})
			report.Score -= 5
		}
		if !report.KeywordInLede {
			report.Issues = append(report.Issues, AuditIssue{
				Severity: SeverityInfo, Category: "Keyword Usage",
				Message: "Target keyword missing from the first 100 words",
			})
			report.Score -= 3
		}
	}

	if report.Score < 0 {
		report.Score = 0
	}
	return report, nil
}

func headingLevel(tag string) int {
	if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
		return int(tag[1] - '0')
	}
	return 0
}

// nodeText extracts all descendant text of a node
func nodeText(n *html.Node) string {
	var sb strings.Builder
	var collect func(*html.Node)
	collect = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
			sb.WriteString(" ")
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(n)
	return sb.String()
}
