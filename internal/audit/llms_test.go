package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"antigravity-seo/internal/crawler"
)

func TestInspectLLMSTxt(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/llms.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("# Example Docs\n\n## Pages\n\n- [Home](https://example.com/): landing\n"))
	})
	mux.HandleFunc("/llms-full.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("# Example Full\n\nEverything."))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := crawler.NewSafeClient(crawler.ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := InspectLLMSTxt(context.Background(), client, ts.URL+"/docs")
	if err != nil {
		t.Fatal(err)
	}
	if !report.LLMSTxtExists || !report.FullTxtExists {
		t.Errorf("both llms files should exist: %+v", report)
	}
	if report.LLMSTxtLinks != 1 {
		t.Errorf("expected 1 markdown link, got %d", report.LLMSTxtLinks)
	}
	if report.Score != 100 {
		t.Errorf("healthy llms.txt should score 100, got %d", report.Score)
	}
}

func TestInspectLLMSTxtMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := crawler.NewSafeClient(crawler.ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := InspectLLMSTxt(context.Background(), client, ts.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if report.LLMSTxtExists || report.Score != 0 {
		t.Errorf("missing llms.txt should score 0: %+v", report)
	}
}
