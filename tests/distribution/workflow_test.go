package distribution

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistributionCI(t *testing.T) {
	workflow := projectFile(t, ".github/workflows/ci.yml")
	for _, check := range []string{"ci-tools.sh", "shellcheck", "actionlint", "goreleaser check", "FLOWD_GORELEASER", "govulncheck@v1.9.0 -test ./..."} {
		if !strings.Contains(workflow, check) {
			t.Errorf("missing distribution CI step %s", check)
		}
	}
	tools := projectFile(t, "scripts/ci-tools.sh")
	for _, version := range []string{"2.20.0", "1.7.12", "0.11.0"} {
		if !strings.Contains(tools, version) {
			t.Errorf("missing pinned tool version %s", version)
		}
	}
	if strings.Contains(tools, "releases/latest") {
		t.Error("CI tool downloads must be pinned")
	}
}

func TestConventionalTitles(t *testing.T) {
	script := filepath.Join(projectRoot(t), "scripts/check-pr-title.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		title string
		valid bool
	}{
		{"feat(cli): add review", true}, {"fix: correct installer", true}, {"feat!: break response shape", true}, {"chore(main): release 0.1.0", true},
		{"Add a feature", false}, {"feat: ", false}, {"feat: ok\nrun something", false}, {"$(touch /tmp/unrequested)", false},
	} {
		command := exec.Command("sh", script, test.title)
		output, err := command.CombinedOutput()
		if (err == nil) != test.valid {
			t.Errorf("title %q: %v; %s", test.title, err, output)
		}
	}
	workflow := projectFile(t, ".github/workflows/ci.yml")
	if !strings.Contains(workflow, "PR_TITLE:") || !strings.Contains(workflow, "check-pr-title.sh") {
		t.Error("PR title must flow through a quoted environment value")
	}
	for _, line := range strings.Split(workflow, "\n") {
		if strings.Contains(line, "run:") && strings.Contains(line, "github.event.pull_request.title") {
			t.Error("untrusted title interpolated into shell")
		}
	}
}

func TestNativeCIWorkflow(t *testing.T) {
	workflow := projectFile(t, ".github/workflows/ci.yml")
	for _, check := range []string{"ubuntu-latest", "macos-latest", "go test ./... -count=1", "go test -race ./... -count=1", "go vet ./...", "go build ./...", "gofmt -l cmd internal tests", "go-version-file: go.mod", "contents: read", "persist-credentials: false"} {
		if !strings.Contains(workflow, check) {
			t.Errorf("missing native CI contract %q", check)
		}
	}
	if strings.Contains(workflow, "pull_request_target") || strings.Contains(workflow, "secrets.") {
		t.Error("untrusted CI must not receive release privileges/secrets")
	}
}
