package drift

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"antigravity-seo/internal/crawler"
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

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
