package cli

import "testing"

func TestHeading(t *testing.T) {
	if got := heading("2026-11-17"); got != "Tue 17 Nov 2026" {
		t.Errorf("got %q", got)
	}
	if got := heading("nonsense"); got != "nonsense" {
		t.Errorf("got %q", got)
	}
}
