package crawler

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"antigravity-seo/internal/urlnorm"
)

// CrawledPage is one page discovered during a bounded site crawl
type CrawledPage struct {
	URL        string `json:"url"`
	FinalURL   string `json:"final_url,omitempty"` // where the fetch actually landed, if different (redirects)
	StatusCode int    `json:"status_code"`
	Title      string `json:"title,omitempty"`
	LastMod    string `json:"lastmod,omitempty"` // from Last-Modified header, W3C datetime
	Noindex    bool   `json:"noindex,omitempty"`
}

// CrawlReport is the outcome of a bounded same-origin crawl
type CrawlReport struct {
	StartURL   string        `json:"start_url"`
	Pages      []CrawledPage `json:"pages"`
	Excluded   []string      `json:"excluded,omitempty"` // noindex or non-200 pages skipped from output
	Errors     []string      `json:"errors,omitempty"`
	Visited    int           `json:"visited"`
	DurationMS int64         `json:"duration_ms"`
}

type crawlJob struct {
	URL string
}

// CrawlSite performs a bounded breadth-first crawl over one origin,
// skipping robots-disallowed and noindex pages. Used for sitemap generation.
func (c *SafeClient) CrawlSite(ctx context.Context, startURL string, maxPages int) (*CrawlReport, error) {
	rawStart := normalizeScheme(startURL)
	parsedStart, err := urlnorm.Normalize(rawStart, nil)
	if err != nil {
		return nil, err
	}
	start := parsedStart.String()

	// Respect robots.txt disallow rules for "*". Per RFC 9309: a robots.txt
	// that is unreachable or errors server-side (5xx) means the whole site
	// is disallowed; one that 404s/4xx means there are no restrictions.
	var blocked func(string) bool
	var crawlDelaySec float64
	robotsRes, rerr := c.Fetch(ctx, parsedStart.Scheme+"://"+parsedStart.Host+"/robots.txt")
	switch {
	case rerr != nil || (robotsRes != nil && robotsRes.StatusCode >= 500):
		blocked = func(string) bool { return true }
	case robotsRes.StatusCode == http.StatusOK:
		parsedRobots := parseRobots(string(robotsRes.Body))
		blocked = func(path string) bool {
			allowed, _ := parsedRobots.allowsPath("*", path)
			return !allowed
		}
		if group := parsedRobots.groupForAgent("*"); group != nil && group.hasDelay {
			crawlDelaySec = group.crawlDelay
			if crawlDelaySec > 30 {
				crawlDelaySec = 30
			}
		}
	default:
		blocked = func(string) bool { return false }
	}
	crawlDelay := time.Duration(crawlDelaySec * float64(time.Second))

	report := &CrawlReport{StartURL: start}

	var (
		mu        sync.Mutex
		visited   = map[string]bool{}
		queue     []crawlJob
		workers   = 8
		wg        sync.WaitGroup
		extractMu sync.Mutex
	)
	if crawlDelaySec > 0 {
		// A crawl-delay only makes sense against a single in-flight request.
		workers = 1
	}
	sem := make(chan struct{}, workers)

	enqueue := func(raw string) {
		normalized, nerr := urlnorm.Normalize(raw, parsedStart)
		if nerr != nil {
			return
		}
		key := urlnorm.Key(raw, parsedStart)
		mu.Lock()
		defer mu.Unlock()
		if visited[key] {
			return
		}
		if len(visited) >= maxPages*2 { // allow headroom for excluded pages
			return
		}
		visited[key] = true
		queue = append(queue, crawlJob{URL: normalized.String()})
	}

	// The seed URL must itself respect robots.txt before ever being fetched.
	if blocked(parsedStart.Path) {
		report.Excluded = append(report.Excluded, start)
		return report, nil
	}
	enqueue(start)

	for len(queue) > 0 {
		batch := queue
		queue = nil

		for _, job := range batch {
			mu.Lock()
			done := report.Visited >= maxPages
			mu.Unlock()
			if done {
				break
			}
			wg.Add(1)
			go func(u string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if crawlDelaySec > 0 {
					// Runs after this fetch completes but before the semaphore
					// is released, so the next worker can't start early.
					defer func() {
						select {
						case <-time.After(crawlDelay):
						case <-ctx.Done():
						}
					}()
				}

				mu.Lock()
				if report.Visited >= maxPages {
					mu.Unlock()
					return
				}
				report.Visited++
				mu.Unlock()

				res, err := c.Fetch(ctx, u)
				if err != nil {
					extractMu.Lock()
					report.Errors = append(report.Errors, u+": "+err.Error())
					extractMu.Unlock()
					return
				}

				// Don't re-crawl (or re-report) wherever this redirected to.
				if res.FinalURL != "" && res.FinalURL != u {
					if fk := urlnorm.Key(res.FinalURL, parsedStart); fk != "" {
						mu.Lock()
						visited[fk] = true
						mu.Unlock()
					}
				}

				if res.StatusCode != http.StatusOK {
					extractMu.Lock()
					report.Excluded = append(report.Excluded, u)
					extractMu.Unlock()
					return
				}

				if finalNorm, ferr := urlnorm.Normalize(res.FinalURL, parsedStart); ferr == nil && !strings.EqualFold(finalNorm.Host, parsedStart.Host) {
					// Redirected off-origin: not this site's page, exclude it.
					extractMu.Lock()
					report.Excluded = append(report.Excluded, u)
					extractMu.Unlock()
					return
				}

				title, links, metaNoindex := extractTitleLinks(res.Body)
				noindex := metaNoindex || hasNoindexXRobotsTag(res.Headers["X-Robots-Tag"])
				page := CrawledPage{
					URL:        u,
					FinalURL:   res.FinalURL,
					StatusCode: res.StatusCode,
					Title:      title,
					Noindex:    noindex,
					LastMod:    lastModFromHeaders(res),
				}

				extractMu.Lock()
				if noindex {
					report.Excluded = append(report.Excluded, u)
				} else {
					report.Pages = append(report.Pages, page)
				}
				extractMu.Unlock()

				linkBase := res.FinalURL
				if linkBase == "" {
					linkBase = u
				}
				for _, link := range links {
					abs, ok := sameOriginURL(parsedStart, linkBase, link)
					if !ok {
						continue
					}
					if blocked(abs.Path) {
						continue
					}
					if isCrawlablePath(abs.Path) {
						enqueue(abs.String())
					}
				}
			}(job.URL)
		}
		wg.Wait()
		if report.Visited >= maxPages {
			break
		}
	}

	return report, nil
}

// extractTitleLinks pulls title, same-document links, and noindex state from HTML
func extractTitleLinks(body []byte) (title string, links []string, noindex bool) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", nil, false
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "title":
				if title == "" {
					var sb strings.Builder
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.TextNode {
							sb.WriteString(c.Data)
						}
					}
					title = strings.TrimSpace(sb.String())
				}
			case "meta":
				var name, content string
				for _, a := range n.Attr {
					switch strings.ToLower(a.Key) {
					case "name":
						name = strings.ToLower(a.Val)
					case "content":
						content = strings.ToLower(a.Val)
					}
				}
				if name == "robots" && containsNoindexDirective(content) {
					noindex = true
				}
			case "a":
				for _, a := range n.Attr {
					if strings.EqualFold(a.Key, "href") {
						href := strings.TrimSpace(a.Val)
						if href != "" && !strings.HasPrefix(href, "#") &&
							!strings.HasPrefix(href, "mailto:") && !strings.HasPrefix(href, "tel:") &&
							!strings.HasPrefix(href, "javascript:") {
							links = append(links, href)
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return title, links, noindex
}

// containsNoindexDirective reports whether a comma-separated robots directive
// list (an X-Robots-Tag value or a <meta name="robots"> content attribute)
// contains a "noindex" or "none" token. Matching is case-insensitive and
// tolerates an optional "botname:" prefix, e.g. "googlebot: noindex".
func containsNoindexDirective(raw string) bool {
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, ":"); idx >= 0 {
			part = strings.TrimSpace(part[idx+1:])
		}
		switch strings.ToLower(part) {
		case "noindex", "none":
			return true
		}
	}
	return false
}

// hasNoindexXRobotsTag reports whether any X-Robots-Tag header value (a page
// may send several) carries a noindex/none directive.
func hasNoindexXRobotsTag(values []string) bool {
	for _, v := range values {
		if containsNoindexDirective(v) {
			return true
		}
	}
	return false
}

// lastModFromHeaders converts a Last-Modified header to W3C datetime
func lastModFromHeaders(res *FetchResult) string {
	lm := strings.Join(res.Headers["Last-Modified"], "")
	if lm == "" {
		return ""
	}
	if t, err := http.ParseTime(lm); err == nil {
		return t.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return ""
}

// sameOriginURL resolves a link against its source page and enforces origin match
func sameOriginURL(origin *url.URL, pageURL, link string) (*url.URL, bool) {
	page, err := urlnorm.Normalize(pageURL, origin)
	if err != nil {
		return nil, false
	}
	abs, err := urlnorm.Normalize(link, page)
	if err != nil {
		return nil, false
	}
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return nil, false
	}
	if !strings.EqualFold(abs.Host, origin.Host) {
		return nil, false
	}
	return abs, true
}

// isCrawlablePath filters out asset-like and non-content paths
func isCrawlablePath(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".svg", ".ico", ".css", ".js", ".json", ".xml", ".pdf", ".zip", ".gz", ".mp4", ".webm", ".woff", ".woff2", ".ttf", ".eot"} {
		if strings.HasSuffix(lower, ext) {
			return false
		}
	}
	for _, seg := range strings.Split(lower, "/") {
		if seg == "wp-admin" || seg == "admin" || seg == "cart" || seg == "checkout" || seg == "login" || seg == "logout" || seg == "search" || seg == "feed" {
			return false
		}
	}
	return true
}

func normalizeScheme(raw string) string {
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return "https://" + raw
	}
	return raw
}

// GenerateSitemapXML renders crawled pages as a sitemap <urlset> document
func GenerateSitemapXML(pages []CrawledPage) []byte {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, p := range pages {
		loc := p.FinalURL
		if loc == "" {
			loc = p.URL
		}
		sb.WriteString("  <url>\n    <loc>")
		sb.WriteString(xmlEscape(loc))
		sb.WriteString("</loc>\n")
		if p.LastMod != "" {
			sb.WriteString("    <lastmod>")
			sb.WriteString(xmlEscape(p.LastMod))
			sb.WriteString("</lastmod>\n")
		}
		sb.WriteString("  </url>\n")
	}
	sb.WriteString("</urlset>\n")
	return []byte(sb.String())
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(s)
}
