// Package repocheck holds tests that keep the repository's own rules, so they
// cannot drift as the code grows. It is tooling for the repository and is
// never built into flowd.
//
// Every directory that holds Go code has a doc.go describing its package,
// and every test file belongs to exactly one source file with the same name:
// calendar_test.go tests calendar.go and nothing else. No source or doc file
// may contain an invisible or direction-changing character.
package repocheck
