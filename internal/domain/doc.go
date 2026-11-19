// Package domain holds the rules of daily effort with no file, network or
// clock access of its own.
//
// An Item is something you want to put effort into on one day. It has a
// Duration, which is a budget rather than a time slot, and a status that only
// changes through the actions in this package. Every change is recorded as an
// Event, and the work and paused time of an item are always worked out from
// its events, never stored.
//
// All instants are whole seconds in UTC, so every number this package
// produces is a whole number of seconds.
package domain
