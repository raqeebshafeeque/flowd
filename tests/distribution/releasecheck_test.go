package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/releasecheck"
)

func publishAssets(t *testing.T, f *releaseFixture) string {
	t.Helper()
	directory := filepath.Join(f.assets, "v0.1.0")
	installer := []byte(projectFile(t, "install.sh"))
	if err := os.WriteFile(filepath.Join(directory, "install.sh"), installer, 0o600); err != nil {
		t.Fatal(err)
	}
	checksums, err := os.ReadFile(filepath.Join(directory, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(installer)
	checksums = append(checksums, []byte(hex.EncodeToString(digest[:])+"  install.sh\n")...)
	if err := os.WriteFile(filepath.Join(directory, "checksums.txt"), checksums, 0o600); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestReleaseVerificationUsesPrivateEnvironment(t *testing.T) {
	t.Setenv("FLOWD_REVIEW_SECRET", "not-for-the-release-executable")
	f := newReleaseFixture(t)
	f.release("0.1.0", func(entries []archiveEntry) []archiveEntry {
		entries[0].data = []byte(`#!/bin/sh
set -eu
[ -z "${FLOWD_REVIEW_SECRET:-}" ] || exit 42
[ "$(pwd -P)" = "$(cd "$HOME" && pwd -P)" ] || exit 43
case "$1" in
 --version) printf 'flowd 0.1.0\n';;
 --json) printf '{"version":"0.1.0"}\n';;
 --help) printf 'Usage: flowd\n';;
 *) exit 44;;
esac
`)
		return entries
	})
	if err := releasecheck.Verify(publishAssets(t, f), "0.1.0"); err != nil {
		t.Fatal(err)
	}
}

func TestPublishedAssetVerification(t *testing.T) {
	f := newReleaseFixture(t)
	directory := publishAssets(t, f)
	if err := releasecheck.Verify(directory, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(directory, "flowd_0.1.0_linux_arm64.tar.gz")); err != nil {
		t.Fatal(err)
	}
	if err := releasecheck.Verify(directory, "0.1.0"); err == nil {
		t.Error("missing platform accepted")
	}
	f.release("0.1.0", nil)
	directory = publishAssets(t, f)
	if err := os.WriteFile(filepath.Join(directory, "install.sh"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := releasecheck.Verify(directory, "0.1.0"); err == nil {
		t.Error("mismatched checksum accepted")
	}
	f.release("0.1.0", func(entries []archiveEntry) []archiveEntry { entries[2].data = []byte("9.9.9\n"); return entries })
	directory = publishAssets(t, f)
	if err := releasecheck.Verify(directory, "0.1.0"); err == nil {
		t.Error("wrong package version accepted")
	}
}
