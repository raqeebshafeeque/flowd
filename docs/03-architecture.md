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

One binary with small parts:

- CLI: parses flags and commands, coordinates domain operations and formats text or JSON.
  `cmd/flowd` handles signals and process exit; `internal/cli` accepts a clock and output writers.

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
cli -> domain
cli -> store
tui -> domain
tui -> store -> domain
scheduler -> store (read only)
domain never calls store or tui
```

Every write walks screen to rules to save. The screen never writes SQL around the rules.
Store reuses domain data types and pure final-plan validation; it does not choose transitions.
Creation with an initial event, grouped changes and archive are atomic. A new day needed by a
creation is saved in the same transaction. Automatic pause and start must use one grouped save.

On the command line: `flowd` opens today, `flowd YYYY-MM-DD` opens that date, `flowd doctor` checks
the store, `flowd --help` lists keys and commands.

CLI options precede the command. Read commands are `list` (the default), `now` and `next`.
`--date YYYY-MM-DD` or a positional date selects a day. `--db PATH` overrides the default file;
`--timezone ZONE` sets the zone for a new day and refuses conflicts with a saved zone. The default
is a detected IANA system zone, falling back to UTC. The binary embeds fallback timezone data.
`--json` produces parseable stdout, with null for absent now/next and an uncountable score.
Text shows local planned times, progress and overrun flags; read views do not insert planner rows.

`add START END TITLE` creates a todo block with its first history event, including a new day in
that transaction when needed. `edit --title/--start/--end BLOCK` saves all requested fields together;
an overlapping edit changes nothing. `delete BLOCK` archives and retains history; repeating a
full-id deletion is harmless. Visible row numbers are temporary selectors; full ids are stable.

`start`, `pause`, `resume`, `finish`, `skip` and `reopen` take a block selector. Commands enforce
which source statuses they accept; reopening a done block also requires the saved local date.
Starting, resuming or reopening pauses another runner in the same grouped transaction. One clock
instant stamps both events. `extend` adds 15 minutes without changing status, refusing overlaps.
`events` reads ordered history, including archived work selected by its full id. An output error
returns nonzero but does not undo a mutation already committed to SQLite.

## How rows get saved

Day persistence uses `CreateDay`, `DayByDate` and `DayByID` in `internal/store/day.go`.
Creation never replaces an existing id or date. Missing lookups return `ErrDayNotFound`;
other failures preserve their underlying causes. Reads retain saved identities and UTC timestamps.
`BlockByID` includes archived rows; `BlocksByDay` lists only visible rows on the selected day.
Planned order compares parsed instants, so whole-second and fractional timestamp text can coexist.

- SQLite uses WAL with `synchronous=FULL` to synchronize commits. Go does not separately sync the
  live database file. Tables follow [domain.md](02-domain.md).
- One keypress means one save. The block change plus its diary row land together or neither lands.
- Version 0 installs schema version 1 atomically; version 1 is reopened without a data rewrite.
  Other versions are refused. A future upgrade must take a backup before changing existing data.
- Indexes on `blocks(day_id, planned_start_utc)` and `block_events(block_id, seq)`.
- SQLite still manages transaction locks. A private persistent `.lock` sibling also carries a
  nonblocking OS lock, covering the whole command and future offline repair. Never delete it on
  release: another process could be holding the old inode. Only cooperating flowd operations use
  this maintenance protocol; external SQLite tools must be closed before offline repair.
- Normal open creates missing directories at 0700 and the database at 0600. Existing parent modes
  are not changed. Paths are canonicalized; hard-linked databases and unsafe lock paths are refused.
- One connection is retained per store. Failed opens and close release all owned resources.

## A keypress, start to finish

Start: a start key arrives. Domain checks overlap and the one runner rule. Store saves block plus
diary row in one go. The screen works out now and next again and draws.

Fresh open: the private file opens or gets made under ownership, a quick integrity check runs,
and the version and expected schema are checked. The selected local day can then load.

After a kill: OS ownership is released automatically. SQLite recovers committed WAL data on
reopening; `quick_check(1)` runs before use. Unfinished transactions do not become partial saves.

Clock watch: now and next get worked out on every key plus once a second while idle. A jump past 30
seconds causes one calm recalc, never a burst.

## Time

- UTC in RFC 3339 on disk, local zone on screen through the day zone.
- The day flips on local date at draw time. Blocks never slide alone. Only an explicit copy moves
  one, per the carryover rules in [requirements.md](01-requirements.md) (F4) and
  [domain.md](02-domain.md) (D7).

## Errors and the doctor

- Errors read like `ERR_SOMETHING` plus plain words, on stderr and as a human line on screen. Codes
  used: `ERR_OVERLAP`, `ERR_ILLEGAL_TRANSITION`, `ERR_LOCKED`, `ERR_CORRUPT`, `ERR_DISK_FULL`,
  `ERR_VERSION`.
- Store returns errors and preserves their original SQLite, filesystem or parsing causes. It does
  not print or log errors; the application boundary assigns display codes and hints.
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
