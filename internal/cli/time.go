package cli

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func loadTimezone(name string) (*time.Location, error) {
	if name == "" || name == "Local" {
		return nil, usage("use a stable IANA timezone such as Asia/Kolkata or UTC")
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		return nil, usage("unknown timezone %q", name)
	}
	return zone, nil
}

func systemTimezone() string {
	return detectTimezone(os.Getenv("TZ"), "/etc/localtime", "/etc/timezone")
}

func detectTimezone(environment, localtime, zoneFile string) string {
	name := strings.TrimPrefix(environment, ":")
	if _, err := loadTimezone(name); err == nil {
		return name
	}
	if target, err := filepath.EvalSymlinks(localtime); err == nil {
		if _, name, ok := strings.Cut(target, "/zoneinfo/"); ok {
			if _, err := loadTimezone(name); err == nil {
				return name
			}
		}
	}
	if content, err := os.ReadFile(zoneFile); err == nil {
		name := strings.TrimSpace(string(content))
		if _, err := loadTimezone(name); err == nil {
			return name
		}
	}
	return "UTC"
}

// parseClock does not let time.Date silently normalize a missing or repeated hour.
func parseClock(date, clock string, zone *time.Location, end bool) (time.Time, error) {
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return time.Time{}, usage("invalid date %q", date)
	}
	if clock == "24:00" && end {
		return parseClock(day.AddDate(0, 0, 1).Format("2006-01-02"), "00:00", zone, false)
	}
	if exact, err := time.Parse(time.RFC3339Nano, clock); err == nil {
		local := exact.In(zone)
		localDate := local.Format("2006-01-02")
		if localDate == date || (end && localDate > date) {
			return local, nil
		}
		return time.Time{}, usage("time %q is outside %s in %s", clock, date, zone)
	}
	parsed, err := time.Parse("15:04", clock)
	if err != nil || parsed.Format("15:04") != clock {
		return time.Time{}, usage("invalid time %q; use HH:MM or RFC 3339 with an offset", clock)
	}
	candidate := time.Date(day.Year(), day.Month(), day.Day(), parsed.Hour(), parsed.Minute(), 0, 0, zone)
	wall := date + " " + clock
	if candidate.Format("2006-01-02 15:04") != wall {
		return time.Time{}, usage("%s does not exist in %s", wall, zone)
	}
	_, offset := candidate.Zone()
	// Nearby offsets detect half-hour and other changes, not only one-hour DST.
	for hour := -48; hour <= 48; hour++ {
		_, other := candidate.Add(time.Duration(hour) * time.Hour).Zone()
		if offset == other {
			continue
		}
		alternative := candidate.Add(time.Duration(offset-other) * time.Second)
		if alternative.Format("2006-01-02 15:04") == wall {
			return time.Time{}, usage("%s occurs twice in %s; use an RFC 3339 timestamp with an offset", wall, zone)
		}
	}
	return candidate, nil
}
