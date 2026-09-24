package audit

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"antigravity-seo/internal/textutil"
	"antigravity-seo/internal/urlnorm"
)

// TechnicalAuditReport provides a full on-page DOM evaluation
type TechnicalAuditReport struct {
	URL              string       `json:"url"`
	Title            string       `json:"title"`
	TitleLength      int          `json:"title_length"` // rune count
	TitleWidth       int          `json:"title_width"`  // textutil.DisplayWidth units
	MetaDescription  string       `json:"meta_description"`
	MetaDescLength   int          `json:"meta_description_length"` // rune count
	MetaDescWidth    int          `json:"meta_description_width"`  // textutil.DisplayWidth units
	Script           string       `json:"script"`                  // detected writing system used for length/width thresholds
	Canonical        string       `json:"canonical"`
	IsSelfCanonical  bool         `json:"is_self_canonical"`
	MetaRobots       string       `json:"meta_robots,omitempty"`
	HasMetaNoindex   bool         `json:"has_meta_noindex"`
	HasMetaNofollow  bool         `json:"has_meta_nofollow"`
	Charset          string       `json:"charset,omitempty"`
	Lang             string       `json:"lang,omitempty"`
	HasViewport      bool         `json:"has_viewport"`
	H1Count          int          `json:"h1_count"`
	H1Text           []string     `json:"h1_text"`
	H2Count          int          `json:"h2_count"`
	H3Count          int          `json:"h3_count"`
	TotalImages      int          `json:"total_images"`
	ImagesMissingAlt int          `json:"images_missing_alt"`
	InternalLinks    int          `json:"internal_links"`
	ExternalLinks    int          `json:"external_links"`
	OGTitle          string       `json:"og_title,omitempty"`
	OGDescription    string       `json:"og_description,omitempty"`
	OGImage          string       `json:"og_image,omitempty"`
	Score            int          `json:"score"`
	Issues           []AuditIssue `json:"issues"`
}

// InspectHTML performs an exhaustive on-page technical SEO audit of raw HTML
func InspectHTML(pageURL string, rawHTML []byte) (*TechnicalAuditReport, error) {
	doc, err := html.Parse(bytes.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("failed parsing HTML document: %w", err)
	}

	report := &TechnicalAuditReport{
		URL:    pageURL,
		H1Text: make([]string, 0),
		Score:  100,
		Issues: make([]AuditIssue, 0),
	}

	baseURL, _ := url.Parse(pageURL)
	normBase, _ := urlnorm.Normalize(pageURL, nil)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "html":
				for _, a := range n.Attr {
					if a.Key == "lang" {
						report.Lang = a.Val
					}
				}

			case "title":
				if report.Title == "" && n.FirstChild != nil {
					// Concatenate ALL text children, not just the first —
					// entities/inline markup can split a <title>'s text
					// into multiple text nodes.
					report.Title = strings.TrimSpace(nodeText(n))
				}

			case "meta":
				var name, property, content, charset string
				for _, a := range n.Attr {
					switch strings.ToLower(a.Key) {
					case "name":
						name = strings.ToLower(a.Val)
					case "property":
						property = strings.ToLower(a.Val)
					case "content":
						content = strings.TrimSpace(a.Val)
					case "charset":
						charset = strings.ToLower(a.Val)
					}
				}

				if charset != "" {
					report.Charset = charset
				}
				if name == "description" {
					report.MetaDescription = content
				}
				if name == "robots" {
					report.MetaRobots = content
					lower := strings.ToLower(content)
					if strings.Contains(lower, "noindex") {
						report.HasMetaNoindex = true
					}
					if strings.Contains(lower, "nofollow") {
						report.HasMetaNofollow = true
					}
				}
				if name == "viewport" {
					report.HasViewport = true
				}
				if property == "og:title" {
					report.OGTitle = content
				}
				if property == "og:description" {
					report.OGDescription = content
				}
				if property == "og:image" {
					report.OGImage = content
				}

			case "link":
				var rel, href string
				for _, a := range n.Attr {
					if strings.ToLower(a.Key) == "rel" {
						rel = strings.ToLower(a.Val)
					}
					if strings.ToLower(a.Key) == "href" {
						href = strings.TrimSpace(a.Val)
					}
				}
				if rel == "canonical" && href != "" {
					report.Canonical = href
				}

			case "h1":
				report.H1Count++
				var text strings.Builder
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.TextNode {
						text.WriteString(c.Data)
					}
				}
				if t := strings.TrimSpace(text.String()); t != "" {
					report.H1Text = append(report.H1Text, t)
				}

			case "h2":
				report.H2Count++
			case "h3":
				report.H3Count++

			case "img":
				report.TotalImages++
				hasAlt := false
				for _, a := range n.Attr {
					if strings.ToLower(a.Key) == "alt" && strings.TrimSpace(a.Val) != "" {
						hasAlt = true
						break
					}
				}
				if !hasAlt {
					report.ImagesMissingAlt++
				}

			case "a":
				for _, a := range n.Attr {
					if strings.ToLower(a.Key) != "href" {
						continue
					}
					href := strings.TrimSpace(a.Val)
					if href == "" {
						continue
					}
					lowerHref := strings.ToLower(href)
					switch {
					case strings.HasPrefix(lowerHref, "mailto:"),
						strings.HasPrefix(lowerHref, "tel:"),
						strings.HasPrefix(lowerHref, "javascript:"):
						continue
					case strings.HasPrefix(href, "#"):
						// Pure same-page fragment link — not a navigable
						// internal link.
						continue
					}
					resolved, err := urlnorm.Normalize(href, normBase)
					if err != nil {
						continue
					}
					if normBase != nil && strings.EqualFold(resolved.Host, normBase.Host) {
						report.InternalLinks++
					} else {
						report.ExternalLinks++
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)

	report.TitleLength = len([]rune(report.Title))
	report.MetaDescLength = len([]rune(report.MetaDescription))
	report.TitleWidth = textutil.DisplayWidth(report.Title)
	report.MetaDescWidth = textutil.DisplayWidth(report.MetaDescription)

	// Script drives which meta-description width band applies (see
	// textutil.MetaDescLimitsFor); classify from the page's textual
	// metadata since technical.go doesn't walk full body text.
	script := textutil.ClassifyWithHint(report.Title+" "+report.MetaDescription+" "+strings.Join(report.H1Text, " "), report.Lang)
	report.Script = script.String()

	// Validate Canonical self-reference: resolves a relative canonical
	// against the page URL before comparing.
	if report.Canonical != "" {
		report.IsSelfCanonical = urlnorm.Same(report.Canonical, pageURL, baseURL)
	}

	// Score Deductions and Issue Diagnoses
	if report.Title == "" {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityCritical,
			Category: "Meta",
			Message:  "Missing <title> tag",
		})
		report.Score -= 20
	} else if report.TitleWidth < textutil.TitleLimits.Min || report.TitleWidth > textutil.TitleLimits.Max {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning,
			Category: "Meta",
			Message: fmt.Sprintf("<title> display width (%d units, %d chars) outside recommended %d-%d unit range (~600px in Google's SERP)",
				report.TitleWidth, report.TitleLength, textutil.TitleLimits.Min, textutil.TitleLimits.Max),
		})
		report.Score -= 5
	}

	if report.MetaDescription == "" {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning,
			Category: "Meta",
			Message:  "Missing meta description tag",
		})
		report.Score -= 10
	} else {
		descLimits := textutil.MetaDescLimitsFor(script)
		if report.MetaDescWidth < descLimits.Min || report.MetaDescWidth > descLimits.Max {
			report.Issues = append(report.Issues, AuditIssue{
				Severity: SeverityInfo,
				Category: "Meta",
				Message: fmt.Sprintf("Meta description display width (%d units, %d chars) outside recommended %d-%d unit range for %s content",
					report.MetaDescWidth, report.MetaDescLength, descLimits.Min, descLimits.Max, script),
			})
		}
	}

	if report.HasMetaNoindex {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityCritical,
			Category: "Indexability",
			Message:  "<meta name='robots' content='noindex'> detected",
			Details:  "Page is explicitly hidden from search engine indexes.",
		})
		report.Score -= 40
	}

	if report.Canonical == "" {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning,
			Category: "Canonical",
			Message:  "Missing <link rel='canonical'> tag",
		})
		report.Score -= 10
	}

	if !report.HasViewport {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityCritical,
			Category: "Mobile",
			Message:  "Missing mobile viewport meta tag",
		})
		report.Score -= 15
	}

	if report.H1Count == 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning,
			Category: "Content Structure",
			Message:  "Missing <h1> heading tag",
		})
		report.Score -= 10
	} else if report.H1Count > 1 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo,
			Category: "Content Structure",
			Message:  fmt.Sprintf("Multiple <h1> headings detected (%d found). Best practice is exactly 1 per page.", report.H1Count),
		})
	}

	if report.ImagesMissingAlt > 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning,
			Category: "Accessibility / Image SEO",
			Message:  fmt.Sprintf("%d of %d images missing 'alt' text", report.ImagesMissingAlt, report.TotalImages),
		})
		report.Score -= 5
	}

	if report.Score < 0 {
		report.Score = 0
	}

	return report, nil
}
