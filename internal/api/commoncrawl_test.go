package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommonCrawlLatestIndex(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id": "CC-MAIN-2026-34", "name": "August 2026 Index", "cdx-api": "https://index.commoncrawl.org/CC-MAIN-2026-34-index"}
		]`))
	}))
	defer ts.Close()

	orig := CCCollinfoEndpoint
	CCCollinfoEndpoint = ts.URL
	defer func() { CCCollinfoEndpoint = orig }()

	idx, err := GetLatestCrawlIndex(context.Background())
	if err != nil {
		t.Fatalf("GetLatestCrawlIndex failed: %v", err)
	}
	if idx != "CC-MAIN-2026-34" {
		t.Errorf("expected 'CC-MAIN-2026-34', got %s", idx)
	}
}

func TestQueryCommonCrawlBacklinks(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(`{"urlkey":"com,example)/","timestamp":"20260810","url":"https://example.com/","mime":"text/html","status":"200","digest":"ABC","languages":"eng"}
{"urlkey":"com,example)/blog","timestamp":"20260811","url":"https://example.com/blog","mime":"text/html","status":"200","digest":"DEF","languages":"eng,jpn"}
`))
	}))
	defer ts.Close()

	orig := CCIndexBaseURL
	CCIndexBaseURL = ts.URL
	defer func() { CCIndexBaseURL = orig }()

	report, err := QueryCommonCrawlBacklinks(context.Background(), "example.com", 10, "CC-TEST")
	if err != nil {
		t.Fatalf("QueryCommonCrawlBacklinks failed: %v", err)
	}

	if report.TotalFound != 2 {
		t.Errorf("expected 2 records, got %d", report.TotalFound)
	}
	if report.UniqueURLs != 2 {
		t.Errorf("expected 2 unique URLs, got %d", report.UniqueURLs)
	}
	if report.MimeBreakdown["text/html"] != 2 {
		t.Errorf("expected 2 text/html, got %d", report.MimeBreakdown["text/html"])
	}
	if report.Languages["eng"] != 2 || report.Languages["jpn"] != 1 {
		t.Errorf("unexpected language breakdown: %v", report.Languages)
	}
}

func TestQueryCommonCrawl404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	orig := CCIndexBaseURL
	CCIndexBaseURL = ts.URL
	defer func() { CCIndexBaseURL = orig }()

	report, err := QueryCommonCrawlBacklinks(context.Background(), "nonexistent-domain.xyz", 10, "CC-TEST")
	if err != nil {
		t.Fatalf("expected nil error on 404, got %v", err)
	}
	if report.TotalFound != 0 {
		t.Errorf("expected 0 records on 404, got %d", report.TotalFound)
	}
}
