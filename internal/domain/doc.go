// Package domain holds the rules of the day planner. Blocks, days and the moves
// between statuses live here, exactly as docs/02-domain.md shapes them.
//
// The package stays pure. It takes plain values in and answers yes or no,
// with no database and no screen calls. Anything needing the outside world
// belongs to the caller. That keeps every rule testable with made up inputs.
package domain
