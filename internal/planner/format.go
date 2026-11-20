package planner

import (
	"fmt"
	"strings"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// These keep numbers and words identical in every frontend.

// Span writes whole seconds as 38s, 10m5s or 1h35m6s. Zero is 0m.
func Span(ns int64) string {
	seconds := ns / int64(time.Second)
	if seconds == 0 {
		return "0m"
	}
	h, m, s := seconds/3600, seconds%3600/60, seconds%60
	var b strings.Builder
	if h > 0 {
		fmt.Fprintf(&b, "%dh", h)
	}
	if m > 0 {
		fmt.Fprintf(&b, "%dm", m)
	}
	if s > 0 {
		fmt.Fprintf(&b, "%ds", s)
	}
	return b.String()
}

// Paused writes how long an item has waited as paused. An item that never
// started shows a dash, because it could not have paused.
func Paused(item ItemView) string {
	if item.PausedNS == nil {
		return "-"
	}
	return Span(*item.PausedNS)
}

// Effort is "work / duration" once an item has started, and only its
// duration before that.
func Effort(item ItemView) string {
	duration, _ := domain.FormatDuration(item.DurationMinutes)
	if item.StartedAt == nil {
		return duration
	}
	return Span(item.WorkNS) + " / " + duration
}

// TotalDuration is the sum of a day's durations, like 2h30m.
func TotalDuration(v View) string {
	if v.DurationMinutes == 0 {
		return "0m"
	}
	text, _ := domain.FormatDuration(v.DurationMinutes)
	return text
}

// Label is a status as people read it, like IN PROGRESS.
func Label(s domain.Status) string {
	return strings.ToUpper(Words(s))
}

// Words is a status in lower case words, like in progress.
func Words(s domain.Status) string {
	return strings.ReplaceAll(string(s), "_", " ")
}
