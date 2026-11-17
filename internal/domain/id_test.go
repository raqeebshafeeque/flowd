package domain

import (
	"strings"
	"testing"
)

func TestNewIDIsPrefixedAndUnique(t *testing.T) {
	first, second := NewID("it"), NewID("it")
	if !strings.HasPrefix(string(first), "it_") || !strings.HasPrefix(string(second), "it_") {
		t.Fatalf("missing prefix: %q %q", first, second)
	}
	if first == second {
		t.Fatalf("got the same id twice: %q", first)
	}
}
