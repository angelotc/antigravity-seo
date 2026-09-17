package audit

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// HreflangAlternate is one <link rel="alternate" hreflang="..."> entry
type HreflangAlternate struct {
	Hreflang   string `json:"hreflang"`
	Href       string `json:"href"`
	ValidCode  bool   `json:"valid_code"`
	IsXDefault bool   `json:"is_x_default"`
}

// HreflangAuditReport evaluates international targeting annotations
type HreflangAuditReport struct {
	URL             string              `json:"url"`
	Alternates      []HreflangAlternate `json:"alternates"`
	Count           int                 `json:"count"`
	InvalidCodes    []string            `json:"invalid_codes,omitempty"`
	Duplicates      []string            `json:"duplicates,omitempty"`
	HasXDefault     bool                `json:"has_x_default"`
	HasSelfRef      bool                `json:"has_self_reference"`
	Languages       []string            `json:"languages"`
	Score           int                 `json:"score"`
	Issues          []AuditIssue        `json:"issues"`
}

// BCP47-ish language tag: 2-3 letter language, optional script (4) or region (2 | 3-digit)
var langTagRe = regexp.MustCompile(`^[a-zA-Z]{2,3}(-[a-zA-Z]{4})?(-([a-zA-Z]{2}|\d{3}))?$`)

// InspectHreflang audits hreflang link annotations in raw HTML
func InspectHreflang(pageURL string, rawHTML []byte) (*HreflangAuditReport, error) {
	doc, err := html.Parse(bytes.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("failed parsing HTML document: %w", err)
	}

	report := &HreflangAuditReport{
		URL:         pageURL,
		Alternates:  []HreflangAlternate{},
		Languages:   []string{},
		Score:       100,
		Issues:      []AuditIssue{},
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.ToLower(n.Data) == "link" {
			var rel, href, hreflang string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "rel":
					rel = strings.ToLower(strings.TrimSpace(a.Val))
				case "href":
					href = strings.TrimSpace(a.Val)
				case "hreflang":
					hreflang = strings.TrimSpace(a.Val)
				}
			}
			if strings.Contains(rel, "alternate") && hreflang != "" {
				alt := HreflangAlternate{
					Hreflang:   hreflang,
					Href:       href,
					IsXDefault: strings.EqualFold(hreflang, "x-default"),
					ValidCode:  strings.EqualFold(hreflang, "x-default") || langTagRe.MatchString(hreflang),
				}
				report.Alternates = append(report.Alternates, alt)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	report.Count = len(report.Alternates)

	if report.Count == 0 {
		report.Score = 0
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo, Category: "International",
			Message: "No hreflang annotations found (fine for single-locale sites)",
		})
		return report, nil
	}

	// Duplicates and self-reference
	seen := map[string]int{}
	selfPath := strings.TrimRight(pageURL, "/")
	for _, alt := range report.Alternates {
		seen[alt.Hreflang]++
		if strings.EqualFold(alt.Hreflang, "x-default") {
			report.HasXDefault = true
		} else if alt.ValidCode {
			lang := strings.ToLower(strings.SplitN(alt.Hreflang, "-", 2)[0])
			if !containsString(report.Languages, lang) {
				report.Languages = append(report.Languages, lang)
			}
		}
		if strings.TrimRight(alt.Href, "/") == selfPath {
			report.HasSelfRef = true
		}
	}
	for code, n := range seen {
		if n > 1 {
			report.Duplicates = append(report.Duplicates, fmt.Sprintf("%s (x%d)", code, n))
		}
		if code != "x-default" && !langTagRe.MatchString(code) {
			report.InvalidCodes = append(report.InvalidCodes, code)
		}
	}

	if len(report.InvalidCodes) > 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityCritical, Category: "International",
			Message: fmt.Sprintf("Invalid hreflang codes: %s (expected BCP47 like en, ja, pt-BR, or x-default)", strings.Join(report.InvalidCodes, ", ")),
		})
		report.Score -= 25
	}
	if len(report.Duplicates) > 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning, Category: "International",
			Message: fmt.Sprintf("Duplicate hreflang declarations: %s", strings.Join(report.Duplicates, ", ")),
		})
		report.Score -= 10
	}
	if !report.HasXDefault {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning, Category: "International",
			Message: "Missing x-default hreflang for unmatched-language users",
		})
		report.Score -= 10
	}
	if !report.HasSelfRef {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning, Category: "International",
			Message: "No self-referencing hreflang entry — each page must declare its own locale",
		})
		report.Score -= 15
	}

	if report.Score < 0 {
		report.Score = 0
	}
	return report, nil
}
