package audit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"antigravity-seo/internal/crawler"
)

const siteBase = "https://ex.test"

// sp builds an indexable 200 HTML page with unique title and description
func sp(path string) SitePage {
	return SitePage{
		URL: siteBase + path, Status: 200, IsHTML: true,
		Title: "Title " + path, MetaDescription: "Description " + path,
	}
}

func abs(path string) string { return siteBase + path }

func findIssue(issues []SiteIssue, category, contains string) *SiteIssue {
	for i := range issues {
		if issues[i].Category == category && strings.Contains(issues[i].Message, contains) {
			return &issues[i]
		}
	}
	return nil
}

func hasNote(notes []string, contains string) bool {
	for _, n := range notes {
		if strings.Contains(n, contains) {
			return true
		}
	}
	return false
}

func TestAnalyzeSiteBrokenLinks(t *testing.T) {
	gone := SitePage{URL: abs("/gone"), Status: 404, Depth: 1, InLinks: 2, inFrom: []string{abs("/"), abs("/a")}}
	errp := SitePage{URL: abs("/dead"), Error: "dial tcp: refused", Depth: 1, InLinks: 1, inFrom: []string{abs("/")}}
	unlinked := SitePage{URL: abs("/x"), Status: 404}
	blocked := SitePage{URL: abs("/waf"), Status: 403, InLinks: 1, inFrom: []string{abs("/")}}
	unavailable := SitePage{URL: abs("/busy"), Status: 503, InLinks: 1, inFrom: []string{abs("/")}}
	issues, notes := AnalyzeSite([]SitePage{sp("/"), gone, errp, unlinked, blocked, unavailable}, nil, false, false, nil)

	is := findIssue(issues, "Broken Links", "2 internally linked URLs are")
	if is == nil || is.Severity != SeverityCritical || is.Count != 2 {
		t.Fatalf("broken links issue = %+v (all: %+v)", is, issues)
	}
	if is.Fix == "" || len(is.Affected) != 2 {
		t.Errorf("fix/affected: %+v", is)
	}
	if is.Affected[1].URL != abs("/gone") || len(is.Affected[1].From) != 2 {
		t.Errorf("affected = %+v", is.Affected)
	}
	if !hasNote(notes, "2 pages returned (401/403/429/503") {
		t.Errorf("blocked page should produce a note: %v", notes)
	}

	// negative: healthy pages produce no issue
	if issues, _ := AnalyzeSite([]SitePage{sp("/"), sp("/a")}, nil, false, false, nil); len(issues) != 0 {
		t.Errorf("clean site produced issues: %+v", issues)
	}
}

func TestAnalyzeSiteRedirects(t *testing.T) {
	old := sp("/old")
	old.Depth, old.RedirectHops, old.FinalURL, old.InLinks, old.inFrom = 1, 1, abs("/new"), 1, []string{abs("/")}
	chain := sp("/chain")
	chain.Depth, chain.RedirectHops, chain.FinalURL = 1, 3, abs("/end")
	start := sp("/")
	start.RedirectHops, start.FinalURL, start.InLinks = 1, abs("/en"), 5 // depth 0: not flagged
	issues, _ := AnalyzeSite([]SitePage{old, chain, start}, nil, false, false, nil)

	if is := findIssue(issues, "Redirects", "redirect instead of the final URL"); is == nil || is.Count != 1 || is.Severity != SeverityWarning {
		t.Errorf("link-to-redirect: %+v", is)
	} else if !strings.Contains(is.Affected[0].Detail, abs("/new")) {
		t.Errorf("detail should name the final URL: %+v", is.Affected)
	}
	if is := findIssue(issues, "Redirects", "redirect chain"); is == nil || is.Count != 1 || !strings.Contains(is.Affected[0].Detail, "3 hops") {
		t.Errorf("chain: %+v", is)
	}
	if issues, _ := AnalyzeSite([]SitePage{sp("/a")}, nil, false, false, nil); len(issues) != 0 {
		t.Errorf("negative: %+v", issues)
	}
}

func TestAnalyzeSiteTitlesAndDescriptions(t *testing.T) {
	a, b, c := sp("/a"), sp("/b"), sp("/c")
	a.Title, b.Title = "Same Title", "same  title" // case/whitespace-insensitive
	a.MetaDescription, c.MetaDescription = "Shared", "Shared"
	noTitle, noDesc := sp("/nt"), sp("/nd")
	noTitle.Title = ""
	noDesc.MetaDescription = ""
	noidx := sp("/noindex")
	noidx.Noindex, noidx.Title, noidx.MetaDescription = true, "Same Title", ""
	moved := sp("/moved")
	moved.Canonical, moved.Title = abs("/a"), "Same Title"
	b.Title = collapseSpace(b.Title)
	issues, _ := AnalyzeSite([]SitePage{a, b, c, noTitle, noDesc, noidx, moved}, nil, false, false, nil)

	dt := findIssue(issues, "Titles", "share a <title>")
	if dt == nil || dt.Severity != SeverityWarning || dt.Count != 1 || len(dt.Affected) != 1 {
		t.Fatalf("duplicate titles: %+v", dt)
	}
	if dt.Affected[0].URL != abs("/a") || len(dt.Affected[0].From) != 1 || dt.Affected[0].From[0] != abs("/b") {
		t.Errorf("group should be /a with /b (noindex and canonicalized pages excluded): %+v", dt.Affected)
	}
	if dd := findIssue(issues, "Meta Descriptions", "share a meta description"); dd == nil || dd.Count != 1 {
		t.Errorf("duplicate descriptions: %+v", dd)
	}
	if mt := findIssue(issues, "Titles", "missing a <title>"); mt == nil || mt.Severity != SeverityCritical || mt.Affected[0].URL != abs("/nt") {
		t.Errorf("missing title: %+v", mt)
	}
	md := findIssue(issues, "Meta Descriptions", "missing a meta description")
	if md == nil || md.Severity != SeverityWarning || md.Count != 1 || md.Affected[0].URL != abs("/nd") {
		t.Errorf("missing description (noindex page must be ignored): %+v", md)
	}
}

func TestAnalyzeSiteDuplicateContent(t *testing.T) {
	a, b, c := sp("/a"), sp("/b"), sp("/c")
	a.textHash, b.textHash = "h1", "h1"
	c.textHash = "h2"
	empty1, empty2 := sp("/e1"), sp("/e2") // hash "" (too short) must not collide
	noidx := sp("/n")
	noidx.textHash, noidx.Noindex = "h2", true
	issues, _ := AnalyzeSite([]SitePage{a, b, c, empty1, empty2, noidx}, nil, false, false, nil)
	is := findIssue(issues, "Duplicate Content", "identical visible text")
	if is == nil || is.Count != 1 || is.Severity != SeverityWarning || is.Affected[0].URL != abs("/a") || is.Affected[0].From[0] != abs("/b") {
		t.Fatalf("duplicate content: %+v", is)
	}
	if issues, _ := AnalyzeSite([]SitePage{c, empty1, empty2, noidx}, nil, false, false, nil); findIssue(issues, "Duplicate Content", "") != nil {
		t.Errorf("negative: %+v", issues)
	}
}

func TestVisibleTextHash(t *testing.T) {
	long := strings.Repeat("Some visible words here. ", 20)
	page := func(extra string) []byte {
		return []byte("<html><head><style>x{}</style></head><body><script>var a=1;</script>" + extra + "<p>" + long + "</p><noscript>no js</noscript></body></html>")
	}
	if h := visibleTextHash(page("")); h == "" || h != visibleTextHash(page("<script>other()</script>")) {
		t.Error("scripts must not affect the hash")
	}
	if visibleTextHash(page("")) == visibleTextHash(page("<p>extra</p>")) {
		t.Error("visible text must affect the hash")
	}
	if h := visibleTextHash([]byte("<html><body>tiny</body></html>")); h != "" {
		t.Errorf("short page hash = %q", h)
	}
}

func TestAnalyzeSiteCanonical(t *testing.T) {
	target404 := SitePage{URL: abs("/t404"), Status: 404}
	targetRedir := sp("/tred")
	targetRedir.RedirectHops, targetRedir.FinalURL = 1, abs("/tfinal")
	targetNoindex := sp("/tnoi")
	targetNoindex.Noindex = true
	targetOK := sp("/tok")
	landing := sp("/landing-src")
	landing.RedirectHops, landing.FinalURL = 1, abs("/landed")

	mk := func(path, canonical string) SitePage {
		p := sp(path)
		p.Canonical = canonical
		return p
	}
	pages := []SitePage{
		target404, targetRedir, targetNoindex, targetOK, landing,
		mk("/p1", abs("/t404")), mk("/p2", abs("/tred")), mk("/p3", abs("/tnoi")),
		mk("/p4", abs("/tok")), mk("/p5", abs("/landed")), mk("/p6", "https://other.example/x"),
		mk("/p7", abs("/never-crawled")), mk("/self", abs("/self/")),
	}
	// A noindex page canonicalizing to another noindex page (filter variants)
	// is deliberate and must not be flagged.
	variant := mk("/variant?city=x", abs("/tnoi"))
	variant.Noindex = true
	pages = append(pages, variant)
	issues, notes := AnalyzeSite(pages, nil, false, false, nil)

	if is := findIssue(issues, "Canonical", "does not return 200"); is == nil || is.Severity != SeverityCritical || is.Count != 1 || is.Affected[0].URL != abs("/p1") {
		t.Errorf("non-200 canonical: %+v", is)
	}
	if is := findIssue(issues, "Canonical", "URL that redirects"); is == nil || is.Severity != SeverityWarning || is.Count != 1 || is.Affected[0].URL != abs("/p2") {
		t.Errorf("redirecting canonical (a canonical naming only a redirect's landing URL must be fine): %+v", is)
	}
	if is := findIssue(issues, "Canonical", "noindex"); is == nil || is.Severity != SeverityCritical || is.Affected[0].URL != abs("/p3") {
		t.Errorf("noindex canonical: %+v", is)
	}
	if is := findIssue(issues, "Canonical", "different domain"); is == nil || is.Severity != SeverityInfo || is.Affected[0].URL != abs("/p6") {
		t.Errorf("cross-origin canonical: %+v", is)
	}
	if !hasNote(notes, "1 canonical target not verified (not crawled)") {
		t.Errorf("notes = %v", notes)
	}
	for _, is := range issues {
		if is.Category == "Canonical" && is.Count != 1 {
			t.Errorf("unexpected extra canonical hits: %+v", is)
		}
	}
}

func TestAnalyzeSiteHreflang(t *testing.T) {
	alt := func(href string) HreflangAlternate { return HreflangAlternate{Hreflang: "x", Href: href} }
	en, ja, fr, gone := sp("/en"), sp("/ja"), sp("/fr"), SitePage{URL: abs("/gone"), Status: 404}
	en.Hreflang = []HreflangAlternate{alt(abs("/en")), alt(abs("/ja")), alt(abs("/fr")), alt(abs("/gone")), alt(abs("/uncrawled"))}
	ja.Hreflang = []HreflangAlternate{alt(abs("/ja")), alt(abs("/en"))} // returns to en: fine
	fr.Hreflang = []HreflangAlternate{alt(abs("/fr"))}                  // no return link to en
	issues, notes := AnalyzeSite([]SitePage{en, ja, fr, gone}, nil, false, false, nil)

	ret := findIssue(issues, "Hreflang", "return link")
	if ret == nil || ret.Severity != SeverityWarning || ret.Count != 1 || ret.Affected[0].URL != abs("/fr") || ret.Affected[0].From[0] != abs("/en") {
		t.Errorf("missing return link: %+v", ret)
	}
	if bad := findIssue(issues, "Hreflang", "does not return 200"); bad == nil || bad.Count != 1 || bad.Affected[0].URL != abs("/gone") {
		t.Errorf("non-200 alternate: %+v", bad)
	}
	if !hasNote(notes, "1 hreflang target not verified (not crawled)") {
		t.Errorf("notes = %v", notes)
	}

	// negative: hreflang on a noindex or canonicalized-away page is ignored, so it is not audited
	variant := sp("/en?utm=1")
	variant.Canonical = abs("/en")
	variant.Hreflang = []HreflangAlternate{alt(abs("/fr"))}
	if issues, _ := AnalyzeSite([]SitePage{variant, fr}, nil, false, false, nil); findIssue(issues, "Hreflang", "return link") != nil {
		t.Errorf("canonicalized page's hreflang was audited: %+v", issues)
	}

	// negative: fully reciprocal pair
	fr.Hreflang = append(fr.Hreflang, alt(abs("/en")))
	en.Hreflang = en.Hreflang[:3]
	if issues, _ := AnalyzeSite([]SitePage{en, ja, fr}, nil, false, false, nil); findIssue(issues, "Hreflang", "") != nil {
		t.Errorf("reciprocal set flagged: %+v", issues)
	}
}

func TestAnalyzeSiteSitemapListedProblems(t *testing.T) {
	ok, dead := sp("/ok"), SitePage{URL: abs("/dead"), Status: 500}
	redir := sp("/redir")
	redir.RedirectHops, redir.FinalURL = 1, abs("/redir-final")
	noidx := sp("/noidx")
	noidx.Noindex = true
	canon := sp("/canon")
	canon.Canonical = abs("/ok")
	notListed := sp("/unlisted")
	sitemap := []string{abs("/ok"), abs("/dead"), abs("/redir"), abs("/noidx"), abs("/canon")}
	pages := []SitePage{ok, dead, redir, noidx, canon, notListed}

	issues, notes := AnalyzeSite(pages, sitemap, true, false, nil)
	for _, want := range []struct{ contains, url string }{
		{"do not return 200", abs("/dead")}, {"that redirect", abs("/redir")},
		{"that are noindex", abs("/noidx")}, {"canonicalize elsewhere", abs("/canon")},
	} {
		is := findIssue(issues, "Sitemap", want.contains)
		if is == nil || is.Severity != SeverityWarning || is.Count != 1 || is.Affected[0].URL != want.url {
			t.Errorf("%s: %+v", want.contains, is)
		}
	}
	if is := findIssue(issues, "Sitemap", "missing from the sitemap"); is == nil || is.Severity != SeverityInfo || is.Affected[0].URL != abs("/unlisted") {
		t.Errorf("missing-from-sitemap: %+v", is)
	}
	if len(notes) != 0 {
		t.Errorf("notes = %v", notes)
	}

	// no sitemap loaded: no sitemap issues at all
	issues, _ = AnalyzeSite(pages, nil, false, false, nil)
	if findIssue(issues, "Sitemap", "") != nil {
		t.Errorf("sitemap checks ran without a sitemap: %+v", issues)
	}
}

func TestAnalyzeSiteMissingFromSitemapGatedOnComplete(t *testing.T) {
	pages := []SitePage{sp("/a"), sp("/b")}
	sitemap := []string{abs("/a")}
	issues, notes := AnalyzeSite(pages, sitemap, false, false, nil)
	if findIssue(issues, "Sitemap", "missing from the sitemap") != nil {
		t.Errorf("incomplete sitemap must not yield missing-from-sitemap: %+v", issues)
	}
	if !hasNote(notes, "missing-from-sitemap check skipped") {
		t.Errorf("notes = %v", notes)
	}
	issues, _ = AnalyzeSite(pages, sitemap, true, false, nil)
	if is := findIssue(issues, "Sitemap", "missing from the sitemap"); is == nil || is.Count != 1 || is.Affected[0].URL != abs("/b") {
		t.Errorf("complete sitemap: %+v", is)
	}
}

func TestAnalyzeSiteOrphansGatedOnTruncation(t *testing.T) {
	home := sp("/")
	home.outLinks = []string{abs("/linked")}
	linked := sp("/linked")
	sitemap := []string{abs("/"), abs("/linked"), abs("/orphan"), abs("/doc.pdf"), abs("/blocked-by-robots")}
	pages := []SitePage{home, linked}

	issues, notes := AnalyzeSite(pages, sitemap, true, false, []string{abs("/blocked-by-robots")})
	is := findIssue(issues, "Sitemap", "possible orphan")
	if is == nil || is.Severity != SeverityWarning || is.Count != 1 || is.Affected[0].URL != abs("/orphan") {
		t.Fatalf("orphans (assets and robots-blocked URLs are not orphans): %+v", is)
	}
	if hasNote(notes, "orphan check skipped") {
		t.Errorf("notes = %v", notes)
	}

	issues, notes = AnalyzeSite(pages, sitemap, true, true, nil)
	if findIssue(issues, "Sitemap", "possible orphan") != nil {
		t.Errorf("truncated crawl must not report orphans: %+v", issues)
	}
	if !hasNote(notes, "orphan check skipped: crawl stopped at --max-pages") {
		t.Errorf("notes = %v", notes)
	}
}

func TestAnalyzeSiteSitemapOtherHost(t *testing.T) {
	_, notes := AnalyzeSite([]SitePage{sp("/a")}, []string{"https://elsewhere.test/a"}, true, false, nil)
	if !hasNote(notes, "sitemap checks skipped") {
		t.Errorf("notes = %v", notes)
	}
}

func TestAnalyzeSiteDepthRobotsAndCaps(t *testing.T) {
	shallow, deepPage, noidx := sp("/s"), sp("/deep"), sp("/deep-noindex")
	shallow.Depth, deepPage.Depth, noidx.Depth, noidx.Noindex = 3, 4, 5, true
	issues, _ := AnalyzeSite([]SitePage{shallow, deepPage, noidx}, nil, false, false, []string{abs("/private/x")})
	if is := findIssue(issues, "Click Depth", "more than 3 clicks"); is == nil || is.Severity != SeverityInfo || is.Count != 1 || is.Affected[0].URL != abs("/deep") {
		t.Errorf("depth: %+v", is)
	}
	if is := findIssue(issues, "Robots", "robots.txt-disallowed"); is == nil || is.Severity != SeverityInfo || is.Count != 1 {
		t.Errorf("robots: %+v", is)
	}
	if issues, _ := AnalyzeSite([]SitePage{shallow}, nil, false, false, nil); len(issues) != 0 {
		t.Errorf("negative: %+v", issues)
	}

	// Affected is capped at 20 while Count stays true
	var many []SitePage
	for i := 0; i < 25; i++ {
		many = append(many, SitePage{URL: fmt.Sprintf("%s/g%02d", siteBase, i), Status: 404, InLinks: 1, inFrom: []string{abs("/")}})
	}
	issues, _ = AnalyzeSite(many, nil, false, false, nil)
	if is := findIssue(issues, "Broken Links", ""); is == nil || is.Count != 25 || len(is.Affected) != 20 {
		t.Errorf("cap: %+v", is)
	}
}

func TestAnalyzeSiteOrdering(t *testing.T) {
	info := sp("/info")
	info.Depth = 9
	noTitle := sp("/nt")
	noTitle.Title = ""
	a, b := sp("/a"), sp("/b")
	a.MetaDescription, b.MetaDescription = "", ""
	issues, _ := AnalyzeSite([]SitePage{info, noTitle, a, b}, nil, false, false, nil)
	want := []string{"CRITICAL Titles", "WARNING Meta Descriptions", "INFO Click Depth"}
	var got []string
	for _, is := range issues {
		got = append(got, string(is.Severity)+" "+is.Category)
		if is.Fix == "" {
			t.Errorf("issue without fix: %+v", is)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestLinkPages(t *testing.T) {
	pages := []SitePage{
		{URL: abs("/"), outLinks: []string{abs("/a"), abs("/a/"), abs("/"), abs("/b")}},
		{URL: abs("/a"), outLinks: []string{abs("/b")}},
		{URL: abs("/b")},
	}
	linkPages(pages)
	if pages[0].InLinks != 0 || pages[1].InLinks != 1 || pages[2].InLinks != 2 {
		t.Errorf("inlinks = %d %d %d", pages[0].InLinks, pages[1].InLinks, pages[2].InLinks)
	}
	if len(pages[2].inFrom) != 2 || pages[2].inFrom[0] != abs("/") {
		t.Errorf("inFrom = %v", pages[2].inFrom)
	}
}

func TestAuditSiteEndToEnd(t *testing.T) {
	longText := strings.Repeat("Distinct body copy for this page. ", 12)
	page := func(w http.ResponseWriter, head, body string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, "<html lang=\"en\"><head>%s</head><body>%s</body></html>", head, body)
	}
	desc := func(s string) string { return `<meta name="description" content="` + s + `">` }

	mux := http.NewServeMux()
	var host string
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		page(w, "<title>Home</title>"+desc("home"),
			`<a href="/a">A</a><a href="/b">B</a><a href="/gone">gone</a><a href="/hidden">hidden</a><a href="/en">en</a><a href="/ja">ja</a>`+longText+"home")
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		page(w, "<title>Shared Title</title>"+desc("a"), longText+"a")
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		page(w, "<title>Shared Title</title>"+desc("b"), longText+"b")
	})
	mux.HandleFunc("/hidden", func(w http.ResponseWriter, r *http.Request) {
		page(w, "<title>Hidden</title>"+desc("h")+`<meta name="robots" content="noindex">`, longText+"h")
	})
	mux.HandleFunc("/en", func(w http.ResponseWriter, r *http.Request) {
		page(w, "<title>English</title>"+desc("en")+`<link rel="alternate" hreflang="en" href="/en"><link rel="alternate" hreflang="ja" href="/ja">`, longText+"en")
	})
	mux.HandleFunc("/ja", func(w http.ResponseWriter, r *http.Request) {
		// no hreflang back to /en
		page(w, "<title>Japanese</title>"+desc("ja"), longText+"ja")
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`+
			`<url><loc>%[1]s/</loc></url><url><loc>%[1]s/a</loc></url><url><loc>%[1]s/hidden</loc></url><url><loc>%[1]s/lonely</loc></url></urlset>`, host)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	host = ts.URL

	client := crawler.NewSafeClient(crawler.ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := AuditSite(context.Background(), client, ts.URL, SiteAuditOptions{MaxPages: 50})
	if err != nil {
		t.Fatal(err)
	}
	if report.PagesCrawled != 7 || report.Truncated {
		t.Errorf("crawled %d truncated=%v", report.PagesCrawled, report.Truncated)
	}
	if report.Sitemap == nil || report.Sitemap.TotalURLs != 4 || !report.Sitemap.Complete || report.Sitemap.Error != "" {
		t.Errorf("sitemap = %+v", report.Sitemap)
	}
	u := func(p string) string { return ts.URL + p }

	if is := findIssue(report.Issues, "Broken Links", ""); is == nil || is.Severity != SeverityCritical || is.Affected[0].URL != u("/gone") || is.Affected[0].From[0] != u("/") {
		t.Errorf("broken link: %+v", is)
	}
	if is := findIssue(report.Issues, "Titles", "share a <title>"); is == nil || is.Affected[0].URL != u("/a") || is.Affected[0].From[0] != u("/b") {
		t.Errorf("duplicate title: %+v", is)
	}
	if is := findIssue(report.Issues, "Sitemap", "that are noindex"); is == nil || is.Affected[0].URL != u("/hidden") {
		t.Errorf("noindex in sitemap: %+v", is)
	}
	if is := findIssue(report.Issues, "Hreflang", "return link"); is == nil || is.Affected[0].URL != u("/ja") || is.Affected[0].From[0] != u("/en") {
		t.Errorf("hreflang: %+v", is)
	}
	if is := findIssue(report.Issues, "Sitemap", "possible orphan"); is == nil || is.Affected[0].URL != u("/lonely") {
		t.Errorf("orphan: %+v", is)
	}
	if is := findIssue(report.Issues, "Sitemap", "missing from the sitemap"); is == nil || is.Count != 3 { // /b, /en, /ja
		t.Errorf("missing from sitemap: %+v", is)
	}
	if report.Summary.Critical != 1 || report.Summary.Warning < 4 {
		t.Errorf("summary = %+v", report.Summary)
	}
	if report.Issues[0].Severity != SeverityCritical {
		t.Errorf("issues not sorted by severity: %+v", report.Issues[0])
	}
	var home *SitePage
	for i := range report.Pages {
		if report.Pages[i].URL == u("/") {
			home = &report.Pages[i]
		}
	}
	if home == nil || home.Title != "Home" || home.Depth != 0 || home.OutLinks != 6 || home.InLinks != 0 {
		t.Errorf("home page = %+v", home)
	}

	// --no-sitemap skips sitemap work entirely
	report, err = AuditSite(context.Background(), client, ts.URL, SiteAuditOptions{MaxPages: 50, NoSitemap: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Sitemap != nil || findIssue(report.Issues, "Sitemap", "") != nil {
		t.Errorf("NoSitemap still produced sitemap output: %+v", report.Sitemap)
	}

	// truncated crawl: orphan check is skipped and noted
	report, err = AuditSite(context.Background(), client, ts.URL, SiteAuditOptions{MaxPages: 2, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Truncated || findIssue(report.Issues, "Sitemap", "possible orphan") != nil || !hasNote(report.Notes, "orphan check skipped") {
		t.Errorf("truncated audit: truncated=%v notes=%v", report.Truncated, report.Notes)
	}
}

func TestAnalyzeSiteRedirectAndTargetAreNotDuplicates(t *testing.T) {
	target := sp("/page")
	target.textHash = "h"
	viaRedirect := sp("/page.html") // redirects to /page: same title, description and text by construction
	viaRedirect.Title, viaRedirect.MetaDescription, viaRedirect.textHash = target.Title, target.MetaDescription, "h"
	viaRedirect.Depth, viaRedirect.RedirectHops, viaRedirect.FinalURL = 1, 1, abs("/page")
	// two redirects into one uncrawled landing page count once
	r1, r2 := sp("/r1"), sp("/r2")
	for _, r := range []*SitePage{&r1, &r2} {
		r.Title, r.MetaDescription, r.textHash = "Landing", "Landing desc", "hl"
		r.RedirectHops, r.FinalURL = 1, abs("/landing")
	}
	issues, _ := AnalyzeSite([]SitePage{target, viaRedirect, r1, r2}, nil, false, false, nil)
	for _, cat := range []string{"Titles", "Meta Descriptions", "Duplicate Content"} {
		if is := findIssue(issues, cat, "share"); is != nil {
			t.Errorf("%s flagged a redirect and its own target: %+v", cat, is)
		}
		if is := findIssue(issues, cat, "identical"); is != nil {
			t.Errorf("%s flagged a redirect and its own target: %+v", cat, is)
		}
	}
}
