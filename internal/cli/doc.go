// Package cli is flowd's command line. It parses arguments, calls the
// planner and prints either plain text for people or JSON for scripts and
// agents. Errors go to stderr as a stable code, a sentence and a hint, and
// the exit status says what kind of failure it was.
package cli
