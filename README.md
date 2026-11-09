# flowd

A local-only terminal day planner. One binary, fully offline; no sync or background service.

## Run

```sh
go build -o flowd ./cmd/flowd
./flowd --help
./flowd
./flowd --date 2026-10-14 add 09:00 10:00 "Study"
./flowd --date 2026-10-14 edit --title "Deep study" 1
./flowd --date 2026-10-14 start 1
./flowd --date 2026-10-14 finish 1
./flowd --date 2026-10-14 events 1
./flowd --date 2026-10-14 review
./flowd --date 2026-10-14 list
./flowd --json now
```

Global options go before the command. `--db PATH` chooses a database; `--timezone Asia/Kolkata`
chooses a new day's zone. Existing days retain their saved timezone. Empty day views create no
planner rows. `delete BLOCK` archives without erasing history. A block selector is a visible row
number, full id or unique visible-id prefix. Row numbers change; use full ids in scripts.
`copy [--start TIME] [--end TIME] BLOCK` copies unfinished work to the next calendar day, keeping
local clock times unless overridden. Use `24:00` only for an end, or an offset RFC 3339 timestamp
for an overnight end or a repeated DST clock. Missing/repeated local clocks are refused.
`review` excludes skipped work from progress; a never-started todo is missed after local day end.
`doctor` checks an existing database without creating data or changing its permissions.
`backup` writes a private `.bak` SQLite snapshot, including committed WAL contents. A failed
publication leaves the previous snapshot; a reported directory-sync failure may occur after the
new snapshot is already published. These checks are not a physical power-loss test.
`doctor --repair` explicitly restores `.bak`, discarding newer live changes and preserving old
main/journal files in the reported private recovery directory. An interrupted repair blocks normal
opening; run the same repair command to finish. Do not remove `.lock` or `.repair` files manually.
Close external SQLite tools before repair; they do not participate in flowd ownership.
`watch` refreshes once a second and releases database ownership between snapshots. Ctrl-C exits
cleanly. Implicit dates follow local midnight; `--date` pins the view. Pipe output stays plain;
`--json watch` emits one complete snapshot per line. See `--help` for command details.

Data lives at `$XDG_DATA_HOME/flowd/flowd.sqlite`, otherwise
`~/.local/share/flowd/flowd.sqlite`.

## Agents

The portable skill source is in `skills/flowd/`. Released packages install it at
`~/.agents/skills/flowd/` alongside a matching executable. A compatible harness must discover
that directory; in Pi, use `/reload` after installation and `/skill:flowd` to select it explicitly.
Source-checkout use is a developer workflow and requires an explicitly chosen binary.
Skill instructions do not enforce permissions. Planner data shown to an external-model agent
enters its context and follows the provider's policy.

## Docs

- [Vision](docs/00-vision.md)
- [Requirements](docs/01-requirements.md)
- [Domain rules](docs/02-domain.md)
- [Architecture](docs/03-architecture.md)
