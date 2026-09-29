package crawler

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseSitemapURLSet(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page1</loc>
    <lastmod>2026-09-01</lastmod>
    <changefreq>daily</changefreq>
    <priority>0.8</priority>
  </url>
  <url>
    <loc>https://example.com/page2</loc>
    <lastmod>2026-09-02</lastmod>
  </url>
</urlset>`)

	urls, children, isIndex, err := ParseSitemap(xmlData, false)
	if err != nil {
		t.Fatalf("ParseSitemap error: %v", err)
	}

	if isIndex {
		t.Errorf("Expected urlset, but detected sitemap index")
	}

	if len(children) != 0 {
		t.Errorf("Expected 0 child sitemaps, got %d", len(children))
	}

	if len(urls) != 2 {
		t.Fatalf("Expected 2 URLs, got %d", len(urls))
	}

	if urls[0].Loc != "https://example.com/page1" {
		t.Errorf("Expected loc https://example.com/page1, got %s", urls[0].Loc)
	}
	if urls[0].Priority != 0.8 {
		t.Errorf("Expected priority 0.8, got %f", urls[0].Priority)
	}
}

func TestParseSitemapIndex(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap>
    <loc>https://example.com/sub-sitemap1.xml</loc>
    <lastmod>2026-09-10</lastmod>
  </sitemap>
  <sitemap>
    <loc>https://example.com/sub-sitemap2.xml</loc>
  </sitemap>
</sitemapindex>`)

	urls, children, isIndex, err := ParseSitemap(xmlData, false)
	if err != nil {
		t.Fatalf("ParseSitemap error: %v", err)
	}

	if !isIndex {
		t.Errorf("Expected sitemap index, but detected urlset")
	}

	if len(children) != 2 {
		t.Fatalf("Expected 2 child sitemaps, got %d", len(children))
	}

	if children[0] != "https://example.com/sub-sitemap1.xml" {
		t.Errorf("Expected child sitemap loc https://example.com/sub-sitemap1.xml, got %s", children[0])
	}

	if len(urls) != 0 {
		t.Errorf("Expected 0 direct URLs in sitemap index, got %d", len(urls))
	}
}

// TestInspectSitemapBlockedVsBroken covers task 9: 401/403/429 responses
// must land in BlockedURLs (auth/bot-protected, not verified broken), while
// 404/410/5xx remain in BrokenURLs.
func TestInspectSitemapBlockedVsBroken(t *testing.T) {
	mux := http.NewServeMux()
	statusFor := map[string]int{
		"/ok":        http.StatusOK,
		"/notfound":  http.StatusNotFound,
		"/gone":      http.StatusGone,
		"/servererr": http.StatusInternalServerError,
		"/unauth":    http.StatusUnauthorized,
		"/forbidden": http.StatusForbidden,
		"/limited":   http.StatusTooManyRequests,
	}
	for path, status := range statusFor {
		status := status
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})
	}
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// The sitemap body needs real server URLs, which aren't known until
	// ts.URL exists.
	mux.HandleFunc("/sitemap-real.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		var sb strings.Builder
		sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
		for path := range statusFor {
			sb.WriteString(fmt.Sprintf("<url><loc>%s%s</loc></url>", ts.URL, path))
		}
		sb.WriteString(`</urlset>`)
		_, _ = w.Write([]byte(sb.String()))
	})

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.InspectSitemap(context.Background(), ts.URL+"/sitemap-real.xml", len(statusFor))
	if err != nil {
		t.Fatal(err)
	}

	joinedBroken := strings.Join(report.BrokenURLs, "\n")
	joinedBlocked := strings.Join(report.BlockedURLs, "\n")

	for _, path := range []string{"/notfound", "/gone", "/servererr"} {
		if !strings.Contains(joinedBroken, ts.URL+path) {
			t.Errorf("expected %s in BrokenURLs, got %v", path, report.BrokenURLs)
		}
		if strings.Contains(joinedBlocked, ts.URL+path) {
			t.Errorf("did not expect %s in BlockedURLs, got %v", path, report.BlockedURLs)
		}
	}
	for _, path := range []string{"/unauth", "/forbidden", "/limited"} {
		if !strings.Contains(joinedBlocked, ts.URL+path) {
			t.Errorf("expected %s in BlockedURLs, got %v", path, report.BlockedURLs)
		}
		if strings.Contains(joinedBroken, ts.URL+path) {
			t.Errorf("did not expect %s in BrokenURLs, got %v", path, report.BrokenURLs)
		}
	}
	if strings.Contains(joinedBroken, ts.URL+"/ok") || strings.Contains(joinedBlocked, ts.URL+"/ok") {
		t.Errorf("200 URL should not appear as broken or blocked")
	}
}

func smURLSet(locs ...string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		sb.WriteString("<url><loc>" + l + "</loc></url>")
	}
	sb.WriteString(`</urlset>`)
	return sb.String()
}

func smIndex(locs ...string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, l := range locs {
		sb.WriteString("<sitemap><loc>" + l + "</loc></sitemap>")
	}
	sb.WriteString(`</sitemapindex>`)
	return sb.String()
}

func smGzip(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// smServer serves the given path->body map (200, application/xml); other
// paths 404. "{HOST}" in a body expands to the server URL, a "GZ:" prefix
// serves the rest gzipped with no gzip hints, and "HTML:" serves text/html.
// The returned map counts requests per path.
func smServer(t *testing.T, pages map[string]string) (*httptest.Server, *sync.Map) {
	t.Helper()
	hits := &sync.Map{}
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := hits.LoadOrStore(r.URL.Path, new(int32))
		atomic.AddInt32(n.(*int32), 1)
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		body = strings.ReplaceAll(body, "{HOST}", ts.URL)
		switch {
		case strings.HasPrefix(body, "GZ:"):
			_, _ = w.Write(smGzip(t, body[3:]))
		case strings.HasPrefix(body, "HTML:"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(body[5:]))
		default:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(ts.Close)
	return ts, hits
}

func smClient() *SafeClient {
	return NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
}

func smHits(hits *sync.Map, path string) int {
	n, ok := hits.Load(path)
	if !ok {
		return 0
	}
	return int(atomic.LoadInt32(n.(*int32)))
}

func TestInspectSitemapIndexMixedChildren(t *testing.T) {
	ts, _ := smServer(t, map[string]string{
		"/sitemap.xml": smIndex("{HOST}/a.xml", "{HOST}/b-noext", "{HOST}/missing.xml"),
		"/a.xml":       smURLSet("{HOST}/a1", "{HOST}/a2"),
		"/b-noext":     "GZ:" + smURLSet("{HOST}/b1", "{HOST}/b2", "{HOST}/b3"),
	})
	report, err := smClient().InspectSitemapWithOptions(context.Background(), ts.URL+"/sitemap.xml", SitemapOptions{SampleSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !report.IsSitemapIndex || report.TotalURLs != 5 || len(report.SampleURLs) != 5 {
		t.Fatalf("unexpected report: index=%v total=%d sample=%d", report.IsSitemapIndex, report.TotalURLs, len(report.SampleURLs))
	}
	if report.Complete {
		t.Error("Complete should be false with a 404 child")
	}
	if len(report.Children) != 3 || len(report.ChildSitemaps) != 3 {
		t.Fatalf("expected 3 children, got %+v", report.Children)
	}
	if c := report.Children[0]; !c.Fetched || c.URLCount != 2 {
		t.Errorf("child a: %+v", c)
	}
	if c := report.Children[1]; !c.Fetched || c.URLCount != 3 {
		t.Errorf("gzip magic-only child: %+v", c)
	}
	if c := report.Children[2]; c.Fetched || !strings.Contains(c.Error, "HTTP 404") {
		t.Errorf("404 child: %+v", c)
	}
	if !strings.Contains(strings.Join(report.Errors, "\n"), "/missing.xml") {
		t.Errorf("expected error for missing child, got %v", report.Errors)
	}

	// aggregate follows child order, not completion order
	col, err := smClient().CollectSitemapURLs(context.Background(), ts.URL+"/sitemap.xml", SitemapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/a1", "/a2", "/b1", "/b2", "/b3"}
	if len(col.URLs) != len(want) {
		t.Fatalf("got %d URLs", len(col.URLs))
	}
	for i, u := range col.URLs {
		if u.Loc != ts.URL+want[i] {
			t.Errorf("URL %d = %s, want %s", i, u.Loc, want[i])
		}
	}
}

func TestCollectSitemapNestedIndex(t *testing.T) {
	ts, _ := smServer(t, map[string]string{
		"/root.xml":  smIndex("{HOST}/mid.xml", "{HOST}/leaf1.xml"),
		"/mid.xml":   smIndex("{HOST}/leaf2.xml"),
		"/leaf1.xml": smURLSet("{HOST}/1"),
		"/leaf2.xml": smURLSet("{HOST}/2", "{HOST}/3"),
	})
	col, err := smClient().CollectSitemapURLs(context.Background(), ts.URL+"/root.xml", SitemapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(col.URLs) != 3 || !col.Complete || !col.IsIndex || len(col.Children) != 3 {
		t.Fatalf("urls=%d complete=%v index=%v children=%d", len(col.URLs), col.Complete, col.IsIndex, len(col.Children))
	}
	// depth-first in listed order: mid's subtree comes before leaf1
	if col.URLs[0].Loc != ts.URL+"/2" || col.URLs[2].Loc != ts.URL+"/1" {
		t.Errorf("unexpected order: %+v", col.URLs)
	}
}

func TestCollectSitemapCyclicIndex(t *testing.T) {
	ts, hits := smServer(t, map[string]string{
		"/loop.xml": smIndex("{HOST}/loop.xml", "{HOST}/leaf.xml"),
		"/leaf.xml": smURLSet("{HOST}/x"),
	})
	col, err := smClient().CollectSitemapURLs(context.Background(), ts.URL+"/loop.xml", SitemapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(col.URLs) != 1 || !col.Complete {
		t.Errorf("urls=%d complete=%v errors=%v", len(col.URLs), col.Complete, col.Errors)
	}
	if n := smHits(hits, "/loop.xml"); n != 1 {
		t.Errorf("loop.xml fetched %d times", n)
	}
}

func TestCollectSitemapMaxChildrenCap(t *testing.T) {
	pages := map[string]string{"/i.xml": smIndex("{HOST}/c1.xml", "{HOST}/c2.xml", "{HOST}/c3.xml", "{HOST}/c4.xml")}
	for i := 1; i <= 4; i++ {
		pages[fmt.Sprintf("/c%d.xml", i)] = smURLSet(fmt.Sprintf("{HOST}/p%d", i))
	}
	ts, hits := smServer(t, pages)
	col, err := smClient().CollectSitemapURLs(context.Background(), ts.URL+"/i.xml", SitemapOptions{MaxChildren: 2})
	if err != nil {
		t.Fatal(err)
	}
	if col.Complete || len(col.Children) != 4 || len(col.URLs) != 2 {
		t.Fatalf("complete=%v children=%d urls=%d", col.Complete, len(col.Children), len(col.URLs))
	}
	for i, ch := range col.Children {
		if ch.Fetched != (i < 2) {
			t.Errorf("child %d fetched=%v", i, ch.Fetched)
		}
	}
	if smHits(hits, "/c3.xml") != 0 || smHits(hits, "/c4.xml") != 0 {
		t.Error("children beyond the cap must not be fetched")
	}
}

func TestInspectSitemapSampling(t *testing.T) {
	pages := map[string]string{}
	var kids []string
	for i := 0; i < 5; i++ {
		var locs []string
		for j := 0; j < 30; j++ {
			locs = append(locs, fmt.Sprintf("{HOST}/c%d/p%d", i, j))
		}
		pages[fmt.Sprintf("/c%d.xml", i)] = smURLSet(locs...)
		kids = append(kids, fmt.Sprintf("{HOST}/c%d.xml", i))
	}
	pages["/idx.xml"] = smIndex(kids...)
	ts, _ := smServer(t, pages)

	report, err := smClient().InspectSitemapWithOptions(context.Background(), ts.URL+"/idx.xml", SitemapOptions{SampleSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, u := range report.SampleURLs {
		seen[strings.Split(strings.TrimPrefix(u.Loc, ts.URL+"/"), "/")[0]] = true
	}
	if len(report.SampleURLs) != 5 || len(seen) != 5 {
		t.Errorf("sample should cover every child: %d urls, %d children", len(report.SampleURLs), len(seen))
	}

	report, err = smClient().InspectSitemapWithOptions(context.Background(), ts.URL+"/idx.xml", SitemapOptions{SampleSize: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.SampleURLs) != 100 {
		t.Errorf("sample should clamp to 100, got %d", len(report.SampleURLs))
	}
}

func TestInspectSitemapSampleSpreadWithinURLSet(t *testing.T) {
	var locs []string
	for i := 0; i < 100; i++ {
		locs = append(locs, fmt.Sprintf("{HOST}/p%d", i))
	}
	ts, hits := smServer(t, map[string]string{"/s.xml": smURLSet(locs...)})
	report, err := smClient().InspectSitemap(context.Background(), ts.URL+"/s.xml", 4)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, u := range report.SampleURLs {
		got = append(got, strings.TrimPrefix(u.Loc, ts.URL))
	}
	if strings.Join(got, ",") != "/p0,/p25,/p50,/p75" {
		t.Errorf("sample not evenly spread: %v", got)
	}
	// the health check hits exactly the sample, not the first N
	for i := 0; i < 100; i++ {
		want := 0
		if i%25 == 0 {
			want = 1
		}
		if n := smHits(hits, fmt.Sprintf("/p%d", i)); n != want {
			t.Errorf("/p%d health-checked %d times, want %d", i, n, want)
		}
	}

	// limit 0 still yields the default 10-URL sample
	report, err = smClient().InspectSitemap(context.Background(), ts.URL+"/s.xml", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.SampleURLs) != 10 {
		t.Errorf("limit 0 sample = %d, want 10", len(report.SampleURLs))
	}
}

func TestSitemapPerFileURLLimitWarning(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<urlset>`)
	for i := 0; i < 50001; i++ {
		fmt.Fprintf(&sb, "<url><loc>http://x/%d</loc></url>", i)
	}
	sb.WriteString(`</urlset>`)
	ts, _ := smServer(t, map[string]string{"/big.xml": sb.String()})
	report, err := smClient().InspectSitemapWithOptions(context.Background(), ts.URL+"/big.xml", SitemapOptions{SampleSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalURLs != 50001 || !strings.Contains(strings.Join(report.Errors, "\n"), "50,000 URL limit") {
		t.Errorf("total=%d errors=%v", report.TotalURLs, report.Errors)
	}
}

func TestDiscoverSitemapsFromRobots(t *testing.T) {
	ts, _ := smServer(t, map[string]string{
		"/robots.txt": "User-agent: *\nDisallow:\nSitemap: {HOST}/one.xml\nSitemap: {HOST}/two.xml\n",
		"/one.xml":    smURLSet("{HOST}/a"),
		"/two.xml":    smURLSet("{HOST}/b"),
	})
	report, err := smClient().InspectSitemap(context.Background(), ts.URL, 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.DiscoveredFrom != "robots.txt" || report.SitemapURL != ts.URL+"/one.xml" {
		t.Errorf("discovered_from=%q sitemap_url=%q", report.DiscoveredFrom, report.SitemapURL)
	}
	if !report.IsSitemapIndex || report.TotalURLs != 2 || len(report.Children) != 2 || !report.Complete {
		t.Errorf("multi-root: index=%v total=%d children=%d complete=%v", report.IsSitemapIndex, report.TotalURLs, len(report.Children), report.Complete)
	}
}

func TestDiscoverSitemapsFallback(t *testing.T) {
	ts, _ := smServer(t, map[string]string{
		"/robots.txt":  "User-agent: *\nDisallow: /private\n",
		"/sitemap.xml": smURLSet("{HOST}/a", "{HOST}/b"),
	})
	sm, source, err := smClient().DiscoverSitemaps(context.Background(), ts.URL+"/")
	if err != nil || source != "/sitemap.xml" || len(sm) != 1 || sm[0] != ts.URL+"/sitemap.xml" {
		t.Fatalf("sm=%v source=%q err=%v", sm, source, err)
	}

	ts2, _ := smServer(t, map[string]string{"/sitemap_index.xml": smIndex("{HOST}/c.xml"), "/c.xml": smURLSet("{HOST}/z")})
	sm, source, err = smClient().DiscoverSitemaps(context.Background(), ts2.URL)
	if err != nil || source != "/sitemap_index.xml" || len(sm) != 1 {
		t.Fatalf("sm=%v source=%q err=%v", sm, source, err)
	}
}

func TestDiscoverSitemapsNoneFound(t *testing.T) {
	// a catch-all HTML 200 for /sitemap.xml must not be accepted
	ts, _ := smServer(t, map[string]string{"/sitemap.xml": "HTML:<html><body>hi</body></html>"})
	report, err := smClient().InspectSitemap(context.Background(), ts.URL, 10)
	if err == nil || report != nil || !strings.Contains(err.Error(), "no sitemap found: robots.txt has no Sitemap: directive") {
		t.Fatalf("expected no-sitemap error, got report=%v err=%v", report, err)
	}
}

func TestInspectSitemapHTMLAndNon200(t *testing.T) {
	ts, _ := smServer(t, map[string]string{
		"/page":    "HTML:<!DOCTYPE html><html><body>&nbsp;</body></html>",
		"/sniffed": "  \n<html><head></head></html>",
	})
	for _, path := range []string{"/page", "/sniffed"} {
		_, err := smClient().InspectSitemap(context.Background(), ts.URL+path, 5)
		if err == nil || !strings.Contains(err.Error(), "returned HTML, not a sitemap") {
			t.Errorf("%s: expected HTML error, got %v", path, err)
		}
	}
	_, err := smClient().InspectSitemap(context.Background(), ts.URL+"/nope.xml", 5)
	if err == nil || !strings.Contains(err.Error(), "sitemap returned HTTP 404") {
		t.Errorf("expected HTTP 404 error, got %v", err)
	}
}

func TestCollectSitemapTruncatedBodyKeepsPartialURLs(t *testing.T) {
	var locs []string
	for i := 0; i < 200; i++ {
		locs = append(locs, fmt.Sprintf("{HOST}/p%d", i))
	}
	ts, _ := smServer(t, map[string]string{"/big.xml": smURLSet(locs...)})
	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true, MaxBodyBytes: 2000})
	col, err := client.CollectSitemapURLs(context.Background(), ts.URL+"/big.xml", SitemapOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if col.Complete || len(col.URLs) == 0 || len(col.URLs) >= 200 {
		t.Errorf("complete=%v urls=%d", col.Complete, len(col.URLs))
	}
	if len(col.Errors) != 1 || !strings.Contains(col.Errors[0], "truncated") {
		t.Errorf("errors=%v", col.Errors)
	}
}
