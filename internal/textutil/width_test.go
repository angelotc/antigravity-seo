package textutil

import "testing"

func TestDisplayWidthFullwidthVsHalfwidth(t *testing.T) {
	// SUUMO (halfwidth Latin) vs full-width SUUMO: full-width forms are 2
	// units each, halfwidth are 1.
	half := DisplayWidth("SUUMO")
	full := DisplayWidth("ＳＵＵＭＯ")
	if half != 5 {
		t.Errorf("halfwidth SUUMO width = %d, want 5", half)
	}
	if full != 10 {
		t.Errorf("fullwidth ＳＵＵＭＯ width = %d, want 10", full)
	}
}

func TestDisplayWidthKatakana(t *testing.T) {
	// ｶﾀﾅ = halfwidth katakana, "katakana" spelled ｶﾀｶﾅ (4 chars, each 1
	// unit); カタカナ = the same word in fullwidth katakana (each 2 units).
	half := DisplayWidth("ｶﾀｶﾅ")
	full := DisplayWidth("カタカナ")
	if half != 4 {
		t.Errorf("halfwidth katakana width = %d, want 4", half)
	}
	if full != 8 {
		t.Errorf("fullwidth katakana width = %d, want 8", full)
	}
}

func TestDisplayWidthCombiningMarks(t *testing.T) {
	// "e" + combining acute accent (U+0301) should still measure as 1 unit,
	// not 2, since the combining mark itself is zero-width.
	decomposed := "é"
	if w := DisplayWidth(decomposed); w != 1 {
		t.Errorf("decomposed e-acute width = %d, want 1", w)
	}
}

func TestTitleLimitsCollapseAcrossScripts(t *testing.T) {
	// A typical English title and a 22-char full-width Japanese title both
	// land inside TitleLimits (30-60), demonstrating the single width rule
	// covers both scripts.
	en := DisplayWidth("Affordable Akiya Homes For Sale In Rural Japan")
	if en < TitleLimits.Min || en > TitleLimits.Max {
		t.Errorf("English title width %d outside %v", en, TitleLimits)
	}
	jaTitle := ""
	for i := 0; i < 22; i++ {
		jaTitle += "日"
	}
	ja := DisplayWidth(jaTitle)
	if ja < TitleLimits.Min || ja > TitleLimits.Max {
		t.Errorf("Japanese title width %d outside %v", ja, TitleLimits)
	}
}

func TestMetaDescLimitsAreScriptAware(t *testing.T) {
	latin := MetaDescLimitsFor(Latin)
	cjk := MetaDescLimitsFor(CJK)
	if latin == cjk {
		t.Error("meta description limits should differ between Latin and CJK (a single width rule doesn't fit both)")
	}
}
