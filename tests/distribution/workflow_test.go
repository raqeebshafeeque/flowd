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
	for _, test := range []struct {
		name  string
		title string
		valid bool
	}{
		{"scoped feature", "feat(cli): add review", true},
		{"fix", "fix: correct installer", true},
		{"breaking change", "feat!: break response shape", true},
		{"release bot", "chore(main): release 0.1.0", true},
		{"literal shell substitution", "feat: $(touch title-ran)", true},
		{"literal backticks", "fix: `touch title-ran`", true},
		{"literal shell separator", "fix: ok; touch title-ran", true},
		{"missing type", "Add a feature", false},
		{"empty description", "feat: ", false},
		{"multiple lines", "feat: ok\nrun something", false},
		{"invalid shell-looking title", "$(touch title-ran)", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			command := exec.Command("sh", script, test.title)
			command.Dir = directory
			output, err := command.CombinedOutput()
			if (err == nil) != test.valid {
				t.Errorf("title %q: %v; %s", test.title, err, output)
			}
			if _, err := os.Stat(filepath.Join(directory, "title-ran")); !os.IsNotExist(err) {
				t.Errorf("title executed shell content: %v", err)
			}
		})
	}
	workflow := projectFile(t, ".github/workflows/pr-title.yml")
	if !strings.Contains(workflow, "PR_TITLE:") || !strings.Contains(workflow, "check-pr-title.sh") {
		t.Error("PR title must flow through a quoted environment value")
	}
	for _, line := range strings.Split(workflow, "\n") {
		if strings.Contains(line, "run:") && strings.Contains(line, "github.event.pull_request.title") {
			t.Error("untrusted title interpolated into shell")
		}
	}
}

func TestPRTitleWorkflow(t *testing.T) {
	workflow := projectFile(t, ".github/workflows/pr-title.yml")
	for _, contract := range []string{
		"name: PR title\n",
		`run-name: "Check title for PR #${{ github.event.pull_request.number }}"`,
		"types: [opened, synchronize, reopened, edited]",
		"group: pr-title-${{ github.event.pull_request.number }}",
		"cancel-in-progress: true",
		"name: Conventional title",
		"contents: read",
		"actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1",
		"persist-credentials: false",
		"PR_TITLE: ${{ github.event.pull_request.title }}",
		`run: sh scripts/check-pr-title.sh "$PR_TITLE"`,
	} {
		if !strings.Contains(workflow, contract) {
			t.Errorf("missing title workflow contract %q", contract)
		}
	}
	for _, forbidden := range []string{"if:", "push:", "setup-go@", "ci-tools.sh", "pull_request_target", "secrets.", "contents: write"} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("title checks must be unconditional and read-only: %q", forbidden)
		}
	}
	ci := projectFile(t, ".github/workflows/ci.yml")
	if strings.Contains(ci, "group: pr-title-") {
		t.Error("title edits must not cancel native CI")
	}
}

// workflowBlock checks a bounded section rather than accepting a contract in another job.
func workflowBlock(t *testing.T, workflow, start, end string) string {
	t.Helper()
	_, rest, ok := strings.Cut(workflow, start)
	if !ok {
		t.Fatalf("missing workflow section %q", start)
	}
	block, _, ok := strings.Cut(rest, end)
	if !ok {
		t.Fatalf("missing workflow section boundary %q", end)
	}
	return block
}

func TestSourceCIEvents(t *testing.T) {
	workflow := projectFile(t, ".github/workflows/ci.yml")
	events := workflowBlock(t, workflow, "\non:\n", "\npermissions:")
	want := "  pull_request:\n    branches: [main]\n    types: [opened, synchronize, reopened]\n  push:\n    branches: [main]"
	if strings.TrimSpace(events) != strings.TrimSpace(want) {
		t.Errorf("source event routing:\n%s\nwant:\n%s", events, want)
	}
	for _, forbidden := range []string{"Conventional title", "PR_TITLE", "paths:", "paths-ignore:", "continue-on-error:"} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("source CI must not skip or replace required checks: %q", forbidden)
		}
	}
	native := workflowBlock(t, workflow, "  native:\n", "    steps:\n")
	if strings.Contains(native, "if:") {
		t.Error("native jobs must not become accepted skipped checks on metadata edits")
	}
	for _, contract := range []string{
		"format('CI for PR #{0}', github.event.pull_request.number)", "'CI for main'",
		"group: ci-", "github.event.pull_request.number || github.ref", "cancel-in-progress: true",
	} {
		if !strings.Contains(workflow, contract) {
			t.Errorf("missing source run identity or concurrency %q", contract)
		}
	}
}

func TestNativeCIIdentityAndLint(t *testing.T) {
	workflow := projectFile(t, ".github/workflows/ci.yml")
	native := workflowBlock(t, workflow, "  native:\n", "    steps:\n")
	for _, contract := range []string{"name: ${{ matrix.check }}", "os: ubuntu-latest", "check: Linux checks", "os: macos-latest", "check: macOS checks", "fail-fast: false"} {
		if !strings.Contains(native, contract) {
			t.Errorf("missing native matrix identity %q", contract)
		}
	}
	for _, name := range []string{"Distribution lint and configuration", "Formatting"} {
		step := workflowBlock(t, workflow, "      - name: "+name+"\n", "      - name:")
		if !strings.Contains(step, "if: runner.os == 'Linux'") {
			t.Errorf("%s must run only on Linux", name)
		}
	}
	for _, name := range []string{"Tests", "Race checks", "Known Go vulnerabilities"} {
		step := workflowBlock(t, workflow, "      - name: "+name+"\n", "      - name:")
		if strings.Contains(step, "if:") {
			t.Errorf("%s must run on both native platforms", name)
		}
	}
	for _, contract := range []string{
		"actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1",
		"actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e",
		`echo "FLOWD_GORELEASER=$RUNNER_TEMP/flowd-tools/goreleaser" >> "$GITHUB_ENV"`,
	} {
		if !strings.Contains(workflow, contract) {
			t.Errorf("missing pinned native tool contract %q", contract)
		}
	}
}

func TestConfiguredPackagingToolMissing(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestNoPublishBuild$", "-test.v")
	command.Env = append(os.Environ(), "CI=true", "FLOWD_GORELEASER="+filepath.Join(t.TempDir(), "absent-goreleaser"))
	output, err := command.CombinedOutput()
	if err == nil || strings.Contains(string(output), "--- SKIP") {
		t.Fatalf("missing required tool must fail, not skip: %v; %s", err, output)
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
