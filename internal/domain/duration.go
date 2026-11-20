package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidDuration means a Duration is not a positive number of whole minutes.
var ErrInvalidDuration = errors.New("invalid duration")

// ParseDuration reads a Duration written as whole minutes ("30" or "30m"),
// whole hours ("2h") or both ("1h30m"). Whether it fits in the day is for
// the planner to decide.
func ParseDuration(text string) (int64, error) {
	hours, minutes := "0", strings.TrimSuffix(text, "m")
	if h, m, ok := strings.Cut(text, "h"); ok {
		hours, minutes = h, "0"
		if m != "" {
			if !strings.HasSuffix(m, "m") {
				return 0, invalidDuration()
			}
			minutes = strings.TrimSuffix(m, "m")
		}
	}
	h, errH := digits(hours)
	m, errM := digits(minutes)
	total := h*60 + m
	if errH != nil || errM != nil || total <= 0 {
		return 0, invalidDuration()
	}
	return total, nil
}

func invalidDuration() error {
	return fmt.Errorf("%w: use whole minutes or hours, like 30, 30m, 2h or 1h30m", ErrInvalidDuration)
}

// digits reads a short run of plain digits, refusing signs, spaces and
// anything long enough to overflow.
func digits(s string) (int64, error) {
	if s == "" || len(s) > 5 || strings.Trim(s, "0123456789") != "" {
		return 0, ErrInvalidDuration
	}
	return strconv.ParseInt(s, 10, 64)
}

// FormatDuration writes minutes in the shortest exact form, so 90 becomes
// "1h30m" and 60 becomes "1h".
func FormatDuration(minutes int64) (string, error) {
	if minutes <= 0 {
		return "", fmt.Errorf("%w: minutes must be positive", ErrInvalidDuration)
	}
	hours, remainder := minutes/60, minutes%60
	if hours == 0 {
		return strconv.FormatInt(remainder, 10) + "m", nil
	}
	result := strconv.FormatInt(hours, 10) + "h"
	if remainder != 0 {
		result += strconv.FormatInt(remainder, 10) + "m"
	}
	return result, nil
}
