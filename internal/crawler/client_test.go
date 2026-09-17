package crawler

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
