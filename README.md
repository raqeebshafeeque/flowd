# flowd

A local-only terminal day planner. One binary, fully offline; no sync or background service.

## Install

Public downloads require a published release. On macOS/Linux (arm64 or amd64), installation needs
curl, tar, gzip, standard shell utilities and `sha256sum` or `shasum`, not Go, GitHub credentials
or sudo.

```sh
installer=$(mktemp)
curl --proto '=https' --proto-redir '=https' -fsSL \
  https://github.com/raqeebshafeeque/flowd/releases/latest/download/install.sh -o "$installer"
# Inspect the downloaded script before running it.
sh "$installer"
export PATH="$HOME/.local/bin:$PATH"
flowd --version
```

The installer verifies archive checksums and installs both `~/.local/bin/flowd` and the matching
`~/.agents/skills/flowd/`. HTTPS/bootstrap trust is required; same-channel checksums are not
independent publisher authentication. Installation never operates on planner data. Project and
third-party license notices are retained in the installed skill directory.

## Update or pin

Download the installer again for an update, then reuse its path in the same shell:

```sh
sh "$installer"
sh "$installer" --version v0.1.0
```

Use `--bin-dir DIR` and `--skills-dir DIR` to override parent directories. Unmanaged collisions
need review before `--force`; symlink/special-file destinations are always refused. Managed skill
edits are replaced on upgrade. Failed restoration reports recovery paths; replacement across the
two destinations is not crash-atomic. Concurrent installers are refused; an interrupted install
may leave an install lock needing inspection. See [architecture](docs/03-architecture.md) for
recovery.

## Use

```sh
flowd --help
flowd --date 2026-10-14 add 09:00 10:00 "Study"
flowd --date 2026-10-14 start 1
flowd --date 2026-10-14 finish 1
flowd --date 2026-10-14 review
flowd --json now
```

Global options precede the command. `--db PATH` selects a database; `--timezone Asia/Kolkata`
chooses a new day's zone. Saved days retain their zone. Full block ids are stable; row numbers
change. `delete` archives, `copy` explicitly carries unfinished work to the next calendar day,
and `watch` refreshes until Ctrl-C. `doctor` inspects; `doctor --repair` requires deliberate
approval to discard newer changes and restore `.bak`, preserving originals. Never remove lock
or incomplete-repair files manually. See `--help` and the docs for all commands.

Data lives at `$XDG_DATA_HOME/flowd/flowd.sqlite`, otherwise
`~/.local/share/flowd/flowd.sqlite`.

## Agents

The portable source is in `skills/flowd/`. A compatible harness must discover the installed
Agent Skills directory; in Pi, use `/reload` then `/skill:flowd` to select it explicitly.
Source-checkout use requires explicit developer intent and a chosen binary. Skills do not enforce
permissions. Planner data shown to an external-model agent enters its context/provider policy.

## Development

Building source is optional for contributors, not an installer fallback. Use Go 1.27.3 or newer:

```sh
go build -o flowd ./cmd/flowd
```

## Docs

- [Vision](docs/00-vision.md)
- [Requirements](docs/01-requirements.md)
- [Domain rules](docs/02-domain.md)
- [Architecture](docs/03-architecture.md)
