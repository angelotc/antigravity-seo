package textutil

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		text string
		want Script
	}{
		{"japanese", "東京都港区の中古マンション", CJK},
		{"chinese", "北京市朝阳区的公寓出售", CJK},
		{"korean", "서울특별시 강남구 아파트 매매", Hangul},
		{"thai", "คอนโดมิเนียมในกรุงเทพมหานคร", NoSpace},
		{"english", "condos for sale in Tokyo", Latin},
		{"german", "Wohnungen zu verkaufen in München", Latin},
		{"french", "appartements à vendre à Paris", Latin},
		{"arabic", "شقق للبيع في طوكيو", Latin}, // Arabic buckets as Latin (space-delimited, word-based)
		{"empty", "", Latin},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.text); got != c.want {
				t.Errorf("Classify(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

func TestClassifyWithHint(t *testing.T) {
	// Empty text defers entirely to the hint.
	if got := ClassifyWithHint("", "ja"); got != CJK {
		t.Errorf("empty text should defer to hint, got %v", got)
	}
	// Text that is a Latin/CJK mix, where the CJK side is a minority by rune
	// count (so bare Classify would say Latin), should have the hint break
	// the tie toward CJK since the text does contain at least one CJK
	// letter plausible for the hint.
	if got := ClassifyWithHint("SUUMO物件", "ja"); got != CJK {
		t.Errorf("hint should win when text contains a plausible CJK letter, got %v", got)
	}
	// A stray lang=ja on demonstrably English content (zero CJK letters)
	// should NOT force CJK — this is the "stray copy-pasted template" case
	// the hint-plausibility check guards against.
	if got := ClassifyWithHint("This is a full English paragraph about Tokyo real estate.", "ja"); got != Latin {
		t.Errorf("implausible hint should not override clear Latin text, got %v", got)
	}
	// No hint: pure detection.
	if got := ClassifyWithHint("東京都港区", ""); got != CJK {
		t.Errorf("expected CJK with no hint, got %v", got)
	}
}
