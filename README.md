<p align="center">
  <img src="docs/assets/flowd-icon.png" alt="flowd" width="128">
</p>

<h1 align="center">flowd</h1>

<p align="center">
  <strong>Plan it. Lock in. Stay in flow.</strong>
</p>

<p align="center">
  A terminal first daily planner for focused work.
</p>

<p align="center">
  <a href="https://github.com/raqeebshafeeque/flowd/releases/latest"><img src="https://img.shields.io/github/v/release/raqeebshafeeque/flowd?color=teal" alt="Latest release"></a>
  <a href="https://github.com/raqeebshafeeque/flowd/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/raqeebshafeeque/flowd/ci.yml?branch=main&label=CI" alt="CI"></a>
  <img src="https://img.shields.io/badge/macOS%20%7C%20Linux-informational" alt="macOS and Linux">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue" alt="Apache 2.0"></a>
</p>

<p align="center">
  <a href="#install">Install</a> &nbsp;&nbsp;
  <a href="#quick-start">Quick start</a> &nbsp;&nbsp;
  <a href="#the-workspace">The workspace</a> &nbsp;&nbsp;
  <a href="#with-an-ai-agent">AI agents</a> &nbsp;&nbsp;
  <a href="#learn-more">Docs</a>
</p>

<br>

Most planners ask you to promise *when* you will do things, and by mid morning the day has
already broken the plan. flowd asks something easier: **how much of yourself do you want to give
each thing today?**

An hour for study, thirty minutes for that bug, two hours for the feature. Then start anything, in
any order, and let flowd keep count.

```
┌───────┬─────────────────────────────────────────┬─────────────────┬──────────┐
│ flowd │                                         │ Tue 17 Nov 2026 │ 10:59:57 │
├───────┴─────────────────────────────────────────┴─────────────────┴──────────┤
│   Review open pull requests                           38m / 45m         DONE │
│ > Deep work on the parser                          1h3m54s / 2h  IN PROGRESS │
│   Read the scheduler paper                           18m3s / 1h       PAUSED │
│   Write release notes                                       30m         TODO │
│   Inbox and messages                                        20m         TODO │
│                                                                              │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│ pause [space]  finish [f]  skip [s]  new [n]  help [?]  quit [q]             │
└──────────────────────────────────────────────────────────────────────────────┘
```

<br>

## Why flowd

- **One thing at a time.** One key starts, pauses or finishes. Starting something else pauses what
  was running.
- **No nagging.** No alarms, no timers going off, no guilt when you run over.
- **Honest numbers.** You see the time you really spent, not the time you hoped to.
- **Every day starts fresh.** At midnight unfinished work is marked incomplete and tomorrow begins
  empty.
- **Lives in your terminal.** Planning takes a minute and never pulls you out of the work.
- **Yours alone.** One file on your computer. Offline, no account, nothing is ever sent anywhere.

<br>

## Install

```sh
curl -fsSL https://github.com/raqeebshafeeque/flowd/releases/latest/download/install.sh | sh
```

Runs on macOS and Linux, Intel and ARM. No Go and no sudo needed. Run it again any time to update.

<details>
<summary>What the installer does, and how to verify it</summary>

<br>

The installer puts `flowd` in `~/.local/bin` and the Agent Skill in `~/.agents/skills/flowd`. It
needs curl, tar, gzip and `shasum` or `sha256sum`, checks the download against the release
checksums before installing anything, and tells you if `~/.local/bin` is not on your PATH yet.

Every release is signed with build provenance. If the [GitHub CLI](https://cli.github.com) is
installed and signed in, the installer checks that proof too. To refuse installing without it:

```sh
curl -fsSLO https://github.com/raqeebshafeeque/flowd/releases/latest/download/install.sh
gh attestation verify install.sh --repo raqeebshafeeque/flowd
sh install.sh --require-attestation
```

</details>

<br>

## Quick start

```sh
flowd add 1h "Study"        # plan an item, which prints its ID
flowd add 2h "Code review"
flowd list                  # see the day, with every item's ID
flowd start <ID>            # start an item by its ID
flowd finish <ID>           # done with it for today
flowd review                # how the day really went
```

A few things to know:

- **Duration** is a budget, not a time slot. Write it as `30`, `30m`, `2h` or `1h30m`.
- **Work** is the time an item actually ran. **Elapsed** is the time since it first started,
  pauses included.
- **Only today** can be tracked. Today and later days can be planned. Past days are for looking
  back.

Add `--json` to any command for output scripts can read, and `--date 2026-11-20` to work on
another day. `flowd --help` lists everything.

<br>

## The workspace

Run `flowd` with no arguments to open it. Everything is one key.

| Key | Action | Key | Action |
| :---: | --- | :---: | --- |
| `j` `k` | Move up and down | `n` | New item |
| `space` | Start, pause or resume | `e` | Edit the selected item |
| `f` | Finish | `d` | Delete, after asking |
| `s` `u` | Skip and bring back | `c` | Calendar, to see or plan another day |
| `R` | Reopen a finished item | `t` | Back to today |
| `enter` | Details | `r` | Review of the day |
| `?` | Every key | `q` | Quit |

In a form, type the title, press Tab for the duration and save with Ctrl+S. The window needs to
be at least 80 by 24.

<br>

## With an AI agent

The installer also adds an Agent Skill, so an agent that supports skills can plan and track your
day when you ask it to. It reads the day as JSON, changes only what you asked for, and treats item
titles as plain data. Reload your agent after installing to pick it up.

<br>

## Your data

Everything lives in one SQLite file at `~/.local/share/flowd/flowd.sqlite`, or under
`$XDG_DATA_HOME` when that is set to an absolute path. Use `--db PATH` to pick another file.

```sh
flowd backup            # write a backup next to the database
flowd doctor            # check the database
flowd doctor --repair   # put the backup back, keeping the current files aside
```

<br>

## Learn more

- [Vision](docs/00-vision.md) for why flowd exists
- [Requirements](docs/01-requirements.md) for everything it does
- [Domain](docs/02-domain.md) for the rules a day follows
- [Architecture](docs/03-architecture.md) for how it is built

To build from source you need Go 1.27.4 or newer, then `go build -o flowd ./cmd/flowd`.

<br>

## License

flowd is licensed under the [Apache License 2.0](LICENSE). `flowd --licenses` prints the licenses
of the libraries it is built with.
