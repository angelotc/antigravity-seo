package audit

import (
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
