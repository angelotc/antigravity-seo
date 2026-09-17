package audit

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"antigravity-seo/internal/crawler"
)

// IssueSeverity levels for SEO diagnostics
type IssueSeverity string

const (
	SeverityCritical IssueSeverity = "CRITICAL"
	SeverityWarning  IssueSeverity = "WARNING"
	SeverityInfo     IssueSeverity = "INFO"
	SeverityPass     IssueSeverity = "PASS"
)

// AuditIssue represents a diagnosed SEO problem
type AuditIssue struct {
	Severity IssueSeverity `json:"severity"`
	Category string        `json:"category"`
	Message  string        `json:"message"`
	Details  string        `json:"details,omitempty"`
}

// HeaderAuditResult provides a complete SEO evaluation of HTTP transport headers
type HeaderAuditResult struct {
	URL             string              `json:"url"`
	FinalURL        string              `json:"final_url"`
	StatusCode      int                 `json:"status_code"`
	IsSuccess       bool                `json:"is_success"`
	IsRedirected    bool                `json:"is_redirected"`
	RedirectHops    int                 `json:"redirect_hops"`
	RedirectChain   []crawler.RedirectHop `json:"redirect_chain,omitempty"`
	XRobotsTag      string              `json:"x_robots_tag,omitempty"`
	HasNoindex      bool                `json:"has_noindex"`
	HasNofollow     bool                `json:"has_nofollow"`
	HeaderCanonical string              `json:"header_canonical,omitempty"`
	ContentEncoding string              `json:"content_encoding,omitempty"`
	CacheControl    string              `json:"cache_control,omitempty"`
	HSTS            bool                `json:"has_hsts"`
	Server          string              `json:"server,omitempty"`
	TTFBMS          int64               `json:"ttfb_ms"`
	TotalDurationMS int64               `json:"total_duration_ms"`
	Issues          []AuditIssue        `json:"issues"`
}

// InspectHeaders analyzes a FetchResult specifically for HTTP header SEO compliance
func InspectHeaders(res *crawler.FetchResult) *HeaderAuditResult {
	result := &HeaderAuditResult{
		URL:             res.RequestedURL,
		FinalURL:        res.FinalURL,
		StatusCode:      res.StatusCode,
		IsSuccess:       res.StatusCode >= 200 && res.StatusCode < 300,
		IsRedirected:    len(res.Redirects) > 0,
		RedirectHops:    len(res.Redirects),
		RedirectChain:   res.Redirects,
		ContentEncoding: strings.Join(res.Headers["Content-Encoding"], ", "),
		CacheControl:    strings.Join(res.Headers["Cache-Control"], ", "),
		HSTS:            len(res.Headers["Strict-Transport-Security"]) > 0,
		Server:          strings.Join(res.Headers["Server"], ", "),
		TTFBMS:          res.Timings.TTFBMS,
		TotalDurationMS: res.Timings.TotalMS,
		Issues:          make([]AuditIssue, 0),
	}

	// 1. Status Code Evaluation
	switch {
	case res.StatusCode == http.StatusOK:
		result.Issues = append(result.Issues, AuditIssue{
			Severity: SeverityPass,
			Category: "Status",
			Message:  "Page returned 200 OK",
		})
	case res.StatusCode == http.StatusNotFound:
		result.Issues = append(result.Issues, AuditIssue{
			Severity: SeverityCritical,
			Category: "Status",
			Message:  "Page returned 404 Not Found",
			Details:  "Broken URL leads to crawl budget waste and poor user experience.",
		})
	case res.StatusCode == http.StatusGone:
		result.Issues = append(result.Issues, AuditIssue{
			Severity: SeverityWarning,
			Category: "Status",
			Message:  "Page returned 410 Gone (Permanently removed)",
		})
	case res.StatusCode >= 500:
		result.Issues = append(result.Issues, AuditIssue{
			Severity: SeverityCritical,
			Category: "Status",
			Message:  fmt.Sprintf("Server error %d %s", res.StatusCode, res.StatusText),
			Details:  "Search engine bots will drop or delay indexing during persistent server errors.",
		})
	}

	// 2. Redirect Chain Evaluation
	if result.IsRedirected {
		if result.RedirectHops > 2 {
			result.Issues = append(result.Issues, AuditIssue{
				Severity: SeverityWarning,
				Category: "Redirects",
				Message:  fmt.Sprintf("Long redirect chain detected (%d hops)", result.RedirectHops),
				Details:  "Redirect chains waste crawl budget and dilute PageRank link equity.",
			})
		}

		for i, hop := range res.Redirects {
			if hop.StatusCode == http.StatusFound || hop.StatusCode == http.StatusTemporaryRedirect {
				result.Issues = append(result.Issues, AuditIssue{
					Severity: SeverityWarning,
					Category: "Redirects",
					Message:  fmt.Sprintf("Temporary redirect %d used at hop #%d (%s)", hop.StatusCode, i+1, hop.URL),
					Details:  "Use 301 Moved Permanently or 308 Permanent Redirect for SEO value transfer.",
				})
			}
		}

		reqParsed, _ := url.Parse(res.RequestedURL)
		finParsed, _ := url.Parse(res.FinalURL)
		if reqParsed != nil && finParsed != nil && reqParsed.Host != finParsed.Host {
			result.Issues = append(result.Issues, AuditIssue{
				Severity: SeverityInfo,
				Category: "Redirects",
				Message:  fmt.Sprintf("Cross-domain redirect: %s -> %s", reqParsed.Host, finParsed.Host),
			})
		}
	}

	// 3. X-Robots-Tag Inspection
	for _, val := range res.Headers["X-Robots-Tag"] {
		result.XRobotsTag = val
		lower := strings.ToLower(val)
		if strings.Contains(lower, "noindex") {
			result.HasNoindex = true
			result.Issues = append(result.Issues, AuditIssue{
				Severity: SeverityCritical,
				Category: "Indexability",
				Message:  "X-Robots-Tag contains 'noindex'",
				Details:  "This header completely blocks search engines from indexing this page.",
			})
		}
		if strings.Contains(lower, "nofollow") {
			result.HasNofollow = true
			result.Issues = append(result.Issues, AuditIssue{
				Severity: SeverityWarning,
				Category: "Indexability",
				Message:  "X-Robots-Tag contains 'nofollow'",
				Details:  "Search engine crawlers will not follow outgoing links from this URL.",
			})
		}
	}

	// 4. HTTP Link Canonical Header
	for _, link := range res.Headers["Link"] {
		if strings.Contains(link, `rel="canonical"`) || strings.Contains(link, `rel=canonical`) {
			parts := strings.Split(link, ";")
			rawURL := strings.Trim(parts[0], "<> ")
			result.HeaderCanonical = rawURL
			result.Issues = append(result.Issues, AuditIssue{
				Severity: SeverityInfo,
				Category: "Canonical",
				Message:  fmt.Sprintf("HTTP Link header canonical set: %s", rawURL),
			})
		}
	}

	// 5. Compression & TTFB
	if result.ContentEncoding == "" {
		result.Issues = append(result.Issues, AuditIssue{
			Severity: SeverityWarning,
			Category: "Performance",
			Message:  "Response is not compressed (missing Content-Encoding: gzip/br/zstd)",
		})
	}
	if res.Timings.TTFBMS > 800 {
		result.Issues = append(result.Issues, AuditIssue{
			Severity: SeverityWarning,
			Category: "Performance",
			Message:  fmt.Sprintf("Slow Time-To-First-Byte (TTFB: %dms > 800ms threshold)", res.Timings.TTFBMS),
			Details:  "High TTFB negatively impacts Core Web Vitals (LCP) and bot crawl frequency.",
		})
	} else if res.Timings.TTFBMS > 0 && res.Timings.TTFBMS <= 200 {
		result.Issues = append(result.Issues, AuditIssue{
			Severity: SeverityPass,
			Category: "Performance",
			Message:  fmt.Sprintf("Excellent TTFB (%dms <= 200ms)", res.Timings.TTFBMS),
		})
	}

	// 6. Security (HSTS)
	if !result.HSTS && strings.HasPrefix(res.FinalURL, "https://") {
		result.Issues = append(result.Issues, AuditIssue{
			Severity: SeverityInfo,
			Category: "Security",
			Message:  "Missing Strict-Transport-Security (HSTS) header on HTTPS page",
		})
	}

	return result
}
