package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestGSCTokenMissing(t *testing.T) {
	// Ensure env vars are unset
	os.Unsetenv("GSC_ACCESS_TOKEN")
	os.Unsetenv("GOOGLE_ACCESS_TOKEN")
	os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
	os.Unsetenv("GSC_CREDENTIALS_FILE")
	os.Unsetenv("GSC_CREDENTIALS_JSON")

	_, err := ResolveGSCToken(context.Background())
	if err != ErrNoGSCAuth {
		t.Fatalf("expected ErrNoGSCAuth, got %v", err)
	}

	_, err = QuerySearchAnalytics(context.Background(), "", GSCQueryOptions{SiteURL: "https://example.com"})
	if err != ErrNoGSCAuth {
		t.Fatalf("expected ErrNoGSCAuth on empty token, got %v", err)
	}

	_, err = InspectURL(context.Background(), "", GSCInspectOptions{SiteURL: "https://example.com", InspectionURL: "https://example.com/page"})
	if err != ErrNoGSCAuth {
		t.Fatalf("expected ErrNoGSCAuth on empty token, got %v", err)
	}
}

func TestGSCSearchAnalytics(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-gsc-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		resp := map[string]interface{}{
			"rows": []map[string]interface{}{
				{
					"keys":        []string{"best tokyo homes", "https://example.com/tokyo"},
					"clicks":      42.0,
					"impressions": 1200.0,
					"ctr":         0.035,
					"position":    4.2,
				},
				{
					"keys":        []string{"japan real estate guide", "https://example.com/guide"},
					"clicks":      18.0,
					"impressions": 650.0,
					"ctr":         0.027,
					"position":    6.8,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	orig := GSCSearchAnalyticsEndpoint
	GSCSearchAnalyticsEndpoint = ts.URL
	defer func() { GSCSearchAnalyticsEndpoint = orig }()

	report, err := QuerySearchAnalytics(context.Background(), "test-gsc-token", GSCQueryOptions{
		SiteURL:    "https://example.com",
		StartDate:  "2026-08-01",
		EndDate:    "2026-08-28",
		Dimensions: []string{"query", "page"},
		RowLimit:   10,
	})
	if err != nil {
		t.Fatalf("QuerySearchAnalytics failed: %v", err)
	}

	if report.TotalRows != 2 {
		t.Fatalf("expected 2 rows, got %d", report.TotalRows)
	}
	if report.Rows[0].Clicks != 42.0 {
		t.Errorf("expected 42 clicks, got %v", report.Rows[0].Clicks)
	}
	if report.Rows[0].Keys[0] != "best tokyo homes" {
		t.Errorf("expected 'best tokyo homes', got %s", report.Rows[0].Keys[0])
	}
}

func TestGSCInspectURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-gsc-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		resp := map[string]interface{}{
			"inspectionResult": map[string]interface{}{
				"verdict": "PASS",
				"indexStatusResult": map[string]interface{}{
					"verdict":         "PASS",
					"coverageState":   "Submitted and indexed",
					"robotsTxtState":  "ALLOWED",
					"indexingState":   "INDEXING_ALLOWED",
					"lastCrawlTime":   "2026-09-15T12:00:00Z",
					"pageFetchState":  "SUCCESSFUL",
					"googleCanonical": "https://example.com/tokyo",
					"userCanonical":   "https://example.com/tokyo",
					"referringUrls":   []string{"https://example.com/", "https://example.com/sitemap.xml"},
				},
				"mobileUsabilityResult": map[string]interface{}{
					"verdict": "PASS",
				},
				"richResultsResult": map[string]interface{}{
					"verdict": "PASS",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	orig := GSCInspectEndpoint
	GSCInspectEndpoint = ts.URL
	defer func() { GSCInspectEndpoint = orig }()

	report, err := InspectURL(context.Background(), "test-gsc-token", GSCInspectOptions{
		SiteURL:       "https://example.com",
		InspectionURL: "https://example.com/tokyo",
	})
	if err != nil {
		t.Fatalf("InspectURL failed: %v", err)
	}

	if report.Verdict != "PASS" {
		t.Errorf("expected verdict PASS, got %s", report.Verdict)
	}
	if report.CoverageState != "Submitted and indexed" {
		t.Errorf("expected 'Submitted and indexed', got %s", report.CoverageState)
	}
	if report.GoogleCanonical != "https://example.com/tokyo" {
		t.Errorf("expected googleCanonical 'https://example.com/tokyo', got %s", report.GoogleCanonical)
	}
	if len(report.ReferringURLs) != 2 {
		t.Errorf("expected 2 referring URLs, got %d", len(report.ReferringURLs))
	}
}

func TestGSCServiceAccountJWT(t *testing.T) {
	// Generate an in-memory RSA key
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(rsaKey)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.FormValue("assertion") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"mock-service-account-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	orig := GSCOAuthTokenEndpoint
	GSCOAuthTokenEndpoint = tokenServer.URL
	defer func() { GSCOAuthTokenEndpoint = orig }()

	saData := ServiceAccountKey{
		Type:        "service_account",
		ProjectID:   "test-project",
		ClientEmail: "sa@test-project.iam.gserviceaccount.com",
		PrivateKey:  string(pemBytes),
		TokenURI:    tokenServer.URL,
	}
	saJSON, _ := json.Marshal(saData)

	token, err := ExchangeServiceAccountJWT(context.Background(), saJSON, GSCReadonlyScope)
	if err != nil {
		t.Fatalf("ExchangeServiceAccountJWT failed: %v", err)
	}
	if token != "mock-service-account-token" {
		t.Errorf("expected 'mock-service-account-token', got %s", token)
	}
}
