package store

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// A full integrity scan reads the whole file, so it only runs when it can
// tell us something. Every open does the cheap checks (header, schema version
// and tables). The full scan is due when no fresh private record matches the
// file, when a journal was left behind by a crash, and whenever a caller asks
// for it. Any doubt about the record means a scan, never a skip.

// scanValidity is how long a passed scan is trusted for an unchanged file.
const scanValidity = 24 * time.Hour

// scans counts full scans, so tests can prove when they are skipped.
var scans atomic.Int64

// OpenOption changes how Open checks the file.
type OpenOption func(*openOptions)

type openOptions struct{ fullCheck bool }

// WithFullCheck makes Open run the full integrity scan whatever the record
// says. Backup and the start of a workspace session use it.
func WithFullCheck() OpenOption { return func(o *openOptions) { o.fullCheck = true } }

// fingerprint identifies the state of the file without opening it with SQLite.
type fingerprint struct {
	Size       int64  `json:"size"`
	MtimeNS    int64  `json:"mtime_ns"`
	WalSize    int64  `json:"wal_size"`
	WalMtimeNS int64  `json:"wal_mtime_ns"`
	Counter    uint32 `json:"change_counter"`
	journal    bool
}

// checkRecord is written next to the database after flowd's own clean closes.
type checkRecord struct {
	Fingerprint   fingerprint `json:"fingerprint"`
	SchemaVersion int         `json:"schema_version"`
	ScannedAt     time.Time   `json:"scanned_at"`
}

func recordPath(database string) string { return database + ".check" }

// fingerprintOf stats the database and its WAL and reads the SQLite change
// counter from the header. ok is false when the header is not SQLite's.
func fingerprintOf(database string) (f fingerprint, ok bool, err error) {
	info, err := os.Lstat(database)
	if err != nil {
		return f, false, err
	}
	f.Size, f.MtimeNS = info.Size(), info.ModTime().UnixNano()
	// An empty WAL holds nothing, so it counts the same as no WAL. A
	// read-only reader such as a status line can leave one behind, and that
	// must not look like a change. Any saved change still shows in the
	// database itself or in a WAL that is not empty.
	if wal, err := os.Lstat(database + "-wal"); err == nil && wal.Size() > 0 {
		f.WalSize, f.WalMtimeNS = wal.Size(), wal.ModTime().UnixNano()
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return f, false, err
	}
	if _, err := os.Lstat(database + "-journal"); err == nil {
		f.journal = true
	}
	file, err := os.Open(database)
	if err != nil {
		return f, false, err
	}
	defer file.Close()
	header := make([]byte, 100)
	if _, err := io.ReadFull(file, header); err != nil {
		return f, false, nil
	}
	f.Counter = binary.BigEndian.Uint32(header[24:28])
	return f, bytes.HasPrefix(header, []byte("SQLite format 3\x00")), nil
}

func readRecord(database string) (checkRecord, bool) {
	var r checkRecord
	if plainFile(recordPath(database)) != nil {
		return r, false
	}
	raw, err := os.ReadFile(recordPath(database))
	if err != nil || json.Unmarshal(raw, &r) != nil || r.ScannedAt.IsZero() || r.SchemaVersion != SchemaVersion {
		return checkRecord{}, false
	}
	return r, true
}

// writeRecord replaces the record atomically. Failing is harmless: the next
// open simply scans.
func writeRecord(database string, r checkRecord) {
	path := recordPath(database)
	raw, err := json.Marshal(r)
	if err != nil || plainFile(path) != nil {
		return
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return
	}
	_, writeErr := temporary.Write(raw)
	if errors.Join(writeErr, temporary.Chmod(0o600), temporary.Close()) != nil || os.Rename(temporary.Name(), path) != nil {
		os.Remove(temporary.Name())
	}
}

// scanDue decides whether this open needs the full scan.
func scanDue(database string, f fingerprint, now time.Time, forced bool) bool {
	if forced || f.journal {
		return true
	}
	r, ok := readRecord(database)
	if !ok || now.Sub(r.ScannedAt) > scanValidity || r.ScannedAt.After(now.Add(time.Minute)) {
		return true
	}
	return r.Fingerprint != f
}

// refreshRecord runs on a clean close: the file now holds what flowd itself
// wrote, so its fingerprint is recorded with the time of the last passed scan.
func (s *Store) refreshRecord() {
	f, ok, err := fingerprintOf(s.path)
	if err != nil || !ok || f.journal {
		return
	}
	r := checkRecord{Fingerprint: f, SchemaVersion: SchemaVersion, ScannedAt: s.scannedAt}
	if previous, found := readRecord(s.path); found && previous == r {
		return
	}
	writeRecord(s.path, r)
}

// Signature is a stat-only fingerprint of a database and its WAL. Taking it
// never opens the database, so it is cheap enough to compare every second.
type Signature struct {
	Size, MtimeNS, WalSize, WalMtimeNS int64
}

// StatSignature fingerprints the database at path.
func StatSignature(path string) (Signature, error) {
	var s Signature
	info, err := os.Stat(path)
	if err != nil {
		return s, err
	}
	s.Size, s.MtimeNS = info.Size(), info.ModTime().UnixNano()
	// An empty WAL counts as none, as in fingerprintOf.
	if wal, err := os.Stat(path + "-wal"); err == nil && wal.Size() > 0 {
		s.WalSize, s.WalMtimeNS = wal.Size(), wal.ModTime().UnixNano()
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return s, err
	}
	return s, nil
}
