package textutil

// Truncate returns the first maxRunes runes of s. If s has maxRunes runes or
// fewer, s is returned unchanged. Unlike byte slicing, this never splits a
// multi-byte UTF-8 sequence.
func Truncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}
