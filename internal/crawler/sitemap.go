package crawler

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SitemapURL represents a single URL entry in a sitemap
type SitemapURL struct {
	Loc        string  `xml:"loc" json:"loc"`
	LastMod    string  `xml:"lastmod" json:"lastmod,omitempty"`
	ChangeFreq string  `xml:"changefreq" json:"changefreq,omitempty"`
	Priority   float64 `xml:"priority" json:"priority,omitempty"`
}

// SitemapIndexEntry represents a child sitemap in a sitemapindex
type SitemapIndexEntry struct {
	Loc     string `xml:"loc" json:"loc"`
	LastMod string `xml:"lastmod" json:"lastmod,omitempty"`
}

// SitemapReport captures full analysis of sitemap health
type SitemapReport struct {
	SitemapURL      string       `json:"sitemap_url"`
	IsSitemapIndex  bool         `json:"is_sitemap_index"`
	ChildSitemaps   []string     `json:"child_sitemaps,omitempty"`
	TotalURLs       int          `json:"total_urls"`
	SampleURLs      []SitemapURL `json:"sample_urls"`
	BrokenURLs      []string     `json:"broken_urls,omitempty"`
	BlockedURLs     []string     `json:"blocked_urls,omitempty"` // 401/403/429/503: auth, bot protection or temporarily unavailable; not verified broken
	RedirectingURLs []string     `json:"redirecting_urls,omitempty"`
	Errors          []string     `json:"errors,omitempty"`
	DurationMS      int64        `json:"duration_ms"`

	Children       []SitemapChild `json:"children,omitempty"`
	Complete       bool           `json:"complete"` // every child sitemap was fetched and parsed
	DiscoveredFrom string         `json:"discovered_from,omitempty"`
}

const (
	sitemapSampleDefault  = 10
	sitemapSampleMax      = 100
	sitemapChildrenDef    = 50
	sitemapConcurrencyDef = 4
	sitemapMaxDepth       = 3
	sitemapMaxAggregated  = 200000
	sitemapMaxFileURLs    = 50000            // protocol limit per sitemap file
	sitemapMaxDecompress  = 50 * 1024 * 1024 // protocol limit per uncompressed file
)

// SitemapOptions tunes how a sitemap index tree is followed and sampled
type SitemapOptions struct {
	SampleSize  int // URLs sampled AND health-checked; <=0 -> 10; clamped to 100
	MaxChildren int // max child sitemaps fetched across the whole index tree; <=0 -> 50
	Concurrency int // parallel child fetches; <=0 -> 4
}

func (o SitemapOptions) normalized() SitemapOptions {
	if o.SampleSize <= 0 {
		o.SampleSize = sitemapSampleDefault
	}
	if o.SampleSize > sitemapSampleMax {
		o.SampleSize = sitemapSampleMax
	}
	if o.MaxChildren <= 0 {
		o.MaxChildren = sitemapChildrenDef
	}
	if o.Concurrency <= 0 {
		o.Concurrency = sitemapConcurrencyDef
	}
	return o
}

// SitemapChild describes one child sitemap found while following an index
type SitemapChild struct {
	Loc      string `json:"loc"`
	URLCount int    `json:"url_count"`
	Fetched  bool   `json:"fetched"`
	Error    string `json:"error,omitempty"`
}

// SitemapCollection is the aggregated result of following a sitemap (tree)
type SitemapCollection struct {
	URLs           []SitemapURL // aggregated in child order (not completion order), at most 200,000
	TotalURLs      int          // every URL parsed from fetched sitemaps, including any beyond the 200,000 storage cap
	Children       []SitemapChild
	Complete       bool // true only if every child sitemap was fetched and parsed
	Errors         []string
	Roots          []string // root sitemap URL(s) actually used; several when discovered via robots.txt
	IsIndex        bool     // the root was a sitemapindex (or a discovered multi-root)
	DiscoveredFrom string   // "robots.txt", "/sitemap.xml", "/sitemap_index.xml", or "" if the URL was used as given

	groups [][2]int // [start,end) ranges into URLs, one per non-empty urlset file
	files  []sitemapFileCount
}

// sitemapFileCount records how many URLs a single sitemap file held
type sitemapFileCount struct {
	loc string
	n   int
}

// sitemapNode is one sitemap file in the index tree
type sitemapNode struct {
	loc     string
	depth   int
	fetched bool
	err     string
	skipped bool // not fetched because of the depth or child cap (not a failure)
	isIndex bool
	urls    []SitemapURL
	kidLocs []string
	kids    []*sitemapNode
}

// UnverifiedStatus reports whether an HTTP status says the URL could not be
// checked (auth, bot protection, rate limiting, or a temporary 503) rather
// than that it is gone, so crawlers must not report it as a broken link.
func UnverifiedStatus(code int) bool {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return true
	}
	return false
}

func hasGzipMagic(b []byte) bool { return len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b }

// looksLikeHTML reports whether a response is an HTML page rather than XML
func looksLikeHTML(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return true
	}
	head := body
	if len(head) > 512 {
		head = head[:512]
	}
	s := strings.ToLower(strings.TrimLeft(string(head), " \t\r\n\ufeff"))
	return strings.HasPrefix(s, "<!doctype html") || strings.HasPrefix(s, "<html")
}

// sitemapBody returns the plain XML of a fetched sitemap. Gzip is detected by
// magic bytes (so a mislabeled .xml that is really gzip still works) and the
// decompressed size is capped at the protocol's 50 MB.
func sitemapBody(res *FetchResult) ([]byte, error) {
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sitemap returned HTTP %d", res.StatusCode)
	}
	body := res.Body
	if !hasGzipMagic(body) {
		if looksLikeHTML(res.ContentType, body) {
			return nil, fmt.Errorf("URL returned HTML, not a sitemap — pass the site root to auto-discover, or the sitemap URL")
		}
		return body, nil
	}
	if res.Truncated {
		return nil, fmt.Errorf("gzip sitemap exceeds the fetch size cap (%d bytes read)", len(res.Body))
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed decompressing gzip sitemap: %w", err)
	}
	defer zr.Close()
	plain, err := io.ReadAll(io.LimitReader(zr, sitemapMaxDecompress+1))
	if err != nil {
		return nil, fmt.Errorf("failed decompressing gzip sitemap: %w", err)
	}
	if len(plain) > sitemapMaxDecompress {
		return nil, fmt.Errorf("decompressed sitemap exceeds the 50 MB protocol limit")
	}
	return plain, nil
}

// ParseSitemap decodes an XML sitemap (supports .gz and <sitemapindex>)
func ParseSitemap(raw []byte, isGzip bool) (urls []SitemapURL, childSitemaps []string, isIndex bool, err error) {
	urls, childSitemaps, isIndex, err = parseSitemapPartial(raw, isGzip)
	if err != nil {
		return nil, nil, false, err
	}
	return urls, childSitemaps, isIndex, nil
}

// parseSitemapPartial is ParseSitemap that also returns everything decoded
// before an XML error, which is what a size-truncated sitemap yields
func parseSitemapPartial(raw []byte, isGzip bool) (urls []SitemapURL, childSitemaps []string, isIndex bool, err error) {
	var reader io.Reader = bytes.NewReader(raw)
	if isGzip {
		gzReader, gzErr := gzip.NewReader(reader)
		if gzErr != nil {
			return nil, nil, false, fmt.Errorf("failed decompressing gzip sitemap: %w", gzErr)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	decoder := xml.NewDecoder(reader)

	for {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				break
			}
			return urls, childSitemaps, isIndex, tokenErr
		}

		switch se := token.(type) {
		case xml.StartElement:
			if se.Name.Local == "sitemapindex" {
				isIndex = true
			} else if se.Name.Local == "sitemap" {
				var sm SitemapIndexEntry
				if err := decoder.DecodeElement(&sm, &se); err == nil && sm.Loc != "" {
					childSitemaps = append(childSitemaps, strings.TrimSpace(sm.Loc))
				}
			} else if se.Name.Local == "url" {
				var u SitemapURL
				if err := decoder.DecodeElement(&u, &se); err == nil && u.Loc != "" {
					u.Loc = strings.TrimSpace(u.Loc)
					urls = append(urls, u)
				}
			}
		}
	}

	return urls, childSitemaps, isIndex, nil
}

// fetchSitemapFile fetches and parses a single sitemap file. A plain body cut
// off by the client's size cap yields the URLs parsed before the cut plus a
// non-empty warn (the file is then incomplete); any other failure is err.
func (c *SafeClient) fetchSitemapFile(ctx context.Context, loc string) (urls []SitemapURL, kids []string, isIndex bool, warn string, err error) {
	res, err := c.Fetch(ctx, loc)
	if err != nil {
		return nil, nil, false, "", fmt.Errorf("failed fetching sitemap: %w", err)
	}
	body, err := sitemapBody(res)
	if err != nil {
		return nil, nil, false, "", err
	}
	urls, kids, isIndex, err = parseSitemapPartial(body, false)
	if res.Truncated {
		return urls, kids, isIndex, fmt.Sprintf("truncated: sitemap exceeds the %d-byte fetch cap, only the first %d URLs were read", len(res.Body), len(urls)), nil
	}
	if err != nil {
		return nil, nil, false, "", fmt.Errorf("failed parsing sitemap XML: %w", err)
	}
	return urls, kids, isIndex, "", nil
}

// DiscoverSitemaps finds a site's sitemap(s) from any URL on its origin. It
// prefers robots.txt "Sitemap:" directives, then probes /sitemap.xml and
// /sitemap_index.xml (accepted only on HTTP 200 with a <urlset> or
// <sitemapindex> body). source is "robots.txt", "/sitemap.xml" or
// "/sitemap_index.xml"; when nothing is found it is "" and err explains why.
func (c *SafeClient) DiscoverSitemaps(ctx context.Context, siteURL string) (sitemaps []string, source string, err error) {
	parsed, perr := url.Parse(siteURL)
	if perr != nil || parsed.Host == "" {
		return nil, "", fmt.Errorf("invalid site URL: %s", siteURL)
	}
	base := parsed.Scheme + "://" + parsed.Host

	if res, ferr := c.Fetch(ctx, base+"/robots.txt"); ferr == nil && res.StatusCode == http.StatusOK {
		seen := map[string]bool{}
		for _, sm := range parseRobots(string(res.Body)).sitemaps {
			if sm != "" && !seen[sm] {
				seen[sm] = true
				sitemaps = append(sitemaps, sm)
			}
		}
		if len(sitemaps) > 0 {
			return sitemaps, "robots.txt", nil
		}
	}

	for _, path := range []string{"/sitemap.xml", "/sitemap_index.xml"} {
		res, ferr := c.Fetch(ctx, base+path)
		if ferr != nil {
			continue
		}
		body, berr := sitemapBody(res)
		if berr != nil {
			continue
		}
		lower := strings.ToLower(string(body))
		if strings.Contains(lower, "<urlset") || strings.Contains(lower, "<sitemapindex") {
			return []string{base + path}, path, nil
		}
	}

	return nil, "", fmt.Errorf("no sitemap found: robots.txt has no Sitemap: directive and /sitemap.xml, /sitemap_index.xml are missing")
}

// CollectSitemapURLs follows a sitemap (or sitemapindex tree) and aggregates
// its URLs. A site root URL (empty path or "/") triggers DiscoverSitemaps
// first; several discovered sitemaps act as a virtual index. Child sitemaps
// are fetched concurrently, level by level, at most opts.MaxChildren across
// the whole tree and to a nesting depth of 3; cycles and duplicates are
// ignored. Per-child failures are recorded (Children, Errors, Complete=false)
// instead of aborting; an error is returned only if no root could be read.
// URLs come back in child order, so output is deterministic. At most 200,000
// URLs are kept (Complete=false and an error note if more).
func (c *SafeClient) CollectSitemapURLs(ctx context.Context, sitemapURL string, opts SitemapOptions) (*SitemapCollection, error) {
	opts = opts.normalized()
	col := &SitemapCollection{Complete: true, Errors: []string{}}

	roots := []string{sitemapURL}
	if u, err := url.Parse(sitemapURL); err == nil && (u.Path == "" || u.Path == "/") {
		found, source, derr := c.DiscoverSitemaps(ctx, sitemapURL)
		if derr != nil {
			return nil, derr
		}
		roots, col.DiscoveredFrom = found, source
	}

	visited := map[string]*sitemapNode{}
	var rootNodes []*sitemapNode
	for _, r := range roots {
		if visited[r] == nil {
			n := &sitemapNode{loc: r}
			visited[r] = n
			rootNodes = append(rootNodes, n)
			col.Roots = append(col.Roots, r)
		}
	}

	// Breadth-first: fetch a whole level concurrently, then spend the child
	// budget in listed order so which children get fetched never depends on
	// goroutine timing.
	budget := opts.MaxChildren
	level := rootNodes
	for len(level) > 0 {
		var wg sync.WaitGroup
		sem := make(chan struct{}, opts.Concurrency)
		for _, n := range level {
			wg.Add(1)
			go func(n *sitemapNode) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				urls, kids, isIndex, warn, err := c.fetchSitemapFile(ctx, n.loc)
				if err != nil {
					n.err = err.Error()
					return
				}
				n.fetched, n.urls, n.kidLocs, n.isIndex, n.err = true, urls, kids, isIndex, warn
			}(n)
		}
		wg.Wait()

		var next []*sitemapNode
		for _, n := range level {
			for _, kid := range n.kidLocs {
				if visited[kid] != nil {
					continue // cycle or duplicate listing
				}
				k := &sitemapNode{loc: kid, depth: n.depth + 1}
				visited[kid] = k
				n.kids = append(n.kids, k)
				switch {
				case k.depth > sitemapMaxDepth:
					k.err, k.skipped = fmt.Sprintf("skipped: index nesting deeper than %d levels", sitemapMaxDepth), true
				case budget <= 0:
					k.err, k.skipped = fmt.Sprintf("skipped: child sitemap cap (%d) reached", opts.MaxChildren), true
				default:
					budget--
					next = append(next, k)
				}
			}
		}
		level = next
	}

	col.IsIndex = len(rootNodes) > 1 || rootNodes[0].isIndex
	hiddenRoot := len(rootNodes) == 1 // a lone root is the report subject, not a "child"
	limitNoted := false
	skipped := 0

	// Depth-first assembly in listed order
	var walk func(n *sitemapNode, isRoot bool)
	walk = func(n *sitemapNode, isRoot bool) {
		if !(isRoot && hiddenRoot) {
			col.Children = append(col.Children, SitemapChild{Loc: n.loc, URLCount: len(n.urls), Fetched: n.fetched, Error: n.err})
		}
		if n.err != "" {
			col.Complete = false
			if n.skipped {
				skipped++ // summarized once below; each child still carries its reason
			} else {
				col.Errors = append(col.Errors, fmt.Sprintf("%s: %s", n.loc, n.err))
			}
		}
		if n.fetched && !n.isIndex {
			col.files = append(col.files, sitemapFileCount{n.loc, len(n.urls)})
			col.TotalURLs += len(n.urls)
			take := n.urls
			if room := sitemapMaxAggregated - len(col.URLs); len(take) > room {
				take = take[:room]
				col.Complete = false
				if !limitNoted {
					limitNoted = true
					col.Errors = append(col.Errors, fmt.Sprintf("aggregated URL limit of %d reached; remaining URLs not collected", sitemapMaxAggregated))
				}
			}
			if len(take) > 0 {
				start := len(col.URLs)
				col.URLs = append(col.URLs, take...)
				col.groups = append(col.groups, [2]int{start, len(col.URLs)})
			}
			n.urls = nil
		}
		for _, k := range n.kids {
			walk(k, false)
		}
	}
	for _, r := range rootNodes {
		walk(r, true)
	}
	if skipped > 0 {
		col.Errors = append(col.Errors, fmt.Sprintf("%d child sitemaps not fetched (child cap %d or nesting deeper than %d levels); raise --max-sitemaps to fetch more", skipped, opts.MaxChildren, sitemapMaxDepth))
	}

	allFailed := true
	for _, r := range rootNodes {
		if r.fetched {
			allFailed = false
		}
	}
	if allFailed {
		return nil, fmt.Errorf("%s", rootNodes[0].err)
	}
	return col, nil
}

// sampleGroups picks size URLs spread across groups. Quota is dealt out
// round-robin (one per non-empty group per round, capped by group length),
// each group contributes evenly spaced indices (i*n/q), and the result is
// interleaved by group. Deterministic, no randomness.
func sampleGroups(groups [][]SitemapURL, size int) []SitemapURL {
	var live [][]SitemapURL
	total := 0
	for _, g := range groups {
		if len(g) > 0 {
			live = append(live, g)
			total += len(g)
		}
	}
	if size > total {
		size = total
	}
	// Fewer slots than sitemaps: take evenly spaced sitemaps rather than the first few
	if size < len(live) {
		spread := make([][]SitemapURL, size)
		for i := range spread {
			spread[i] = live[i*len(live)/size]
		}
		live = spread
	}
	quota := make([]int, len(live))
	for left := size; left > 0; {
		for i := range live {
			if left > 0 && quota[i] < len(live[i]) {
				quota[i]++
				left--
			}
		}
	}
	picks := make([][]SitemapURL, len(live))
	for i, g := range live {
		for j := 0; j < quota[i]; j++ {
			picks[i] = append(picks[i], g[j*len(g)/quota[i]])
		}
	}
	out := make([]SitemapURL, 0, size)
	for r := 0; len(out) < size; r++ {
		for _, p := range picks {
			if r < len(p) {
				out = append(out, p[r])
			}
		}
	}
	return out
}

// InspectSitemap fetches, parses, and health-checks a sample of a sitemap.
// checkHealthLimit is the sample size (<=0 means 10).
func (c *SafeClient) InspectSitemap(ctx context.Context, sitemapURL string, checkHealthLimit int) (*SitemapReport, error) {
	return c.InspectSitemapWithOptions(ctx, sitemapURL, SitemapOptions{SampleSize: checkHealthLimit})
}

// InspectSitemapWithOptions follows sitemap indexes (and site-root discovery),
// samples URLs spread across the child sitemaps, and health-checks exactly
// that sample.
func (c *SafeClient) InspectSitemapWithOptions(ctx context.Context, sitemapURL string, opts SitemapOptions) (*SitemapReport, error) {
	start := time.Now()
	opts = opts.normalized()

	col, err := c.CollectSitemapURLs(ctx, sitemapURL, opts)
	if err != nil {
		return nil, err
	}

	report := &SitemapReport{
		SitemapURL:     col.Roots[0],
		IsSitemapIndex: col.IsIndex,
		TotalURLs:      col.TotalURLs,
		Errors:         append([]string{}, col.Errors...),
		Children:       col.Children,
		Complete:       col.Complete,
		DiscoveredFrom: col.DiscoveredFrom,
	}
	for _, ch := range col.Children {
		report.ChildSitemaps = append(report.ChildSitemaps, ch.Loc)
	}

	groups := make([][]SitemapURL, len(col.groups))
	for i, g := range col.groups {
		groups[i] = col.URLs[g[0]:g[1]]
	}
	report.SampleURLs = sampleGroups(groups, opts.SampleSize)

	// Verify Google Sitemaps constraints (the limit is per file)
	for _, f := range col.files {
		if f.n > sitemapMaxFileURLs {
			report.Errors = append(report.Errors, fmt.Sprintf("Sitemap %s exceeds Google's 50,000 URL limit (%d URLs found)", f.loc, f.n))
		}
	}

	// Validate sitemap URL host matches
	parsedOrigin, _ := url.Parse(report.SitemapURL)
	if parsedOrigin != nil {
		for _, u := range report.SampleURLs {
			uParsed, err := url.Parse(u.Loc)
			if err != nil || uParsed.Host != parsedOrigin.Host {
				report.Errors = append(report.Errors, fmt.Sprintf("Cross-origin or malformed URL in sitemap: %s", u.Loc))
				break
			}
		}
	}

	// Concurrent health verification (check HTTP status codes) of the sample
	if len(report.SampleURLs) > 0 {
		var (
			brokenLock sync.Mutex
			wg         sync.WaitGroup
			sem        = make(chan struct{}, 10) // 10 workers
		)

		for _, item := range report.SampleURLs {
			wg.Add(1)
			go func(target string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				r, fetchErr := c.Fetch(ctx, target)
				brokenLock.Lock()
				defer brokenLock.Unlock()

				switch {
				case fetchErr != nil:
					report.BrokenURLs = append(report.BrokenURLs, fmt.Sprintf("%s (Status: %d)", target, 0))
				case UnverifiedStatus(r.StatusCode):
					// Auth/bot protection or a temporary outage, not a real dead link; keep it out of BrokenURLs.
					report.BlockedURLs = append(report.BlockedURLs, fmt.Sprintf("%s (Status: %d, blocked or temporarily unavailable, not verified)", target, r.StatusCode))
				case r.StatusCode >= 400:
					report.BrokenURLs = append(report.BrokenURLs, fmt.Sprintf("%s (Status: %d)", target, r.StatusCode))
				case len(r.Redirects) > 0:
					report.RedirectingURLs = append(report.RedirectingURLs, fmt.Sprintf("%s -> %s (%d)", target, r.FinalURL, r.Redirects[0].StatusCode))
				}
			}(item.Loc)
		}

		wg.Wait()
	}

	report.DurationMS = time.Since(start).Milliseconds()
	return report, nil
}
