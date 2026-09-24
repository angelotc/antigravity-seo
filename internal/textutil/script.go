// Package textutil provides script-aware text measurement, thresholds,
// display-width, and keyword-matching helpers so that SEO checks behave
// sensibly on non-Latin pages (Japanese, Chinese, Korean, Thai, ...) instead
// of assuming whitespace-delimited words everywhere.
package textutil

import "unicode"

// Script is the coarse writing-system bucket a piece of text is classified
// into. It drives length measurement, thin-content thresholds, and
// display-width rules.
type Script int

const (
	// Latin is the default bucket: Latin, Cyrillic, Greek, Arabic, Hebrew,
	// Devanagari, etc. — any space-delimited script not called out below.
	// Word-based measurement applies.
	Latin Script = iota
	// CJK covers Han (Chinese characters / Kanji), Hiragana, and Katakana.
	// These scripts are not space-delimited; length is measured in
	// characters.
	CJK
	// Hangul is Korean. Korean text is space-delimited at the word/phrase
	// level (unlike CJK), so it is measured in words like Latin, but its
	// glyphs render full-width like CJK, so it shares CJK's display-width
	// treatment.
	Hangul
	// NoSpace covers other scripts that do not use inter-word spaces but
	// are not CJK: Thai, Lao, Khmer, Myanmar. Length is measured in
	// characters, same shape as CJK, but with distinct thresholds because
	// these scripts are not East-Asian-wide (see width.go) and pack more
	// characters per "word" than CJK ideographs.
	NoSpace
)

func (s Script) String() string {
	switch s {
	case CJK:
		return "cjk"
	case Hangul:
		return "hangul"
	case NoSpace:
		return "no-space"
	default:
		return "latin"
	}
}

// scriptOfRune classifies a single letter rune. Non-letters return (Latin,
// false) since they don't count toward classification.
func scriptOfRune(r rune) (Script, bool) {
	if !unicode.IsLetter(r) {
		return Latin, false
	}
	switch {
	case unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana):
		return CJK, true
	case unicode.In(r, unicode.Hangul):
		return Hangul, true
	case unicode.In(r, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar):
		return NoSpace, true
	default:
		return Latin, true
	}
}

// Classify returns the dominant script among the letters in text, by rune
// count. Non-letter runes (digits, punctuation, whitespace) are ignored.
// Empty or letter-less text defaults to Latin.
func Classify(text string) Script {
	var counts [4]int
	for _, r := range text {
		sc, ok := scriptOfRune(r)
		if !ok {
			continue
		}
		counts[sc]++
	}
	best := Latin
	bestN := counts[Latin]
	for _, sc := range []Script{CJK, Hangul, NoSpace} {
		if counts[sc] > bestN {
			best = sc
			bestN = counts[sc]
		}
	}
	return best
}

// scriptFromLangHint maps an HTML lang/hreflang-style tag (e.g. "ja",
// "zh-Hant", "ko", "th", "lo", "km", "my") to a Script. It only looks at the
// primary subtag before the first '-'.
func scriptFromLangHint(lang string) (Script, bool) {
	primary := lang
	for i, c := range lang {
		if c == '-' || c == '_' {
			primary = lang[:i]
			break
		}
	}
	switch normalizeLangTag(primary) {
	case "ja", "zh":
		return CJK, true
	case "ko":
		return Hangul, true
	case "th", "lo", "km", "my":
		return NoSpace, true
	default:
		return Latin, false
	}
}

func normalizeLangTag(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// ClassifyWithHint is like Classify but lets an HTML lang attribute break
// ties or correct misclassification on short/mixed text. The hint wins when
// it names a recognized script AND that script is actually plausible for
// the text: either the text is empty/too short to classify confidently, or
// the text does contain at least one letter from the hinted script. This
// stops a stray "lang=ja" on an all-English page (e.g. a copy-pasted
// template) from forcing CJK measurement on content that is demonstrably
// Latin.
func ClassifyWithHint(text, langHint string) Script {
	detected := Classify(text)
	hinted, ok := scriptFromLangHint(langHint)
	if !ok {
		return detected
	}
	if text == "" {
		return hinted
	}
	for _, r := range text {
		if sc, isLetter := scriptOfRune(r); isLetter && sc == hinted {
			return hinted
		}
	}
	return detected
}

// ParseScript is the inverse of Script.String. Unknown names map to Latin.
func ParseScript(name string) Script {
	switch name {
	case "cjk":
		return CJK
	case "hangul":
		return Hangul
	case "no-space":
		return NoSpace
	default:
		return Latin
	}
}
