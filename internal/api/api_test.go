package api

import (
	"context"
	"encoding/json"
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

func TestPSISendsAllCategories(t *testing.T) {
	var got []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()["category"]
		_, _ = w.Write([]byte(`{"lighthouseResult":{"categories":{
			"performance":{"score":0.9},"accessibility":{"score":0.8},
			"best-practices":{"score":0.7},"seo":{"score":1}}}}`))
	}))
	defer ts.Close()
	old := PSIEndpoint
	PSIEndpoint = ts.URL
	defer func() { PSIEndpoint = old }()

	report, err := RunPSI(context.Background(), "https://example.com", "mobile", "k")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"performance", "accessibility", "best-practices", "seo"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("category params = %v, want %v", got, want)
	}
	if len(report.LabScores) != 4 || report.LabScores["accessibility"] != 80 || report.LabScores["seo"] != 100 {
		t.Errorf("expected all four lab scores, got %v", report.LabScores)
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
		_, _ = w.Write([]byte(`{"record":{"key":{"origin":"https://example.com"},"metrics":{
			"largest_contentful_paint":{"histogramTimeseries":[],"percentilesTimeseries":{"p75s":[2500,null,2300]}},
			"cumulative_layout_shift":{"percentilesTimeseries":{"p75s":["0.05",null,"0.04"]}},
			"interaction_to_next_paint":{"percentilesTimeseries":{"p75s":[150,140,130]}}},
			"collectionPeriods":[{"firstDate":{"year":2026,"month":8,"day":1},"lastDate":{"year":2026,"month":8,"day":7}}]}}`))
	}))
	defer ts.Close()
	old := CrUXHistoryEndpoint
	CrUXHistoryEndpoint = ts.URL
	defer func() { CrUXHistoryEndpoint = old }()

	report, err := RunCrUX(context.Background(), "https://example.com", "", "k", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.History) != 3 {
		t.Fatalf("history parse failed: %+v", report)
	}
	// Sorted by metric name: CLS, INP, LCP.
	if report.History[0].Metric != "CLS" || report.History[1].Metric != "INP" || report.History[2].Metric != "LCP" {
		t.Errorf("expected history sorted by metric name, got %+v", report.History)
	}
	cls, lcp := report.History[0].P75s, report.History[2].P75s
	if len(cls) != 3 || cls[0] == nil || *cls[0] != "0.05" || cls[1] != nil || *cls[2] != "0.04" {
		t.Errorf("CLS string p75s with null not decoded: %v", cls)
	}
	if len(lcp) != 3 || *lcp[0] != "2500" || lcp[1] != nil || *lcp[2] != "2300" {
		t.Errorf("LCP numeric p75s with null not decoded: %v", lcp)
	}
	out, _ := json.Marshal(report.History[2])
	if !strings.Contains(string(out), `"p75s":["2500",null,"2300"]`) {
		t.Errorf("ineligible period should serialize as null, got %s", out)
	}
	if len(report.CollectionPeriods) != 1 || report.CollectionPeriods[0].FirstDate.Year != 2026 {
		t.Errorf("expected collection periods to be parsed, got %+v", report.CollectionPeriods)
	}
}

func TestCrUXCurrent(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"record":{"key":{"url":"https://example.com/page"},"metrics":{
			"interaction_to_next_paint":{"histogram":[],"percentiles":{"p75":180}},
			"largest_contentful_paint":{"percentiles":{"p75":2500.0}},
			"cumulative_layout_shift":{"histogram":[],"percentiles":{"p75":"0.05"}}}}}`))
	}))
	defer ts.Close()
	old := CrUXEndpoint
	CrUXEndpoint = ts.URL
	defer func() { CrUXEndpoint = old }()

	report, err := RunCrUX(context.Background(), "https://example.com/page", "PHONE", "k", false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, m := range report.Metrics {
		got[m.Name] = m.P75
	}
	if len(got) != 3 || got["INP"] != "180" || got["LCP"] != "2500" || got["CLS"] != "0.05" {
		t.Errorf("metric parse failed: %+v", report.Metrics)
	}
}

func TestCrUXOriginPayload(t *testing.T) {
	var body map[string]interface{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"record":{"metrics":{}}}`))
	}))
	defer ts.Close()
	old := CrUXEndpoint
	CrUXEndpoint = ts.URL
	defer func() { CrUXEndpoint = old }()

	if _, err := RunCrUX(context.Background(), "https://example.com/", "", "k", false); err != nil {
		t.Fatal(err)
	}
	if body["origin"] != "https://example.com" || body["url"] != nil {
		t.Errorf("expected origin without trailing slash, got %v", body)
	}
}

func TestCrUXNoData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":404,"message":"chrome ux report data not found","status":"NOT_FOUND"}}`, http.StatusNotFound)
	}))
	defer ts.Close()
	old := CrUXEndpoint
	CrUXEndpoint = ts.URL
	defer func() { CrUXEndpoint = old }()

	report, err := RunCrUX(context.Background(), "https://example.com", "", "k", false)
	if err != nil {
		t.Fatalf("404 must not be an error: %v", err)
	}
	if !report.NoData || len(report.Metrics) != 0 {
		t.Errorf("expected NoData with no metrics, got %+v", report)
	}
}

func TestIsOrigin(t *testing.T) {
	cases := map[string]bool{
		"https://example.com":      true,
		"https://example.com/":     true,
		"https://example.com/page": false,
		"https://example.com/?a=b": false,
		"https://example.com/a/b/": false,
	}
	for u, want := range cases {
		if got := isOrigin(u); got != want {
			t.Errorf("isOrigin(%q) = %v, want %v", u, got, want)
		}
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

func TestAPIErrorMessage(t *testing.T) {
	body := []byte("{\n  \"error\": {\n    \"code\": 400,\n    \"message\": \"API key not valid.\\nPlease pass a valid API key.\"\n  }\n}")
	if got := apiErrorMessage(body); got != "API key not valid. Please pass a valid API key." {
		t.Errorf("got %q", got)
	}
	if got := apiErrorMessage([]byte("<html>\n  oops\n</html>")); got != "<html> oops </html>" {
		t.Errorf("fallback got %q", got)
	}
}
