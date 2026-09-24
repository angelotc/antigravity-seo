package textutil

import "testing"

func TestTruncateRuneSafe(t *testing.T) {
	text := "東京都港区の中古マンション"
	got := Truncate(text, 5)
	want := "東京都港区"
	if got != want {
		t.Errorf("Truncate = %q, want %q", got, want)
	}
	if len([]rune(got)) != 5 {
		t.Errorf("truncated rune count = %d, want 5", len([]rune(got)))
	}
}

func TestTruncateShorterThanMax(t *testing.T) {
	text := "hello"
	if got := Truncate(text, 100); got != text {
		t.Errorf("Truncate should return unchanged text, got %q", got)
	}
}

func TestTruncateZeroOrNegative(t *testing.T) {
	if got := Truncate("hello", 0); got != "" {
		t.Errorf("Truncate with maxRunes=0 should return empty, got %q", got)
	}
	if got := Truncate("hello", -1); got != "" {
		t.Errorf("Truncate with negative maxRunes should return empty, got %q", got)
	}
}
