package crawler

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"sort"
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

const (
	crawlPagesDefault       = 100
	crawlConcurrencyDefault = 8
	crawlMaxRobotsBlocked   = 200
)

// CrawlOptions tunes a bounded same-origin crawl
type CrawlOptions struct {
	MaxPages    int // fetch budget (<=0 -> 100)
	Concurrency int // workers (<=0 -> 8); forced to 1 when robots.txt sets Crawl-delay
	// Visit, if set, is called once per fetched URL (any status, or a fetch error) from a
	// worker goroutine; it must be safe for concurrent use. v.Result.Body is only valid during the call.
	Visit func(v *CrawlVisit)
}

// CrawlVisit describes one fetched URL, handed to CrawlOptions.Visit
type CrawlVisit struct {
	URL    string       // normalized URL that was requested
	Depth  int          // clicks from the start URL (start = 0)
	Result *FetchResult // nil when Err != nil
	Err    error
	// Same-origin, normalized, de-duplicated outlinks found on this page
	// (before robots/path filtering); nil for non-200 or off-origin.
	Links   []string
	Title   string
	Noindex bool // meta robots or X-Robots-Tag noindex/none
}

// CrawlResult summarizes a crawl run; per-page data goes through Visit
type CrawlResult struct {
	StartURL      string
	Visited       int
	Truncated     bool     // budget ran out while discovered URLs were still unfetched
	RobotsBlocked []string // same-origin URLs linked from crawled pages but disallowed by robots.txt (deduped, capped at 200)
	DurationMS    int64
}

type crawlJob struct {
	URL   string
	Depth int
}

// CrawlSite performs a bounded breadth-first crawl over one origin,
// skipping robots-disallowed and noindex pages. Used for sitemap generation.
func (c *SafeClient) CrawlSite(ctx context.Context, startURL string, maxPages int) (*CrawlReport, error) {
	parsedStart, err := urlnorm.Normalize(normalizeScheme(startURL), nil)
	if err != nil {
		return nil, err
	}

	report := &CrawlReport{}
	var mu sync.Mutex
	res, err := c.Crawl(ctx, startURL, CrawlOptions{
		MaxPages: maxPages,
		Visit: func(v *CrawlVisit) {
			mu.Lock()
			defer mu.Unlock()
			if v.Err != nil {
				report.Errors = append(report.Errors, v.URL+": "+v.Err.Error())
				return
			}
			r := v.Result
			if r.StatusCode != http.StatusOK {
				report.Excluded = append(report.Excluded, v.URL)
				return
			}
			if finalNorm, ferr := urlnorm.Normalize(r.FinalURL, parsedStart); ferr == nil && !strings.EqualFold(finalNorm.Host, parsedStart.Host) {
				// Redirected off-origin: not this site's page, exclude it.
				report.Excluded = append(report.Excluded, v.URL)
				return
			}
			if v.Noindex {
				report.Excluded = append(report.Excluded, v.URL)
				return
			}
			report.Pages = append(report.Pages, CrawledPage{
				URL:        v.URL,
				FinalURL:   r.FinalURL,
				StatusCode: r.StatusCode,
				Title:      v.Title,
				Noindex:    v.Noindex,
				LastMod:    lastModFromHeaders(r),
			})
		},
	})
	if err != nil {
		return nil, err
	}
	report.StartURL = res.StartURL
	report.Visited = res.Visited
	report.DurationMS = res.DurationMS
	if res.Visited == 0 && len(res.RobotsBlocked) == 1 && res.RobotsBlocked[0] == res.StartURL {
		report.Excluded = append(report.Excluded, res.StartURL) // seed disallowed by robots.txt
	}
	return report, nil
}

// robotsTargets returns the forms of u that robots.txt rules are matched
// against: the percent-encoded path plus "?query" (RFC 9309), and the decoded
// equivalent so rules written with raw Unicode still apply.
func robotsTargets(u *url.URL) [2]string {
	enc, dec := u.EscapedPath(), u.Path
	if enc == "" {
		enc, dec = "/", "/"
	}
	if u.RawQuery != "" {
		enc += "?" + u.RawQuery
		dec += "?" + u.RawQuery
	}
	return [2]string{enc, dec}
}

// Crawl performs a bounded breadth-first crawl over one origin, reporting
// every fetched URL through opts.Visit. Batch index is click depth.
func (c *SafeClient) Crawl(ctx context.Context, startURL string, opts CrawlOptions) (*CrawlResult, error) {
	began := time.Now()
	maxPages, workers := opts.MaxPages, opts.Concurrency
	if maxPages <= 0 {
		maxPages = crawlPagesDefault
	}
	if workers <= 0 {
		workers = crawlConcurrencyDefault
	}
	visit := opts.Visit
	if visit == nil {
		visit = func(*CrawlVisit) {}
	}

	parsedStart, err := urlnorm.Normalize(normalizeScheme(startURL), nil)
	if err != nil {
		return nil, err
	}
	start := parsedStart.String()

	// Respect robots.txt disallow rules for "*". Per RFC 9309: a robots.txt
	// that is unreachable or errors server-side (5xx) means the whole site
	// is disallowed; one that 404s/4xx means there are no restrictions.
	var blocked func(*url.URL) bool
	var crawlDelaySec float64
	robotsRes, rerr := c.Fetch(ctx, parsedStart.Scheme+"://"+parsedStart.Host+"/robots.txt")
	switch {
	case rerr != nil || (robotsRes != nil && robotsRes.StatusCode >= 500):
		blocked = func(*url.URL) bool { return true }
	case robotsRes.StatusCode == http.StatusOK:
		parsedRobots := parseRobots(string(robotsRes.Body))
		blocked = func(u *url.URL) bool {
			for _, target := range robotsTargets(u) {
				if allowed, _ := parsedRobots.allowsPath("*", target); !allowed {
					return true
				}
			}
			return false
		}
		if group := parsedRobots.groupForAgent("*"); group != nil && group.hasDelay {
			crawlDelaySec = group.crawlDelay
			if crawlDelaySec > 30 {
				crawlDelaySec = 30
			}
		}
	default:
		blocked = func(*url.URL) bool { return false }
	}
	crawlDelay := time.Duration(crawlDelaySec * float64(time.Second))

	result := &CrawlResult{StartURL: start}

	var (
		mu         sync.Mutex
		visited    = map[string]bool{}
		queue      []crawlJob
		wg         sync.WaitGroup
		blockedSet = map[string]bool{}
	)
	if crawlDelaySec > 0 {
		// A crawl-delay only makes sense against a single in-flight request.
		workers = 1
	}
	sem := make(chan struct{}, workers)

	// enqueue and the robots bookkeeping only run on the coordinating
	// goroutine, between batches, so they need no locking.
	enqueue := func(raw string, depth int) {
		normalized, nerr := urlnorm.Normalize(raw, parsedStart)
		if nerr != nil {
			return
		}
		key := urlnorm.Key(raw, parsedStart)
		if visited[key] {
			return
		}
		if len(visited) >= maxPages*2 { // allow headroom for excluded pages
			result.Truncated = true
			return
		}
		visited[key] = true
		queue = append(queue, crawlJob{URL: normalized.String(), Depth: depth})
	}

	finish := func() (*CrawlResult, error) {
		sort.Strings(result.RobotsBlocked)
		result.DurationMS = time.Since(began).Milliseconds()
		return result, nil
	}

	// The seed URL must itself respect robots.txt before ever being fetched.
	if blocked(parsedStart) {
		result.RobotsBlocked = []string{start}
		return finish()
	}
	enqueue(start, 0)

	type foundLink struct {
		url   *url.URL
		depth int
	}

	for len(queue) > 0 {
		// Worker timing must not change what gets crawled: take the batch in
		// URL order, assign the budget before any fetch starts, and build the
		// next batch only once this one has fully finished.
		batch := queue
		queue = nil
		sort.Slice(batch, func(i, j int) bool { return batch[i].URL < batch[j].URL })
		if room := maxPages - result.Visited; len(batch) > room {
			batch = batch[:room]
			result.Truncated = true
		}
		result.Visited += len(batch)

		var (
			found  []foundLink // same-origin links discovered by this batch
			finals []string    // keys of redirect destinations reached by this batch
		)
		for _, job := range batch {
			wg.Add(1)
			go func(job crawlJob) {
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

				v := &CrawlVisit{URL: job.URL, Depth: job.Depth}
				res, err := c.Fetch(ctx, job.URL)
				if err != nil {
					v.Err = err
					visit(v)
					return
				}
				v.Result = res

				// Don't re-crawl (or re-report) wherever this redirected to.
				if res.FinalURL != "" && res.FinalURL != job.URL {
					if fk := urlnorm.Key(res.FinalURL, parsedStart); fk != "" {
						mu.Lock()
						finals = append(finals, fk)
						mu.Unlock()
					}
				}

				if res.StatusCode != http.StatusOK {
					visit(v)
					return
				}
				if finalNorm, ferr := urlnorm.Normalize(res.FinalURL, parsedStart); ferr == nil && !strings.EqualFold(finalNorm.Host, parsedStart.Host) {
					// Redirected off-origin: not this site's page.
					visit(v)
					return
				}

				title, links, metaNoindex := extractTitleLinks(res.Body)
				v.Title = title
				v.Noindex = metaNoindex || hasNoindexXRobotsTag(res.Headers["X-Robots-Tag"])

				linkBase := res.FinalURL
				if linkBase == "" {
					linkBase = job.URL
				}
				var pageLinks []foundLink
				seen := map[string]bool{}
				for _, link := range links {
					abs, ok := sameOriginURL(parsedStart, linkBase, link)
					if !ok {
						continue
					}
					if key := abs.String(); !seen[key] {
						seen[key] = true
						v.Links = append(v.Links, key)
						pageLinks = append(pageLinks, foundLink{abs, job.Depth + 1})
					}
				}
				visit(v)

				mu.Lock()
				found = append(found, pageLinks...)
				mu.Unlock()
			}(job)
		}
		wg.Wait()

		for _, fk := range finals {
			visited[fk] = true
		}
		sort.SliceStable(found, func(i, j int) bool { return found[i].url.String() < found[j].url.String() })
		for _, l := range found {
			if !isCrawlablePath(l.url.Path) {
				continue
			}
			if blocked(l.url) {
				if k := l.url.String(); !blockedSet[k] && len(result.RobotsBlocked) < crawlMaxRobotsBlocked {
					blockedSet[k] = true
					result.RobotsBlocked = append(result.RobotsBlocked, k)
				}
				continue
			}
			enqueue(l.url.String(), l.depth)
		}

		if result.Visited >= maxPages {
			if len(queue) > 0 {
				result.Truncated = true
			}
			break
		}
	}

	return finish()
}

// CrawlablePath reports whether a URL path looks like a content page the
// crawler would follow (not an asset, admin, cart, search, or feed path).
func CrawlablePath(path string) bool { return isCrawlablePath(path) }

// WithMaxBodyBytes returns a client with the same options but a different
// response body cap (e.g. 52 MB for sitemaps).
func (c *SafeClient) WithMaxBodyBytes(n int64) *SafeClient {
	opts := c.options
	opts.MaxBodyBytes = n
	return NewSafeClient(opts)
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
