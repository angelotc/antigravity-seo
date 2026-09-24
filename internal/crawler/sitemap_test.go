package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
