package textutil

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var foldCaser = cases.Fold()

// normalizeForMatch applies NFKC normalization then Unicode case folding, so
// full-width/half-width variants (ＳＵＵＭＯ vs SUUMO, ｶﾀｶﾅ vs カタカナ) and
// case differences (café vs CAFÉ) compare equal, then collapses any run of
// whitespace to a single space so incidental HTML whitespace differences
// don't break phrase matching.
func normalizeForMatch(s string) string {
	return collapseSpace(foldCaser.String(norm.NFKC.String(s)))
}

func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// CountKeyword counts occurrences of kw in text.
//
// For space-delimited keywords (Latin, Hangul, ...) it does whole-word /
// whole-phrase matching on Unicode letter/digit boundaries, so "cat" does
// not match inside "category" or "indicate", but a multi-word phrase like
// "tokyo property" matches as a unit.
//
// For CJK keywords, which have no natural word boundaries, it falls back to
// plain substring counting — the correct behavior for CJK, where splitting
// into "words" would require a dictionary-based segmenter this package
// doesn't have.
func CountKeyword(text, kw string) int {
	kw = strings.TrimSpace(kw)
	if kw == "" {
		return 0
	}
	normText := normalizeForMatch(text)
	normKw := normalizeForMatch(kw)
	if normKw == "" {
		return 0
	}
	if sc := Classify(normKw); sc == CJK || sc == NoSpace {
		return strings.Count(normText, normKw)
	}
	return countWholeWordMatches(normText, normKw)
}

// ContainsKeyword reports whether kw appears at least once in text under the
// same matching rules as CountKeyword.
func ContainsKeyword(text, kw string) bool {
	return CountKeyword(text, kw) > 0
}

func countWholeWordMatches(text, kw string) int {
	textRunes := []rune(text)
	kwRunes := []rune(kw)
	n := len(kwRunes)
	if n == 0 {
		return 0
	}
	count := 0
	for i := 0; i+n <= len(textRunes); i++ {
		if !runesEqual(textRunes[i:i+n], kwRunes) {
			continue
		}
		if i > 0 && isWordBoundaryRune(textRunes[i-1]) {
			continue
		}
		if end := i + n; end < len(textRunes) && isWordBoundaryRune(textRunes[end]) {
			continue
		}
		count++
	}
	return count
}

// isWordBoundaryRune reports whether r can extend a space-delimited
// (Latin/Hangul/...) word, i.e. whether its presence right before/after a
// match should disqualify that match as a whole-word hit. Digits always
// block ("cat3" is not a whole-word "cat"). CJK/NoSpace letters (Han,
// Hiragana, Katakana, Thai, ...) do NOT block: those scripts don't use
// spaces, so a Latin brand name embedded directly in flowing Japanese text
// ("...はSUUMOに...") has no separator on either side, yet SUUMO is still a
// discrete word there. Only letters from the same space-delimited family as
// the keyword count as boundary-breaking.
func isWordBoundaryRune(r rune) bool {
	if unicode.IsDigit(r) {
		return true
	}
	sc, isLetter := scriptOfRune(r)
	if !isLetter {
		return false
	}
	return sc != CJK && sc != NoSpace
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
