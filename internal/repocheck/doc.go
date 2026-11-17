// Package repocheck enforces the layout rules of this repository so they
// cannot drift as the code grows.
//
// Every directory that holds Go code has a doc.go describing its package,
// and every test file belongs to exactly one source file with the same name:
// calendar_test.go tests calendar.go and nothing else.
package repocheck
