package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckTextAcceptsRealWorldText(t *testing.T) {
	for _, ok := range []string{
		"Plain ASCII title", "Écrire le rapport", "日本語のタスク", "한국어 작업", "Задача на русском", "مهمة بالعربية",
		"می\u200cخواهم",                         // Persian ZWNJ between letters
		"क\u094d\u200dष स\u0902य\u0941क\u094dत", // Devanagari with ZWJ after virama
		"👩\u200d💻 ship it", "👨\u200d👩\u200d👧 family", "❤\ufe0f heart", "1\ufe0f\u20e3 first", "🏳\ufe0f\u200d🌈 flag", "👍🏽 ok", "text ✔\ufe0e symbol",
		"Ünïcödé e\u0301 combining", "v2.0 \u2013 done • 100%", "a\u3000b",
	} {
		if p := CheckText(ok); p != nil {
			t.Errorf("%q must be accepted, got %v", ok, p)
		}
	}
}

// Anything a reader cannot see, or that changes the order text reads in, is
// refused, so a title always shows what it holds.
func TestCheckTextRefusesHiddenCharacters(t *testing.T) {
	cases := map[string]string{
		"control":             "a\x07b",
		"escape":              "a\x1b[31mb",
		"newline":             "a\nb",
		"tab":                 "a\tb",
		"line separator":      "a\u2028b",
		"paragraph separator": "a\u2029b",
		"bidi override RLO":   "evil\u202etxt.exe",
		"bidi embedding LRE":  "a\u202ab",
		"bidi isolate LRI":    "a\u2066b",
		"left-to-right mark":  "a\u200eb",
		"arabic letter mark":  "a\u061cb",
		"zero width space":    "a\u200bb",
		"word joiner":         "a\u2060b",
		"soft hyphen":         "a\u00adb",
		"byte order mark":     "a\ufeffb",
		"tag character":       "a\U000E0041b",
		"private use":         "a\ue000b",
		"unassigned":          "a\U0003FFFDb",
		"nbsp":                "a\u00a0b",
		"en space":            "a\u2003b",
		"hangul filler":       "a\u3164b",
		"braille blank":       "a\u2800b",
		"object replacement":  "a\ufffcb",
		"lone ZWJ":            "\u200dab",
		"trailing ZWJ":        "ab\u200d",
		"ZWJ next to a space": "a \u200db",
		"lone VS16":           "\ufe0fa",
		"invalid utf-8":       "a\xffb",
	}
	for name, text := range cases {
		p := CheckText(text)
		if p == nil {
			t.Errorf("%s: %q must be refused", name, text)
			continue
		}
		if !errors.Is(p, ErrUnsafeText) || p.Position < 1 {
			t.Errorf("%s: bad problem %+v", name, p)
		}
	}
}

func TestTheProblemNamesTheCharacterAndWhere(t *testing.T) {
	p := CheckText("evil\u202etxt")
	if p == nil || p.Rune != 0x202E || p.Position != 5 {
		t.Fatalf("got %+v", p)
	}
	if !strings.Contains(p.Error(), "U+202E at position 5") {
		t.Fatalf("got %q", p.Error())
	}
}
