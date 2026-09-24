package textutil

// Thresholds bundles content-quality thresholds for one script, all
// expressed in the unit Length() returns for that script (words for
// Latin/Hangul, characters for CJK/NoSpace).
type Thresholds struct {
	// ThinContentMin is the minimum length before "thin content" fires.
	ThinContentMin int
	// ReadingSpeed is units consumed per minute (words/min or chars/min).
	ReadingSpeed int
	// LongParagraphMax is the length above which a paragraph is flagged as
	// too long for scannability.
	LongParagraphMax int
}

// thresholdsByScript holds one row per Script. Values are deliberately
// approximate — content quality gates, not hard limits — but are grounded in
// commonly cited reading-speed and content-depth research per script:
//
//   - Latin: 300 words is the long-standing "thin content" floor for
//     English-language ranking competitiveness; 220 wpm is the average adult
//     silent reading speed; 150 words is where a paragraph starts hurting
//     scannability. This matches the pre-existing (Latin-only) behavior.
//   - CJK (Japanese/Chinese): Han/Kana characters are far denser
//     information carriers than Latin letters — roughly 2 English words'
//     worth of content per character on average for expository Japanese
//     text — so thresholds are ~2x the Latin word thresholds in raw
//     character terms: ~600 char minimum, ~500 chars/min reading speed
//     (commonly cited for Japanese/Chinese silent reading), ~300 char long
//     paragraph cap.
//   - Hangul (Korean): space-delimited like Latin, so measured in words.
//     Korean's agglutinative morphology packs more meaning per word than
//     English, so thresholds sit a bit below Latin's word thresholds.
//   - NoSpace (Thai/Lao/Khmer/Myanmar): character-counted like CJK, but
//     these are simple (non-logographic) alphabets, not East-Asian-Wide
//     ideographs — each character carries much less information than a Han
//     glyph, so a comparable amount of substance needs more raw characters.
//     Thresholds sit meaningfully above CJK's in character terms.
var thresholdsByScript = map[Script]Thresholds{
	Latin: {
		ThinContentMin:   300,
		ReadingSpeed:     220,
		LongParagraphMax: 150,
	},
	CJK: {
		ThinContentMin:   600,
		ReadingSpeed:     500,
		LongParagraphMax: 300,
	},
	Hangul: {
		ThinContentMin:   250,
		ReadingSpeed:     200,
		LongParagraphMax: 130,
	},
	NoSpace: {
		ThinContentMin:   800,
		ReadingSpeed:     700,
		LongParagraphMax: 400,
	},
}

// ThresholdsFor returns the content-quality thresholds for a script,
// falling back to Latin's for any script not in the table.
func ThresholdsFor(sc Script) Thresholds {
	if t, ok := thresholdsByScript[sc]; ok {
		return t
	}
	return thresholdsByScript[Latin]
}
