package audit

import (
	"testing"
)

func TestExtractAndInspectSchema(t *testing.T) {
	htmlContent := []byte(`
<!DOCTYPE html>
<html>
<head>
  <script type="application/ld+json">
  {
    "@context": "https://schema.org",
    "@type": "Organization",
    "name": "Nipponhomes",
    "url": "https://nipponhomes.com"
  }
  </script>
  <script type="application/ld+json">
  {
    "@context": "https://schema.org",
    "@type": "FAQPage",
    "mainEntity": [
      {
        "@type": "Question",
        "name": "Can foreigners buy property in Japan?",
        "acceptedAnswer": {
          "@type": "Answer",
          "text": "Yes, with equal ownership rights."
        }
      }
    ]
  }
  </script>
</head>
<body></body>
</html>`)

	report := InspectSchema("https://example.com", htmlContent)

	if report.BlocksCount != 2 {
		t.Fatalf("Expected 2 schema blocks, got %d", report.BlocksCount)
	}

	if !containsString(report.TypesFound, "Organization") {
		t.Errorf("Expected Organization in types, got %v", report.TypesFound)
	}

	if !containsString(report.TypesFound, "FAQPage") {
		t.Errorf("Expected FAQPage in types, got %v", report.TypesFound)
	}

	if report.Score < 90 {
		t.Errorf("Expected high schema score for valid blocks, got %d", report.Score)
	}
}

func TestInvalidSchemaSyntax(t *testing.T) {
	htmlContent := []byte(`
<html>
<head>
  <script type="application/ld+json">
  { "broken": json trailing, }
  </script>
</head>
</html>`)

	report := InspectSchema("https://example.com", htmlContent)

	if report.BlocksCount != 1 {
		t.Fatalf("Expected 1 block, got %d", report.BlocksCount)
	}

	if report.Blocks[0].IsValid {
		t.Errorf("Expected invalid JSON syntax to be flagged")
	}

	if len(report.Blocks[0].Errors) == 0 {
		t.Errorf("Expected error to be recorded for invalid JSON syntax")
	}
}
