# flowd

**Plan it. Lock in. Stay in flow.**

Most planners ask you to promise *when* you will do things, and by mid morning
the day has already broken the plan. flowd asks something easier: how much of
yourself do you want to give each thing today? An hour for study, thirty minutes
for that bug, two hours for the feature. Then start anything, in any order, and
let flowd keep count.

- **One thing at a time.** One key starts, pauses or finishes. Starting something
  else pauses what was running, so your attention stays on the work.
- **No nagging.** No alarms, no timers going off, no guilt when you run over.
- **Honest numbers.** You see the time you really spent, not the time you hoped
  to. The evening review tells you how the day actually went.
- **Every day starts fresh.** At midnight unfinished work stays in today's record,
  and tomorrow begins empty. Nothing carries over unless you decide it should.
- **Lives in your terminal.** Planning takes a couple of minutes and never pulls
  you out of the work you are in.
- **Yours alone.** One file on your computer. Offline, no account, nothing is
  ever sent anywhere.

It works just as well with an AI agent: ask it to plan or log your day, and it
uses flowd for you, safely.

## Install

flowd runs on macOS and Linux, on Intel and ARM. You need curl, tar, gzip and `shasum` or
`sha256sum`. You do not need Go or sudo.

```sh
curl -fsSL https://github.com/raqeebshafeeque/flowd/releases/latest/download/install.sh | sh
```

The installer puts `flowd` in `~/.local/bin` and the Agent Skill in `~/.agents/skills/flowd`. It
checks the download against the release checksums before installing anything, and it tells you if
`~/.local/bin` is not on your PATH yet. Run the same command again to update.

Every release is signed with build provenance that proves it was built by this repository. If the
[GitHub CLI](https://cli.github.com) is installed and signed in, the installer checks that proof too.
To refuse installing without it, download the installer, check it, and run it with
`--require-attestation`:

```sh
curl -fsSLO https://github.com/raqeebshafeeque/flowd/releases/latest/download/install.sh
gh attestation verify install.sh --repo raqeebshafeeque/flowd
sh install.sh --require-attestation
```

## Use it

```sh
flowd add 1h "Study"        # plan an item with a duration
flowd add 2h "Code review"
flowd start 1               # start working on Study
flowd list                  # see the day
flowd start 2               # switch to Code review, which pauses Study
flowd finish 2              # done with Code review for today
flowd review                # totals, progress and every item by status
```

- A duration is a budget, not a time slot. Write it as `30`, `30m`, `2h` or `1h30m`.
- The durations planned on a day cannot add up to more than the time left in it. flowd tells you how
  much is left when something does not fit.
- Work is the time an item actually ran. Elapsed is the time since it first started, pauses
  included.
- Starting an item pauses the one that was running.
- Titles are visible text. Hidden or direction-changing characters are refused, so a title always
  reads the way it is stored, for you and for an AI agent.
- Items you started are listed first, in the order you started them. Commands take an item's row
  number or its ID.
- At midnight anything unfinished becomes incomplete. Tomorrow starts empty.
- Only today can be tracked. Today and later days can be planned. Past days are for looking back.

Run `flowd` with no arguments for the terminal workspace. Move with `j` and `k`, open the details
of an item with `enter`, close them with `esc` and quit with `q`. Space starts, pauses or resumes
the selected item, `f` finishes it, `s` skips it, `u` brings back a skipped item and `R` reopens a
finished one. `n` adds an item and `e` edits the selected one: type the title, press Tab, type the
duration and save with Ctrl+S. `c` opens a calendar to look at or plan another day, `t` comes back to
today, `r` shows the review, `d` deletes the selected item after asking, and `?` lists every key. The window needs to be at least 80
by 24.

`flowd delete 2` asks before deleting, because a deleted item cannot be brought back. Scripts and
agents have to add `--yes`.

Add `--json` to any command for output that scripts can read, and `--date 2026-11-20` to work on
another day. `flowd --help` lists every command.

## Use it with an AI agent

The installer also adds an Agent Skill, so an agent that supports skills can plan and track your
day when you ask it to. It reads the day as JSON, changes only what you asked for, and treats item
titles as plain data. Reload your agent after installing to pick it up.

## Your data

The database is `$XDG_DATA_HOME/flowd/flowd.sqlite`, or `~/.local/share/flowd/flowd.sqlite` when
`XDG_DATA_HOME` is not set to an absolute path. Use `--db PATH` to pick another file.

```sh
flowd backup              # write flowd.sqlite.bak next to the database
flowd doctor              # check the database, prints OK or the problem
flowd doctor --repair     # put the backup back in place
```

Repair checks the backup first and never deletes your current files. It moves them into a
recovery folder next to the database.

## Build from source

You need Go 1.27.4 or newer.

```sh
go build -o flowd ./cmd/flowd
go test ./...
```

## More

- [Vision](docs/00-vision.md)
- [Requirements](docs/01-requirements.md)
- [Domain](docs/02-domain.md)
- [Architecture](docs/03-architecture.md)

flowd is licensed under the [Apache License 2.0](LICENSE). `flowd --licenses` prints the licenses of the libraries it is
built with.
