package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrUnsafeText marks text that could hide content, change the order a reader
// sees it in, or carry invisible instructions to an AI agent. This file is the
// one policy for it: titles are checked with it on the way in and cleaned
// with it on the way out.
var ErrUnsafeText = errors.New("unsafe text")

// TextProblem names the first unsafe code point in a string.
type TextProblem struct {
	Rune     rune   // offending code point
	Position int    // 1-based position counted in code points
	Class    string // human-readable class, e.g. "direction-changing character"
}

func (p *TextProblem) Error() string {
	return fmt.Sprintf("remove the %s U+%04X at position %d", p.Class, p.Rune, p.Position)
}
func (p *TextProblem) Unwrap() error { return ErrUnsafeText }

// Maximum consecutive combining marks before a run is treated as visual abuse.
const maxCombiningRun = 4

// invisibleLookalikes render as blank or near-blank while being letters,
// symbols or marks, so a category check alone would accept them.
var invisibleLookalikes = map[rune]bool{
	0x034F: true,               // combining grapheme joiner
	0x115F: true, 0x1160: true, // Hangul choseong/jungseong fillers
	0x17B4: true, 0x17B5: true, // Khmer inherent vowels
	0x180E: true, // Mongolian vowel separator
	0x2800: true, // braille pattern blank
	0x3164: true, // Hangul filler
	0xFFA0: true, // halfwidth Hangul filler
	0xFFFC: true, // object replacement character
}

// joiningScripts are the scripts in which ZWJ/ZWNJ change letter shaping.
var joiningScripts = []*unicode.RangeTable{
	unicode.Arabic, unicode.Syriac, unicode.Devanagari, unicode.Bengali, unicode.Gurmukhi,
	unicode.Gujarati, unicode.Oriya, unicode.Tamil, unicode.Telugu, unicode.Kannada,
	unicode.Malayalam, unicode.Sinhala, unicode.Khmer, unicode.Myanmar, unicode.Mongolian,
}

func isLetterOrMark(r rune) bool { return unicode.IsLetter(r) || unicode.IsMark(r) }
func inJoiningScript(r rune) bool {
	return unicode.In(r, joiningScripts...)
}

// emojiish reports symbols that can take part in an emoji sequence.
func emojiish(r rune) bool {
	return unicode.Is(unicode.So, r) || unicode.Is(unicode.Sk, r) || r == 0xFE0F || r == 0x20E3
}

func classOf(r rune) string {
	switch {
	case unicode.IsControl(r):
		return "control character"
	case r >= 0xE0000 && r <= 0xE007F:
		return "tag character"
	case unicode.Is(unicode.Cf, r):
		return "invisible or direction-changing character"
	case unicode.Is(unicode.Co, r):
		return "private-use character"
	case unicode.Is(unicode.Cs, r):
		return "invalid surrogate"
	case unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r):
		return "line or paragraph separator"
	case unicode.Is(unicode.Zs, r):
		return "unsupported space"
	}
	return "unassigned or unsupported character"
}

// problemAt decides one code point in the context of its neighbours. It returns
// the class of the problem, or "" when the code point is acceptable.
func problemAt(runes []rune, i int) string {
	r := runes[i]
	switch {
	case r == utf8.RuneError:
		return "invalid UTF-8"
	case invisibleLookalikes[r]:
		return "invisible look-alike character"
	case r == ' ' || r == 0x3000:
		return ""
	case r == 0x200D || r == 0x200C: // ZWJ, ZWNJ
		if i == 0 || i == len(runes)-1 {
			return "misplaced joiner"
		}
		prev, next := runes[i-1], runes[i+1]
		if r == 0x200D && emojiish(prev) && emojiish(next) {
			return ""
		}
		if isLetterOrMark(prev) && isLetterOrMark(next) && (inJoiningScript(prev) || inJoiningScript(next)) {
			return ""
		}
		return "misplaced joiner"
	case unicode.IsMark(r) && ((r >= 0xFE00 && r <= 0xFE0F) || (r >= 0xE0100 && r <= 0xE01EF) || (r >= 0x180B && r <= 0x180D) || r == 0x180F):
		if i == 0 {
			return "misplaced variation selector"
		}
		base := runes[i-1]
		switch {
		case r == 0xFE0E || r == 0xFE0F: // text/emoji presentation
			if unicode.In(base, unicode.So, unicode.Sk, unicode.Sm, unicode.Nd) || base == '#' || base == '*' {
				return ""
			}
		case r >= 0x180B && r <= 0x180F:
			if unicode.Is(unicode.Mongolian, base) {
				return ""
			}
		default: // ideographic and standardized variation selectors
			if unicode.Is(unicode.Han, base) {
				return ""
			}
		}
		return "misplaced variation selector"
	case unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S):
		return ""
	}
	return classOf(r)
}

// problems returns, for every code point, the class of its problem or "".
func problems(runes []rune) []string {
	found := make([]string, len(runes))
	run := 0
	for i := range runes {
		found[i] = problemAt(runes, i)
		if found[i] == "" && unicode.IsMark(runes[i]) {
			run++
			if run > maxCombiningRun {
				found[i] = "excessive combining marks"
			}
		} else {
			run = 0
		}
	}
	return found
}

// CheckText reports the first unsafe code point, or nil when the text is made
// only of visible, unambiguous characters.
func CheckText(s string) *TextProblem {
	if !utf8.ValidString(s) {
		for i, r := range []rune(strings.ToValidUTF8(s, string(utf8.RuneError))) {
			if r == utf8.RuneError {
				return &TextProblem{Rune: r, Position: i + 1, Class: "invalid UTF-8"}
			}
		}
	}
	runes := []rune(s)
	for i, class := range problems(runes) {
		if class != "" {
			return &TextProblem{Rune: runes[i], Position: i + 1, Class: class}
		}
	}
	return nil
}

// SanitizeText replaces every unsafe code point with a visible <U+XXXX>
// placeholder and leaves safe text unchanged. The result always passes
// CheckText, and sanitizing twice changes nothing more.
func SanitizeText(s string) string {
	if CheckText(s) == nil {
		return s
	}
	s = strings.ToValidUTF8(s, "<U+FFFD>")
	// Replacing a code point can change its neighbours' context, so repeat
	// until stable; every pass only adds ASCII, so this terminates.
	for pass := 0; pass <= utf8.RuneCountInString(s); pass++ {
		runes := []rune(s)
		bad := problems(runes)
		var out strings.Builder
		changed := false
		for i, r := range runes {
			if bad[i] != "" {
				fmt.Fprintf(&out, "<U+%04X>", r)
				changed = true
				continue
			}
			out.WriteRune(r)
		}
		s = out.String()
		if !changed {
			break
		}
	}
	return s
}
