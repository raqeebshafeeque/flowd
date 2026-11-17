// Package tui is flowd's keyboard-first terminal workspace. It shows a day
// from the planner and never holds the database between keys: every refresh
// is a short planner read.
//
// The workspace needs at least 80 by 24 cells. In a smaller window it shows
// only a hint to make the window larger and ignores every key except Ctrl+C,
// so nothing can happen that the person cannot see.
package tui
