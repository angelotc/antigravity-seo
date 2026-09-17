package audit

import (
	"strings"
	"testing"
)

func TestSchemaExpandedTypes(t *testing.T) {
	html := []byte(`
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"Event","name":"Open House","startDate":"2026-10-01T10:00","location":{"@type":"Place","name":"Tokyo"}}
</script>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"VideoObject"}
</script>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting","title":"Agent","datePosted":"2026-09-01"}
</script>`)
	report := InspectSchema("https://example.com", html)

	joined := strings.Join(report.TypesFound, ",")
	for _, want := range []string{"Event", "VideoObject", "JobPosting"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected type %s in %v", want, report.TypesFound)
		}
	}

	// VideoObject missing 3 required fields, JobPosting missing 2 — warnings recorded
	totalWarnings := 0
	for _, b := range report.Blocks {
		totalWarnings += len(b.Warnings)
	}
	if totalWarnings < 5 {
		t.Errorf("expected >=5 missing-property warnings, got %d", totalWarnings)
	}
	if report.Score >= 80 {
		t.Errorf("incomplete schema should reduce score, got %d", report.Score)
	}
}

func TestSchemaDeprecatedTypeWarning(t *testing.T) {
	html := []byte(`<script type="application/ld+json">
{"@context":"https://schema.org","@type":"HowTo","name":"Fix a faucet","step":[]}
</script>`)
	report := InspectSchema("https://example.com", html)
	found := false
	for _, b := range report.Blocks {
		for _, w := range b.Warnings {
			if strings.Contains(w, "deprecated") {
				found = true
			}
		}
	}
	if !found {
		t.Error("HowTo should produce a deprecation warning")
	}
}

func TestSchemaPlaceholderError(t *testing.T) {
	html := []byte(`<script type="application/ld+json">
{"@context":"https://schema.org","@type":"Organization","name":"REPLACE_WITH_NAME","url":"https://example.com"}
</script>`)
	report := InspectSchema("https://example.com", html)
	if len(report.Blocks) != 1 || len(report.Blocks[0].Errors) == 0 {
		t.Fatalf("placeholder in schema value should produce block error, got %+v", report.Blocks)
	}
}

func TestSchemaProductOffersDetail(t *testing.T) {
	html := []byte(`<script type="application/ld+json">
{"@context":"https://schema.org","@type":"Product","name":"Sofa","image":"https://x/i.jpg","offers":{"@type":"Offer","availability":"https://schema.org/InStock"}}
</script>`)
	report := InspectSchema("https://example.com", html)
	found := false
	for _, b := range report.Blocks {
		for _, w := range b.Warnings {
			if strings.Contains(w, "offers") {
				found = true
			}
		}
	}
	if !found {
		t.Error("offers without price/priceCurrency should warn")
	}
}
