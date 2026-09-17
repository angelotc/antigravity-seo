// Package api implements optional key-based integrations (Google PageSpeed,
// CrUX, IndexNow). Every client degrades gracefully when credentials are
// absent so the engine core stays 100% keyless.
package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrNoAPIKey is returned when an optional integration has no credentials
var ErrNoAPIKey = errors.New("API key not configured")

// Endpoints are package vars so tests can redirect them to local servers
var (
	PSIEndpoint       = "https://www.googleapis.com/pagespeedonline/v5/runPagespeed"
	CrUXEndpoint      = "https://chromeuxreport.googleapis.com/v1/records:queryRecord"
	CrUXHistoryEndpoint = "https://chromeuxreport.googleapis.com/v1/records:queryHistoryRecord"
	IndexNowEndpoint  = "https://api.indexnow.org/indexnow"
)

var httpClient = &http.Client{Timeout: 90 * time.Second}

// FieldMetric is one CrUX field-data metric
type FieldMetric struct {
	Name       string  `json:"name"`
	Percentile int64   `json:"percentile"` // ms for durations, score*100 for CLS
	Category   string  `json:"category"`   // FAST/SLOW/AVERAGE or GOOD/NEEDS-IMPROVEMENT/POOR
}

// LabMetric is one Lighthouse lab metric
type LabMetric struct {
	Name         string  `json:"name"`
	NumericValue float64 `json:"numeric_value"`
	DisplayValue string  `json:"display_value"`
}

// PSIReport condenses a PageSpeed Insights run
type PSIReport struct {
	URL            string             `json:"url"`
	Strategy       string             `json:"strategy"`
	FieldOverall   string             `json:"field_overall_category,omitempty"`
	FieldMetrics   []FieldMetric      `json:"field_metrics,omitempty"`
	LabScores     map[string]int     `json:"lab_scores,omitempty"` // category name -> 0-100
	LabMetrics    []LabMetric        `json:"lab_metrics,omitempty"`
}

// metricUnits describes how to interpret each CrUX metric key
var psiMetricMeta = map[string]struct{ name, unit string }{
	"LARGEST_CONTENTFUL_PAINT_MS":   {"LCP", "ms"},
	"INTERACTION_TO_NEXT_PAINT_MS":  {"INP", "ms"},
	"FIRST_CONTENTFUL_PAINT_MS":     {"FCP", "ms"},
	"CUMULATIVE_LAYOUT_SHIFT_SCORE": {"CLS", "score"},
	"TIME_TO_FIRST_BYTE_MS":         {"TTFB", "ms"},
}

type psiAPIResponse struct {
	LoadingExperience struct {
		OverallCategory string `json:"overall_category"`
		Metrics         map[string]struct {
			Percentile int64  `json:"percentile"`
			Category   string `json:"category"`
		} `json:"metrics"`
	} `json:"loadingExperience"`
	LighthouseResult struct {
		Categories map[string]struct {
			Score *float64 `json:"score"`
		} `json:"categories"`
		Audits map[string]struct {
			NumericValue float64 `json:"numericValue"`
			DisplayValue string  `json:"displayValue"`
		} `json:"audits"`
	} `json:"lighthouseResult"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// RunPSI queries PageSpeed Insights (lab + CrUX field data) for a URL.
func RunPSI(ctx context.Context, targetURL, strategy, apiKey string) (*PSIReport, error) {
	if apiKey == "" {
		return nil, ErrNoAPIKey
	}
	if strategy == "" {
		strategy = "mobile"
	}

	q := url.Values{}
	q.Set("url", targetURL)
	q.Set("strategy", strategy)
	q.Set("key", apiKey)
	q.Set("category", "performance")
	q.Set("category", "accessibility")
	q.Set("category", "best-practices")
	q.Set("category", "seo")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, PSIEndpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PSI API returned %d: %s", resp.StatusCode, truncateFor(string(body), 200))
	}

	var apiResp psiAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("invalid PSI response JSON: %w", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("PSI API error: %s", apiResp.Error.Message)
	}

	report := &PSIReport{
		URL:          targetURL,
		Strategy:     strategy,
		FieldOverall: apiResp.LoadingExperience.OverallCategory,
		FieldMetrics: []FieldMetric{},
		LabMetrics:   []LabMetric{},
	}

	for key, m := range apiResp.LoadingExperience.Metrics {
		if meta, ok := psiMetricMeta[key]; ok {
			report.FieldMetrics = append(report.FieldMetrics, FieldMetric{
				Name:       meta.name + " (" + meta.unit + ")",
				Percentile: m.Percentile,
				Category:   m.Category,
			})
		}
	}

	report.LabScores = map[string]int{}
	for cat, c := range apiResp.LighthouseResult.Categories {
		if c.Score != nil {
			report.LabScores[cat] = int(*c.Score * 100)
		}
	}
	for _, auditKey := range []string{
		"first-contentful-paint", "largest-contentful-paint", "total-blocking-time",
		"cumulative-layout-shift", "speed-index", "interactive",
	} {
		if a, ok := apiResp.LighthouseResult.Audits[auditKey]; ok {
			report.LabMetrics = append(report.LabMetrics, LabMetric{
				Name:         auditKey,
				NumericValue: a.NumericValue,
				DisplayValue: a.DisplayValue,
			})
		}
	}

	return report, nil
}

// CrUXMetric is one CrUX record metric p75
type CrUXMetric struct {
	Name string `json:"name"`
	P75  string `json:"p75"`
	Unit string `json:"unit"`
}

// CrUXReport condenses a CrUX record (current or 25-week history)
type CrUXReport struct {
	OriginOrURL string       `json:"origin_or_url"`
	FormFactor  string       `json:"form_factor,omitempty"`
	Metrics     []CrUXMetric `json:"metrics"`
	History     [][]string   `json:"history,omitempty"` // per metric: 25 weekly p75 values
}

type cruxAPIResponse struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Record struct {
		Metrics map[string]json.RawMessage `json:"metrics"`
	} `json:"record"`
}

var cruxMetricMeta = map[string]struct{ name, unit string }{
	"largest_contentful_paint": {"LCP", "ms"},
	"interaction_to_next_paint": {"INP", "ms"},
	"cumulative_layout_shift":   {"CLS", "score"},
	"first_contentful_paint":    {"FCP", "ms"},
	"experimental_time_to_first_byte": {"TTFB", "ms"},
}

// RunCrUX queries the Chrome UX Report API. When history is true, the
// 25-week p75 time series is returned instead of the current record.
func RunCrUX(ctx context.Context, targetURL, formFactor, apiKey string, history bool) (*CrUXReport, error) {
	if apiKey == "" {
		return nil, ErrNoAPIKey
	}

	endpoint := CrUXEndpoint
	if history {
		endpoint = CrUXHistoryEndpoint
	}

	payload := map[string]interface{}{}
	if isOrigin(targetURL) {
		payload["origin"] = targetURL
	} else {
		payload["url"] = targetURL
	}
	if formFactor != "" {
		payload["formFactor"] = formFactor
	}
	raw, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"?key="+url.QueryEscape(apiKey), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CrUX API returned %d: %s", resp.StatusCode, truncateFor(string(body), 200))
	}

	var apiResp cruxAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("invalid CrUX response JSON: %w", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("CrUX API error: %s", apiResp.Error.Message)
	}

	report := &CrUXReport{
		OriginOrURL: targetURL,
		FormFactor:  formFactor,
		Metrics:     []CrUXMetric{},
	}
	for key, rawMetric := range apiResp.Record.Metrics {
		meta, ok := cruxMetricMeta[key]
		if !ok {
			continue
		}
		if history {
			var ts struct {
				TimeSeries []struct {
					P75s []string `json:"p75s"`
				} `json:"timeSeries"`
			}
			if err := json.Unmarshal(rawMetric, &ts); err == nil && len(ts.TimeSeries) > 0 {
				report.History = append(report.History, ts.TimeSeries[0].P75s)
			}
		} else {
			var rec struct {
				Percentiles []struct {
					P75 string `json:"p75"`
				} `json:"percentiles"`
			}
			if err := json.Unmarshal(rawMetric, &rec); err == nil && len(rec.Percentiles) > 0 {
				report.Metrics = append(report.Metrics, CrUXMetric{
					Name: meta.name, P75: rec.Percentiles[0].P75, Unit: meta.unit,
				})
			}
		}
	}
	return report, nil
}

// IndexNowReport is the outcome of an IndexNow submission
type IndexNowReport struct {
	Host        string   `json:"host"`
	Key         string   `json:"key"`
	URLs        []string `json:"urls"`
	Status      int      `json:"status"`
	StatusText  string   `json:"status_text"`
	Accepted    bool     `json:"accepted"`
}

// GenerateIndexNowKey creates a random 32-hex-char key
func GenerateIndexNowKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}

// SubmitIndexNow submits URLs to the IndexNow protocol (Bing, Yandex,
// Seznam, Naver). key must also be served at keyLocation.
func SubmitIndexNow(ctx context.Context, urls []string, key, keyLocation string) (*IndexNowReport, error) {
	if key == "" {
		return nil, ErrNoAPIKey
	}
	if len(urls) == 0 {
		return nil, errors.New("no URLs to submit")
	}

	host, err := extractHost(urls[0])
	if err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"host":    host,
		"key":     key,
		"urlList": urls,
	}
	if keyLocation != "" {
		payload["keyLocation"] = keyLocation
	}
	raw, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, IndexNowEndpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	report := &IndexNowReport{
		Host:       host,
		Key:        key,
		URLs:       urls,
		Status:     resp.StatusCode,
		StatusText: resp.Status,
	}
	switch resp.StatusCode {
	case 200:
		report.Accepted = true
		report.StatusText = "OK — URLs submitted and key verified"
	case 202:
		report.Accepted = true
		report.StatusText = "Accepted — key not verified yet, will re-check before indexing"
	default:
		report.StatusText = fmt.Sprintf("Rejected (%d) — validate key file at keyLocation", resp.StatusCode)
	}
	return report, nil
}

func isOrigin(u string) bool {
	parsed, err := url.Parse(u)
	return err == nil && parsed.Path == "" && parsed.RawQuery == ""
}

func extractHost(u string) (string, error) {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("cannot determine host from %q", u)
	}
	return parsed.Hostname(), nil
}

func truncateFor(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
