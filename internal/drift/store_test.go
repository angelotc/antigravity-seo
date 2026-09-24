package drift

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"antigravity-seo/internal/crawler"
	"antigravity-seo/internal/runtime"
)

func testClient(t *testing.T) *crawler.SafeClient {
	t.Helper()
	return crawler.NewSafeClient(crawler.ClientOptions{
		Timeout:         5 * time.Second,
		AllowPrivateIPs: true,
	})
}

func TestBaselineCompareHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ANTIGRAVITY_SEO_DATA_DIR", dir)

	title := "Original Title"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title>" + title + "</title><meta name='description' content='d'></head><body><h1>H</h1>" +
			"<script type=\"application/ld+json\">{\"@context\":\"https://schema.org\",\"@type\":\"Organization\",\"name\":\"X\"}</script>" +
			"<p>" + repeat("word ", 80) + "</p></body></html>"))
	}))
	defer ts.Close()

	client := testClient(t)
	res, err := client.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	snap, path, err := Baseline(ts.URL, res)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}
	if snap.Title != "Original Title" || snap.WordCount < 80 || len(snap.SchemaTypes) != 1 {
		t.Errorf("snapshot fields wrong: %+v", snap)
	}

	history, err := History(ts.URL)
	if err != nil || len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d (err %v)", len(history), err)
	}

	// Mutate the page and compare
	title = "New Title"
	res2, err := client.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Compare(ts.URL, res2)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed {
		t.Fatal("expected changed report after title mutation")
	}
	foundTitle := false
	for _, c := range report.Changes {
		if c.Field == "title" && c.Before == "Original Title" && c.After == "New Title" {
			foundTitle = true
		}
	}
	if !foundTitle {
		t.Errorf("title change not in diff: %+v", report.Changes)
	}

	history, _ = History(ts.URL)
	if len(history) != 1 {
		t.Errorf("history should list stored snapshots only (compare must not save), got %d", len(history))
	}
}

func TestLatestWithoutBaseline(t *testing.T) {
	t.Setenv("ANTIGRAVITY_SEO_DATA_DIR", t.TempDir())
	if _, err := Latest("https://never-baselined.example.com"); err == nil {
		t.Error("expected error when no baseline exists")
	}
}

func TestURLKeyCaseSensitivePaths(t *testing.T) {
	if urlKey("https://example.com/About") == urlKey("https://example.com/about") {
		t.Error("expected /About and /about to produce different drift keys (urlnorm.Key preserves path case)")
	}
}

func TestHistoryLegacyKeyFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ANTIGRAVITY_SEO_DATA_DIR", dir)

	rawURL := "https://example.com/SomePage"

	// The legacy key scheme lowercased the whole URL, so it collapses
	// /SomePage to /somepage; the current urlnorm.Key scheme preserves path
	// case, so the two keys must differ for this fixture to be meaningful.
	if urlKey(rawURL) == legacyURLKey(rawURL) {
		t.Fatal("test fixture invalid: new and legacy keys collide for this URL")
	}

	if err := os.MkdirAll(runtime.DriftDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	snap := Snapshot{URL: rawURL, Timestamp: time.Now().UTC(), Title: "Legacy Snapshot"}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(runtime.DriftDir(), fmt.Sprintf("%s-%d.json", legacyURLKey(rawURL), snap.Timestamp.Unix()))
	if err := os.WriteFile(legacyPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	history, err := History(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Title != "Legacy Snapshot" {
		t.Fatalf("expected legacy-keyed snapshot to be found via fallback, got %+v", history)
	}
}

func TestCompareTTFBNoiseDoesNotFlagChanged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ANTIGRAVITY_SEO_DATA_DIR", dir)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title>T</title></head><body><h1>H</h1></body></html>"))
	}))
	defer ts.Close()

	client := testClient(t)
	res, err := client.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Baseline(ts.URL, res); err != nil {
		t.Fatal(err)
	}

	res2, err := client.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Compare(ts.URL, res2)
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed {
		t.Errorf("expected small local TTFB jitter alone not to flag Changed: %+v", report.Changes)
	}
}

func TestCompareTTFBLargeJumpFlagsChanged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ANTIGRAVITY_SEO_DATA_DIR", dir)

	slow := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slow {
			time.Sleep(700 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title>T</title></head><body><h1>H</h1></body></html>"))
	}))
	defer ts.Close()

	client := testClient(t)
	res, err := client.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Baseline(ts.URL, res); err != nil {
		t.Fatal(err)
	}

	slow = true
	res2, err := client.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Compare(ts.URL, res2)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed {
		t.Errorf("expected a >500ms, >50%% TTFB jump to flag Changed: %+v", report.Changes)
	}
	found := false
	for _, c := range report.Changes {
		if c.Field == "ttfb_ms" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ttfb_ms diff to still be reported informationally: %+v", report.Changes)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
