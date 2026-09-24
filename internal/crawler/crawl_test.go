package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
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

// TestCrawlSiteFinalURLAndNoDoubleCrawl covers task 5: CrawledPage.FinalURL
// is recorded, the sitemap lists the final (post-redirect) URL, and a page
// reached only via a redirect is not separately queued/fetched again.
func TestCrawlSiteFinalURLAndNoDoubleCrawl(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Home</title></head><body><a href="/old">Old</a></body></html>`))
	})
	var newFetches int
	var newMu sync.Mutex
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		newMu.Lock()
		newFetches++
		newMu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>New</title></head><body><a href="/new">Self</a></body></html>`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.CrawlSite(context.Background(), ts.URL, 20)
	if err != nil {
		t.Fatal(err)
	}

	if report.Visited != 2 {
		t.Fatalf("expected 2 fetches (start, /old); /new must not be separately crawled, got %d visited: %+v", report.Visited, report.Pages)
	}
	newMu.Lock()
	defer newMu.Unlock()
	if newFetches != 1 {
		t.Errorf("expected /new's handler to run exactly once (via the /old redirect), got %d", newFetches)
	}

	var oldPage *CrawledPage
	for i := range report.Pages {
		if report.Pages[i].URL == ts.URL+"/old" {
			oldPage = &report.Pages[i]
		}
	}
	if oldPage == nil {
		t.Fatalf("expected a page entry for /old, got: %+v", report.Pages)
	}
	if oldPage.FinalURL != ts.URL+"/new" {
		t.Errorf("expected FinalURL %s, got %s", ts.URL+"/new", oldPage.FinalURL)
	}

	xmlOut := string(GenerateSitemapXML(report.Pages))
	if !strings.Contains(xmlOut, ts.URL+"/new") {
		t.Errorf("sitemap should list the final URL:\n%s", xmlOut)
	}
	if strings.Contains(xmlOut, ts.URL+"/old") {
		t.Errorf("sitemap should not list the pre-redirect URL:\n%s", xmlOut)
	}
}

// TestCrawlSiteExcludesOffOriginRedirects covers task 5: a same-origin URL
// that redirects off-origin must not appear in Pages.
func TestCrawlSiteExcludesOffOriginRedirects(t *testing.T) {
	var external *httptest.Server
	external = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>external</body></html>`))
	}))
	defer external.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><a href="/leave">Leave</a></body></html>`))
	})
	mux.HandleFunc("/leave", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, external.URL+"/target", http.StatusFound)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.CrawlSite(context.Background(), ts.URL, 20)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range report.Pages {
		if strings.Contains(p.URL, "/leave") || strings.Contains(p.FinalURL, external.URL) {
			t.Errorf("off-origin redirect must not appear in Pages: %+v", p)
		}
	}
	found := false
	for _, e := range report.Excluded {
		if strings.Contains(e, "/leave") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected /leave in Excluded, got %+v", report.Excluded)
	}
}

// TestCrawlSiteSeedBlockedByRobots covers task 6a: the seed URL itself must
// be checked against robots.txt before it is ever fetched.
func TestCrawlSiteSeedBlockedByRobots(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Error("seed page must not be fetched when robots.txt disallows it")
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.CrawlSite(context.Background(), ts.URL, 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.Visited != 0 {
		t.Errorf("expected 0 visited pages, got %d", report.Visited)
	}
	if len(report.Excluded) != 1 {
		t.Fatalf("expected the seed URL reported as excluded, got %+v", report.Excluded)
	}
}

// TestCrawlSiteRobotsServerErrorDisallowsAll covers task 6b: a 5xx (or
// unreachable) robots.txt means the whole site is disallowed per RFC 9309.
func TestCrawlSiteRobotsServerErrorDisallowsAll(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Error("seed page must not be fetched when robots.txt 5xx forces disallow-all")
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.CrawlSite(context.Background(), ts.URL, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Excluded) != 1 {
		t.Fatalf("expected seed excluded due to robots.txt 5xx disallow-all, got %+v", report.Excluded)
	}
}

// TestCrawlSiteRobotsNotFoundAllowsAll covers task 6b: a 4xx/missing
// robots.txt means there are no restrictions.
func TestCrawlSiteRobotsNotFoundAllowsAll(t *testing.T) {
	mux := http.NewServeMux()
	// No /robots.txt handler registered -> 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.CrawlSite(context.Background(), ts.URL, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Pages) != 1 {
		t.Fatalf("expected the seed page crawled when robots.txt is missing (404), got %+v", report.Pages)
	}
}

// TestCrawlSiteHonorsCrawlDelay covers task 6c: a Crawl-delay for "*" must
// serialize fetches and space them out.
func TestCrawlSiteHonorsCrawlDelay(t *testing.T) {
	var mu sync.Mutex
	var times []time.Time
	var inFlight, maxInFlight int32

	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("User-agent: *\nCrawl-delay: 0.05\n"))
	})
	record := func(w http.ResponseWriter, body string) {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		times = append(times, time.Now())
		mu.Unlock()

		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))

		mu.Lock()
		inFlight--
		mu.Unlock()
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		record(w, `<html><body><a href="/a">a</a><a href="/b">b</a></body></html>`)
	})
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) { record(w, `<html><body>a</body></html>`) })
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) { record(w, `<html><body>b</body></html>`) })
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	start := time.Now()
	report, err := client.CrawlSite(context.Background(), ts.URL, 10)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if report.Visited != 3 {
		t.Fatalf("expected 3 fetches, got %d", report.Visited)
	}
	if maxInFlight > 1 {
		t.Errorf("expected fetches to be serialized (max 1 concurrent), saw %d concurrent", maxInFlight)
	}
	// 3 fetches, serialized, with a 50ms delay after each => at least 2 gaps.
	if elapsed < 90*time.Millisecond {
		t.Errorf("expected crawl-delay to space out requests, elapsed only %s", elapsed)
	}

	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	for i := 1; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		if gap < 30*time.Millisecond {
			t.Errorf("expected >=~50ms gap between serialized fetches, got %s between fetch %d and %d", gap, i-1, i)
		}
	}
}

// TestCrawlSiteExcludesNoindexSignals covers task 7: pages must be excluded
// both for an X-Robots-Tag noindex/none header and a <meta robots
// content="none"> tag.
func TestCrawlSiteExcludesNoindexSignals(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><a href="/tagged">tagged</a><a href="/none-meta">none-meta</a></body></html>`))
	})
	mux.HandleFunc("/tagged", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Robots-Tag", "googlebot: noindex")
		_, _ = w.Write([]byte(`<html><body>tagged</body></html>`))
	})
	mux.HandleFunc("/none-meta", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><meta name="robots" content="none"></head><body>none</body></html>`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.CrawlSite(context.Background(), ts.URL, 10)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range report.Pages {
		if strings.Contains(p.URL, "/tagged") || strings.Contains(p.URL, "/none-meta") {
			t.Errorf("noindex page leaked into Pages: %+v", p)
		}
	}
	excludedSet := map[string]bool{}
	for _, e := range report.Excluded {
		excludedSet[e] = true
	}
	if !excludedSet[ts.URL+"/tagged"] {
		t.Errorf("expected X-Robots-Tag noindex page excluded, got %+v", report.Excluded)
	}
	if !excludedSet[ts.URL+"/none-meta"] {
		t.Errorf("expected meta robots=none page excluded, got %+v", report.Excluded)
	}
}

// TestCrawlSiteDedupeKeepsPaginationParams covers task 8: the visited set
// dedupes by urlnorm.Key (stripping tracking params) while keeping distinct
// non-tracking params like pagination as separate pages.
func TestCrawlSiteDedupeKeepsPaginationParams(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
			<a href="/list?page=2">p2</a>
			<a href="/list?page=2&utm_source=newsletter">p2-tracked</a>
			<a href="/list?page=3">p3</a>
			</body></html>`))
	})
	mux.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.RawQuery]++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>list</body></html>`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.CrawlSite(context.Background(), ts.URL, 10)
	if err != nil {
		t.Fatal(err)
	}

	// start + /list?page=2 (deduped w/ its utm_source variant) + /list?page=3
	if report.Visited != 3 {
		t.Fatalf("expected 3 fetches (start, page=2 deduped, page=3), got %d: %+v", report.Visited, report.Pages)
	}

	mu.Lock()
	defer mu.Unlock()
	if n := hits["page=2"] + hits["page=2&utm_source=newsletter"]; n != 1 {
		t.Errorf("expected /list?page=2 (with or without utm_source) fetched exactly once, got hits=%v", hits)
	}
	if hits["page=3"] != 1 {
		t.Errorf("expected /list?page=3 fetched exactly once, got hits=%v", hits)
	}

	foundPage2, foundPage3 := false, false
	for _, p := range report.Pages {
		if strings.Contains(p.URL, "page=2") {
			foundPage2 = true
			if strings.Contains(p.URL, "utm_source") {
				t.Errorf("tracking param should be stripped from reported URL: %s", p.URL)
			}
		}
		if strings.Contains(p.URL, "page=3") {
			foundPage3 = true
		}
	}
	if !foundPage2 || !foundPage3 {
		t.Errorf("expected both page=2 and page=3 reported as distinct pages, got %+v", report.Pages)
	}
}
