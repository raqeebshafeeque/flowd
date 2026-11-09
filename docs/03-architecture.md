# Architecture - flowd

Where new code lives, how the parts connect, and what happens when things break.

## What matters here

- One static file that opens the moment it gets called, fully offline.
- Rules live in one place so screen code stays thin.
- A killed app comes back whole, and a broken file says so plainly.

## Limits accepted

- Go on Mac and Linux terminals first.
- No network, no background service, no second app.
- SQLite is the only store. File lives at `$XDG_DATA_HOME/flowd/flowd.sqlite`, else
  `~/.local/share/flowd/flowd.sqlite`, mode 0600.
- One writer. A second open copy gets a clear lock error. No merge.

## The big picture

Someone in a terminal plans and tracks the day. flowd reads the system clock for now and next, and
reads and writes one local SQLite file plus its `.bak` beside it. Nothing touches the network.

## Inside the binary

One binary with four small parts:

- TUI: draws the day with now, next and review, and takes keys. No SQL here and no rule math here.
- Domain: checks the D1 to D8 rules and works out now, next, overrun and score. Pure logic, no file
  or screen calls.
- Store: owns SQLite for `days`, `blocks` and `block_events`. Owns saves, schema version in `PRAGMA
  user_version` and moves between versions. Table SQL lives in `internal/store/schema.sql`.
- Scheduler: a tiny now and next calc from Store plus the clock with a 1 second tick. No saves, no
  background service.

Call direction, enforced by folders `internal/tui, internal/domain, internal/store,
internal/scheduler`:

```text
tui -> domain -> store
scheduler -> store (read only)
domain never calls tui, store never calls domain
```

Every write walks screen to rules to save. The screen never writes SQL around the rules.

On the command line: `flowd` opens today, `flowd YYYY-MM-DD` opens that date, `flowd doctor` checks
the store, `flowd --help` lists keys and commands.

## How rows get saved

Day persistence uses `CreateDay`, `DayByDate` and `DayByID` in `internal/store/day.go`.
Creation never replaces an existing id or date. Missing lookups return `ErrDayNotFound`;
other failures preserve their underlying causes. Reads retain saved identities and UTC timestamps.
`BlockByID` includes archived rows; `BlocksByDay` lists only visible rows on the selected day.
Planned order compares parsed instants, so whole-second and fractional timestamp text can coexist.

- SQLite in WAL mode with `fsync` per save. Tables are `days`, `blocks` and `block_events` exactly
  as [domain.md](02-domain.md) shapes them.
- One keypress means one save. The block change plus its diary row land together or neither lands.
- Schema version sits in `PRAGMA user_version`. Moves go forward only at start, with a `.bak` copy
  taken first. Only one `.bak` is kept; each migrate overwrites it.
- Indexes on `blocks(day_id, planned_start)` and `block_events(block_id, seq)`.
- File locking comes from SQLite itself. A second writer hears `ERR_LOCKED` in plain words.

## A keypress, start to finish

Start: a start key arrives. Domain checks overlap and the one runner rule. Store saves block plus
diary row in one go. The screen works out now and next again and draws.

Fresh open: the file opens or gets made, version gets checked, schema moves forward with a backup
when needed, today loads by local date and the screen draws.

After a kill: the file opens, a quick `integrity_check` runs, the last saved state loads and draws.
Only the key pressed mid kill can be missing.

Clock watch: now and next get worked out on every key plus once a second while idle. A jump past 30
seconds causes one calm recalc, never a burst.

## Time

- UTC in RFC 3339 on disk, local zone on screen through the day zone.
- The day flips on local date at draw time. Blocks never slide alone. Only an explicit copy moves
  one, per the carryover rules in [requirements.md](01-requirements.md) (F4) and [domain.md](02-domain.md) (D7).

## Errors and the doctor

- Errors read like `ERR_SOMETHING` plus plain words, on stderr and as a human line on screen. Codes
  used: `ERR_OVERLAP`, `ERR_ILLEGAL_TRANSITION`, `ERR_LOCKED`, `ERR_CORRUPT`, `ERR_DISK_FULL`, `ERR_VERSION`.
- Logs go to stderr. No tracking, nothing personal leaves the file.
- `flowd doctor` checks path and permissions, `integrity_check`, schema version and a lock probe.
  Prints `OK` or the first problem with a fix hint. Exit 0 on OK, else 1. Data changes only with
  explicit `--repair`, which brings back the `.bak`.

## When things break

| What happens | How it shows | What the app does | How to test it |
|---|---|---|---|
| Kill mid keypress with `SIGKILL` | A save is atomic | Last saved state stays whole, at most one key lost | Loop writes with `kill -9`, reopen and diff |
| Disk full | Write fails | That save fails, old data stays, plain `ERR_DISK_FULL` shows | Fill a small tmpfs and try a write |
| Clock jumps | Jump past 30s | One recalc, no flood | Step a fake clock |
| SQLite broken | Open or check fails | Print `ERR_CORRUPT` with path, point at doctor, touch nothing | Flip bytes in a copy and open it |
| Second copy open | SQLite lock | Second copy exits with `ERR_LOCKED` | Open twice in a test |
| Schema newer than app | Version check | Refuse with `ERR_VERSION` and an upgrade hint | Bump a version fixture |

## Choices made

- SQLite over plain JSON: a block plus its diary row must land together with ordered reads and no
  hand file code. WAL with one writer fits an offline day tool.
- One binary over a background service: pings while closed sit out of scope, so a split would buy
  IPC pain for nothing.
- Diary as audit, not source: live state sits in `blocks`. Replay stays a debug path, not the read
  path. Reads stay simple.

## Left out on purpose

Sync, sharing, calendar import, pings and deep history search. All out of scope in
[requirements.md](01-requirements.md). No hidden hooks for them.
