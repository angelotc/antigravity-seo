package crawler

import (
	"testing"
)

func TestParseSitemapURLSet(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page1</loc>
    <lastmod>2026-09-01</lastmod>
    <changefreq>daily</changefreq>
    <priority>0.8</priority>
  </url>
  <url>
    <loc>https://example.com/page2</loc>
    <lastmod>2026-09-02</lastmod>
  </url>
</urlset>`)

	urls, children, isIndex, err := ParseSitemap(xmlData, false)
	if err != nil {
		t.Fatalf("ParseSitemap error: %v", err)
	}

	if isIndex {
		t.Errorf("Expected urlset, but detected sitemap index")
	}

	if len(children) != 0 {
		t.Errorf("Expected 0 child sitemaps, got %d", len(children))
	}

	if len(urls) != 2 {
		t.Fatalf("Expected 2 URLs, got %d", len(urls))
	}

	if urls[0].Loc != "https://example.com/page1" {
		t.Errorf("Expected loc https://example.com/page1, got %s", urls[0].Loc)
	}
	if urls[0].Priority != 0.8 {
		t.Errorf("Expected priority 0.8, got %f", urls[0].Priority)
	}
}

func TestParseSitemapIndex(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap>
    <loc>https://example.com/sub-sitemap1.xml</loc>
    <lastmod>2026-09-10</lastmod>
  </sitemap>
  <sitemap>
    <loc>https://example.com/sub-sitemap2.xml</loc>
  </sitemap>
</sitemapindex>`)

	urls, children, isIndex, err := ParseSitemap(xmlData, false)
	if err != nil {
		t.Fatalf("ParseSitemap error: %v", err)
	}

	if !isIndex {
		t.Errorf("Expected sitemap index, but detected urlset")
	}

	if len(children) != 2 {
		t.Fatalf("Expected 2 child sitemaps, got %d", len(children))
	}

	if children[0] != "https://example.com/sub-sitemap1.xml" {
		t.Errorf("Expected child sitemap loc https://example.com/sub-sitemap1.xml, got %s", children[0])
	}

	if len(urls) != 0 {
		t.Errorf("Expected 0 direct URLs in sitemap index, got %d", len(urls))
	}
}
