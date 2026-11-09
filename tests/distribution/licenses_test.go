package distribution

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestThirdPartyNoticesAreCurrent(t *testing.T) {
	command := exec.Command("go", "run", "./scripts/notices")
	command.Dir = projectRoot(t)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	expected, err := command.Output()
	if err != nil {
		t.Fatalf("generate dependency notices: %v; %s", err, &stderr)
	}
	if string(expected) != projectFile(t, "THIRD_PARTY_NOTICES") {
		t.Error("refresh THIRD_PARTY_NOTICES with go run ./scripts/notices")
	}
}

func TestInstallerRetainsLicenses(t *testing.T) {
	f := newReleaseFixture(t)
	if output, err := f.install("--version", "0.1.0"); err != nil {
		t.Fatalf("install: %v; %s", err, output)
	}
	for _, name := range []string{"LICENSE", "THIRD_PARTY_NOTICES"} {
		actual, err := os.ReadFile(filepath.Join(f.home, ".agents/skills/flowd", name))
		if err != nil {
			t.Error(err)
			continue
		}
		if string(actual) != projectFile(t, name) {
			t.Errorf("installed %s does not preserve the shipped notice", name)
		}
	}
}
