package distribution

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestReleaseWorkflowRoles(t *testing.T) {
	workflow := projectFile(t, ".github/workflows/release.yml")
	if !strings.HasPrefix(workflow, "name: Releases\n") {
		t.Error("release workflow must be named Releases")
	}
	runName := workflowBlock(t, workflow, "run-name:", "\non:")
	for _, contract := range []string{"github.event_name == 'workflow_dispatch'", "format('Retry release {0}', inputs.tag)", "'Releases for main'"} {
		if !strings.Contains(runName, contract) {
			t.Errorf("missing release run identity %q", contract)
		}
	}
	if strings.Contains(runName, "needs.") || strings.Contains(runName, "steps.") {
		t.Error("run titles cannot depend on outputs produced later")
	}
	for _, job := range []struct {
		id   string
		name string
	}{
		{"prepare", "Update release PR"},
		{"publish", "Build and publish"},
	} {
		block := workflowBlock(t, workflow, "  "+job.id+":\n", "    steps:\n")
		if !strings.Contains(block, "name: "+job.name+"\n") {
			t.Errorf("job %s must be named %s", job.id, job.name)
		}
	}
}

func TestReleaseWorkflowTrust(t *testing.T) {
	workflow := projectFile(t, ".github/workflows/release.yml")
	for _, contract := range []string{"RELEASE_PLEASE_TOKEN", "refs/heads/main", "contents: read", "persist-credentials: false", "cancel-in-progress: false", "release_created", "tag_name", "45996ed1f6d02564a971a2fa1b5860e934307cf7", "govulncheck@v1.9.0 -test ./..."} {
		if !strings.Contains(workflow, contract) {
			t.Errorf("missing release trust contract %s", contract)
		}
	}
	if strings.Contains(workflow, "pull_request_target") {
		t.Error("release workflow must not execute untrusted PR code")
	}
}

func TestReleasePublicationRouting(t *testing.T) {
	workflow := projectFile(t, ".github/workflows/release.yml")
	for _, contract := range []string{"needs: prepare", "needs.prepare.outputs.release_created", "steps.select.outputs.tag", "fetch-depth: 0", "contents: write", "check-release-tag.sh", "publish-release.sh"} {
		if !strings.Contains(workflow, contract) {
			t.Errorf("missing exact-tag publication contract %s", contract)
		}
	}
	if strings.Contains(workflow, "tags:") {
		t.Error("publication must not depend on another tag event")
	}
}

func TestBootstrapBoundarySurvivesSetupSquash(t *testing.T) {
	var config struct {
		Bootstrap string `json:"bootstrap-sha"`
	}
	if err := json.Unmarshal([]byte(projectFile(t, "release-please-config.json")), &config); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = projectRoot(t)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("squash fixture %v: %v; %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	root := git("rev-list", "--max-parents=0", "HEAD")
	head := git("rev-parse", "HEAD")
	repository := filepath.Join(t.TempDir(), "squash.git")
	git("clone", "--shared", "--bare", ".", repository)
	// Match the setup squash: the feature tree, with only pre-implementation main as parent.
	squash := git("-C", repository, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
		"commit-tree", head+"^{tree}", "-p", root, "-m", "feat: bootstrap\n\nRelease-As: 0.1.0")
	command := exec.Command("git", "-C", repository, "merge-base", "--is-ancestor", config.Bootstrap, squash)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("bootstrap boundary is absent from squash-merged main: %v; %s", err, output)
	}
}

func TestReleasePolicy(t *testing.T) {
	var config struct {
		ReleaseType   string                     `json:"release-type"`
		Bootstrap     string                     `json:"bootstrap-sha"`
		BumpMinor     bool                       `json:"bump-minor-pre-major"`
		PatchFeatures bool                       `json:"bump-patch-for-minor-pre-major"`
		Draft         bool                       `json:"draft"`
		ForceTag      bool                       `json:"force-tag-creation"`
		ReleaseAs     string                     `json:"release-as"`
		Packages      map[string]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal([]byte(projectFile(t, "release-please-config.json")), &config); err != nil {
		t.Fatal(err)
	}
	if config.ReleaseType != "simple" || !config.BumpMinor || config.PatchFeatures || !config.Draft || !config.ForceTag || config.ReleaseAs != "" {
		t.Error("release policy must preserve approval, draft tags and evolving semantic versions")
	}
	if len(config.Bootstrap) != 40 || len(config.Packages) != 1 || config.Packages["."] == nil {
		t.Error("release policy needs one root package and explicit history boundary")
	}
	var manifest map[string]string
	if err := json.Unmarshal([]byte(projectFile(t, ".release-please-manifest.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	// Release Please writes the proposed version into its release PR before tagging.
	if len(manifest) > 1 {
		t.Error("manifest must describe only the root package")
	}
	for path, version := range manifest {
		if path != "." || !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(version) {
			t.Error("invalid generated root release metadata")
		}
	}
}
