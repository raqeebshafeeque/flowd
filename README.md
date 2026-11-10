# flowd

Plan your day in the terminal, work through your tasks, and see what got done.

flowd stores your plan on your computer in a SQLite database. It works offline, without an
account or background service. Use it directly or through an agent that supports Agent Skills.

## Install

flowd supports macOS and Linux on arm64 and amd64. You'll need curl, tar, gzip and either
`shasum` or `sha256sum`. You don't need Go or sudo.

Download and run the installer:

```sh
curl -fsSL https://github.com/raqeebshafeeque/flowd/releases/latest/download/install.sh | sh
```

The installer shows progress while selecting, downloading, verifying and installing the release.
It prints the executable and skill locations, then confirms PATH is set or tells you which folder
to add. It does not change your shell settings. Reload your AI harness to load the flowd skill,
and check the installed version with `flowd --version`.

## Plan your day

Add blocks with a start time, an end time and a title:

```sh
flowd add 09:00 10:00 "Study"
flowd add 10:00 11:00 "Code"
flowd list
```

Commands use today by default. flowd checks for overlapping times when you add or edit a block.

When you begin working, start the block using its row number. Finish it when you're done:

```sh
flowd start 1
flowd finish 1
flowd review
```

Use `flowd pause 1` and `flowd resume 1` to take a break. Starting another block automatically
pauses the one already running on that day.

`flowd now` shows your current work. `flowd next` shows what's coming up. `flowd watch` keeps the
plan on screen until you press Ctrl+C.

To view another day, use `flowd --date 2027-01-15 list`. Global options go before the command.
You can choose a new day's timezone with `--timezone Asia/Kolkata`. Saved days keep their timezone.

For scripts, use `flowd --json list` and full block ids instead of row numbers, which can change.
Run `flowd --help` for more commands, including editing, archiving and copying unfinished work.

## Use an agent

The installer puts the flowd skill in `~/.agents/skills/flowd/`. It guides your agent through the
same CLI commands.

In Pi, reload the skills and ask for your plan:

```text
/reload
/skill:flowd Show my plan for today
```

Other agents may need `~/.agents/skills` added to their skill settings. If your agent uses an
external AI service, planner data it reads also enters that service's context.

## Where your files live

### Database

flowd chooses the database in this order:

1. The path passed with `--db`.
2. `$XDG_DATA_HOME/flowd/flowd.sqlite`, if `XDG_DATA_HOME` is an absolute path.
3. `~/.local/share/flowd/flowd.sqlite` otherwise.

These defaults apply on both macOS and Linux. The database is created on your first planner
command, including `list`. Installation, help and version checks don't create it.

To select another database for a command:

```sh
flowd --db "$HOME/work-plan.sqlite" list
```

This doesn't move your existing plan or change the default for later commands. Relative paths
are resolved from your current working directory.

### Logs and history

There is no application log file or logs directory. Errors go to your terminal through stderr.
Block history is saved in the database and can be read with `flowd events 1`.

SQLite may create `-wal` and `-shm` files beside the database. A `.lock` file coordinates commands.
flowd and SQLite manage these files, so you don't need to remove them.

### Backups

```sh
flowd backup
flowd doctor
```

`backup` saves a copy beside the selected database with `.bak` added to its name, replacing the
previous backup. `doctor` checks the database without restoring anything.

Restoring a backup discards newer changes. See the
[recovery guide](docs/03-architecture.md#errors-and-the-doctor) before restoring one.

### Executable and skill

| File | Default location |
| --- | --- |
| Executable | `~/.local/bin/flowd` |
| Agent skill | `~/.agents/skills/flowd/` |

These paths use your home directory, not `XDG_DATA_HOME`. The installer accepts `--bin-dir DIR`
and `--skills-dir DIR` to choose other parent folders. If you choose another skill location,
add it to your agent's skill settings.

## Update

Run the command in the Install section again to get the latest release. It updates the executable
and skill together without changing your database. To choose a specific release:

```sh
curl -fsSL https://github.com/raqeebshafeeque/flowd/releases/latest/download/install.sh \
  | sh -s -- --version v0.1.0
```

Updates replace the installed skill files, so keep personal notes outside that folder. If the
installer finds files it doesn't recognize, check them before choosing `--force`.

The installer checks release archives against their checksums. Installation still relies on
GitHub and the publisher being trustworthy. See the
[installation notes](docs/03-architecture.md#distribution-downloads) for more details.

## Development

To build from source, use Go 1.27.3 or newer:

```sh
go build -o flowd ./cmd/flowd
go test ./...
```

## More about the project

* [Vision](docs/00-vision.md)
* [Requirements](docs/01-requirements.md)
* [Domain rules](docs/02-domain.md)
* [Architecture](docs/03-architecture.md)

flowd uses the [MIT license](LICENSE). Dependency notices are in
[THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES).
