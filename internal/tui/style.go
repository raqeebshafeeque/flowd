package tui

import "github.com/raqeebshafeeque/flowd/internal/domain"

// style turns a role into terminal colors. With NO_COLOR every role is
// plain text, and the meaning is always carried by words and markers too.
type style struct{ noColor bool }

const reset = "\x1b[m"

var roles = map[string]string{
	"brand":    "\x1b[36m", // teal
	"heading":  "\x1b[36m",
	"muted":    "\x1b[2m",
	"selected": "\x1b[36m", // teal, like the brand: whatever is active
	"error":    "\x1b[31m",
	"border":   "\x1b[2m",
	// For the review, matching the status colors.
	"label":  "\x1b[2m",
	"work":   "\x1b[34m",
	"paused": "\x1b[33m",
	"good":   "\x1b[32m",
	"today":  "\x1b[1;36m",
}

var statusColors = map[domain.Status]string{
	domain.Todo:       "",
	domain.InProgress: "\x1b[34m", // blue: being worked on
	domain.Paused:     "\x1b[33m", // yellow: waiting
	domain.Done:       "\x1b[32m", // green: finished
	domain.Skipped:    "\x1b[2m",
	domain.Incomplete: "\x1b[2;31m",
}

func (s style) paint(role, text string) string {
	return s.wrap(roles[role], text)
}

func (s style) status(status domain.Status, text string) string {
	return s.wrap(statusColors[status], text)
}

func (s style) wrap(code, text string) string {
	if s.noColor || code == "" || text == "" {
		return text
	}
	return code + text + reset
}
