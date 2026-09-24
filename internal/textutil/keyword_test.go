package textutil

import "testing"

func TestCountKeywordWholeWordLatin(t *testing.T) {
	text := "This category page has one cat, and we indicate the cat is a good cat."
	if got := CountKeyword(text, "cat"); got != 3 {
		t.Errorf("CountKeyword(cat) = %d, want 3 (not matching category/indicate)", got)
	}
	if ContainsKeyword("only category and indicate here", "cat") {
		t.Error("ContainsKeyword should not match 'cat' inside category/indicate")
	}
}

func TestCountKeywordPhrase(t *testing.T) {
	text := "Buy tokyo property today. Tokyo Property investment is popular."
	if got := CountKeyword(text, "tokyo property"); got != 2 {
		t.Errorf("CountKeyword(phrase) = %d, want 2 (case-insensitive)", got)
	}
}

func TestCountKeywordCJKSubstring(t *testing.T) {
	text := "東京都港区の中古マンションと東京都渋谷区の物件"
	if got := CountKeyword(text, "東京都"); got != 2 {
		t.Errorf("CountKeyword CJK substring = %d, want 2", got)
	}
}

func TestCountKeywordFullwidthHalfwidthFold(t *testing.T) {
	// Full-width "ＳＵＵＭＯ" in the text should match half-width "SUUMO"
	// keyword after NFKC normalization.
	if !ContainsKeyword("この物件はＳＵＵＭＯに掲載されています", "SUUMO") {
		t.Error("NFKC should fold fullwidth SUUMO to match halfwidth SUUMO keyword")
	}
	// Halfwidth katakana keyword should match fullwidth katakana in text.
	if !ContainsKeyword("これはカタカナです", "ｶﾀｶﾅ") {
		t.Error("NFKC should fold halfwidth katakana keyword to match fullwidth katakana text")
	}
}

func TestCountKeywordCaseFold(t *testing.T) {
	if !ContainsKeyword("München has great apartments", "münchen") {
		t.Error("case folding should match accented Latin regardless of case")
	}
}

func TestCountKeywordEmpty(t *testing.T) {
	if CountKeyword("some text", "") != 0 {
		t.Error("empty keyword should count 0")
	}
}
