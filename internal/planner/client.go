package planner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/raqeebshafeeque/flowd/internal/domain"
	"github.com/raqeebshafeeque/flowd/internal/store"
)

// Client knows where the database is and which clock and timezone to trust.
// It never keeps the database open between operations.
type Client struct {
	Path string
	// Now reads the clock. Tests replace it; it defaults to time.Now.
	Now func() time.Time
	// Zone is this computer's timezone. It decides what "today" is for
	// planning, independently of any timezone a day was saved with.
	Zone *time.Location
}

// Selection picks a day. An empty Date means today in Zone. Timezone is only
// used when the day does not exist yet.
type Selection struct {
	Date     string
	Timezone string
}

// Revision identifies the exact saved state of an item, including its whole
// history, so a change can tell if it is out of date.
type Revision struct {
	ItemID      domain.ID
	DayID       domain.ID
	Seq         int64
	Fingerprint [32]byte
}

// State is everything a frontend needs to show a day.
type State struct {
	At        time.Time
	Day       domain.Day
	View      View
	History   map[domain.ID][]domain.Event
	Revisions map[domain.ID]Revision
}

// ErrStale means the item changed after it was read.
var ErrStale = errors.New("the item changed since it was read")

// SystemZone returns this computer's timezone by its IANA name, read from TZ,
// /etc/localtime or /etc/timezone, and UTC if none of them names one. A named
// zone matters because a day stores it, and "Local" would change with the
// machine.
func SystemZone() *time.Location {
	return detectZone(os.Getenv("TZ"), "/etc/localtime", "/etc/timezone")
}

func detectZone(env, localtime, zoneFile string) *time.Location {
	candidates := []string{strings.TrimPrefix(env, ":")}
	if target, err := filepath.EvalSymlinks(localtime); err == nil {
		if _, name, ok := strings.Cut(target, "/zoneinfo/"); ok {
			candidates = append(candidates, name)
		}
	}
	if content, err := os.ReadFile(zoneFile); err == nil {
		candidates = append(candidates, strings.TrimSpace(string(content)))
	}
	for _, name := range candidates {
		if name == "" || name == "Local" {
			continue
		}
		if zone, err := time.LoadLocation(name); err == nil {
			return zone
		}
	}
	return time.UTC
}

func (c Client) now() time.Time {
	if c.Now != nil {
		return domain.Capture(c.Now())
	}
	return domain.Capture(time.Now())
}

func (c Client) zone() *time.Location {
	if c.Zone != nil {
		return c.Zone
	}
	return time.UTC
}

// session is one opened database and one captured instant.
type session struct {
	db   *store.Store
	now  time.Time
	zone *time.Location
	day  domain.Day // has no ID when the day is not saved yet
}

// open owns the database for one operation and closes due days first.
func (c Client) open(ctx context.Context, selection Selection) (*session, error) {
	db, err := store.Open(ctx, c.Path)
	if err != nil {
		return nil, err
	}
	s := &session{db: db, now: c.now(), zone: c.zone()}
	if err := db.SettleDue(ctx, s.now); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if s.day, err = s.selectDay(ctx, selection); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

// selectDay finds the saved day or describes an unsaved one. Browsing an
// empty date never creates anything.
func (s *session) selectDay(ctx context.Context, selection Selection) (domain.Day, error) {
	date := selection.Date
	if date == "" {
		date = s.now.In(s.zone).Format("2006-01-02")
	}
	day, err := s.db.DayByDate(ctx, date)
	if errors.Is(err, store.ErrNotFound) {
		name := selection.Timezone
		if name == "" {
			name = s.zone.String()
		}
		unsaved := domain.Day{Date: date, Timezone: name}
		if _, _, err := unsaved.Bounds(); err != nil {
			return domain.Day{}, InputError(err, "Check the date and timezone", "Use a date like 2026-11-17 and a timezone like Asia/Kolkata")
		}
		return unsaved, nil
	}
	if err != nil {
		return domain.Day{}, err
	}
	if selection.Timezone != "" && selection.Timezone != day.Timezone {
		return domain.Day{}, InputError(nil, "This day already uses "+day.Timezone, "Leave out --timezone to use the saved one")
	}
	return day, nil
}

// Read returns the selected day with every item measured at one instant.
func (c Client) Read(ctx context.Context, selection Selection) (state State, err error) {
	s, err := c.open(ctx, selection)
	if err != nil {
		return State{}, err
	}
	defer func() { err = errors.Join(err, s.db.Close()) }()
	return s.state(ctx)
}

func (s *session) state(ctx context.Context) (State, error) {
	state := State{At: s.now, Day: s.day, History: map[domain.ID][]domain.Event{}, Revisions: map[domain.ID]Revision{}}
	var items []domain.Item
	if s.day.ID != "" {
		var err error
		if items, err = s.db.Items(ctx, s.day.ID); err != nil {
			return State{}, err
		}
		if state.History, err = s.db.DayEvents(ctx, s.day.ID); err != nil {
			return State{}, err
		}
	}
	for _, item := range items {
		revision, err := revisionOf(s.day, item, state.History[item.ID])
		if err != nil {
			return State{}, err
		}
		state.Revisions[item.ID] = revision
	}
	view, err := NewView(s.day, items, state.History, s.now)
	state.View = view
	return state, err
}

// revisionOf fingerprints an item and its history. A restored backup can
// reuse sequence numbers, so the number alone is not enough.
func revisionOf(day domain.Day, item domain.Item, events []domain.Event) (Revision, error) {
	r := Revision{ItemID: item.ID, DayID: day.ID}
	if len(events) > 0 {
		r.Seq = events[len(events)-1].Seq
	}
	payload, err := json.Marshal(struct {
		Item   domain.Item
		Events []domain.Event
	}{item, events})
	if err != nil {
		return Revision{}, err
	}
	r.Fingerprint = sha256.Sum256(payload)
	return r, nil
}

// current loads an item for a change and checks it still matches the
// revision the change was based on.
func (s *session) current(ctx context.Context, id domain.ID, based Revision) (domain.Item, error) {
	item, err := s.db.Item(ctx, id)
	if err != nil {
		return domain.Item{}, err
	}
	if item.DayID != s.day.ID || s.day.ID == "" {
		return domain.Item{}, fmt.Errorf("%w: %s is not on %s", store.ErrNotFound, id, s.day.Date)
	}
	if item.Archived {
		return domain.Item{}, fmt.Errorf("%w: %s is deleted", store.ErrNotFound, id)
	}
	events, err := s.db.Events(ctx, id)
	if err != nil {
		return domain.Item{}, err
	}
	revision, err := revisionOf(s.day, item, events)
	if err != nil {
		return domain.Item{}, err
	}
	if revision != based {
		return domain.Item{}, ErrStale
	}
	return item, nil
}
