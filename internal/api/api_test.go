package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPSINoKey(t *testing.T) {
	if _, err := RunPSI(context.Background(), "https://example.com", "mobile", ""); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestPSISuccess(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-goog-api-key") != "k" || r.URL.Query().Get("strategy") != "mobile" {
			http.Error(w, "bad params", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("key") != "" {
			http.Error(w, "api key must not be sent as a query param", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"loadingExperience": {
				"overall_category": "GOOD",
				"metrics": {
					"LARGEST_CONTENTFUL_PAINT_MS": {"percentile": 1900, "category": "GOOD"},
					"INTERACTION_TO_NEXT_PAINT_MS": {"percentile": 150, "category": "GOOD"},
					"CUMULATIVE_LAYOUT_SHIFT_SCORE": {"percentile": 500, "category": "GOOD"}
				}
			},
			"lighthouseResult": {
				"categories": {"performance": {"score": 0.87}},
				"audits": {"first-contentful-paint": {"numericValue": 1200, "displayValue": "1.2 s"}}
			}
		}`))
	}))
	defer ts.Close()
	old := PSIEndpoint
	PSIEndpoint = ts.URL
	defer func() { PSIEndpoint = old }()

	report, err := RunPSI(context.Background(), "https://example.com", "mobile", "k")
	if err != nil {
		t.Fatal(err)
	}
	if report.FieldOverall != "GOOD" || len(report.FieldMetrics) != 3 {
		t.Errorf("field data parse failed: %+v", report)
	}
	if report.LabScores["performance"] != 87 {
		t.Errorf("performance score should be 87, got %d", report.LabScores["performance"])
	}
	if len(report.LabMetrics) != 1 {
		t.Errorf("lab metrics parse failed: %+v", report.LabMetrics)
	}
}

func TestCrUXHistory(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-goog-api-key") != "k" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}
		if strings.Contains(r.URL.RawQuery, "key=") {
			http.Error(w, "api key must not be sent as a query param", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"record":{"metrics":{"largest_contentful_paint":{"timeSeries":[{"p75s":["2500","2400","2300"]}]},"interaction_to_next_paint":{"timeSeries":[{"p75s":["150","140","130"]}]}},"collectionPeriods":[{"firstDate":{"year":2026,"month":8,"day":1},"lastDate":{"year":2026,"month":8,"day":7}}]}}`))
	}))
	defer ts.Close()
	old := CrUXHistoryEndpoint
	CrUXHistoryEndpoint = ts.URL
	defer func() { CrUXHistoryEndpoint = old }()

	report, err := RunCrUX(context.Background(), "https://example.com", "", "k", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.History) != 2 || len(report.History[0].P75s) != 3 {
		t.Errorf("history parse failed: %+v", report)
	}
	// Sorted by metric name: INP before LCP.
	if report.History[0].Metric != "INP" || report.History[1].Metric != "LCP" {
		t.Errorf("expected history sorted by metric name, got %+v", report.History)
	}
	if len(report.CollectionPeriods) != 1 || report.CollectionPeriods[0].FirstDate.Year != 2026 {
		t.Errorf("expected collection periods to be parsed, got %+v", report.CollectionPeriods)
	}
}

func TestCrUXCurrent(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"record":{"metrics":{"interaction_to_next_paint":{"percentiles":[{"p75":"180"}]}}}}`))
	}))
	defer ts.Close()
	old := CrUXEndpoint
	CrUXEndpoint = ts.URL
	defer func() { CrUXEndpoint = old }()

	report, err := RunCrUX(context.Background(), "https://example.com/page", "PHONE", "k", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Metrics) != 1 || report.Metrics[0].Name != "INP" || report.Metrics[0].P75 != "180" {
		t.Errorf("metric parse failed: %+v", report.Metrics)
	}
}

func TestIndexNowSubmit(t *testing.T) {
	var calls int32
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()
	old := IndexNowEndpoint
	IndexNowEndpoint = ts.URL
	defer func() { IndexNowEndpoint = old }()

	report, err := SubmitIndexNow(context.Background(), []string{"https://example.com/a", "https://example.com/b"}, "mykey", "")
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("expected 1 submission, got %d", calls)
	}
	if !report.Accepted || report.Host != "example.com" || len(report.URLs) != 2 {
		t.Errorf("unexpected report: %+v", report)
	}
}

func TestIndexNowNoKey(t *testing.T) {
	if _, err := SubmitIndexNow(context.Background(), []string{"https://example.com"}, "", ""); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestGenerateIndexNowKey(t *testing.T) {
	k1, err := GenerateIndexNowKey()
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := GenerateIndexNowKey()
	if len(k1) != 32 || k1 == k2 {
		t.Errorf("keys should be 32 hex chars and unique: %s %s", k1, k2)
	}
}
