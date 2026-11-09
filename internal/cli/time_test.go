package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseClock(t *testing.T) {
	for _, test := range []struct {
		name, date, clock, zone, want string
		end, invalid                  bool
	}{
		{"local clock", "2026-10-14", "09:00", "Asia/Kolkata", "2026-10-14T03:30:00Z", false, false},
		{"end midnight", "2026-10-14", "24:00", "UTC", "2026-10-15T00:00:00Z", true, false},
		{"start midnight shorthand refused", "2026-10-14", "24:00", "UTC", "", false, true},
		{"missing hour", "2026-03-08", "02:30", "America/New_York", "", false, true},
		{"repeated hour", "2026-11-01", "01:30", "America/New_York", "", false, true},
		{"half-hour change", "2026-04-05", "01:45", "Australia/Lord_Howe", "", false, true},
		{"first offset", "2026-11-01", "2026-11-01T01:30:00-04:00", "America/New_York", "2026-11-01T05:30:00Z", false, false},
		{"second offset", "2026-11-01", "2026-11-01T01:30:00-05:00", "America/New_York", "2026-11-01T06:30:00Z", false, false},
		{"explicit overnight", "2026-10-14", "2026-10-15T01:00:00Z", "UTC", "2026-10-15T01:00:00Z", true, false},
		{"start wrong date", "2026-10-14", "2026-10-15T09:00:00Z", "UTC", "", false, true},
		{"short hour", "2026-10-14", "9:00", "UTC", "", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			zone, err := time.LoadLocation(test.zone)
			if err != nil {
				t.Fatal(err)
			}
			got, err := parseClock(test.date, test.clock, zone, test.end)
			if test.invalid {
				if err == nil {
					t.Error("ambiguous, missing or invalid clock should be refused")
				}
				return
			}
			if err != nil || got.UTC().Format(time.RFC3339Nano) != test.want {
				t.Errorf("got %v and %v, want %s", got, err, test.want)
			}
		})
	}
}

func TestTimezoneDetection(t *testing.T) {
	directory := t.TempDir()
	missing := filepath.Join(directory, "missing")
	zoneFile := filepath.Join(directory, "timezone")
	if err := os.WriteFile(zoneFile, []byte("Asia/Kolkata\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, env, file, want string }{
		{"environment", "America/New_York", zoneFile, "America/New_York"},
		{"timezone file", "", zoneFile, "Asia/Kolkata"},
		{"fallback", "not/a-zone", missing, "UTC"},
		{"floating Local is not saved", "Local", missing, "UTC"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := detectTimezone(test.env, missing, test.file); got != test.want {
				t.Errorf("got zone %q, want %q", got, test.want)
			}
		})
	}
	target := filepath.Join(directory, "zoneinfo", "Europe", "London")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "localtime")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if got := detectTimezone("", link, missing); got != "Europe/London" {
		t.Errorf("got symlink zone %q", got)
	}
	if _, err := loadTimezone("Local"); err == nil {
		t.Error("new days require a stable zone name")
	}
}
