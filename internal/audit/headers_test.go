package audit

import (
	"testing"

	"antigravity-seo/internal/crawler"
)

func TestInspectHeadersNoneDirective(t *testing.T) {
	res := &crawler.FetchResult{
		RequestedURL: "https://example.com",
		FinalURL:     "https://example.com",
		StatusCode:   200,
		Headers:      map[string][]string{"X-Robots-Tag": {"none"}},
	}
	r := InspectHeaders(res)
	if !r.HasNoindex || !r.HasNofollow {
		t.Errorf("'none' should imply both noindex and nofollow, got noindex=%v nofollow=%v", r.HasNoindex, r.HasNofollow)
	}
}

func TestInspectHeadersGooglebotScoped(t *testing.T) {
	res := &crawler.FetchResult{
		RequestedURL: "https://example.com",
		FinalURL:     "https://example.com",
		StatusCode:   200,
		Headers:      map[string][]string{"X-Robots-Tag": {"googlebot: noindex"}},
	}
	r := InspectHeaders(res)
	if !r.HasNoindex {
		t.Error("'googlebot: noindex' should set HasNoindex")
	}
	if r.HasNofollow {
		t.Error("'googlebot: noindex' should not set HasNofollow")
	}
}

func TestInspectHeadersCommaList(t *testing.T) {
	res := &crawler.FetchResult{
		RequestedURL: "https://example.com",
		FinalURL:     "https://example.com",
		StatusCode:   200,
		Headers:      map[string][]string{"X-Robots-Tag": {"noarchive, NOINDEX, nofollow"}},
	}
	r := InspectHeaders(res)
	if !r.HasNoindex || !r.HasNofollow {
		t.Errorf("comma list with mixed case should detect both directives, got noindex=%v nofollow=%v", r.HasNoindex, r.HasNofollow)
	}
}

func TestInspectHeadersMultipleHeaderInstances(t *testing.T) {
	res := &crawler.FetchResult{
		RequestedURL: "https://example.com",
		FinalURL:     "https://example.com",
		StatusCode:   200,
		Headers:      map[string][]string{"X-Robots-Tag": {"noarchive", "noindex"}},
	}
	r := InspectHeaders(res)
	if !r.HasNoindex {
		t.Error("noindex in the second X-Robots-Tag header instance should still be detected")
	}
}

func TestInspectHeadersParameterizedDirectiveNotMisparsed(t *testing.T) {
	res := &crawler.FetchResult{
		RequestedURL: "https://example.com",
		FinalURL:     "https://example.com",
		StatusCode:   200,
		Headers:      map[string][]string{"X-Robots-Tag": {"max-snippet:-1"}},
	}
	r := InspectHeaders(res)
	if r.HasNoindex || r.HasNofollow {
		t.Error("max-snippet:-1 is not a noindex/nofollow directive and should not be misparsed as a UA-prefixed one")
	}
}

func TestInspectHeadersNoXRobotsTag(t *testing.T) {
	res := &crawler.FetchResult{
		RequestedURL: "https://example.com",
		FinalURL:     "https://example.com",
		StatusCode:   200,
		Headers:      map[string][]string{},
	}
	r := InspectHeaders(res)
	if r.HasNoindex || r.HasNofollow {
		t.Error("no X-Robots-Tag header should mean no noindex/nofollow")
	}
}

func TestInspectHeadersClientErrorIsCritical(t *testing.T) {
	for _, code := range []int{401, 403, 429} {
		res := &crawler.FetchResult{
			RequestedURL: "https://example.com",
			FinalURL:     "https://example.com",
			StatusCode:   code,
			Headers:      map[string][]string{},
		}
		r := InspectHeaders(res)
		found := false
		for _, iss := range r.Issues {
			if iss.Category == "Status" && iss.Severity == SeverityCritical {
				found = true
			}
		}
		if !found {
			t.Errorf("status %d: expected a CRITICAL Status issue, got %+v", code, r.Issues)
		}
	}
}
