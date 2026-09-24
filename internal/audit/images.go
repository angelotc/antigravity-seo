package audit

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// ImageAuditReport evaluates image optimization for SEO and CLS prevention
type ImageAuditReport struct {
	URL                string       `json:"url"`
	TotalImages        int          `json:"total_images"`
	MissingAlt         int          `json:"missing_alt"`
	EmptyAltDecorative int          `json:"empty_alt_decorative"`
	LongAlt            int          `json:"long_alt"`
	MissingDimensions  int          `json:"missing_dimensions"`
	LazyLoaded         int          `json:"lazy_loaded"`
	ModernFormats      int          `json:"modern_formats"` // webp / avif
	LegacyFormats      int          `json:"legacy_formats"` // jpg / png / gif
	InsecureHTTP       int          `json:"insecure_http"`
	DataURI            int          `json:"data_uri"`
	OversizedDataURI   int          `json:"oversized_data_uri"` // > 10KB inlined
	Score              int          `json:"score"`
	Issues             []AuditIssue `json:"issues"`
}

// InspectImages audits every <img> element for SEO-relevant attributes
func InspectImages(pageURL string, rawHTML []byte) (*ImageAuditReport, error) {
	doc, err := html.Parse(bytes.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("failed parsing HTML document: %w", err)
	}

	report := &ImageAuditReport{
		URL:    pageURL,
		Score:  100,
		Issues: []AuditIssue{},
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			report.TotalImages++

			var src, alt string
			var hasAlt, hasWidth, hasHeight, hasLoading bool
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "src":
					src = strings.TrimSpace(a.Val)
				case "alt":
					hasAlt = true
					alt = strings.TrimSpace(a.Val)
				case "width":
					hasWidth = a.Val != ""
				case "height":
					hasHeight = a.Val != ""
				case "loading":
					if strings.EqualFold(a.Val, "lazy") {
						hasLoading = true
					}
				}
			}

			// Alt classification: missing alt hurts; empty alt is valid for decorative images
			if !hasAlt {
				report.MissingAlt++
			} else if alt == "" {
				report.EmptyAltDecorative++
			} else if len([]rune(alt)) > 125 {
				report.LongAlt++
			}

			if !hasWidth || !hasHeight {
				report.MissingDimensions++
			}
			if hasLoading {
				report.LazyLoaded++
			}

			switch {
			case strings.HasPrefix(src, "data:"):
				report.DataURI++
				if len(src) > 10*1024 {
					report.OversizedDataURI++
				}
			case strings.HasPrefix(src, "http://"):
				report.InsecureHTTP++
			}

			lower := strings.ToLower(src)
			switch {
			case strings.Contains(lower, ".webp") || strings.Contains(lower, ".avif"):
				report.ModernFormats++
			case strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") ||
				strings.Contains(lower, ".png") || strings.Contains(lower, ".gif"):
				report.LegacyFormats++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Base scores against deductibles
	if report.TotalImages == 0 {
		report.Score = 100
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo, Category: "Images",
			Message: "No images found on page",
		})
		return report, nil
	}

	if report.MissingAlt > 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning, Category: "Images",
			Message: fmt.Sprintf("%d of %d images missing 'alt' attribute (accessibility + image SEO)", report.MissingAlt, report.TotalImages),
		})
		report.Score -= minInt(report.MissingAlt*5, 25)
	}
	if report.LongAlt > 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo, Category: "Images",
			Message: fmt.Sprintf("%d images have alt text longer than 125 characters", report.LongAlt),
		})
		report.Score -= minInt(report.LongAlt*2, 6)
	}
	if report.MissingDimensions > 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning, Category: "Images",
			Message: fmt.Sprintf("%d images missing width/height attributes (CLS risk)", report.MissingDimensions),
			Details: "Explicit dimensions let the browser reserve layout space and prevent cumulative layout shift.",
		})
		report.Score -= minInt(report.MissingDimensions*3, 15)
	}
	if report.InsecureHTTP > 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning, Category: "Images",
			Message: fmt.Sprintf("%d images loaded over insecure http:// (mixed content)", report.InsecureHTTP),
		})
		report.Score -= minInt(report.InsecureHTTP*2, 10)
	}
	if report.OversizedDataURI > 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityWarning, Category: "Images",
			Message: fmt.Sprintf("%d images inlined as data URIs larger than 10KB (HTML bloat)", report.OversizedDataURI),
		})
		report.Score -= minInt(report.OversizedDataURI*5, 15)
	}
	if report.LegacyFormats > 0 && report.ModernFormats == 0 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo, Category: "Images",
			Message: fmt.Sprintf("%d legacy-format images (jpg/png/gif); consider WebP/AVIF for smaller payloads", report.LegacyFormats),
		})
	}
	if report.LazyLoaded == 0 && report.TotalImages > 3 {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo, Category: "Images",
			Message: "No images use loading=\"lazy\" — defer below-fold images to improve LCP",
		})
	}

	if report.Score < 0 {
		report.Score = 0
	}
	return report, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
