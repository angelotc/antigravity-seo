package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"

	"antigravity-seo/internal/crawler"
	"antigravity-seo/internal/urlnorm"
)

const (
	siteAffectedCap     = 20
	siteFromCap         = 3
	siteMinTextRunes    = 200
	siteMaxIndexedDepth = 3
	siteSitemapBodyMax  = 52 << 20 // sitemap protocol allows 50 MB uncompressed
)

// SiteAuditOptions tunes a site-wide crawl audit
type SiteAuditOptions struct {
	MaxPages    int    // default 100
	Concurrency int    // default 4
	SitemapURL  string // "" = auto-discover from the start URL's origin
	NoSitemap   bool
	MaxSitemaps int // child sitemaps to fetch; default 50
}

// SitePage is one crawled URL; compact, bodies are not retained
type SitePage struct {
	URL             string              `json:"url"`
	FinalURL        string              `json:"final_url,omitempty"` // only when the fetch landed elsewhere
	Status          int                 `json:"status"`
	Error           string              `json:"error,omitempty"`
	Depth           int                 `json:"depth"`
	RedirectHops    int                 `json:"redirect_hops"`
	ContentType     string              `json:"content_type,omitempty"`
	IsHTML          bool                `json:"html"`
	Title           string              `json:"title,omitempty"`
	MetaDescription string              `json:"meta_description,omitempty"`
	Canonical       string              `json:"canonical,omitempty"` // absolute, normalized
	Lang            string              `json:"lang,omitempty"`
	Noindex         bool                `json:"noindex,omitempty"`
	H1Count         int                 `json:"h1_count"`
	Hreflang        []HreflangAlternate `json:"hreflang,omitempty"` // hrefs absolute + normalized
	InLinks         int                 `json:"in_links"`
	OutLinks        int                 `json:"out_links"`

	outLinks []string // normalized same-origin outlinks
	inFrom   []string // first few referring pages
	textHash string   // SHA-256 of visible text; "" when too short to compare
}

// AffectedURL is one URL involved in a site issue. From holds up to 3
// referrers (broken/redirecting links) or related URLs (duplicates,
// canonical and hreflang targets).
type AffectedURL struct {
	URL    string   `json:"url"`
	From   []string `json:"from,omitempty"`
	Detail string   `json:"detail,omitempty"`
}

// SiteIssue is a cross-page finding
type SiteIssue struct {
	Severity IssueSeverity `json:"severity"`
	Category string        `json:"category"`
	Message  string        `json:"message"`
	Fix      string        `json:"fix"`
	Count    int           `json:"count"` // total affected (Affected is capped at 20)
	Affected []AffectedURL `json:"affected"`
}

// SiteSitemapInfo describes the sitemap consulted by a site audit
type SiteSitemapInfo struct {
	URL            string `json:"url,omitempty"`
	DiscoveredFrom string `json:"discovered_from,omitempty"`
	TotalURLs      int    `json:"total_urls"`
	Complete       bool   `json:"complete"`
	Error          string `json:"error,omitempty"`
}

// SiteSummary counts issues by severity
type SiteSummary struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

// SiteAuditReport is the outcome of a site-wide crawl audit
type SiteAuditReport struct {
	StartURL     string           `json:"start_url"`
	PagesCrawled int              `json:"pages_crawled"`
	Truncated    bool             `json:"truncated"`
	DurationMS   int64            `json:"duration_ms"`
	Sitemap      *SiteSitemapInfo `json:"sitemap,omitempty"`
	Summary      SiteSummary      `json:"summary"`
	Issues       []SiteIssue      `json:"issues"`
	Notes        []string         `json:"notes"`
	Pages        []SitePage       `json:"pages"`
}

// AuditSite crawls a site and runs cross-page checks (broken links,
// duplicates, canonicals, hreflang reciprocity, sitemap coverage)
func AuditSite(ctx context.Context, client *crawler.SafeClient, startURL string, opts SiteAuditOptions) (*SiteAuditReport, error) {
	began := time.Now()
	if opts.MaxPages <= 0 {
		opts.MaxPages = 100
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	if opts.MaxSitemaps <= 0 {
		opts.MaxSitemaps = 50
	}
	rawStart := startURL
	if !strings.HasPrefix(rawStart, "http://") && !strings.HasPrefix(rawStart, "https://") {
		rawStart = "https://" + rawStart
	}
	parsedStart, err := urlnorm.Normalize(rawStart, nil)
	if err != nil {
		return nil, err
	}

	var (
		mu    sync.Mutex
		pages []SitePage
	)
	res, err := client.Crawl(ctx, parsedStart.String(), crawler.CrawlOptions{
		MaxPages:    opts.MaxPages,
		Concurrency: opts.Concurrency,
		Visit: func(v *crawler.CrawlVisit) {
			p := buildSitePage(v, parsedStart)
			mu.Lock()
			pages = append(pages, p)
			mu.Unlock()
		},
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].URL < pages[j].URL })
	linkPages(pages)

	report := &SiteAuditReport{
		StartURL:     res.StartURL,
		PagesCrawled: len(pages),
		Truncated:    res.Truncated,
		Issues:       []SiteIssue{},
		Notes:        []string{},
	}
	if len(pages) == 0 {
		report.Notes = append(report.Notes, "no pages fetched: the start URL is disallowed by robots.txt")
	}
	for i := range pages {
		if pages[i].Depth == 0 && (pages[i].Error != "" || pages[i].Status >= 400) {
			report.Notes = append(report.Notes, fmt.Sprintf("start URL failed (%s); the crawl could not proceed", statusDetail(&pages[i])))
		}
	}

	var sitemapURLs []string
	sitemapComplete := false
	if !opts.NoSitemap {
		root := opts.SitemapURL
		if root == "" {
			root = parsedStart.Scheme + "://" + parsedStart.Host + "/"
		}
		info := &SiteSitemapInfo{URL: root}
		report.Sitemap = info
		smClient := client.WithMaxBodyBytes(siteSitemapBodyMax)
		col, serr := smClient.CollectSitemapURLs(ctx, root, crawler.SitemapOptions{MaxChildren: opts.MaxSitemaps})
		if serr != nil {
			info.Error = serr.Error()
			report.Notes = append(report.Notes, "sitemap checks skipped: "+serr.Error())
		} else {
			info.URL = col.Roots[0]
			info.DiscoveredFrom = col.DiscoveredFrom
			info.TotalURLs = col.TotalURLs
			info.Complete = col.Complete
			for _, u := range col.URLs {
				sitemapURLs = append(sitemapURLs, u.Loc)
			}
			sitemapComplete = col.Complete
			if !col.Complete {
				note := fmt.Sprintf("sitemap incomplete (%d error(s), first: %s)", len(col.Errors), firstOr(col.Errors, "unknown"))
				for _, e := range col.Errors {
					if strings.Contains(e, "child sitemap cap") {
						note += "; raise --max-sitemaps to fetch more"
						break
					}
				}
				report.Notes = append(report.Notes, note)
			}
		}
	}

	issues, notes := AnalyzeSite(pages, sitemapURLs, sitemapComplete, res.Truncated, res.RobotsBlocked)
	if issues == nil {
		issues = []SiteIssue{}
	}
	report.Issues = issues
	report.Notes = append(report.Notes, notes...)
	for _, is := range report.Issues {
		switch is.Severity {
		case SeverityCritical:
			report.Summary.Critical++
		case SeverityWarning:
			report.Summary.Warning++
		default:
			report.Summary.Info++
		}
	}
	report.Pages = pages
	report.DurationMS = time.Since(began).Milliseconds()
	return report, nil
}

func firstOr(s []string, def string) string {
	if len(s) == 0 {
		return def
	}
	return s[0]
}

// buildSitePage condenses one crawl visit into a SitePage; the body is not kept
func buildSitePage(v *crawler.CrawlVisit, start *url.URL) SitePage {
	p := SitePage{URL: v.URL, Depth: v.Depth}
	if v.Err != nil {
		p.Error = v.Err.Error()
		return p
	}
	r := v.Result
	p.Status = r.StatusCode
	p.ContentType = r.ContentType
	p.RedirectHops = len(r.Redirects)
	p.Noindex = v.Noindex
	p.outLinks = v.Links
	p.OutLinks = len(v.Links)

	final := v.URL
	if n, err := urlnorm.Normalize(r.FinalURL, nil); err == nil {
		final = n.String()
	}
	if final != v.URL {
		p.FinalURL = final
	}
	if r.StatusCode != 200 {
		return p
	}
	finalParsed, err := urlnorm.Normalize(final, nil)
	if err != nil || !strings.EqualFold(finalParsed.Host, start.Host) {
		return p // redirected off-origin: not this site's page
	}
	p.IsHTML = isHTMLResponse(r.ContentType, r.Body)
	if !p.IsHTML {
		return p
	}

	if rep, err := InspectHTML(final, r.Body); err == nil {
		p.Title = collapseSpace(rep.Title)
		p.MetaDescription = collapseSpace(rep.MetaDescription)
		p.Lang = rep.Lang
		p.H1Count = rep.H1Count
		p.Noindex = p.Noindex || rep.HasMetaNoindex
		if rep.Canonical != "" {
			if n, err := urlnorm.Normalize(rep.Canonical, finalParsed); err == nil {
				p.Canonical = n.String()
			}
		}
	}
	if rep, err := InspectHreflang(final, r.Body); err == nil {
		for _, alt := range rep.Alternates {
			if alt.Href == "" {
				continue
			}
			n, err := urlnorm.Normalize(alt.Href, finalParsed)
			if err != nil {
				continue
			}
			alt.Href = n.String()
			p.Hreflang = append(p.Hreflang, alt)
		}
	}
	p.textHash = visibleTextHash(r.Body)
	return p
}

func isHTMLResponse(contentType string, body []byte) bool {
	ct := strings.ToLower(contentType)
	if ct != "" {
		return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml")
	}
	head := body
	if len(head) > 512 {
		head = head[:512]
	}
	s := strings.ToLower(strings.TrimLeft(string(head), " \t\r\n\uFEFF"))
	return strings.HasPrefix(s, "<!doctype html") || strings.HasPrefix(s, "<html")
}

func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// visibleTextHash hashes the whitespace-collapsed visible text of a page.
// Returns "" for pages with under 200 runes of text so near-empty pages
// (app shells, error stubs) do not all collide.
func visibleTextHash(body []byte) string {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return ""
	}
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "template":
				return
			}
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
			sb.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	text := collapseSpace(sb.String())
	if utf8.RuneCountInString(text) < siteMinTextRunes {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// linkPages fills InLinks (and the first few referrers) from the outlinks of
// crawled pages. A link counts toward the page whose requested URL it names.
func linkPages(pages []SitePage) {
	byURL := make(map[string]int, len(pages))
	for i := range pages {
		byURL[urlnorm.Key(pages[i].URL, nil)] = i
	}
	for src := range pages {
		seen := map[int]bool{}
		for _, link := range pages[src].outLinks {
			dst, ok := byURL[urlnorm.Key(link, nil)]
			if !ok || dst == src || seen[dst] {
				continue
			}
			seen[dst] = true
			pages[dst].InLinks++
			if len(pages[dst].inFrom) < siteFromCap {
				pages[dst].inFrom = append(pages[dst].inFrom, pages[src].URL)
			}
		}
	}
}

// ---- analysis ----

// affectedSet collects affected URLs in first-seen order
type affectedSet struct {
	idx   map[string]int
	items []AffectedURL
}

func newAffectedSet() *affectedSet { return &affectedSet{idx: map[string]int{}} }

// add records url, merging repeated adds; from is capped at 3 distinct entries
func (s *affectedSet) add(u, detail string, from ...string) {
	i, ok := s.idx[u]
	if !ok {
		i = len(s.items)
		s.idx[u] = i
		s.items = append(s.items, AffectedURL{URL: u, Detail: detail})
	}
	a := &s.items[i]
	for _, f := range from {
		if len(a.From) >= siteFromCap {
			break
		}
		dup := false
		for _, e := range a.From {
			dup = dup || e == f
		}
		if !dup {
			a.From = append(a.From, f)
		}
	}
}

func (s *affectedSet) len() int { return len(s.items) }

type issueList struct{ issues []SiteIssue }

func (l *issueList) add(sev IssueSeverity, category, msg, fix string, s *affectedSet) {
	if s.len() == 0 {
		return
	}
	affected := s.items
	if len(affected) > siteAffectedCap {
		affected = affected[:siteAffectedCap]
	}
	l.issues = append(l.issues, SiteIssue{
		Severity: sev, Category: category, Message: msg, Fix: fix,
		Count: s.len(), Affected: affected,
	})
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func effectiveURL(p *SitePage) string {
	if p.FinalURL != "" {
		return p.FinalURL
	}
	return p.URL
}

// blockedStatus: auth/bot protection, rate limiting or a temporary outage, not a verified dead URL
func blockedStatus(code int) bool { return crawler.UnverifiedStatus(code) }

func isBroken(p *SitePage) bool {
	return p.Error != "" || (p.Status >= 400 && !blockedStatus(p.Status))
}

func (p *SitePage) selfCanonical() bool {
	return p.Canonical == "" || urlnorm.Key(p.Canonical, nil) == urlnorm.Key(effectiveURL(p), nil)
}

// indexable: a 200 HTML page that is not noindex and does not canonicalize elsewhere
func (p *SitePage) indexable() bool {
	return p.Error == "" && p.Status == 200 && p.IsHTML && !p.Noindex && p.selfCanonical()
}

func statusDetail(p *SitePage) string {
	if p.Error != "" {
		return "fetch error: " + p.Error
	}
	return fmt.Sprintf("HTTP %d", p.Status)
}

func hostOf(raw string) string {
	u, err := urlnorm.Normalize(raw, nil)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// siteIndex resolves URLs to crawled pages, by requested URL first and then
// by the landing URL of a redirect
type siteIndex struct {
	byURL   map[string]*SitePage
	byFinal map[string]*SitePage
}

// lookup returns the crawled page for key; landing is true when the key only
// matched the destination of a redirect (so the URL itself does not redirect)
func (ix *siteIndex) lookup(key string) (p *SitePage, landing bool) {
	if p = ix.byURL[key]; p != nil {
		return p, false
	}
	if p = ix.byFinal[key]; p != nil {
		return p, true
	}
	return nil, false
}

// AnalyzeSite runs the cross-page checks over crawled pages. It is pure (no
// network). sitemapURLs may be empty when no sitemap was loaded.
func AnalyzeSite(pages []SitePage, sitemapURLs []string, sitemapComplete bool, crawlTruncated bool, robotsBlocked []string) (issues []SiteIssue, notes []string) {
	list := make([]*SitePage, len(pages))
	for i := range pages {
		list[i] = &pages[i]
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].URL < list[j].URL })

	ix := &siteIndex{byURL: map[string]*SitePage{}, byFinal: map[string]*SitePage{}}
	for _, p := range list {
		ix.byURL[urlnorm.Key(p.URL, nil)] = p
	}
	for _, p := range list {
		if p.FinalURL != "" {
			if k := urlnorm.Key(p.FinalURL, nil); ix.byURL[k] == nil && ix.byFinal[k] == nil {
				ix.byFinal[k] = p
			}
		}
	}

	// A URL that redirects to a page shares that page's content; count each
	// landing page once so a redirect and its target are not "duplicates".
	firstAtLanding := map[string]bool{}
	for _, p := range list {
		if p.FinalURL == "" {
			firstAtLanding[urlnorm.Key(p.URL, nil)] = true
		}
	}
	distinct := map[*SitePage]bool{}
	for _, p := range list {
		if p.FinalURL == "" {
			distinct[p] = true
			continue
		}
		if k := urlnorm.Key(p.FinalURL, nil); !firstAtLanding[k] {
			firstAtLanding[k] = true
			distinct[p] = true
		}
	}

	var il issueList

	// 1. Broken internal links
	broken, blocked := newAffectedSet(), 0
	for _, p := range list {
		if p.Error == "" && blockedStatus(p.Status) {
			blocked++
		}
		if isBroken(p) && p.InLinks > 0 {
			broken.add(p.URL, statusDetail(p), p.inFrom...)
		}
	}
	il.add(SeverityCritical, "Broken Links",
		fmt.Sprintf("%s broken (HTTP 4xx/5xx or fetch failure)", plural(broken.len(), "internally linked URL is", "internally linked URLs are")),
		"Update or remove the internal links that point at these URLs, or restore the missing pages (or 301 them to the closest replacement).", broken)
	if blocked > 0 {
		notes = append(notes, fmt.Sprintf("%s (401/403/429/503: auth, bot protection or temporarily unavailable); not counted as broken", plural(blocked, "page returned", "pages returned")))
	}

	// 2. Redirects: links to redirecting URLs, and long chains
	redirected, chains := newAffectedSet(), newAffectedSet()
	for _, p := range list {
		if p.Depth > 0 && p.RedirectHops >= 1 && p.InLinks > 0 {
			redirected.add(p.URL, fmt.Sprintf("redirects to %s (%s)", effectiveURL(p), plural(p.RedirectHops, "hop", "hops")), p.inFrom...)
		}
		if p.RedirectHops >= 2 {
			chains.add(p.URL, fmt.Sprintf("%d hops to %s", p.RedirectHops, effectiveURL(p)))
		}
	}
	il.add(SeverityWarning, "Redirects",
		fmt.Sprintf("%s to a redirect instead of the final URL", plural(redirected.len(), "internally linked URL points", "internally linked URLs point")),
		"Change each internal link to point straight at the final URL shown in the detail.", redirected)
	il.add(SeverityWarning, "Redirects",
		fmt.Sprintf("%s of 2 or more hops", plural(chains.len(), "redirect chain", "redirect chains")),
		"Redirect the first URL directly to the final destination in a single 301 hop.", chains)

	// 3. Titles and descriptions (indexable pages only)
	missingTitle, missingDesc := newAffectedSet(), newAffectedSet()
	titles, descs := map[string][]*SitePage{}, map[string][]*SitePage{}
	hashes := map[string][]*SitePage{}
	deep := newAffectedSet()
	for _, p := range list {
		if !p.indexable() || !distinct[p] {
			continue
		}
		if p.Title == "" {
			missingTitle.add(p.URL, "")
		} else {
			k := strings.ToLower(p.Title)
			titles[k] = append(titles[k], p)
		}
		if p.MetaDescription == "" {
			missingDesc.add(p.URL, "")
		} else {
			k := strings.ToLower(p.MetaDescription)
			descs[k] = append(descs[k], p)
		}
		if p.textHash != "" {
			hashes[p.textHash] = append(hashes[p.textHash], p)
		}
		if p.Depth > siteMaxIndexedDepth {
			deep.add(p.URL, fmt.Sprintf("depth %d", p.Depth))
		}
	}
	il.add(SeverityCritical, "Titles",
		fmt.Sprintf("%s missing a <title>", plural(missingTitle.len(), "indexable page is", "indexable pages are")),
		"Add a unique, descriptive <title> (about 50-60 characters) to each page.", missingTitle)
	il.add(SeverityWarning, "Meta Descriptions",
		fmt.Sprintf("%s missing a meta description", plural(missingDesc.len(), "indexable page is", "indexable pages are")),
		"Add a unique meta description (about 120-160 characters) summarizing each page.", missingDesc)

	dupTitles := duplicateGroups(titles, func(p *SitePage) string { return fmt.Sprintf("title: %q", p.Title) })
	il.add(SeverityWarning, "Titles",
		fmt.Sprintf("%s share a <title> with other pages (%s)", plural(dupTitles.len(), "group of pages", "groups of pages"), plural(groupedPages(titles), "page", "pages")),
		"Give every indexable page a unique <title>; rewrite the duplicates or canonicalize near-identical pages. Each affected entry lists one page plus the others sharing its title.", dupTitles)
	dupDescs := duplicateGroups(descs, func(p *SitePage) string { return fmt.Sprintf("description: %q", truncateRunes(p.MetaDescription, 80)) })
	il.add(SeverityWarning, "Meta Descriptions",
		fmt.Sprintf("%s share a meta description with other pages (%s)", plural(dupDescs.len(), "group of pages", "groups of pages"), plural(groupedPages(descs), "page", "pages")),
		"Write a unique meta description for each page. Each affected entry lists one page plus the others sharing its description.", dupDescs)

	// 4. Duplicate content
	dupContent := duplicateGroups(hashes, func(p *SitePage) string { return "identical visible text" })
	il.add(SeverityWarning, "Duplicate Content",
		fmt.Sprintf("%s serve identical visible text (%s)", plural(dupContent.len(), "group of pages", "groups of pages"), plural(groupedPages(hashes), "page", "pages")),
		"Consolidate the duplicates into one URL (301 redirect) or point rel=canonical at the preferred version. Each affected entry lists one page plus its duplicates.", dupContent)

	// 5. Canonical targets
	canonBad, canonRedirect, canonNoindex, canonCross := newAffectedSet(), newAffectedSet(), newAffectedSet(), newAffectedSet()
	canonUnverified := map[string]bool{}
	for _, p := range list {
		// A noindex page is already out of the index, so where its canonical
		// points cannot cost anything (e.g. noindexed filter variants).
		if p.Error != "" || p.Status != 200 || !p.IsHTML || p.Noindex || p.selfCanonical() || !distinct[p] {
			continue
		}
		if hostOf(p.Canonical) != hostOf(effectiveURL(p)) {
			canonCross.add(p.URL, "canonical: "+p.Canonical)
			continue
		}
		key := urlnorm.Key(p.Canonical, nil)
		t, landing := ix.lookup(key)
		switch {
		case t == nil:
			canonUnverified[key] = true
		case t.Error != "" || t.Status != 200:
			if t.Error == "" && blockedStatus(t.Status) {
				canonUnverified[key] = true
			} else {
				canonBad.add(p.URL, "canonical target: "+statusDetail(t), p.Canonical)
			}
		default:
			if !landing && t.RedirectHops >= 1 {
				canonRedirect.add(p.URL, "canonical target redirects to "+effectiveURL(t), p.Canonical)
			}
			if t.Noindex {
				canonNoindex.add(p.URL, "canonical target is noindex", p.Canonical)
			}
		}
	}
	il.add(SeverityCritical, "Canonical",
		fmt.Sprintf("%s to a URL that does not return 200", plural(canonBad.len(), "canonical points", "canonicals point")),
		"Point rel=canonical at a live, indexable 200 URL (or remove it if the page is its own canonical).", canonBad)
	il.add(SeverityCritical, "Canonical",
		fmt.Sprintf("%s to a noindex page (conflicting signals)", plural(canonNoindex.len(), "canonical points", "canonicals point")),
		"Either remove noindex from the canonical target or canonicalize to an indexable page; the two signals contradict each other.", canonNoindex)
	il.add(SeverityWarning, "Canonical",
		fmt.Sprintf("%s to a URL that redirects", plural(canonRedirect.len(), "canonical points", "canonicals point")),
		"Set rel=canonical to the final destination URL, not a redirecting one.", canonRedirect)
	il.add(SeverityInfo, "Canonical",
		fmt.Sprintf("%s to a different domain", plural(canonCross.len(), "canonical points", "canonicals point")),
		"Confirm the cross-domain canonical is intentional (syndication or a domain move); otherwise self-canonicalize.", canonCross)
	if n := len(canonUnverified); n > 0 {
		notes = append(notes, fmt.Sprintf("%s not verified (not crawled)", plural(n, "canonical target", "canonical targets")))
	}

	// 6. Hreflang reciprocity
	hrefBad, hrefReturn := newAffectedSet(), newAffectedSet()
	hrefUnverified := map[string]bool{}
	for _, a := range list {
		// Google ignores hreflang on noindex or canonicalized-away pages
		if !a.indexable() || !distinct[a] || len(a.Hreflang) == 0 {
			continue
		}
		aKeys := map[string]bool{urlnorm.Key(a.URL, nil): true, urlnorm.Key(effectiveURL(a), nil): true}
		seen := map[string]bool{}
		for _, alt := range a.Hreflang {
			bk := urlnorm.Key(alt.Href, nil)
			if aKeys[bk] || seen[bk] {
				continue
			}
			seen[bk] = true
			b, _ := ix.lookup(bk)
			switch {
			case b == nil:
				hrefUnverified[bk] = true
			case b.Error != "" || b.Status != 200:
				if b.Error == "" && blockedStatus(b.Status) {
					hrefUnverified[bk] = true
				} else {
					hrefBad.add(b.URL, statusDetail(b), a.URL)
				}
			case b.IsHTML && !hreflangLinksBack(b, aKeys):
				hrefReturn.add(b.URL, "does not list "+a.URL+" in its hreflang alternates", a.URL)
			}
		}
	}
	il.add(SeverityWarning, "Hreflang",
		fmt.Sprintf("%s an hreflang alternate that does not return 200", plural(hrefBad.len(), "page is referenced as", "pages are referenced as")),
		"Fix or remove hreflang annotations that point at broken or redirected URLs; alternates must be live 200 pages.", hrefBad)
	il.add(SeverityWarning, "Hreflang",
		fmt.Sprintf("%s the return link (hreflang is not reciprocal)", plural(hrefReturn.len(), "alternate page is missing", "alternate pages are missing")),
		"Add a reciprocal hreflang annotation on each listed page pointing back to the page shown in From (every alternate must link back).", hrefReturn)
	if n := len(hrefUnverified); n > 0 {
		notes = append(notes, fmt.Sprintf("%s not verified (not crawled)", plural(n, "hreflang target", "hreflang targets")))
	}

	// 7. Sitemap
	if len(sitemapURLs) > 0 {
		sm, smNotes := analyzeSitemap(list, distinct, sitemapURLs, sitemapComplete, crawlTruncated, robotsBlocked)
		il.issues = append(il.issues, sm.issues...)
		notes = append(notes, smNotes...)
	}

	// 8. Click depth
	il.add(SeverityInfo, "Click Depth",
		fmt.Sprintf("%s more than %d clicks from the start URL", plural(deep.len(), "indexable page is", "indexable pages are"), siteMaxIndexedDepth),
		"Link deep pages from hubs, category pages or the navigation so they sit within 3 clicks of the homepage.", deep)

	// 9. Robots
	if len(robotsBlocked) > 0 {
		rb := newAffectedSet()
		for _, u := range robotsBlocked {
			rb.add(u, "")
		}
		il.add(SeverityInfo, "Robots",
			fmt.Sprintf("%s to robots.txt-disallowed URLs", plural(rb.len(), "internal link points", "internal links point")),
			"Remove internal links to URLs that robots.txt blocks (or nofollow them), or relax the Disallow rule if they should be crawled.", rb)
	}

	issues = il.issues
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if sevRank(a.Severity) != sevRank(b.Severity) {
			return sevRank(a.Severity) < sevRank(b.Severity)
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Message < b.Message
	})
	return issues, notes
}

func sevRank(s IssueSeverity) int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	}
	return 2
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// duplicateGroups turns key->pages groups of 2+ into one affected entry per
// group: the first page as URL, up to 3 others in From. Largest groups first.
func duplicateGroups(groups map[string][]*SitePage, detail func(*SitePage) string) *affectedSet {
	var dup [][]*SitePage
	for _, g := range groups {
		if len(g) >= 2 {
			dup = append(dup, g)
		}
	}
	sort.Slice(dup, func(i, j int) bool {
		if len(dup[i]) != len(dup[j]) {
			return len(dup[i]) > len(dup[j])
		}
		return dup[i][0].URL < dup[j][0].URL
	})
	s := newAffectedSet()
	for _, g := range dup {
		var others []string
		for _, p := range g[1:] {
			others = append(others, p.URL)
		}
		s.add(g[0].URL, fmt.Sprintf("%s (%s)", detail(g[0]), plural(len(g), "page", "pages")), others...)
	}
	return s
}

func groupedPages(groups map[string][]*SitePage) int {
	n := 0
	for _, g := range groups {
		if len(g) >= 2 {
			n += len(g)
		}
	}
	return n
}

func hreflangLinksBack(b *SitePage, aKeys map[string]bool) bool {
	for _, alt := range b.Hreflang {
		if aKeys[urlnorm.Key(alt.Href, nil)] {
			return true
		}
	}
	return false
}

// analyzeSitemap compares crawled pages with the sitemap's URL list
func analyzeSitemap(list []*SitePage, distinct map[*SitePage]bool, sitemapURLs []string, complete, truncated bool, robotsBlocked []string) (il issueList, notes []string) {
	hosts := map[string]bool{}
	for _, p := range list {
		hosts[hostOf(p.URL)] = true
		hosts[hostOf(effectiveURL(p))] = true
	}
	inSitemap := map[string]string{} // key -> loc, crawled hosts only
	var order []string
	for _, loc := range sitemapURLs {
		if !hosts[hostOf(loc)] {
			continue
		}
		k := urlnorm.Key(loc, nil)
		if _, dup := inSitemap[k]; !dup {
			inSitemap[k] = loc
			order = append(order, k)
		}
	}
	if len(inSitemap) == 0 {
		return il, []string{"sitemap lists no URLs on the crawled host; sitemap checks skipped"}
	}

	nonOK, redirects, noindex, canon, missing := newAffectedSet(), newAffectedSet(), newAffectedSet(), newAffectedSet(), newAffectedSet()
	for _, p := range list {
		_, listed := inSitemap[urlnorm.Key(p.URL, nil)]
		switch {
		case listed && (p.Error != "" || (p.Status != 200 && !blockedStatus(p.Status))):
			nonOK.add(p.URL, statusDetail(p))
		case listed && p.Status == 200 && p.RedirectHops >= 1:
			redirects.add(p.URL, "redirects to "+effectiveURL(p))
		case listed && p.Status == 200 && p.Noindex:
			noindex.add(p.URL, "")
		case listed && p.Status == 200 && p.IsHTML && !p.selfCanonical():
			canon.add(p.URL, "canonical: "+p.Canonical)
		case !listed && p.indexable() && distinct[p]:
			if _, viaFinal := inSitemap[urlnorm.Key(effectiveURL(p), nil)]; !viaFinal {
				missing.add(p.URL, "")
			}
		}
	}
	il.add(SeverityWarning, "Sitemap",
		fmt.Sprintf("%s in the sitemap that do not return 200", plural(nonOK.len(), "URL is", "URLs are")),
		"Remove dead URLs from the sitemap (or restore the pages); a sitemap should list only live 200 URLs.", nonOK)
	il.add(SeverityWarning, "Sitemap",
		fmt.Sprintf("%s in the sitemap that redirect", plural(redirects.len(), "URL is", "URLs are")),
		"List the final destination URL in the sitemap instead of the redirecting one.", redirects)
	il.add(SeverityWarning, "Sitemap",
		fmt.Sprintf("%s in the sitemap that are noindex", plural(noindex.len(), "URL is", "URLs are")),
		"Remove noindex pages from the sitemap, or drop the noindex directive if they should rank.", noindex)
	il.add(SeverityWarning, "Sitemap",
		fmt.Sprintf("%s in the sitemap that canonicalize elsewhere", plural(canon.len(), "URL is", "URLs are")),
		"List only canonical URLs in the sitemap; replace these with the URLs their rel=canonical points to.", canon)

	if missing.len() > 0 {
		if complete {
			il.add(SeverityInfo, "Sitemap",
				fmt.Sprintf("%s missing from the sitemap", plural(missing.len(), "indexable crawled page is", "indexable crawled pages are")),
				"Add these indexable pages to the sitemap (or make them noindex/canonical if they should not rank).", missing)
		} else {
			notes = append(notes, fmt.Sprintf("missing-from-sitemap check skipped: sitemap is incomplete (%s not listed in what was loaded)", plural(missing.len(), "indexable page", "indexable pages")))
		}
	}

	if truncated {
		notes = append(notes, "orphan check skipped: crawl stopped at --max-pages before covering the site")
		return il, notes
	}
	reached := map[string]bool{}
	for _, p := range list {
		reached[urlnorm.Key(p.URL, nil)] = true
		reached[urlnorm.Key(effectiveURL(p), nil)] = true
		for _, l := range p.outLinks {
			reached[urlnorm.Key(l, nil)] = true
		}
	}
	for _, u := range robotsBlocked {
		reached[urlnorm.Key(u, nil)] = true // deliberately not crawled
	}
	orphans := newAffectedSet()
	for _, k := range order {
		loc := inSitemap[k]
		if reached[k] {
			continue
		}
		if u, err := urlnorm.Normalize(loc, nil); err != nil || !crawler.CrawlablePath(u.Path) {
			continue // assets and other paths the crawler never follows
		}
		orphans.add(loc, "")
	}
	il.add(SeverityWarning, "Sitemap",
		fmt.Sprintf("%s: in the sitemap but not linked from any crawled page", plural(orphans.len(), "possible orphan page", "possible orphan pages")),
		"Link these pages from relevant crawled pages or navigation, or remove them from the sitemap if they are obsolete.", orphans)
	return il, notes
}
