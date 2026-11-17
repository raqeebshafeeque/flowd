package domain

import (
	"errors"
	"strings"
	"testing"
	"unicode"
)

func TestCheckTextAcceptsRealWorldText(t *testing.T) {
	for _, ok := range []string{
		"Plain ASCII title", "Écrire le rapport", "日本語のタスク", "한국어 작업", "Задача на русском", "مهمة بالعربية",
		"می\u200cخواهم",                         // Persian ZWNJ between letters
		"क\u094d\u200dष स\u0902य\u0941क\u094dत", // Devanagari with ZWJ after virama
		"👩\u200d💻 ship it", "👨\u200d👩\u200d👧 family", "❤\ufe0f heart", "1\ufe0f\u20e3 first", "🏳\ufe0f\u200d🌈 flag", "👍🏽 ok", "text ✔\ufe0e symbol",
		"葛\U000e0101 variation", // Han + ideographic variation selector
		"Ünïcödé e\u0301 combining", "v2.0 – done • 100%", "a\u3000b",
	} {
		if p := CheckText(ok); p != nil {
			t.Errorf("%q must be accepted, got %v", ok, p)
		}
		if SanitizeText(ok) != ok {
			t.Errorf("%q must pass through the sanitizer unchanged", ok)
		}
	}
}

func TestCheckTextRejectsEachClass(t *testing.T) {
	cases := map[string]string{
		"control":              "a\x07b",
		"escape":               "a\x1b[31mb",
		"newline":              "a\nb",
		"tab":                  "a\tb",
		"line separator":       "a\u2028b",
		"paragraph separator":  "a\u2029b",
		"bidi override RLO":    "evil\u202etxt.exe",
		"bidi embedding LRE":   "a\u202ab",
		"bidi isolate LRI":     "a\u2066b",
		"bidi isolate PDI":     "a\u2069b",
		"left-to-right mark":   "a\u200eb",
		"right-to-left mark":   "a\u200fb",
		"arabic letter mark":   "a\u061cb",
		"zero width space":     "a\u200bb",
		"word joiner":          "a\u2060b",
		"invisible times":      "a\u2062b",
		"soft hyphen":          "a\u00adb",
		"byte order mark":      "a\ufeffb",
		"tag character":        "a\U000E0041b",
		"tag cancel":           "a\U000E007Fb",
		"private use":          "a\ue000b",
		"private use plane 15": "a\U000F0000b",
		"noncharacter":         "a\ufdd0b",
		"noncharacter ffff":    "a\uffffb",
		"unassigned":           "a\U0003FFFDb",
		"nbsp":                 "a\u00a0b",
		"en space":             "a\u2003b",
		"hangul filler":        "aㅤb",
		"braille blank":        "a⠀b",
		"object replacement":   "a￼b",
		"combining joiner":     "a\u034fb",
		"lone ZWJ":             "\u200dab",
		"trailing ZWJ":         "ab\u200d",
		"ZWJ between latin":    "a\u200db",
		"ZWNJ between latin":   "a\u200cb",
		"lone VS16":            "\ufe0fa",
		"VS16 after latin":     "a\ufe0f",
		"VS1 after latin":      "a\ufe00",
		"supplement VS latin":  "a\U000E0100",
		"invalid utf-8":        "a\xffb",
		"zalgo":                "e\u0301\u0302\u0303\u0304\u0305",
	}
	for name, text := range cases {
		p := CheckText(text)
		if p == nil {
			t.Errorf("%s: %q must be rejected", name, text)
			continue
		}
		if !errors.Is(p, ErrUnsafeText) || p.Position < 1 || p.Class == "" {
			t.Errorf("%s: malformed problem %+v", name, p)
		}
		clean := SanitizeText(text)
		if CheckText(clean) != nil {
			t.Errorf("%s: sanitized %q still unsafe", name, clean)
		}
		if SanitizeText(clean) != clean {
			t.Errorf("%s: sanitizer is not idempotent: %q -> %q", name, clean, SanitizeText(clean))
		}
	}
}

func TestCheckTextNamesCodePointAndPosition(t *testing.T) {
	p := CheckText("evil\u202etxt")
	if p == nil || p.Rune != 0x202E || p.Position != 5 || !strings.Contains(p.Class, "direction") {
		t.Fatalf("%+v", p)
	}
	if got := SanitizeText("evil\u202etxt"); got != "evil<U+202E>txt" {
		t.Fatal(got)
	}
	if !strings.Contains(p.Error(), "U+202E") || !strings.Contains(p.Error(), "position 5") {
		t.Fatal(p.Error())
	}
}

// Every code point that is a control, format, surrogate, private-use or
// unassigned character must be refused when it stands alone.
func TestEveryNonVisibleCodePointIsRejected(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue // not representable in valid UTF-8
		}
		invisible := unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Co, unicode.Zl, unicode.Zp) ||
			(unicode.Is(unicode.Zs, r) && r != ' ' && r != 0x3000) ||
			!unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Zs, unicode.C)
		if invisible && CheckText("a"+string(r)+"b") == nil {
			t.Fatalf("U+%04X must be rejected", r)
		}
	}
}

func TestSanitizeNeverLeavesUnsafeTextEvenWhenContextChanges(t *testing.T) {
	// The ZWJ is valid only between two emoji; sanitizing the second emoji's
	// neighbour must not leave a dangling joiner.
	text := "👩\u200d💻\u202e"
	clean := SanitizeText(text)
	if CheckText(clean) != nil {
		t.Fatal(clean)
	}
	text = "👩\u200d\u202e"
	if clean := SanitizeText(text); CheckText(clean) != nil || SanitizeText(clean) != clean {
		t.Fatal(clean)
	}
}

func FuzzSanitizeText(f *testing.F) {
	for _, seed := range []string{"", "plain", "evil\u202etxt", "👩\u200d💻", "a\xffb", "e\u0301\u0302\u0303\u0304\u0305\u0306", "\u200d", "a\U000E0041"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		clean := SanitizeText(text)
		if p := CheckText(clean); p != nil {
			t.Fatalf("sanitized %q -> %q still unsafe: %v", text, clean, p)
		}
		if again := SanitizeText(clean); again != clean {
			t.Fatalf("not idempotent: %q -> %q -> %q", text, clean, again)
		}
		if CheckText(text) == nil && clean != text {
			t.Fatalf("safe text changed: %q -> %q", text, clean)
		}
	})
}
