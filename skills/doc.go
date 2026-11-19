// Package skills holds the Agent Skill that teaches an AI agent to use
// flowd, embedded into the program so flowd skill always installs the skill
// that matches the installed version.
package skills

import "embed"

// Flowd is the flowd skill: SKILL.md and its command reference.
//
//go:embed flowd/SKILL.md flowd/references/commands.md
var Flowd embed.FS
