package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrIncompleteRepair = errors.New("incomplete repair; finish with doctor --repair")

type repairMarker struct {
	Recovery    string `json:"recovery"`
	Replacement string `json:"replacement"`
	Stage       string `json:"stage"`
}

// Repair explicitly replaces live data from .bak and retains original files.
func Repair(ctx context.Context, path string) (recovery string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, err = resolvePath(path, false)
	if err != nil {
		return "", err
	}
	lock, err := lockDatabase(path)
	if err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, lock.Close())
		if err != nil {
			err = fmt.Errorf("repair database %q: %w", path, classify(err))
		}
	}()
	marker, err := readRepairMarker(path)
	if errors.Is(err, os.ErrNotExist) {
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			if err := safeSibling(path + suffix); err != nil {
				return "", err
			}
		}
		replacement, err := prepareRepair(ctx, path)
		if err != nil {
			return "", err
		}
		// Once the marker exists, the snapshot belongs to resumable recovery.
		defer func() {
			if _, statErr := os.Lstat(path + ".repair"); errors.Is(statErr, os.ErrNotExist) {
				os.Remove(replacement)
			}
		}()
		if err := ctx.Err(); err != nil {
			return "", err
		}
		recovery, err = os.MkdirTemp(filepath.Dir(path), filepath.Base(path)+".recovery-")
		if err != nil {
			return "", err
		}
		marker = repairMarker{Recovery: recovery, Replacement: replacement, Stage: "moving"}
		if err := writeRepairMarker(path, marker); err != nil {
			return recovery, err
		}
	} else if err != nil {
		return "", err
	}
	recovery = marker.Recovery
	if marker.Stage == "moving" {
		for _, suffix := range []string{"-wal", "-shm", "-journal", ""} {
			if err := ctx.Err(); err != nil {
				return recovery, err
			}
			source, destination := path+suffix, filepath.Join(recovery, filepath.Base(path)+suffix)
			if err := safeSibling(source); err != nil {
				return recovery, err
			}
			if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				return recovery, err
			}
			if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
				return recovery, fmt.Errorf("%w: refusing to overwrite preserved %q", ErrIncompleteRepair, destination)
			}
			if err := os.Rename(source, destination); err != nil {
				return recovery, err
			}
		}
		if err := syncDirectory(recovery); err != nil {
			return recovery, err
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return recovery, err
		}
		marker.Stage = "publishing"
		if err := writeRepairMarker(path, marker); err != nil {
			return recovery, err
		}
	}
	if err := ctx.Err(); err != nil {
		return recovery, err
	}
	if _, err := os.Lstat(marker.Replacement); err == nil {
		if err := safeSibling(marker.Replacement); err != nil {
			return recovery, err
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return recovery, fmt.Errorf("%w: unexpected live file before publication", ErrIncompleteRepair)
		}
		if err := os.Rename(marker.Replacement, path); err != nil {
			return recovery, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return recovery, err
	}
	// A missing replacement can mean publication happened just before interruption.
	if err := inspectFile(ctx, path); err != nil {
		return recovery, err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return recovery, err
	}
	if err := os.Remove(path + ".repair"); err != nil {
		return recovery, err
	}
	return recovery, syncDirectory(filepath.Dir(path))
}

func prepareRepair(ctx context.Context, path string) (_ string, err error) {
	backup := path + ".bak"
	if err := safeSibling(backup); err != nil {
		return "", err
	}
	owner, err := lockDatabase(backup)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, owner.Close()) }()
	if err := privateFile(backup); err != nil {
		return "", err
	}
	if err := inspectFile(ctx, backup); err != nil {
		return "", err
	}
	source, err := readOnly(ctx, backup)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, source.Close()) }()
	return snapshot(ctx, source, filepath.Dir(path))
}

func checkRepair(path string) error {
	if _, err := os.Lstat(path + ".repair"); err == nil {
		return ErrIncompleteRepair
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func readRepairMarker(path string) (repairMarker, error) {
	var marker repairMarker
	if err := safeSibling(path + ".repair"); err != nil {
		return marker, err
	}
	content, err := os.ReadFile(path + ".repair")
	if err != nil {
		return marker, err
	}
	if err := json.Unmarshal(content, &marker); err != nil {
		return marker, fmt.Errorf("%w: invalid marker: %w", ErrIncompleteRepair, err)
	}
	parent := filepath.Dir(path)
	if filepath.Dir(marker.Recovery) != parent || !strings.HasPrefix(filepath.Base(marker.Recovery), filepath.Base(path)+".recovery-") || filepath.Dir(marker.Replacement) != parent || !strings.HasPrefix(filepath.Base(marker.Replacement), ".flowd-snapshot-") || (marker.Stage != "moving" && marker.Stage != "publishing") {
		return marker, fmt.Errorf("%w: unsupported marker locations or stage", ErrIncompleteRepair)
	}
	info, err := os.Lstat(marker.Recovery)
	if err != nil {
		return marker, err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return marker, fmt.Errorf("%w: unsafe recovery directory", ErrIncompleteRepair)
	}
	return marker, nil
}

func writeRepairMarker(path string, marker repairMarker) (err error) {
	if err := safeSibling(path + ".repair"); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".flowd-marker-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(marker)
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path+".repair"); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
