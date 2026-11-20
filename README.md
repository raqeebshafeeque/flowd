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
  <a href="#tab-completion">Tab completion</a> &nbsp;&nbsp;
  <a href="#the-workspace">The workspace</a> &nbsp;&nbsp;
  <a href="#in-your-status-bar">Status bar</a> &nbsp;&nbsp;
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
│ pause [space]  finish [f]  edit [e]  delete [d]  details [enter]    more [.] │
└──────────────────────────────────────────────────────────────────────────────┘
```

<br>

## Why flowd

- **One thing at a time.** One key starts, pauses or finishes. Starting something else pauses what
  was running.
- **No nagging.** No alarms, no timers going off, no guilt when you run over.
- **Honest numbers.** You see the time you really spent, not the time you hoped to.
- **Every day starts fresh.** At midnight unfinished work is marked incomplete and tomorrow begins
  empty. Carry over only what still matters.
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

The installer puts `flowd` in `~/.local/bin`. It needs curl, tar, gzip and `shasum` or
`sha256sum`, checks the download against the release checksums before installing anything, and
tells you if `~/.local/bin` is not on your PATH yet. It then prints how to add the agent skill and
tab completion, and leaves both to you.

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
flowd carry                 # bring over what is left from the last 7 days
flowd add 1h "Study"        # plan an item
flowd add 2h "Code review"
flowd order                 # arrange the day in the order you will work it
flowd start                 # pick what to start with the arrow keys
flowd finish                # pick what you are done with for today
flowd list                  # see the day
flowd review                # how the day really went
```

Leave out the ID and flowd lets you pick the item with the arrow keys, offering only the items that
command fits. Scripts pass the full ID instead, like `flowd start it_01M7...`. Anything you put
together, like a new order or the items to carry, saves with Ctrl+S.

A few things to know:

- **Duration** is a budget, not a time slot. Write it as `30`, `30m`, `2h` or `1h30m`.
- **Work** is the time an item actually ran. **Paused** is the time it waited after you started
  it.
- **Only today** can be tracked. Today and later days can be planned. Past days are for looking
  back.

Add `--json` to any command for output scripts can read, and `--date 2026-11-20` to work on
another day. `flowd --help` lists everything.

<br>

## Tab completion

Press Tab to complete commands, options, dates and item IDs, with each item's title shown next to
its ID. `flowd start <Tab>` only offers what can be started. The installer prints the line for your
shell. Run it once:

```sh
# fish
mkdir -p ~/.config/fish/completions && flowd completion fish > ~/.config/fish/completions/flowd.fish

# zsh
echo 'source <(flowd completion zsh)' >> ~/.zshrc

# bash
echo 'source <(flowd completion bash)' >> ~/.bashrc
```

<br>

## The workspace

Run `flowd` with no arguments to open it. Everything is one key.

| Key | Action | Key | Action |
| :---: | --- | :---: | --- |
| `j` `k` | Move up and down | `a` | Add an item |
| `space` | Start, pause, resume or reopen | `e` | Edit the selected item |
| `f` | Finish | `d` | Delete, after asking |
| `enter` | Details | `o` | Put the day in your order |
| `r` | Review of the day | `C` | Carry over unfinished work |
| `.` | More keys in the footer | `c` `t` | Calendar, and back to today |
| `?` | Every key | `q` | Quit |

In a form, type the title, press Tab for the duration and save with Ctrl+S. A new order and the
items to carry save with Ctrl+S too. When the keys do not fit in the footer, `.` shows the rest.
The window needs to be at least 80 by 24.

<br>

## In your status bar

See what you are working on without opening anything. `flowd now --short` prints one line, and
nothing at all when nothing is running.

```
Deep work on the parser 1h3m/2h
Deep work on the parser 2h10m/2h +10m
```

Put it in tmux, your prompt or any bar that can run a command:

```sh
# tmux, in ~/.tmux.conf
set -g status-right '#(flowd now --short)  %H:%M'
```

```toml
# starship, in ~/.config/starship.toml
[custom.flowd]
command = "flowd now --short"
when = true
```

```fish
# fish, in ~/.config/fish/config.fish
function fish_right_prompt
    flowd now --short
end
```

It is built to be asked every second. It never takes the lock, never writes and never prints an
error, so it cannot get in the way of your real commands. Long titles are cut to 30 characters.

<br>

## With an AI agent

flowd comes with an Agent Skill, so an agent that supports skills can plan and track your day when
you ask it to. It reads the day as JSON, changes only what you asked for, and treats item titles as
plain data. Install it for the agents you use:

```sh
flowd skill --claude          # Claude Code, in ~/.claude/skills/flowd
flowd skill --codex           # Codex, in ~/.agents/skills/flowd
flowd skill --claude --codex  # both
```

Reload your agent to pick it up. Updating flowd later updates the skill wherever it is installed.

<br>

## Your data

Everything lives in one SQLite file at `~/.local/share/flowd/flowd.sqlite`, or under
`$XDG_DATA_HOME` when that is set to an absolute path. Use `--db PATH` to pick another file.

```sh
flowd backup            # write a backup next to the database
flowd doctor            # check the database
flowd doctor --repair   # put the backup back, keeping the current files aside
flowd reset             # start over with an empty planner, keeping the old files aside
```

<br>

## Learn more

- [Vision](docs/00-vision.md) for why flowd exists
- [Requirements](docs/01-requirements.md) for everything it does
- [Domain](docs/02-domain.md) for the rules a day follows
- [Architecture](docs/03-architecture.md) for how it is built
- [Later](docs/04-later.md) for what is planned next

To build from source you need Go 1.27.4 or newer, then `go build -o flowd ./cmd/flowd`.

<br>

## License

flowd is licensed under the [Apache License 2.0](LICENSE). `flowd --licenses` prints the licenses
of the libraries it is built with.
