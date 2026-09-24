package textutil

import (
	"unicode"

	"golang.org/x/text/width"
)

// DisplayWidth approximates how many "half-width" cells text occupies when
// rendered — the same rough metric behind Google's pixel-based SERP
// truncation of titles/descriptions. Wide and Fullwidth runes (CJK
// ideographs, fullwidth Latin/punctuation forms used in Japanese text) count
// as 2 units; combining marks (accents that render on top of the preceding
// rune, e.g. a decomposed é) count as 0; everything else counts as 1.
func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue // combining mark: zero width
		}
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			w += 2
		default:
			w++
		}
	}
	return w
}

// WidthLimits bounds an on-page text field in DisplayWidth units.
type WidthLimits struct {
	Min int
	Max int
}

// TitleLimits is a single width-based rule for <title> length that happens
// to work for both English and Japanese without going script-aware: Google
// truncates titles at roughly 600px. ~30-60 English characters (width 30-60,
// since half-width Latin glyphs are 1 unit each) and ~15-30 Japanese
// full-width characters (width 30-60, since each full-width glyph is 2
// units) land on the exact same DisplayWidth range, so one rule serves both.
var TitleLimits = WidthLimits{Min: 30, Max: 60}

// metaDescLimitsByScript bounds meta description length in DisplayWidth
// units, per script.
//
// Unlike titles, description pixel budgets do NOT collapse to one width
// range across scripts. Google's ~920px desktop snippet fits roughly
// 70-160 half-width Latin characters (width 70-160) but only ~50-120
// Japanese full-width characters — which is width 100-240, not 70-160. A
// single width-number rule would either falsely flag ordinary Latin
// descriptions as too long (if tightened to 70-160 for everyone) or let
// Japanese descriptions run far over budget (if loosened to 100-240 for
// everyone). So, per the instructions, description limits are script-aware
// instead of a single universal rule — correctness over elegance.
var metaDescLimitsByScript = map[Script]WidthLimits{
	Latin:  {Min: 70, Max: 160},
	CJK:    {Min: 100, Max: 240}, // ~50-120 full-width chars * 2
	Hangul: {Min: 100, Max: 240}, // Hangul syllables are also East-Asian Wide
	// Thai/Lao/Khmer/etc: narrower, non-East-Asian-Wide glyphs than CJK, so
	// treated as effectively half-width like Latin, but these scripts pack
	// more raw characters per "word" (see thresholds.go), so a slightly
	// wider budget than pure Latin is a reasonable middle ground.
	NoSpace: {Min: 80, Max: 180},
}

// MetaDescLimitsFor returns the meta-description DisplayWidth bounds for a
// script, falling back to Latin's for any script not in the table.
func MetaDescLimitsFor(sc Script) WidthLimits {
	if l, ok := metaDescLimitsByScript[sc]; ok {
		return l
	}
	return metaDescLimitsByScript[Latin]
}
