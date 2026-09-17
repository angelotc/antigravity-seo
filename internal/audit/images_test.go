package audit

import (
	"strings"
	"testing"
)

var bigDataURI = "data:image/png;base64," + strings.Repeat("A", 11*1024)

var imagesHTML = `
<html><body>
<img src="/a.webp" alt="Modern format image" width="100" height="50" loading="lazy">
<img src="/b.jpg" alt="Legacy format">
<img src="/c.jpg">
<img src="http://cdn.example.com/d.png" alt="Insecure remote" width="1" height="1">
<img src="` + bigDataURI + `" alt="Big inline">
<img src="/decorative.gif" alt="" width="10" height="10">
</body></html>`

func TestInspectImages(t *testing.T) {
	report, err := InspectImages("https://example.com/page", []byte(imagesHTML))
	if err != nil {
		t.Fatal(err)
	}

	if report.TotalImages != 6 {
		t.Fatalf("expected 6 images, got %d", report.TotalImages)
	}
	if report.MissingAlt != 1 {
		t.Errorf("missing alt: expected 1, got %d", report.MissingAlt)
	}
	if report.EmptyAltDecorative != 1 {
		t.Errorf("empty decorative alt: expected 1, got %d", report.EmptyAltDecorative)
	}
	if report.MissingDimensions != 3 {
		t.Errorf("missing dimensions: expected 3, got %d", report.MissingDimensions)
	}
	if report.LazyLoaded != 1 {
		t.Errorf("lazy loaded: expected 1, got %d", report.LazyLoaded)
	}
	if report.ModernFormats != 1 || report.LegacyFormats != 4 {
		t.Errorf("formats: expected modern=1 legacy=4, got modern=%d legacy=%d", report.ModernFormats, report.LegacyFormats)
	}
	if report.InsecureHTTP != 1 {
		t.Errorf("insecure http: expected 1, got %d", report.InsecureHTTP)
	}
	if report.DataURI != 1 || report.OversizedDataURI != 1 {
		t.Errorf("data uri: expected 1 (1 oversized), got %d (%d)", report.DataURI, report.OversizedDataURI)
	}
	if report.Score >= 100 {
		t.Errorf("flawed image page should score below 100, got %d", report.Score)
	}
}

func TestInspectImagesEmptyPage(t *testing.T) {
	report, err := InspectImages("https://example.com", []byte(`<html><body><p>text only</p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalImages != 0 || report.Score != 100 {
		t.Errorf("no-image page should score 100, got %d", report.Score)
	}
}
