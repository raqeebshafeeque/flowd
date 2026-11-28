package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/raqeebshafeeque/flowd/internal/planner"
	"github.com/raqeebshafeeque/flowd/skills"
)

// agent is a harness the skill can be installed for, and the folder under
// the home folder where it looks for skills.
type agent struct{ flag, name, folder string }

var agents = []agent{
	{"claude", "Claude Code", ".claude/skills"},
	// Codex, OpenCode, Pi and Gemini CLI all read the shared agents folder.
	{"agents", "Codex, OpenCode, Pi and Gemini CLI", ".agents/skills"},
}

// skillMarker starts the SKILL.md of the flowd skill. A folder that holds
// anything else is never replaced.
const skillMarker = "---\nname: flowd\n"

// installSkill writes the embedded skill for each agent that was asked for.
// Each folder is replaced whole, and only when it holds the flowd skill or
// nothing, so an unrelated skill of the same name is never overwritten.
func installSkill(ctx context.Context, out io.Writer, o options, env Env) error {
	version := env.Version
	if len(o.agents) == 0 {
		if o.json || !env.Interactive {
			return planner.InputError(nil, "Choose which agents get the skill", "Use "+agentFlags()+", or run flowd skill at a terminal to pick")
		}
		run := env.pick
		if run == nil {
			run = runPicker(ctx, env, out)
		}
		final, err := run(newAgentPicker(o.look))
		if err != nil {
			return err
		}
		p, ok := final.(agentPicker)
		if !ok || !p.chosen {
			return nil
		}
		o.agents = map[string]bool{}
		for i, on := range p.on {
			if on {
				o.agents[agents[i].flag] = true
			}
		}
		if len(o.agents) == 0 {
			_, err := fmt.Fprintln(out, "No agent was chosen, so nothing was installed")
			return err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return planner.InputError(err, "Cannot find your home folder", "Set HOME and try again")
	}
	type installed struct {
		Agent string `json:"agent"`
		Path  string `json:"path"`
	}
	var done []installed
	for _, a := range agents {
		if !o.agents[a.flag] {
			continue
		}
		target := filepath.Join(home, a.folder, "flowd")
		if err := writeSkill(target, version); err != nil {
			return err
		}
		done = append(done, installed{a.flag, target})
		if !o.json {
			fmt.Fprintf(out, "Installed the flowd skill for %s in %s\n", a.name, tilde(home, target))
		}
	}
	if o.json {
		return writeJSON(out, map[string]any{"installed": done})
	}
	_, err = fmt.Fprintln(out, "Reload your agent to load it.")
	return err
}

func writeSkill(target, version string) (err error) {
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return planner.Refusal(errors.Join(planner.ErrInput, fs.ErrExist), target+" is not a folder flowd can replace", "Move it away, then install the skill again")
		}
		existing, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
		if err != nil || !strings.HasPrefix(string(existing), skillMarker) {
			return planner.Refusal(errors.Join(planner.ErrInput, fs.ErrExist), target+" holds something that is not the flowd skill", "Move it away, then install the skill again")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	// Write the new skill beside the old one and swap them, so the folder
	// is never left half written.
	stage, err := os.MkdirTemp(parent, ".flowd-skill-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(stage)) }()
	fresh := filepath.Join(stage, "new")
	err = fs.WalkDir(skills.Flowd, "flowd", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		into := filepath.Join(fresh, strings.TrimPrefix(path, "flowd"))
		if entry.IsDir() {
			return os.MkdirAll(into, 0o755)
		}
		data, err := skills.Flowd.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(into, data, 0o644)
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(fresh, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
		return err
	}
	old := filepath.Join(stage, "old")
	if err := os.Rename(target, old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Rename(fresh, target); err != nil {
		if _, statErr := os.Stat(old); statErr == nil {
			err = errors.Join(err, os.Rename(old, target))
		}
		return err
	}
	return nil
}

// tilde shortens a path under the home folder to ~/..., for messages.
func tilde(home, path string) string {
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}

// agentFlags lists the agent flags for messages: --claude or --agents.
func agentFlags() string {
	var flags []string
	for _, a := range agents {
		flags = append(flags, "--"+a.flag)
	}
	return strings.Join(flags[:len(flags)-1], ", ") + " or " + flags[len(flags)-1]
}

// agentPicker asks which agents get the skill. Up and down move, Space
// ticks one, Enter installs for the ticked ones, and Esc, q or Ctrl+C cancel.
type agentPicker struct {
	on      []bool
	cursor  int
	chosen  bool
	closing bool
	look    look
}

func newAgentPicker(l look) agentPicker { return agentPicker{on: make([]bool, len(agents)), look: l} }

func (p agentPicker) Init() tea.Cmd { return nil }

func (p agentPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "up", "k", "ctrl+p":
			p.cursor = (p.cursor + len(agents) - 1) % len(agents)
		case "down", "j", "ctrl+n", "tab":
			p.cursor = (p.cursor + 1) % len(agents)
		case "space", " ", "x":
			p.on = append([]bool(nil), p.on...)
			p.on[p.cursor] = !p.on[p.cursor]
		case "enter":
			p.chosen, p.closing = true, true
			return p, tea.Quit
		case "esc", "q", "ctrl+c":
			p.closing = true
			return p, tea.Quit
		}
	}
	return p, nil
}

func (p agentPicker) View() tea.View {
	if p.closing {
		return tea.NewView("")
	}
	rows := []string{p.look.paint("\x1b[1m", "Install the skill for which agents?") + p.look.paint("\x1b[2m", "  space to tick, enter to install, esc to cancel")}
	for i, a := range agents {
		box := "[ ]"
		if p.on[i] {
			box = "[x]"
		}
		row := "  " + box + " " + a.name
		if i == p.cursor {
			row = p.look.paint("\x1b[36m", "> "+box+" "+a.name)
		}
		rows = append(rows, row)
	}
	return tea.NewView(strings.Join(rows, "\n"))
}
