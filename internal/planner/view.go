package planner

import (
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

// ViewSchemaVersion numbers the JSON shape of a day. Fields only change with
// a new number, so scripts and agents can rely on them.
const ViewSchemaVersion = 5

// ItemView is an item with its metrics, as frontends show it.
type ItemView struct {
	domain.Item
	domain.Metrics
}

// View is one day measured at one instant: its items in order, the running
// item if any, and the day's totals.
type View struct {
	SchemaVersion int        `json:"schema_version"`
	Date          string     `json:"date"`
	Timezone      string     `json:"timezone"`
	At            time.Time  `json:"at"`
	Items         []ItemView `json:"items"`
	Running       *ItemView  `json:"running"`
	domain.Summary
}

// NewView measures every visible item of a day at now. A day that is not
// saved yet simply has no items.
func NewView(day domain.Day, items []domain.Item, history map[domain.ID][]domain.Event, now time.Time) (View, error) {
	v := View{SchemaVersion: ViewSchemaVersion, Date: day.Date, Timezone: day.Timezone, At: now, Items: []ItemView{}, Summary: domain.NewSummary()}
	for _, item := range items {
		if item.Archived {
			continue
		}
		metrics, err := domain.Measure(day, item, history[item.ID], now)
		if err != nil {
			return View{}, err
		}
		if err := v.Summary.Add(item, metrics); err != nil {
			return View{}, err
		}
		entry := ItemView{item.Clone(), metrics}
		v.Items = append(v.Items, entry)
		if item.Status == domain.InProgress {
			running := entry
			v.Running = &running
		}
	}
	return v, nil
}

// Finished is how many items are done, out of all visible items.
func (v View) Finished() (done, total int) {
	return v.Counts[domain.Done], len(v.Items)
}
