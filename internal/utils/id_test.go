package utils

import (
	"strings"
	"testing"
)

func TestNewIDWithPrefix(t *testing.T) {
	first, second := NewIDWithPrefix("bl"), NewIDWithPrefix("bl")
	for _, id := range []ID{first, second} {
		if !strings.HasPrefix(string(id), "bl_") {
			t.Errorf("got id %q, want prefix %q", id, "bl_")
		}
	}
	if first == second {
		t.Errorf("got duplicate ids %q, want unique", first)
	}
}

func TestNewID(t *testing.T) {
	first, second := NewID(), NewID()
	if first == "" || second == "" {
		t.Errorf("got empty id, want non-empty")
	}
	if first == second {
		t.Errorf("got duplicate ids %q, want unique", first)
	}
}
