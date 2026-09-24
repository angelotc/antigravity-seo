package api

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	ErrNoGSCAuth = errors.New("Google Search Console authentication not configured")

	GSCSearchAnalyticsEndpoint = "https://www.googleapis.com/webmasters/v3/sites"
	GSCInspectEndpoint         = "https://searchconsole.googleapis.com/v1/urlInspection/index:inspect"
	GSCOAuthTokenEndpoint      = "https://oauth2.googleapis.com/token"
)

const GSCReadonlyScope = "https://www.googleapis.com/auth/webmasters.readonly"

// ServiceAccountKey matches Google Cloud IAM service account key JSON
type ServiceAccountKey struct {
	Type                    string `json:"type"`
	ProjectID               string `json:"project_id"`
	PrivateKeyID            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	AuthURI                 string `json:"auth_uri"`
	TokenURI                string `json:"token_uri"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
}

// GSCQueryOptions defines parameters for searchAnalytics:query
type GSCQueryOptions struct {
	SiteURL    string   `json:"site_url"`
	StartDate  string   `json:"start_date"` // YYYY-MM-DD
	EndDate    string   `json:"end_date"`   // YYYY-MM-DD
	Dimensions []string `json:"dimensions"` // "query", "page", "country", "device", "date"
	RowLimit   int      `json:"row_limit"`  // Default 25
	StartRow   int      `json:"start_row"`
}

// GSCQueryRow is one row of Search Analytics data
type GSCQueryRow struct {
	Keys        []string `json:"keys"`
	Clicks      float64  `json:"clicks"`
	Impressions float64  `json:"impressions"`
	CTR         float64  `json:"ctr"`
	Position    float64  `json:"position"`
}

// GSCQueryReport summarizes a Search Analytics query
type GSCQueryReport struct {
	SiteURL    string        `json:"site_url"`
	StartDate  string        `json:"start_date"`
	EndDate    string        `json:"end_date"`
	Dimensions []string      `json:"dimensions"`
	TotalRows  int           `json:"total_rows"`
	Rows       []GSCQueryRow `json:"rows"`
}

// GSCInspectOptions defines parameters for urlInspection:inspect
type GSCInspectOptions struct {
	SiteURL       string `json:"site_url"`
	InspectionURL string `json:"inspection_url"`
	LanguageCode  string `json:"language_code,omitempty"`
}

// GSCInspectReport summarizes indexation and canonical data from URL Inspection
type GSCInspectReport struct {
	InspectionURL          string   `json:"inspection_url"`
	SiteURL                string   `json:"site_url"`
	Verdict                string   `json:"verdict"` // PASS, PARTIAL, FAIL, NEUTRAL
	CoverageState          string   `json:"coverage_state"`
	RobotsTxtState         string   `json:"robots_txt_state"`
	IndexingState          string   `json:"indexing_state"`
	LastCrawlTime          string   `json:"last_crawl_time"`
	PageFetchState         string   `json:"page_fetch_state"`
	GoogleCanonical        string   `json:"google_canonical"`
	UserCanonical          string   `json:"user_canonical"`
	ReferringURLs          []string `json:"referring_urls,omitempty"`
	MobileUsabilityVerdict string   `json:"mobile_usability_verdict,omitempty"`
	RichResultsVerdict     string   `json:"rich_results_verdict,omitempty"`
}

// ResolveGSCToken discovers a valid OAuth access token from environment variables
// or exchanges a Google Cloud Service Account JSON key for an access token.
func ResolveGSCToken(ctx context.Context) (string, error) {
	// 1. Direct bearer token
	if tok := os.Getenv("GSC_ACCESS_TOKEN"); tok != "" {
		return tok, nil
	}
	if tok := os.Getenv("GOOGLE_ACCESS_TOKEN"); tok != "" {
		return tok, nil
	}

	// 2. Service account key file
	keyPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if keyPath == "" {
		keyPath = os.Getenv("GSC_CREDENTIALS_FILE")
	}
	if keyPath != "" {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return "", fmt.Errorf("reading service account file %q: %w", keyPath, err)
		}
		return ExchangeServiceAccountJWT(ctx, data, GSCReadonlyScope)
	}

	// 3. Service account JSON inline
	if rawJSON := os.Getenv("GSC_CREDENTIALS_JSON"); rawJSON != "" {
		return ExchangeServiceAccountJWT(ctx, []byte(rawJSON), GSCReadonlyScope)
	}

	return "", ErrNoGSCAuth
}

// ExchangeServiceAccountJWT mints a signed RS256 JWT and exchanges it for a Google OAuth access token.
func ExchangeServiceAccountJWT(ctx context.Context, saJSON []byte, scope string) (string, error) {
	var sa ServiceAccountKey
	if err := json.Unmarshal(saJSON, &sa); err != nil {
		return "", fmt.Errorf("parsing service account JSON: %w", err)
	}

	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return "", errors.New("service account JSON missing client_email or private_key")
	}

	tokenURI := sa.TokenURI
	if tokenURI == "" {
		tokenURI = GSCOAuthTokenEndpoint
	}

	// Parse RSA private key
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return "", errors.New("failed to decode PEM block containing private key")
	}

	var rsaKey *rsa.PrivateKey
	if parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		var ok bool
		rsaKey, ok = parsedKey.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("private key is PKCS#8 but not RSA")
		}
	} else if parsedKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		rsaKey = parsedKey
	} else {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	now := time.Now().Unix()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]interface{}{
		"iss":   sa.ClientEmail,
		"scope": scope,
		"aud":   tokenURI,
		"exp":   now + 3600,
		"iat":   now,
	}

	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerEnc := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsEnc := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerEnc + "." + claimsEnc

	hashed := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}
	sigEnc := base64.RawURLEncoding.EncodeToString(sig)
	assertion := signingInput + "." + sigEnc

	// Exchange assertion with Google OAuth token endpoint
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchanging JWT assertion: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oauth token exchange returned %d: %s", resp.StatusCode, truncateFor(string(body), 200))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parsing oauth token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("oauth token error %q: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	return tokenResp.AccessToken, nil
}

// QuerySearchAnalytics runs searchAnalytics:query against Google Search Console
func QuerySearchAnalytics(ctx context.Context, token string, opts GSCQueryOptions) (*GSCQueryReport, error) {
	if token == "" {
		return nil, ErrNoGSCAuth
	}
	if opts.SiteURL == "" {
		return nil, errors.New("site URL is required")
	}
	if opts.StartDate == "" || opts.EndDate == "" {
		// Default to last 28 days
		end := time.Now().AddDate(0, 0, -2) // GSC typically has 2-3 day lag
		start := end.AddDate(0, 0, -28)
		opts.StartDate = start.Format("2006-01-02")
		opts.EndDate = end.Format("2006-01-02")
	}
	if len(opts.Dimensions) == 0 {
		opts.Dimensions = []string{"query"}
	}
	if opts.RowLimit <= 0 {
		opts.RowLimit = 25
	}

	reqBody := map[string]interface{}{
		"startDate":  opts.StartDate,
		"endDate":    opts.EndDate,
		"dimensions": opts.Dimensions,
		"rowLimit":   opts.RowLimit,
		"startRow":   opts.StartRow,
	}
	payload, _ := json.Marshal(reqBody)

	// siteUrl in path must be URL-encoded (e.g. sc-domain:example.com or https%3A%2F%2Fexample.com%2F)
	siteParam := url.PathEscape(opts.SiteURL)
	endpoint := fmt.Sprintf("%s/%s/searchAnalytics/query", GSCSearchAnalyticsEndpoint, siteParam)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("GSC Search Analytics API returned %d: %s", resp.StatusCode, truncateFor(string(body), 300))
	}

	var apiResp struct {
		Rows []struct {
			Keys        []string `json:"keys"`
			Clicks      float64  `json:"clicks"`
			Impressions float64  `json:"impressions"`
			CTR         float64  `json:"ctr"`
			Position    float64  `json:"position"`
		} `json:"rows"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing GSC searchAnalytics response: %w", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("GSC API error: %s", apiResp.Error.Message)
	}

	report := &GSCQueryReport{
		SiteURL:    opts.SiteURL,
		StartDate:  opts.StartDate,
		EndDate:    opts.EndDate,
		Dimensions: opts.Dimensions,
		TotalRows:  len(apiResp.Rows),
		Rows:       make([]GSCQueryRow, 0, len(apiResp.Rows)),
	}

	for _, r := range apiResp.Rows {
		report.Rows = append(report.Rows, GSCQueryRow{
			Keys:        r.Keys,
			Clicks:      r.Clicks,
			Impressions: r.Impressions,
			CTR:         r.CTR,
			Position:    r.Position,
		})
	}

	return report, nil
}

// InspectURL runs urlInspection.index:inspect against Google Search Console
func InspectURL(ctx context.Context, token string, opts GSCInspectOptions) (*GSCInspectReport, error) {
	if token == "" {
		return nil, ErrNoGSCAuth
	}
	if opts.InspectionURL == "" || opts.SiteURL == "" {
		return nil, errors.New("both inspectionURL and siteURL are required")
	}

	reqBody := map[string]interface{}{
		"inspectionUrl": opts.InspectionURL,
		"siteUrl":       opts.SiteURL,
	}
	if opts.LanguageCode != "" {
		reqBody["languageCode"] = opts.LanguageCode
	}
	payload, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GSCInspectEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
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
		return nil, fmt.Errorf("GSC URL Inspection API returned %d: %s", resp.StatusCode, truncateFor(string(body), 300))
	}

	var apiResp struct {
		InspectionResult struct {
			Verdict           string `json:"verdict"`
			IndexStatusResult struct {
				Verdict         string   `json:"verdict"`
				CoverageState   string   `json:"coverageState"`
				RobotsTxtState  string   `json:"robotsTxtState"`
				IndexingState   string   `json:"indexingState"`
				LastCrawlTime   string   `json:"lastCrawlTime"`
				PageFetchState  string   `json:"pageFetchState"`
				GoogleCanonical string   `json:"googleCanonical"`
				UserCanonical   string   `json:"userCanonical"`
				ReferringURLs   []string `json:"referringUrls"`
			} `json:"indexStatusResult"`
			MobileUsabilityResult struct {
				Verdict string `json:"verdict"`
			} `json:"mobileUsabilityResult"`
			RichResultsResult struct {
				Verdict string `json:"verdict"`
			} `json:"richResultsResult"`
		} `json:"inspectionResult"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing GSC URL inspection response: %w", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("GSC API error: %s", apiResp.Error.Message)
	}

	idx := apiResp.InspectionResult.IndexStatusResult
	report := &GSCInspectReport{
		InspectionURL:          opts.InspectionURL,
		SiteURL:                opts.SiteURL,
		Verdict:                apiResp.InspectionResult.Verdict,
		CoverageState:          idx.CoverageState,
		RobotsTxtState:         idx.RobotsTxtState,
		IndexingState:          idx.IndexingState,
		LastCrawlTime:          idx.LastCrawlTime,
		PageFetchState:         idx.PageFetchState,
		GoogleCanonical:        idx.GoogleCanonical,
		UserCanonical:          idx.UserCanonical,
		ReferringURLs:          idx.ReferringURLs,
		MobileUsabilityVerdict: apiResp.InspectionResult.MobileUsabilityResult.Verdict,
		RichResultsVerdict:     apiResp.InspectionResult.RichResultsResult.Verdict,
	}

	return report, nil
}
