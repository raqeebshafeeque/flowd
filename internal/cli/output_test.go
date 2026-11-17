package cli

import "testing"

func TestSpanWritesWholeSeconds(t *testing.T) {
	cases := map[int64]string{0: "0m", 38: "38s", 605: "10m5s", 3600: "1h", 5706: "1h35m6s", 90000: "25h"}
	for seconds, want := range cases {
		if got := span(seconds * 1_000_000_000); got != want {
			t.Errorf("span(%ds) = %q, want %q", seconds, got, want)
		}
	}
}

func TestHeading(t *testing.T) {
	if got := heading("2026-11-17"); got != "Tue 17 Nov 2026" {
		t.Errorf("got %q", got)
	}
	if got := heading("nonsense"); got != "nonsense" {
		t.Errorf("got %q", got)
	}
}
