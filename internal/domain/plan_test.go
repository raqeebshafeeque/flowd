package domain

import (
	"errors"
	"testing"
)

func TestValidatePlan(t *testing.T) {
	first := Block{Title: "Study", Status: InProgress, PlannedStart: timeFromStr("09:00"), PlannedEnd: timeFromStr("10:00")}
	for _, test := range []struct {
		name   string
		second Block
		want   error
	}{
		{"touching intervals", Block{Title: "Code", Status: Todo, PlannedStart: timeFromStr("10:00"), PlannedEnd: timeFromStr("11:00")}, nil},
		{"second runner", Block{Title: "Code", Status: InProgress, PlannedStart: timeFromStr("10:00"), PlannedEnd: timeFromStr("11:00")}, ErrActiveBlock},
		{"overlap", Block{Title: "Code", Status: Todo, PlannedStart: timeFromStr("09:30"), PlannedEnd: timeFromStr("11:00")}, ErrOverlap},
		{"archived interval", Block{Archived: true}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidatePlan([]Block{first, test.second}); !errors.Is(err, test.want) {
				t.Errorf("got error %v, want %v", err, test.want)
			}
		})
	}
}
