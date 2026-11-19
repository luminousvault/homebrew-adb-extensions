package ui

import "testing"

func TestFit(t *testing.T) {
	cases := []struct {
		in   string
		cols int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello\033[0m"},
		{"\033[1;36m➤ abc\033[0m", 3, "\033[1;36m➤ a\033[0m"}, // ANSI kept, not counted
		{"📦 app.apk", 4, "📦 a\033[0m"},                        // emoji is 2 columns
		{"설치 완료", 5, "설치 \033[0m"},                            // Hangul is 2 columns
		{"áb", 2, "áb"},                                     // combining mark is 0 columns
	}
	for _, c := range cases {
		if got := Fit(c.in, c.cols); got != c.want {
			t.Errorf("Fit(%q,%d)=%q want %q", c.in, c.cols, got, c.want)
		}
	}
}

// Hangul as separate jamo (NFD, as macOS stores names) is as wide as the
// composed syllables.
func TestTextWidthNFDHangul(t *testing.T) {
	nfd := "\u110c\u1161\u1106\u1169 \u1107\u116e\u11ab\u1105\u1175" // 자모 분리
	if w := TextWidth(nfd); w != 9 {
		t.Errorf("TextWidth = %d, want 9", w)
	}
	if got := Fit(nfd+"xyz", 4); Plain(got) != "\u110c\u1161\u1106\u1169" {
		t.Errorf("Fit = %q", got)
	}
}
