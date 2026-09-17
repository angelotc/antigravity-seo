package report

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"antigravity-seo/internal/crawler"
)

var sampleHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Tokyo Real Estate Guide — Find Luxury Homes</title>
  <meta name="description" content="Explore comprehensive Tokyo real estate listings, pricing trends, and neighborhood guides for international buyers.">
  <link rel="canonical" href="https://example.com/tokyo">
  <script type="application/ld+json">
  {
    "@context": "https://schema.org",
    "@type": "Article",
    "headline": "Tokyo Real Estate Guide",
    "image": "https://example.com/cover.jpg",
    "author": { "@type": "Person", "name": "Kenji Sato" },
    "publisher": { "@type": "Organization", "name": "Tokyo Homes" },
    "datePublished": "2026-01-15"
  }
  </script>
</head>
<body>
  <h1>Tokyo Real Estate Guide</h1>
  <p class="author">By Kenji Sato on 2026-01-15</p>
  <h2>Neighborhood Overview</h2>
  <p>` + strings.Repeat("Tokyo real estate provides incredible architectural design and investment security. ", 30) + `</p>
  <img src="/img/shibuya.webp" alt="Shibuya Skyline" width="800" height="600">
  <img src="/img/roppongi.webp" alt="Roppongi Hills" width="800" height="600">
</body>
</html>`

func TestReportDataAndScoring(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(sampleHTML))
	}))
	defer ts.Close()

	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout:         5 * time.Second,
		AllowPrivateIPs: true,
	})
	data, err := BuildAuditData(context.Background(), client, ts.URL)
	if err != nil {
		t.Fatalf("BuildAuditData failed: %v", err)
	}

	if data.Scores.Overall < 60 {
		t.Errorf("expected overall score >= 60, got %d", data.Scores.Overall)
	}
	if data.Scores.Technical < 60 {
		t.Errorf("expected technical score >= 60, got %d", data.Scores.Technical)
	}
	if len(data.Issues) == 0 {
		t.Errorf("expected issues/passes to be recorded, got 0")
	}

	htmlContent, err := RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	if !strings.Contains(htmlContent, "Tokyo Real Estate Guide") {
		t.Errorf("expected HTML to contain headline")
	}
	if !strings.Contains(htmlContent, "score-circle") {
		t.Errorf("expected HTML to contain score-circle CSS class")
	}
}

func TestExportPDF(t *testing.T) {
	renderer := DetectPDFRenderer()
	if renderer == nil {
		t.Skip("No PDF renderer installed on host, skipping PDF test")
	}

	tmpDir := t.TempDir()
	outPDF := filepath.Join(tmpDir, "test_audit.pdf")

	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout:         5 * time.Second,
		AllowPrivateIPs: true,
	})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(sampleHTML))
	}))
	defer ts.Close()

	data, err := BuildAuditData(context.Background(), client, ts.URL)
	if err != nil {
		t.Fatalf("BuildAuditData failed: %v", err)
	}

	htmlContent, err := RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	info, err := ExportPDF(context.Background(), htmlContent, outPDF)
	if err != nil {
		t.Fatalf("ExportPDF failed with %s: %v", renderer.Type, err)
	}
	if info.Type != renderer.Type {
		t.Errorf("expected renderer %s, got %s", renderer.Type, info.Type)
	}

	stat, err := os.Stat(outPDF)
	if err != nil || stat.Size() == 0 {
		t.Fatalf("expected non-empty PDF file at %s", outPDF)
	}
}
