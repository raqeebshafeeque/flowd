package store

import (
	"errors"
	"os"
)

// Signature is a stat-only fingerprint of a database and its WAL. Taking it
// never opens the database, so the workspace can compare it every second to
// see whether anything changed.
type Signature struct {
	Size, MtimeNS, WalSize, WalMtimeNS int64
}

// StatSignature fingerprints the database at path. An empty WAL holds
// nothing, so it counts the same as no WAL: a read only reader such as the
// status line can leave one behind without it looking like a change.
func StatSignature(path string) (Signature, error) {
	var s Signature
	info, err := os.Stat(path)
	if err != nil {
		return s, err
	}
	s.Size, s.MtimeNS = info.Size(), info.ModTime().UnixNano()
	if wal, err := os.Stat(path + "-wal"); err == nil && wal.Size() > 0 {
		s.WalSize, s.WalMtimeNS = wal.Size(), wal.ModTime().UnixNano()
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return s, err
	}
	return s, nil
}
