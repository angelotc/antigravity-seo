package crawler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/japanese"
)

func TestSSRFBlockedIPs(t *testing.T) {
	blockedCases := []string{
		"127.0.0.1",
		"10.0.0.5",
		"192.168.1.1",
		"172.16.0.1",
		"169.254.169.254",
		"::1",
	}

	for _, ipStr := range blockedCases {
		ip := net.ParseIP(ipStr)
		if !isBlockedIP(ip) {
			t.Errorf("Expected IP %s to be blocked, but was allowed", ipStr)
		}
	}

	allowedCases := []string{
		"8.8.8.8",
		"1.1.1.1",
		"93.184.216.34", // example.com
	}

	for _, ipStr := range allowedCases {
		ip := net.ParseIP(ipStr)
		if isBlockedIP(ip) {
			t.Errorf("Expected public IP %s to be allowed, but was blocked", ipStr)
		}
	}
}

func TestAcceptLanguageHeader(t *testing.T) {
	var gotLang, gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLang = r.Header.Get("Accept-Language")
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	if _, err := client.Fetch(context.Background(), ts.URL); err != nil {
		t.Fatal(err)
	}
	if gotLang != DefaultAcceptLanguage {
		t.Errorf("default Accept-Language should be %q, got %q", DefaultAcceptLanguage, gotLang)
	}
	if gotUA != DefaultUserAgent {
		t.Errorf("default User-Agent should be %q, got %q", DefaultUserAgent, gotUA)
	}

	client = NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true, AcceptLanguage: "ja,en;q=0.8"})
	if _, err := client.Fetch(context.Background(), ts.URL); err != nil {
		t.Fatal(err)
	}
	if gotLang != "ja,en;q=0.8" {
		t.Errorf("Accept-Language override failed, got %q", gotLang)
	}
}

func TestRedirectChainTracking(t *testing.T) {
	// Setup a mock server that simulates 301 and 302 hops
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first":
			http.Redirect(w, r, ts.URL+"/second", http.StatusMovedPermanently)
		case "/second":
			http.Redirect(w, r, ts.URL+"/final", http.StatusFound)
		case "/final":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html><body>Target</body></html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Allow loopback for this test
	client := NewSafeClient(ClientOptions{
		Timeout:         5 * time.Second,
		AllowPrivateIPs: true,
	})

	res, err := client.Fetch(context.Background(), ts.URL+"/first")
	if err != nil {
		t.Fatalf("Unexpected fetch error: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", res.StatusCode)
	}

	if len(res.Redirects) != 2 {
		t.Fatalf("Expected 2 redirect hops, got %d", len(res.Redirects))
	}

	if res.Redirects[0].StatusCode != http.StatusMovedPermanently {
		t.Errorf("Expected hop 1 to be 301, got %d", res.Redirects[0].StatusCode)
	}

	if res.Redirects[1].StatusCode != http.StatusFound {
		t.Errorf("Expected hop 2 to be 302, got %d", res.Redirects[1].StatusCode)
	}

	if res.FinalURL != ts.URL+"/final" {
		t.Errorf("Expected final URL %s, got %s", ts.URL+"/final", res.FinalURL)
	}
}

func TestCharsetDecodingShiftJIS(t *testing.T) {
	const title = "こんにちは"
	utf8Body := `<html><head><title>` + title + `</title></head><body>本文</body></html>`
	sjisBody, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(utf8Body))
	if err != nil {
		t.Fatalf("failed encoding fixture as Shift_JIS: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=Shift_JIS")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(sjisBody)
	}))
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	res, err := client.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	if res.Charset != "shift_jis" {
		t.Errorf("expected detected charset %q, got %q", "shift_jis", res.Charset)
	}
	if !strings.Contains(string(res.Body), title) {
		t.Errorf("expected decoded body to contain %q, got:\n%s", title, res.Body)
	}
}

func TestCharsetDecodingXMLSitemapEUCJP(t *testing.T) {
	utf8XML := `<?xml version="1.0" encoding="EUC-JP"?>` + "\n" +
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://example.jp/物件/1</loc></url></urlset>`
	eucBody, err := japanese.EUCJP.NewEncoder().Bytes([]byte(utf8XML))
	if err != nil {
		t.Fatalf("failed encoding fixture as EUC-JP: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(eucBody)
	}))
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	res, err := client.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	if res.Charset != "euc-jp" {
		t.Errorf("expected detected charset %q, got %q", "euc-jp", res.Charset)
	}
	if !strings.Contains(string(res.Body), "物件") {
		t.Errorf("expected decoded XML body to contain %q, got:\n%s", "物件", res.Body)
	}
}

func TestBodyTruncation(t *testing.T) {
	const capBytes = 1024
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(make([]byte, capBytes*3))
	}))
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true, MaxBodyBytes: capBytes})
	res, err := client.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Error("expected Truncated=true when body exceeds MaxBodyBytes")
	}
	if res.BodySize != capBytes {
		t.Errorf("expected body capped at %d bytes, got %d", capBytes, res.BodySize)
	}

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(make([]byte, capBytes))
	}))
	defer ts2.Close()

	res2, err := client.Fetch(context.Background(), ts2.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Truncated {
		t.Error("expected Truncated=false when body exactly equals MaxBodyBytes")
	}
	if res2.BodySize != capBytes {
		t.Errorf("expected body of exactly %d bytes, got %d", capBytes, res2.BodySize)
	}
}

func TestSSRFBlockedTargetViaInjectedResolver(t *testing.T) {
	client := NewSafeClient(ClientOptions{
		Timeout: 2 * time.Second,
		Resolver: func(_ context.Context, host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("10.1.2.3")}, nil
		},
	})

	_, err := client.Fetch(context.Background(), "http://internal.invalid.test/")
	if err == nil {
		t.Fatal("expected fetch to a resolver-supplied private IP to be blocked")
	}
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Errorf("expected ErrSSRFBlocked, got %v", err)
	}
}

func TestSSRFProxiedRequestToPrivateTargetRefused(t *testing.T) {
	client := NewSafeClient(ClientOptions{
		Timeout: 2 * time.Second,
		Resolver: func(_ context.Context, host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("192.168.1.50")}, nil
		},
		// Simulate an operator-configured proxy for every request. The proxy
		// address itself (loopback) must not be blocked, but the real target
		// (resolved above to a private IP) must be.
		ProxyFunc: func(req *http.Request) (*url.URL, error) {
			return url.Parse("http://127.0.0.1:9999")
		},
	})

	_, err := client.Fetch(context.Background(), "http://internal.invalid.test/")
	if err == nil {
		t.Fatal("expected proxied fetch to a private target to be refused")
	}
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Errorf("expected ErrSSRFBlocked, got %v", err)
	}
}

func TestSSRFResolverConsultedExactlyOncePerHop(t *testing.T) {
	// Regression guard for the DNS-rebinding TOCTOU: the DialContext dial
	// target must come from the same resolution used to validate it, not a
	// second, uncontrolled lookup (which is what dialing by hostname again
	// would trigger). A resolver that blocks everything after the first
	// call proves only one lookup happens for a single-hop request.
	calls := 0
	client := NewSafeClient(ClientOptions{
		Timeout: 2 * time.Second,
		Resolver: func(_ context.Context, host string) ([]net.IP, error) {
			calls++
			if calls > 1 {
				t.Errorf("unexpected extra DNS resolution (rebinding window) on call %d", calls)
			}
			return []net.IP{net.ParseIP("10.9.9.9")}, nil
		},
	})

	_, err := client.Fetch(context.Background(), "http://rebind.invalid.test/")
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("expected ErrSSRFBlocked, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 DNS resolution, got %d", calls)
	}
}
