package distribution

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/raqeebshafeeque/flowd/internal/domain"
)

func useRealPackages(t *testing.T, f *releaseFixture, directory, version string) {
	t.Helper()
	target := filepath.Join(f.assets, "v"+version)
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"checksums.txt", "flowd_" + version + "_" + runtimePlatform() + ".tar.gz"} {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
func runtimePlatform() string { return runtime.GOOS + "_" + runtime.GOARCH }

func TestCompleteDistributionWorkflow(t *testing.T) {
	first := noPublishPackages(t)
	second := noPublishVersion(t, "0.2.0")
	f := newReleaseFixture(t)
	useRealPackages(t, f, first, "0.1.0")
	useRealPackages(t, f, second, "0.2.0")
	if output, err := f.install("--version", "0.1.0"); err != nil {
		t.Fatalf("install: %v; %s", err, output)
	}
	binary := filepath.Join(f.home, ".local/bin/flowd")
	skill := filepath.Join(f.home, ".agents/skills/flowd")
	verifySkillCopy(t, skill)
	path := filepath.Join(f.root, "sandbox.sqlite")
	var created domain.Block
	run := func(args ...string) []byte {
		t.Helper()
		command := exec.Command(binary, append([]string{"--db", path, "--date", "2026-10-14", "--timezone", "UTC", "--json"}, args...)...)
		command.Dir = f.home
		command.Env = f.environment
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("installed planner: %v; %s", err, out)
		}
		return out
	}
	if err := json.Unmarshal(run("add", "09:00", "10:00", "Sandbox"), &created); err != nil {
		t.Fatal(err)
	}
	run("start", string(created.ID))
	run("review")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"0.1.0", "0.2.0"} {
		if output, err := f.install("--version", version); err != nil {
			t.Fatalf("reinstall/upgrade: %v; %s", err, output)
		}
		verifySkillCopy(t, skill)
	}
	installed, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil || string(installed) != "flowd 0.2.0\n" {
		t.Error("real upgrade version mismatch")
	}
	realMV, err := exec.LookPath("mv")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.tools, "mv")); err != nil {
		t.Fatal(err)
	}
	f.script("mv", `last= previous=
for arg do previous=$last; last=$arg; done
case "$previous" in */.flowd-skill.*/new) exit 1;; esac
exec `+shellLiteral(realMV)+` "$@"`)
	if output, err := f.install("--version", "0.1.0"); err == nil {
		t.Fatalf("failed-upgrade fixture succeeded: %s", output)
	}
	installed, err = exec.Command(binary, "--version").CombinedOutput()
	if err != nil || string(installed) != "flowd 0.2.0\n" {
		t.Error("real failed upgrade did not restore binary")
	}
	record, err := os.ReadFile(filepath.Join(skill, "VERSION"))
	if err != nil || string(record) != "0.2.0\n" {
		t.Error("real failed upgrade did not restore skill")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Error("distribution operations changed saved planner bytes")
	}
	var view struct{ Blocks []domain.Block }
	if err := json.Unmarshal(run("list"), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Blocks) != 1 || view.Blocks[0].ID != created.ID || view.Blocks[0].Status != domain.InProgress {
		t.Error("saved sandbox state did not survive distribution workflow")
	}
}

func verifySkillCopy(t *testing.T, directory string) {
	t.Helper()
	for _, name := range []string{"SKILL.md", "references/commands.md", "references/safety.md"} {
		actual, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(actual) != projectFile(t, "skills/flowd/"+name) {
			t.Errorf("installed reference differs: %s", name)
		}
	}
}

func TestPiGlobalSkillDiscovery(t *testing.T) {
	module := os.Getenv("FLOWD_PI_MODULE")
	if module == "" {
		t.Skip("set FLOWD_PI_MODULE for installed Pi discovery; no model call is made")
	}
	directory := noPublishPackages(t)
	f := newReleaseFixture(t)
	useRealPackages(t, f, directory, "0.1.0")
	if output, err := f.install("--version", "0.1.0"); err != nil {
		t.Fatalf("install: %v; %s", err, output)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(f.root, "unrelated-project")
	if err := os.Mkdir(unrelated, 0o700); err != nil {
		t.Fatal(err)
	}
	program := `import {pathToFileURL} from 'node:url';
import {join} from 'node:path';
const {DefaultResourceLoader}=await import(pathToFileURL(process.argv[1]).href);
const loader=new DefaultResourceLoader({cwd:process.cwd(),agentDir:join(process.env.HOME,'.pi','agent'),noExtensions:true,noPromptTemplates:true,noThemes:true,noContextFiles:true});
await loader.reload();
const result=loader.getSkills();
const flowd=result.skills.find(skill=>skill.name==='flowd');
if(!flowd || result.diagnostics.some(d=>d.path===flowd.filePath))throw new Error('flowd discovery failed');
console.log(flowd.filePath);`
	command := exec.Command(node, "--input-type=module", "-e", program, module)
	command.Dir = unrelated
	command.Env = f.environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Pi skill discovery: %v; %s", err, output)
	}
	if !strings.Contains(string(output), ".agents/skills/flowd/SKILL.md") {
		t.Error("Pi did not discover the global installed skill")
	}
	t.Log("installed Pi discovered the skill from an unrelated project; no agent prompt/model evaluation ran")
}
