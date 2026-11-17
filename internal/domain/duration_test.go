package domain

import (
	"errors"
	"testing"
)

func TestParseDuration(t *testing.T) {
	good := map[string]int64{"30": 30, "30m": 30, "2h": 120, "1h30m": 90, "0h5m": 5, "90m": 90, "1h0m": 60}
	for text, want := range good {
		if got, err := ParseDuration(text); err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %d, %v, want %d", text, got, err, want)
		}
	}
	bad := []string{"", "0", "0m", "0h", "-5", "1.5h", "10:30", "1h30", "h", "m", "1m30h", " 30", "30 ", "abc", "9223372036854775808", "153722867280912931h"}
	for _, text := range bad {
		if _, err := ParseDuration(text); !errors.Is(err, ErrInvalidDuration) {
			t.Errorf("ParseDuration(%q) should be refused, got %v", text, err)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	for minutes, want := range map[int64]string{1: "1m", 59: "59m", 60: "1h", 90: "1h30m", 1440: "24h"} {
		if got, err := FormatDuration(minutes); err != nil || got != want {
			t.Errorf("FormatDuration(%d) = %q, %v, want %q", minutes, got, err, want)
		}
	}
	if _, err := FormatDuration(0); !errors.Is(err, ErrInvalidDuration) {
		t.Error("zero minutes must be refused")
	}
	for _, text := range []string{"45m", "3h", "2h15m"} {
		minutes, _ := ParseDuration(text)
		if back, _ := FormatDuration(minutes); back != text {
			t.Errorf("%q came back as %q", text, back)
		}
	}
}
