package crawler

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/html"
)

// CrawledPage is one page discovered during a bounded site crawl
type CrawledPage struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	Title      string `json:"title,omitempty"`
	LastMod    string `json:"lastmod,omitempty"` // from Last-Modified header, W3C datetime
	Noindex    bool   `json:"noindex,omitempty"`
}

// CrawlReport is the outcome of a bounded same-origin crawl
type CrawlReport struct {
	StartURL  string         `json:"start_url"`
	Pages     []CrawledPage  `json:"pages"`
	Excluded  []string       `json:"excluded,omitempty"` // noindex or non-200 pages skipped from output
	Errors    []string       `json:"errors,omitempty"`
	Visited   int            `json:"visited"`
	DurationMS int64         `json:"duration_ms"`
}

type crawlJob struct {
	URL string
}

// CrawlSite performs a bounded breadth-first crawl over one origin,
// skipping robots-disallowed and noindex pages. Used for sitemap generation.
func (c *SafeClient) CrawlSite(ctx context.Context, startURL string, maxPages int) (*CrawlReport, error) {
	start := normalizeScheme(startURL)
	parsedStart, err := url.Parse(start)
	if err != nil {
		return nil, err
	}
	// Normalize the root so "/" and "" dedupe in the visited set
	if parsedStart.Path == "" {
		parsedStart.Path = "/"
		start = parsedStart.String()
	}

	// Respect robots.txt disallow rules for "*"
	var blocked func(string) bool
	if robotsRes, rerr := c.Fetch(ctx, parsedStart.Scheme+"://"+parsedStart.Host+"/robots.txt"); rerr == nil && robotsRes.StatusCode == http.StatusOK {
		parsedRobots := parseRobots(string(robotsRes.Body))
		blocked = func(path string) bool {
			allowed, _ := parsedRobots.allowsPath("*", path)
			return !allowed
		}
	} else {
		blocked = func(string) bool { return false }
	}

	report := &CrawlReport{StartURL: start}

	var (
		mu        sync.Mutex
		visited   = map[string]bool{}
		queue     []crawlJob
		sem       = make(chan struct{}, 8)
		wg        sync.WaitGroup
		extractMu sync.Mutex
	)

	enqueue := func(u string) {
		mu.Lock()
		defer mu.Unlock()
		if visited[u] {
			return
		}
		if len(visited) >= maxPages*2 { // allow headroom for excluded pages
			return
		}
		visited[u] = true
		queue = append(queue, crawlJob{URL: u})
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

				if res.StatusCode != http.StatusOK {
					extractMu.Lock()
					report.Excluded = append(report.Excluded, u)
					extractMu.Unlock()
					return
				}

				title, links, noindex := extractTitleLinks(res.Body)
				page := CrawledPage{
					URL:        u,
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

				for _, link := range links {
					abs, ok := sameOriginURL(parsedStart, u, link)
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
				if name == "robots" && strings.Contains(content, "noindex") {
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
	page, err := url.Parse(pageURL)
	if err != nil {
		return nil, false
	}
	abs, err := page.Parse(link)
	if err != nil {
		return nil, false
	}
	if abs.Host != origin.Host || (abs.Scheme != "http" && abs.Scheme != "https") {
		return nil, false
	}
	abs.Fragment = ""
	abs.RawFragment = ""
	if abs.Path == "" {
		abs.Path = "/"
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
		sb.WriteString("  <url>\n    <loc>")
		sb.WriteString(xmlEscape(p.URL))
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
