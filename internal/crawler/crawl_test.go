package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCrawlSiteAndGenerateSitemap(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Home</title></head><body>
		<a href="/about">About</a> <a href="/listings/tokyo">Tokyo</a>
		<a href="https://external.example.com/elsewhere">External</a>
		<a href="/styles.css">Asset</a> <a href="#anchor">Anchor</a>
		</body></html>`))
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>About</title><meta name="robots" content="noindex"></head><body></body></html>`))
	})
	mux.HandleFunc("/listings/tokyo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Last-Modified", "Wed, 16 Sep 2026 10:00:00 GMT")
		_, _ = w.Write([]byte(`<html><head><title>Tokyo</title></head><body><a href="/">Back</a></body></html>`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.CrawlSite(context.Background(), ts.URL, 50)
	if err != nil {
		t.Fatal(err)
	}

	// noindex /about must be excluded; home + tokyo included
	if len(report.Pages) != 2 {
		t.Fatalf("expected 2 pages in sitemap, got %d: %+v", len(report.Pages), report.Pages)
	}
	for _, p := range report.Pages {
		if p.URL == ts.URL+"/about" {
			t.Error("noindex page must not appear in generated sitemap")
		}
		if p.URL == ts.URL+"/listings/tokyo" && p.LastMod == "" {
			t.Error("Last-Modified header should populate lastmod")
		}
	}

	xmlOut := string(GenerateSitemapXML(report.Pages))
	if !strings.Contains(xmlOut, "<urlset") || !strings.Contains(xmlOut, ts.URL+"/listings/tokyo") {
		t.Errorf("generated XML missing expected content:\n%s", xmlOut)
	}
	if !strings.Contains(xmlOut, "<lastmod>2026-09-16") {
		t.Errorf("lastmod not rendered:\n%s", xmlOut)
	}
	if strings.Contains(xmlOut, "/about") || strings.Contains(xmlOut, "external.example.com") {
		t.Errorf("excluded URLs leaked into sitemap:\n%s", xmlOut)
	}
}

func TestCrawlSiteRespectsRobots(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("User-agent: *\nDisallow: /private/\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><a href="/private/secret">secret</a><a href="/public">public</a></body></html>`))
	})
	mux.HandleFunc("/public", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>public</body></html>"))
	})
	mux.HandleFunc("/private/secret", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>secret</body></html>"))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.CrawlSite(context.Background(), ts.URL, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range report.Pages {
		if strings.Contains(p.URL, "/private/") {
			t.Errorf("robots-disallowed page crawled: %s", p.URL)
		}
	}
}

func TestXMLEscape(t *testing.T) {
	out := string(GenerateSitemapXML([]CrawledPage{{URL: "https://example.com/a&b?x=1<2>"}}))
	if !strings.Contains(out, "https://example.com/a&amp;b?x=1&lt;2&gt;") {
		t.Errorf("XML escaping failed:\n%s", out)
	}
}
