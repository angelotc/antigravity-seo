package audit

import (
	"strings"
	"testing"
)

func TestInspectHTML(t *testing.T) {
	htmlContent := []byte(`
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Japan Real Estate for Foreigners - Nipponhomes</title>
  <meta name="description" content="Browse thousands of affordable houses, akiya, and condos across Japan in English.">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="canonical" href="https://nipponhomes.com/en">
</head>
<body>
  <h1>Akiya & Homes for Sale in Japan</h1>
  <h2>Tokyo Region</h2>
  <img src="/img/house1.jpg" alt="Kyoto Machiya">
  <img src="/img/house2.jpg">
  <a href="https://nipponhomes.com/en/explore">Explore</a>
  <a href="https://google.com">Google</a>
</body>
</html>`)

	report, err := InspectHTML("https://nipponhomes.com/en", htmlContent)
	if err != nil {
		t.Fatalf("InspectHTML error: %v", err)
	}

	if report.Title != "Japan Real Estate for Foreigners - Nipponhomes" {
		t.Errorf("Unexpected title: %s", report.Title)
	}

	if !report.HasViewport {
		t.Errorf("Expected mobile viewport to be detected")
	}

	if !report.IsSelfCanonical {
		t.Errorf("Expected canonical to be self-referencing")
	}

	if report.H1Count != 1 {
		t.Errorf("Expected exactly 1 H1, got %d", report.H1Count)
	}

	if report.TotalImages != 2 {
		t.Errorf("Expected 2 images, got %d", report.TotalImages)
	}

	if report.ImagesMissingAlt != 1 {
		t.Errorf("Expected 1 image missing alt, got %d", report.ImagesMissingAlt)
	}

	if report.InternalLinks != 1 {
		t.Errorf("Expected 1 internal link, got %d", report.InternalLinks)
	}

	if report.ExternalLinks != 1 {
		t.Errorf("Expected 1 external link, got %d", report.ExternalLinks)
	}
}

func TestInspectHTMLJapanesePage(t *testing.T) {
	// Title: 15 full-width chars (width 30, inside TitleLimits 30-60).
	// Meta description: 60 full-width chars (width 120, inside the CJK
	// 100-240 band).
	title := strings.Repeat("東", 15)
	desc := strings.Repeat("京", 60)
	htmlContent := []byte(`<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <title>` + title + `</title>
  <meta name="description" content="` + desc + `">
  <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body><h1>` + title + `</h1></body>
</html>`)

	report, err := InspectHTML("https://example.jp/物件", htmlContent)
	if err != nil {
		t.Fatalf("InspectHTML error: %v", err)
	}
	if report.Script != "cjk" {
		t.Errorf("expected cjk script, got %s", report.Script)
	}
	if report.TitleWidth != 30 {
		t.Errorf("expected title width 30, got %d", report.TitleWidth)
	}
	for _, iss := range report.Issues {
		if iss.Category == "Meta" && strings.Contains(iss.Message, "display width") {
			t.Errorf("did not expect a title/description width issue for well-sized Japanese meta, got: %s", iss.Message)
		}
	}
}

func TestInspectHTMLRelativeCanonical(t *testing.T) {
	htmlContent := []byte(`<html><head>
<link rel="canonical" href="/en/explore">
</head><body></body></html>`)
	report, err := InspectHTML("https://nipponhomes.com/en/explore", htmlContent)
	if err != nil {
		t.Fatal(err)
	}
	if !report.IsSelfCanonical {
		t.Error("relative canonical resolving to the same page should be self-canonical")
	}
}

func TestInspectHTMLTitleMultipleTextNodes(t *testing.T) {
	htmlContent := []byte(`<html><head><title>Foo &amp; Bar</title></head><body></body></html>`)
	report, err := InspectHTML("https://example.com", htmlContent)
	if err != nil {
		t.Fatal(err)
	}
	if report.Title != "Foo & Bar" {
		t.Errorf("expected concatenated title 'Foo & Bar', got %q", report.Title)
	}
}

func TestInspectHTMLLinkClassification(t *testing.T) {
	htmlContent := []byte(`<html><body>
<a href="//other.com/path">protocol-relative external</a>
<a href="#section">pure fragment</a>
<a href="mailto:hi@example.com">mail</a>
<a href="tel:+1234567890">tel</a>
<a href="javascript:void(0)">js</a>
<a href="/local/page">relative internal</a>
</body></html>`)
	report, err := InspectHTML("https://example.com/start", htmlContent)
	if err != nil {
		t.Fatal(err)
	}
	if report.InternalLinks != 1 {
		t.Errorf("expected 1 internal link (relative), got %d", report.InternalLinks)
	}
	if report.ExternalLinks != 1 {
		t.Errorf("expected 1 external link (protocol-relative //other.com), got %d", report.ExternalLinks)
	}
}
