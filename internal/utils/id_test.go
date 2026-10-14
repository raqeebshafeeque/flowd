package utils

import (
	"strings"
	"testing"
)

func TestNewIDWithPrefix(t *testing.T) {
	first, second := NewIDWithPrefix("bl"), NewIDWithPrefix("bl")
	for _, id := range []ID{first, second} {
		if !strings.HasPrefix(string(id), "bl_") {
			t.Errorf("got ID %q, want prefix %q", id, "bl_")
		}
	}
	if first == second {
		t.Errorf("got duplicate IDs %q, want unique", first)
	}
}

func TestNewID(t *testing.T) {
	first, second := NewID(), NewID()
	if first == "" || second == "" {
		t.Errorf("got empty ID, want non-empty")
	}
	if first == second {
		t.Errorf("got duplicate IDs %q, want unique", first)
	}
}
