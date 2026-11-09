package distribution

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func publisherFixture(t *testing.T, draft bool) (*releaseFixture, string) {
	t.Helper()
	f := newReleaseFixture(t)
	assets := publishAssets(t, f)
	work := filepath.Join(f.root, "publisher")
	if err := os.MkdirAll(filepath.Join(work, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"publish-release.sh", "check-release-tag.sh"} {
		if err := os.WriteFile(filepath.Join(work, "scripts", name), []byte(projectFile(t, "scripts/"+name)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(work, ".release-please-manifest.json"), []byte("{\".\":\"0.1.0\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"git", "go", "jq"} {
		if err := os.Remove(filepath.Join(f.tools, tool)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(realGit, filepath.Join(f.tools, "git")); err != nil {
		t.Fatal(err)
	}
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("publisher tests require the CI-only jq utility")
	}
	if err := os.Symlink(jq, filepath.Join(f.tools, "jq")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "Fixture"}, {"config", "user.email", "fixture@example.invalid"}, {"add", "."}, {"commit", "-qm", "fixture"}, {"tag", "v0.1.0"}, {"update-ref", "refs/remotes/origin/main", "HEAD"}} {
		command := exec.Command(realGit, args...)
		command.Dir = work
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v; %s", err, output)
		}
	}
	draftText := "false"
	if draft {
		draftText = "true"
	}
	state := filepath.Join(f.root, "release.json")
	if err := os.WriteFile(state, []byte("{\"isDraft\":"+draftText+",\"isPrerelease\":false,\"tagName\":\"v0.1.0\"}"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.environment = append(f.environment, "FLOWD_TEST_RELEASE_STATE="+state, "FLOWD_TEST_PUBLISH_LOG="+filepath.Join(f.root, "publish.log"), "FLOWD_TEST_ASSETS="+assets, "GH_TOKEN=fixture-not-a-credential")
	f.script("gh", `case "$1/$2" in
 release/view) cat "$FLOWD_TEST_RELEASE_STATE";;
 release/download)
  shift 2; destination=
  while [ "$#" -gt 0 ]; do case "$1" in --dir) destination=$2; shift 2;; *) shift;; esac; done
  [ -n "$destination" ] || exit 1
  cp "$FLOWD_TEST_ASSETS"/* "$destination/";;
 release/edit) printf '%s\n' published >> "$FLOWD_TEST_PUBLISH_LOG";;
 *) exit 1;; esac`)
	f.script("goreleaser", `printf '%s\n' built >> "$FLOWD_TEST_PUBLISH_LOG"`)
	f.script("go", `[ "$1" = run ] && [ "$2" = ./scripts/releasecheck ] || exit 1
shift 2
cd `+shellLiteral(projectRoot(t))+`
exec `+shellLiteral(realGo)+` run ./scripts/releasecheck "$@"`)
	f.script("tr", `exec /usr/bin/tr "$@"`)
	if err := os.Symlink("/bin/sh", filepath.Join(f.tools, "sh")); err != nil {
		t.Fatal(err)
	}
	return f, work
}

func TestPublishedRetryAndTagGuards(t *testing.T) {
	f, work := publisherFixture(t, false)
	run := func(tag string) (string, error) {
		command := exec.Command("/bin/sh", "scripts/publish-release.sh", tag)
		command.Dir = work
		command.Env = f.environment
		output, err := command.CombinedOutput()
		return string(output), err
	}
	if output, err := run("v0.1.0"); err != nil || !strings.Contains(output, "no assets or tags changed") {
		t.Fatalf("published retry: %v; %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(f.root, "publish.log")); !os.IsNotExist(err) {
		t.Error("published retry rebuilt or edited release")
	}
	for _, tag := range []string{"main", "v0.1.0-rc.1", "v0.1.0\nkey=value", "v9.9.9", "v0.1.0;touch nope"} {
		if output, err := run(tag); err == nil {
			t.Errorf("unsafe/unavailable tag %q accepted: %s", tag, output)
		}
	}
	archive := filepath.Join(f.assets, "v0.1.0", "flowd_0.1.0_linux_arm64.tar.gz")
	before, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("corrupt published asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := run("v0.1.0"); err == nil {
		t.Errorf("conflicting published asset accepted: %s", output)
	}
	after, err := os.ReadFile(archive)
	if err != nil || string(after) != "corrupt published asset" {
		t.Error("retry replaced conflicting published bytes")
	}
	if len(before) == 0 {
		t.Fatal("fixture empty")
	}
	if _, err := os.Stat(filepath.Join(f.root, "publish.log")); !os.IsNotExist(err) {
		t.Error("failed published retry rebuilt or edited release")
	}
}

func TestDraftPublicationGate(t *testing.T) {
	f, work := publisherFixture(t, true)
	run := func() (string, error) {
		command := exec.Command("/bin/sh", "scripts/publish-release.sh", "v0.1.0")
		command.Dir = work
		command.Env = f.environment
		output, err := command.CombinedOutput()
		return string(output), err
	}
	archive := filepath.Join(f.assets, "v0.1.0", "flowd_0.1.0_linux_arm64.tar.gz")
	saved, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}
	if output, err := run(); err == nil {
		t.Fatalf("incomplete draft published: %s", output)
	}
	log, _ := os.ReadFile(filepath.Join(f.root, "publish.log"))
	if strings.Contains(string(log), "published") {
		t.Error("publication preceded complete verification")
	}
	if err := os.WriteFile(archive, saved, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := run(); err != nil {
		t.Fatalf("complete draft failed: %v; %s", err, output)
	}
	log, _ = os.ReadFile(filepath.Join(f.root, "publish.log"))
	if !strings.Contains(string(log), "published") {
		t.Error("complete draft was not published")
	}
}
