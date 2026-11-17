package domain

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ErrInvalidDuration means a Duration is not a positive number of whole minutes.
var ErrInvalidDuration = errors.New("invalid duration")

// ParseDuration reads a Duration written as whole minutes ("30" or "30m"),
// whole hours ("2h") or both ("1h30m"). Durations are kept as a count of
// minutes, never as a time.Duration, so very large values cannot overflow.
func ParseDuration(text string) (int64, error) {
	invalid := func() (int64, error) {
		return 0, fmt.Errorf("%w: use a positive whole-minute duration (30, 30m, 2h, 1h30m)", ErrInvalidDuration)
	}
	number := func(s string) (int64, bool) {
		if s == "" {
			return 0, false
		}
		for i := 0; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return 0, false
			}
		}
		n, err := strconv.ParseInt(s, 10, 64)
		return n, err == nil
	}
	var total int64
	if h, tail, ok := strings.Cut(text, "h"); ok {
		hours, valid := number(h)
		if !valid || hours > math.MaxInt64/60 {
			return invalid()
		}
		total = hours * 60
		if tail != "" {
			if !strings.HasSuffix(tail, "m") {
				return invalid()
			}
			minutes, valid := number(strings.TrimSuffix(tail, "m"))
			if !valid || minutes > math.MaxInt64-total {
				return invalid()
			}
			total += minutes
		}
	} else {
		var valid bool
		total, valid = number(strings.TrimSuffix(text, "m"))
		if !valid {
			return invalid()
		}
	}
	if total <= 0 {
		return invalid()
	}
	return total, nil
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
