package textutil

import "unicode"

// LengthUnit names what Length is counting, so callers can render a
// human-readable message ("212 characters" vs "212 words").
type LengthUnit string

const (
	UnitWords      LengthUnit = "words"
	UnitCharacters LengthUnit = "characters"
)

// Words splits text into word tokens: a Unicode-aware split on runs of
// non-letter/non-digit runes (whitespace, punctuation, symbols). This works
// for any space-delimited script (Latin, Cyrillic, Greek, Arabic, Hebrew,
// Devanagari, Hangul, ...), not just ASCII/English.
func Words(text string) []string {
	var words []string
	start := -1
	runes := []rune(text)
	for i, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if start == -1 {
				start = i
			}
			continue
		}
		if start != -1 {
			words = append(words, string(runes[start:i]))
			start = -1
		}
	}
	if start != -1 {
		words = append(words, string(runes[start:]))
	}
	return words
}

// CountWords returns len(Words(text)).
func CountWords(text string) int {
	return len(Words(text))
}

// CountChars counts letters and digits in text, ignoring whitespace and
// punctuation — the pure per-rune character count for CJK/NoSpace scripts.
// Length uses mixedCharLength instead of this directly so that Latin words
// embedded in CJK/NoSpace text (brand names, SKUs) aren't over-counted
// letter-by-letter; CountChars is exposed for callers that want the raw
// count regardless.
func CountChars(text string) int {
	count := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			count++
		}
	}
	return count
}

// Length measures the length of text appropriate to its script:
//
//   - Space-delimited scripts (Latin, Hangul) are measured in words (see
//     Words/CountWords).
//   - CJK and NoSpace (Thai/Lao/Khmer/Myanmar) scripts are measured in
//     characters: each letter/ideographic rune is one unit, since these
//     scripts carry meaning per-glyph rather than per-word.
//
// Real pages mix scripts — a Japanese article with an embedded English brand
// name, a URL, or a product SKU. For CJK/NoSpace text, Length does not count
// embedded Latin/digit runs letter-by-letter (that would wildly inflate
// length relative to how a native reader perceives the page). Instead it
// counts each maximal run of CJK/NoSpace letters as individual characters,
// and each maximal run of Latin-script letters/digits ("SUUMO", "2026") as a
// single unit, then sums the two. The returned unit is still "characters"
// for CJK/NoSpace text, since that's the unit the thresholds table for that
// script is expressed in.
func Length(text string, sc Script) (int, LengthUnit) {
	switch sc {
	case CJK, NoSpace:
		return mixedCharLength(text), UnitCharacters
	default:
		return CountWords(text), UnitWords
	}
}

// FirstN returns a prefix of text containing approximately the first n
// length-units for script sc (words for space-delimited scripts, the same
// mixed CJK-char/Latin-word counting as Length for CJK/NoSpace). Used to
// build a "lede" window for keyword-placement checks. If text has n units or
// fewer, text is returned unchanged (verbatim, for the word case rejoined
// with single spaces).
func FirstN(text string, sc Script, n int) string {
	switch sc {
	case CJK, NoSpace:
		return mixedCharPrefix(text, n)
	default:
		words := Words(text)
		if len(words) <= n {
			return text
		}
		out := words[0]
		for _, w := range words[1:n] {
			out += " " + w
		}
		return out
	}
}

// mixedCharLength implements the CJK/NoSpace-with-embedded-Latin counting
// described on Length.
func mixedCharLength(text string) int {
	runes := []rune(text)
	count := 0
	for i := 0; i < len(runes); {
		advance, isUnit := mixedUnitAt(runes, i)
		i = advance
		if isUnit {
			count++
		}
	}
	return count
}

// mixedCharPrefix returns the prefix of text containing the first n units
// under the same mixed counting rule as mixedCharLength.
func mixedCharPrefix(text string, n int) string {
	runes := []rune(text)
	count := 0
	i := 0
	for i < len(runes) {
		start := i
		advance, isUnit := mixedUnitAt(runes, i)
		if isUnit {
			count++
			if count > n {
				return string(runes[:start])
			}
		}
		i = advance
	}
	return text
}

// mixedUnitAt inspects runes starting at i and returns the index to advance
// to next, plus whether [i, advance) formed one length unit. A single
// CJK/NoSpace letter is one unit; a run of Latin-ish letters/digits is one
// unit; anything else (whitespace, punctuation) advances by one rune and is
// not a unit.
func mixedUnitAt(runes []rune, i int) (advance int, isUnit bool) {
	r := runes[i]
	if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return i + 1, false
	}
	sc, isLetter := scriptOfRune(r)
	if isLetter && (sc == CJK || sc == NoSpace) {
		return i + 1, true
	}
	j := i
	for j < len(runes) {
		rj := runes[j]
		if !unicode.IsLetter(rj) && !unicode.IsDigit(rj) {
			break
		}
		if scj, ok := scriptOfRune(rj); ok && (scj == CJK || scj == NoSpace) {
			break
		}
		j++
	}
	return j, true
}
