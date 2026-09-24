package audit

import "testing"

func TestInspectHreflangFull(t *testing.T) {
	html := []byte(`
<html><head>
<link rel="alternate" hreflang="en" href="https://example.com/en">
<link rel="alternate" hreflang="ja" href="https://example.com/ja">
<link rel="alternate" hreflang="pt-BR" href="https://example.com/br">
<link rel="alternate" hreflang="x-default" href="https://example.com/en">
<link rel="alternate" hreflang="en" href="https://example.com/en/page">
<link rel="canonical" href="https://example.com/en">
</head><body></body></html>`)

	// Self-reference: page URL matches the "en" alternate
	report, err := InspectHreflang("https://example.com/en", html)
	if err != nil {
		t.Fatal(err)
	}

	if report.Count != 5 {
		t.Fatalf("expected 5 alternates, got %d", report.Count)
	}
	if !report.HasXDefault {
		t.Error("x-default missing")
	}
	if !report.HasSelfRef {
		t.Error("self reference (en) not detected")
	}
	if len(report.Duplicates) == 0 {
		t.Error("duplicate 'en' not detected")
	}
	if len(report.InvalidCodes) != 0 {
		t.Errorf("unexpected invalid codes: %v", report.InvalidCodes)
	}
	found := false
	for _, l := range report.Languages {
		if l == "pt" {
			found = true
		}
	}
	if !found {
		t.Errorf("region language pt not extracted: %v", report.Languages)
	}
}

func TestInspectHreflangInvalidCode(t *testing.T) {
	html := []byte(`<link rel="alternate" hreflang="englisch" href="https://example.com">`)
	report, err := InspectHreflang("https://example.com", html)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.InvalidCodes) != 1 || report.InvalidCodes[0] != "englisch" {
		t.Errorf("expected invalid code 'englisch', got %v", report.InvalidCodes)
	}
	if report.Score >= 75 {
		t.Errorf("invalid codes should hurt score, got %d", report.Score)
	}
}

func TestInspectHreflangRelativeSelfRef(t *testing.T) {
	html := []byte(`<link rel="alternate" hreflang="en" href="/en/page?utm_source=x">`)
	report, err := InspectHreflang("https://example.com/en/page", html)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasSelfRef {
		t.Error("relative href (with a stripped utm_ param) resolving to the page URL should count as self-reference")
	}
}

func TestInspectHreflangSortedOutput(t *testing.T) {
	html := []byte(`
<link rel="alternate" hreflang="zz-bad" href="https://example.com/a">
<link rel="alternate" hreflang="aa-bad" href="https://example.com/b">
<link rel="alternate" hreflang="en" href="https://example.com/c">
<link rel="alternate" hreflang="en" href="https://example.com/d">
`)
	report, err := InspectHreflang("https://example.com", html)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.InvalidCodes) != 2 || report.InvalidCodes[0] != "aa-bad" || report.InvalidCodes[1] != "zz-bad" {
		t.Errorf("expected sorted invalid codes [aa-bad zz-bad], got %v", report.InvalidCodes)
	}
}

func TestInspectHreflangNone(t *testing.T) {
	report, err := InspectHreflang("https://example.com", []byte("<html><body></body></html>"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Count != 0 || len(report.Issues) == 0 {
		t.Errorf("empty hreflang should produce an informational issue, got %+v", report)
	}
}
