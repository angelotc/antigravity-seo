package crawler

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SitemapURL represents a single URL entry in a sitemap
type SitemapURL struct {
	Loc        string  `xml:"loc" json:"loc"`
	LastMod    string  `xml:"lastmod" json:"lastmod,omitempty"`
	ChangeFreq string  `xml:"changefreq" json:"changefreq,omitempty"`
	Priority   float64 `xml:"priority" json:"priority,omitempty"`
}

// SitemapIndexEntry represents a child sitemap in a sitemapindex
type SitemapIndexEntry struct {
	Loc     string `xml:"loc" json:"loc"`
	LastMod string `xml:"lastmod" json:"lastmod,omitempty"`
}

// SitemapReport captures full analysis of sitemap health
type SitemapReport struct {
	SitemapURL      string       `json:"sitemap_url"`
	IsSitemapIndex  bool         `json:"is_sitemap_index"`
	ChildSitemaps   []string     `json:"child_sitemaps,omitempty"`
	TotalURLs       int          `json:"total_urls"`
	SampleURLs      []SitemapURL `json:"sample_urls"`
	BrokenURLs      []string     `json:"broken_urls,omitempty"`
	RedirectingURLs []string     `json:"redirecting_urls,omitempty"`
	Errors          []string     `json:"errors,omitempty"`
	DurationMS      int64        `json:"duration_ms"`
}

// ParseSitemap decodes an XML sitemap (supports .gz and <sitemapindex>)
func ParseSitemap(raw []byte, isGzip bool) (urls []SitemapURL, childSitemaps []string, isIndex bool, err error) {
	var reader io.Reader = bytes.NewReader(raw)
	if isGzip {
		gzReader, gzErr := gzip.NewReader(reader)
		if gzErr != nil {
			return nil, nil, false, fmt.Errorf("failed decompressing gzip sitemap: %w", gzErr)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	decoder := xml.NewDecoder(reader)

	for {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				break
			}
			return nil, nil, false, tokenErr
		}

		switch se := token.(type) {
		case xml.StartElement:
			if se.Name.Local == "sitemapindex" {
				isIndex = true
			} else if se.Name.Local == "sitemap" {
				var sm SitemapIndexEntry
				if err := decoder.DecodeElement(&sm, &se); err == nil && sm.Loc != "" {
					childSitemaps = append(childSitemaps, strings.TrimSpace(sm.Loc))
				}
			} else if se.Name.Local == "url" {
				var u SitemapURL
				if err := decoder.DecodeElement(&u, &se); err == nil && u.Loc != "" {
					u.Loc = strings.TrimSpace(u.Loc)
					urls = append(urls, u)
				}
			}
		}
	}

	return urls, childSitemaps, isIndex, nil
}

// InspectSitemap fetches, parses, and optionally health-checks URLs in a sitemap
func (c *SafeClient) InspectSitemap(ctx context.Context, sitemapURL string, checkHealthLimit int) (*SitemapReport, error) {
	start := time.Now()
	res, err := c.Fetch(ctx, sitemapURL)
	if err != nil {
		return nil, fmt.Errorf("failed fetching sitemap: %w", err)
	}

	isGzip := strings.HasSuffix(sitemapURL, ".gz") || res.ContentType == "application/x-gzip"
	urls, children, isIndex, err := ParseSitemap(res.Body, isGzip)
	if err != nil {
		return nil, fmt.Errorf("failed parsing sitemap XML: %w", err)
	}

	report := &SitemapReport{
		SitemapURL:     sitemapURL,
		IsSitemapIndex: isIndex,
		ChildSitemaps:  children,
		TotalURLs:      len(urls),
		DurationMS:     time.Since(start).Milliseconds(),
		Errors:         make([]string, 0),
	}

	// Capture a sample of up to 10 URLs
	sampleCount := 10
	if len(urls) < sampleCount {
		sampleCount = len(urls)
	}
	report.SampleURLs = urls[:sampleCount]

	// Verify Google Sitemaps constraints
	if len(urls) > 50000 {
		report.Errors = append(report.Errors, fmt.Sprintf("Sitemap exceeds Google's 50,000 URL limit (%d URLs found)", len(urls)))
	}

	// Validate sitemap URL host matches
	parsedOrigin, _ := url.Parse(sitemapURL)
	if parsedOrigin != nil {
		for _, u := range report.SampleURLs {
			uParsed, err := url.Parse(u.Loc)
			if err != nil || uParsed.Host != parsedOrigin.Host {
				report.Errors = append(report.Errors, fmt.Sprintf("Cross-origin or malformed URL in sitemap: %s", u.Loc))
				break
			}
		}
	}

	// Optional concurrent health verification (check HTTP status codes)
	if checkHealthLimit > 0 && len(urls) > 0 {
		toCheck := urls
		if len(toCheck) > checkHealthLimit {
			toCheck = toCheck[:checkHealthLimit]
		}

		var (
			brokenLock sync.Mutex
			wg         sync.WaitGroup
			sem        = make(chan struct{}, 10) // 10 workers
		)

		for _, item := range toCheck {
			wg.Add(1)
			go func(target string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				r, fetchErr := c.Fetch(ctx, target)
				brokenLock.Lock()
				defer brokenLock.Unlock()

				if fetchErr != nil || (r != nil && r.StatusCode >= 400) {
					status := 0
					if r != nil {
						status = r.StatusCode
					}
					report.BrokenURLs = append(report.BrokenURLs, fmt.Sprintf("%s (Status: %d)", target, status))
				} else if r != nil && len(r.Redirects) > 0 {
					report.RedirectingURLs = append(report.RedirectingURLs, fmt.Sprintf("%s -> %s (%d)", target, r.FinalURL, r.Redirects[0].StatusCode))
				}
			}(item.Loc)
		}

		wg.Wait()
	}

	report.DurationMS = time.Since(start).Milliseconds()
	return report, nil
}
