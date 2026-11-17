# flowd

A daily planner for people who live in the terminal. Write down what you want to put effort into
today, record the work as you do it, and see at the end of the day what really happened.

flowd keeps everything in one SQLite file on your computer. It works offline, has no account and
sends nothing anywhere.

## Install

flowd runs on macOS and Linux, on Intel and ARM. You need curl, tar, gzip and `shasum` or
`sha256sum`. You do not need Go or sudo.

```sh
curl -fsSL https://github.com/raqeebshafeeque/flowd/releases/latest/download/install.sh | sh
```

The installer puts `flowd` in `~/.local/bin` and the Agent Skill in `~/.agents/skills/flowd`. It
checks the download against the release checksums before installing anything, and it tells you if
`~/.local/bin` is not on your PATH yet. Run the same command again to update.

## Use it

```sh
flowd add 1h "Study"        # plan an item with a duration
flowd add 2h "Code review"
flowd start 1               # start working on it
flowd                       # see the day
flowd pause 1
flowd finish 2
flowd review                # totals, progress and every item by status
```

- A duration is a budget, not a time slot. Write it as `30`, `30m`, `2h` or `1h30m`.
- Work is the time an item actually ran. Elapsed is the time since it first started, pauses
  included.
- Starting an item pauses the one that was running.
- Items you started are listed first, in the order you started them. Commands take an item's row
  number or its ID.
- At midnight anything unfinished becomes incomplete. Tomorrow starts empty.
- Only today can be tracked. Today and later days can be planned. Past days are for looking back.

Run `flowd` with no arguments for the terminal workspace. Move with `j` and `k`, open the details
of an item with `enter`, close them with `esc` and quit with `q`. The window needs to be at least 80
by 24.

Add `--json` to any command for output that scripts can read, and `--date 2026-11-20` to work on
another day. `flowd --help` lists every command.

## Use it with an AI agent

The installer also adds an Agent Skill, so an agent that supports skills can plan and track your
day when you ask it to. It reads the day as JSON, changes only what you asked for, and treats item
titles as plain data. Reload your agent after installing to pick it up.

## Your data

The database is `$XDG_DATA_HOME/flowd/flowd.sqlite`, or `~/.local/share/flowd/flowd.sqlite` if that
variable is not set. Use `--db PATH` to pick another file.

```sh
flowd backup              # write flowd.sqlite.bak next to the database
flowd doctor              # check the database, prints OK or the problem
flowd doctor --repair     # put the backup back in place
```

Repair checks the backup first and never deletes your current files. It moves them into a
recovery folder next to the database.

## Build from source

```sh
go build -o flowd ./cmd/flowd
go test ./...
```

## More

- [Vision](docs/00-vision.md)
- [Requirements](docs/01-requirements.md)
- [Domain](docs/02-domain.md)
- [Architecture](docs/03-architecture.md)

flowd is [MIT licensed](LICENSE). The licenses of the libraries it uses are in
[THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES).
