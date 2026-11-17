package planner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

func TestDescribeGivesStableCodesAndExits(t *testing.T) {
	cases := []struct {
		err  error
		code string
		exit int
	}{
		{fmt.Errorf("open: %w", store.ErrCorrupt), "ERR_CORRUPT", 1},
		{store.ErrLocked, "ERR_LOCKED", 1},
		{store.ErrNotFound, "ERR_NOT_FOUND", 1},
		{domain.ErrIllegalTransition, "ERR_ILLEGAL_TRANSITION", 1},
		{domain.ErrInvalidDuration, "ERR_DURATION", 2},
		{InputError(nil, "Nope", ""), "ERR_USAGE", 2},
		{Refusal(ErrPastDay, "Past", ""), "ERR_PAST_DAY", 2},
		{ErrStale, "ERR_STALE", 1},
		{context.Canceled, "ERR_CANCELLED", 130},
		{errors.New("surprise"), "ERR_INTERNAL", 1},
	}
	for _, c := range cases {
		f := Describe(c.err)
		if f.Code != c.code || f.Exit != c.exit || f.Summary == "" {
			t.Errorf("%v: got %+v", c.err, f)
		}
	}
}

func TestExplanationsWinButKeepTheCode(t *testing.T) {
	_, err := ParseDuration("forever")
	f := Describe(err)
	if f.Code != "ERR_DURATION" || f.Summary != `Invalid duration "forever"` || f.Hint == "" {
		t.Fatalf("got %+v", f)
	}
	if !errors.Is(err, domain.ErrInvalidDuration) {
		t.Fatal("the original cause must stay reachable")
	}
}
