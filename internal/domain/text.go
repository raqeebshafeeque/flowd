package domain

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// ErrUnsafeText marks text that could hide content, change the order a reader
// sees it in, or carry invisible instructions to an AI agent.
var ErrUnsafeText = errors.New("unsafe text")

// TextProblem names the first unsafe character in a string.
type TextProblem struct {
	Rune     rune
	Position int // counted in characters, from 1
}

func (p *TextProblem) Error() string {
	return fmt.Sprintf("remove the hidden or direction-changing character U+%04X at position %d", p.Rune, p.Position)
}

func (p *TextProblem) Unwrap() error { return ErrUnsafeText }

// lookalikes are letters and symbols that show as blank, so they are refused
// even though their category alone would allow them.
var lookalikes = map[rune]bool{
	0x034F: true, 0x115F: true, 0x1160: true, 0x17B4: true, 0x17B5: true,
	0x180E: true, 0x2800: true, 0x3164: true, 0xFFA0: true, 0xFFFC: true,
}

// CheckText reports the first unsafe character, or nil when the text holds
// only what a reader can see: letters, marks, digits, punctuation, symbols
// and plain spaces. A joiner is allowed between two visible characters, so
// joined emoji and scripts that join letters still work, and a variation
// selector right after one.
func CheckText(s string) *TextProblem {
	if !utf8.ValidString(s) {
		return &TextProblem{Rune: utf8.RuneError, Position: 1}
	}
	runes := []rune(s)
	visible := func(i int) bool {
		return i >= 0 && i < len(runes) && runes[i] != ' ' && unicode.In(runes[i], unicode.L, unicode.M, unicode.N, unicode.P, unicode.S)
	}
	for i, r := range runes {
		ok := false
		switch {
		case lookalikes[r]:
		case r == ' ' || r == 0x3000:
			ok = true
		case r == 0x200C || r == 0x200D:
			ok = visible(i-1) && visible(i+1)
		case unicode.Is(unicode.Variation_Selector, r):
			ok = visible(i - 1)
		default:
			ok = visible(i)
		}
		if !ok {
			return &TextProblem{Rune: r, Position: i + 1}
		}
	}
	return nil
}
