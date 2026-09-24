package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var (
	CCCollinfoEndpoint   = "https://index.commoncrawl.org/collinfo.json"
	CCIndexBaseURL       = "https://index.commoncrawl.org"
	DefaultFallbackCrawl = "CC-MAIN-2026-34"
)

// CCCollection represents one Common Crawl archive release
type CCCollection struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	CDXAPI string `json:"cdx-api"`
}

// CCRecord is one captured page from the Common Crawl CDX index
type CCRecord struct {
	URLKey    string `json:"urlkey"`
	Timestamp string `json:"timestamp"`
	URL       string `json:"url"`
	Mime      string `json:"mime"`
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Languages string `json:"languages,omitempty"`
	Filename  string `json:"filename,omitempty"`
}

// BacklinksReport summarizes a domain's own presence in the Common Crawl
// capture index — the archived URLs Common Crawl has crawled on this domain
// — not inbound links from other sites. The CDX query matches *.domain/*,
// which lists this domain's own pages, not who links to them.
type BacklinksReport struct {
	Domain        string         `json:"domain"`
	CrawlIndex    string         `json:"crawl_index"`
	TotalFound    int            `json:"total_found"`
	UniqueURLs    int            `json:"unique_urls"`
	MimeBreakdown map[string]int `json:"mime_breakdown"`
	Languages     map[string]int `json:"languages"`
	Records       []CCRecord     `json:"records"`
}

// GetLatestCrawlIndex queries collinfo.json to discover the most recent crawl collection
func GetLatestCrawlIndex(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, CCCollinfoEndpoint, nil)
	if err != nil {
		return DefaultFallbackCrawl, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return DefaultFallbackCrawl, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return DefaultFallbackCrawl, fmt.Errorf("collinfo returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return DefaultFallbackCrawl, err
	}

	var collections []CCCollection
	if err := json.Unmarshal(body, &collections); err != nil || len(collections) == 0 {
		return DefaultFallbackCrawl, errors.New("empty or invalid collection list")
	}

	return collections[0].ID, nil
}

// QueryCommonCrawlBacklinks queries the open Common Crawl CDX index for this
// domain's own archived captures (URL, timestamp, status, MIME, language).
// This is Common Crawl's capture index for the domain itself — it does NOT
// return inbound links / referring domains, despite the function's name
// (kept for the CLI's existing `backlinks` command). 100% keyless and
// zero-cost.
func QueryCommonCrawlBacklinks(ctx context.Context, domainOrURL string, limit int, crawlID string) (*BacklinksReport, error) {
	cleanDomain := cleanDomainTarget(domainOrURL)
	if cleanDomain == "" {
		return nil, errors.New("invalid domain or URL")
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	if crawlID == "" {
		latest, err := GetLatestCrawlIndex(ctx)
		if err == nil && latest != "" {
			crawlID = latest
		} else {
			crawlID = DefaultFallbackCrawl
		}
	}

	// Query *.domain/* to match all indexed subdomains and paths
	pattern := fmt.Sprintf("*.%s/*", cleanDomain)
	cdxURL := fmt.Sprintf("%s/%s-index?url=%s&output=json&limit=%d",
		CCIndexBaseURL, crawlID, url.QueryEscape(pattern), limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cdxURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying Common Crawl CDX: %w", err)
	}
	defer resp.Body.Close()

	report := &BacklinksReport{
		Domain:        cleanDomain,
		CrawlIndex:    crawlID,
		MimeBreakdown: make(map[string]int),
		Languages:     make(map[string]int),
		Records:       make([]CCRecord, 0),
	}

	// 404 means no captures found for this pattern in the index
	if resp.StatusCode == http.StatusNotFound {
		return report, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("Common Crawl returned %d: %s", resp.StatusCode, truncateFor(string(body), 200))
	}

	// CDX returns newline-delimited JSON
	scanner := bufio.NewScanner(resp.Body)
	seenURLs := make(map[string]bool)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec CCRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}

		report.Records = append(report.Records, rec)
		seenURLs[rec.URL] = true

		if rec.Mime != "" {
			report.MimeBreakdown[rec.Mime]++
		}
		if rec.Languages != "" {
			for _, lang := range strings.Split(rec.Languages, ",") {
				lang = strings.TrimSpace(lang)
				if lang != "" {
					report.Languages[lang]++
				}
			}
		}
	}

	report.TotalFound = len(report.Records)
	report.UniqueURLs = len(seenURLs)

	return report, nil
}

func cleanDomainTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	if idx := strings.IndexByte(raw, '/'); idx != -1 {
		raw = raw[:idx]
	}
	if idx := strings.IndexByte(raw, ':'); idx != -1 {
		raw = raw[:idx]
	}
	return strings.TrimPrefix(raw, "*.")
}
