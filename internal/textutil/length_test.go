package textutil

import "testing"

func TestLengthLatin(t *testing.T) {
	n, unit := Length("condos for sale in Tokyo", Latin)
	if unit != UnitWords {
		t.Fatalf("expected words unit, got %v", unit)
	}
	if n != 5 {
		t.Errorf("expected 5 words, got %d", n)
	}
}

func TestLengthGermanFrenchAccents(t *testing.T) {
	n, unit := Length("Wohnungen zu verkaufen in München", Latin)
	if unit != UnitWords || n != 5 {
		t.Errorf("German: got n=%d unit=%v, want 5 words", n, unit)
	}
	n, unit = Length("appartements à vendre à Paris", Latin)
	if unit != UnitWords || n != 5 {
		t.Errorf("French: got n=%d unit=%v, want 5 words", n, unit)
	}
}

func TestLengthKorean(t *testing.T) {
	n, unit := Length("서울특별시 강남구 아파트 매매", Hangul)
	if unit != UnitWords {
		t.Fatalf("expected words unit for Korean, got %v", unit)
	}
	if n != 4 {
		t.Errorf("expected 4 space-delimited Korean words, got %d", n)
	}
}

func TestLengthJapaneseCharacters(t *testing.T) {
	text := "東京都港区の中古マンション"
	n, unit := Length(text, CJK)
	if unit != UnitCharacters {
		t.Fatalf("expected characters unit for Japanese, got %v", unit)
	}
	want := len([]rune(text)) // every rune here is a CJK letter, no punctuation
	if n != want {
		t.Errorf("expected %d characters, got %d", want, n)
	}
}

func TestLengthThai(t *testing.T) {
	n, unit := Length("คอนโดมิเนียมในกรุงเทพมหานคร", NoSpace)
	if unit != UnitCharacters {
		t.Fatalf("expected characters unit for Thai, got %v", unit)
	}
	if n == 0 {
		t.Error("expected nonzero Thai character count")
	}
}

func TestLengthMixedJapaneseWithLatinBrand(t *testing.T) {
	// "SUUMO" should count as ONE unit (a word), not five characters.
	text := "東京の物件をSUUMOで探す"
	n, _ := Length(text, CJK)
	// 東京 の 物件 を SUUMO で 探す -> count CJK letters individually + SUUMO as 1
	cjkLetters := len([]rune("東京の物件をで探す"))
	want := cjkLetters + 1 // + the "SUUMO" run counted once
	if n != want {
		t.Errorf("mixed length = %d, want %d (CJK chars + 1 Latin word run)", n, want)
	}
}

func TestFirstNWords(t *testing.T) {
	text := "one two three four five"
	got := FirstN(text, Latin, 3)
	want := "one two three"
	if got != want {
		t.Errorf("FirstN = %q, want %q", got, want)
	}
	// Fewer words than n: returned unchanged.
	if got := FirstN(text, Latin, 100); got != text {
		t.Errorf("FirstN with n > word count should return text unchanged, got %q", got)
	}
}

func TestFirstNChars(t *testing.T) {
	text := "東京都港区の中古マンション"
	got := FirstN(text, CJK, 3)
	want := "東京都"
	if got != want {
		t.Errorf("FirstN = %q, want %q", got, want)
	}
}

func TestCountWordsVsCountChars(t *testing.T) {
	if CountWords("hello, world!") != 2 {
		t.Errorf("CountWords should ignore punctuation: got %d", CountWords("hello, world!"))
	}
	if CountChars("東京、大阪。") != 4 {
		t.Errorf("CountChars should ignore punctuation: got %d", CountChars("東京、大阪。"))
	}
}
