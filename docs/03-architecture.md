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

- Domain: checks the D1 to D8 rules and works out now, next, overrun and score. Pure logic, no file
  or screen calls.
- Store: owns SQLite for `days`, `blocks` and `block_events`. Owns saves, schema version in `PRAGMA
  user_version`. Table SQL lives in `internal/store/schema.sql`; there is no later migration yet.

The planned TUI will draw these same rules and take single keys. Watch currently supplies the
one-second CLI refresh; it needs no separate scheduler package or background service.

Call direction, reflected in `internal/cli`, `internal/domain` and `internal/store`:

```text
cli -> domain
cli -> store -> domain
domain never calls store or cli
```

Every command write walks parsing to rules to save. Presentation never writes SQL around rules.
Store reuses domain data types and pure final-plan validation; it does not choose transitions.
Creation with an initial event, grouped changes and archive are atomic. A new day needed by a
creation is saved in the same transaction. Automatic pause and start must use one grouped save.

On the command line: `flowd` opens today, `flowd YYYY-MM-DD` opens that date, `flowd doctor` checks
the store, `flowd --help` lists commands and options.

CLI options precede the command. Read commands are `list` (the default), `now` and `next`.
`--date YYYY-MM-DD` or a positional date selects a day. `--db PATH` overrides the default file;
`--timezone ZONE` sets the zone for a new day and refuses conflicts with a saved zone. The default
is a detected IANA system zone, falling back to UTC. The binary embeds fallback timezone data.
`--json` produces parseable stdout, with null for absent now/next and an uncountable score.
Text shows local planned times, progress and overrun flags; read views do not insert planner rows.

`add START END TITLE` creates a todo block with its first history event, including a new day in
that transaction when needed. `edit --title/--start/--end BLOCK` saves requested fields together;
an overlapping edit changes nothing. `delete BLOCK` archives and retains history; repeating a
full-id deletion is harmless. Visible row numbers are temporary selectors; full ids are stable.

`start`, `pause`, `resume`, `finish`, `skip` and `reopen` take a block selector. Commands enforce
which source statuses they accept; reopening a done block also requires the saved local date.
Starting, resuming or reopening pauses another runner in the same grouped transaction. One clock
instant stamps both events. `extend` adds 15 minutes without changing status, refusing overlaps.
`events` reads ordered history, including archived work selected by its full id. An output error
returns nonzero but does not undo a mutation already committed to SQLite.

`copy [--start TIME] [--end TIME] BLOCK` advances one calendar date, preserving local clocks and
an overnight end. The target uses its saved zone, or the source zone for a new day. Default clocks
that disappear or repeat across DST are refused; offset timestamps select exact instants.
A fresh todo and carryover event are saved together, with source provenance and no source edit.
Repeated copies require a free interval. `review` computes counts, current/completed overruns and
missed todos without saving statuses. Todos become missed only after their saved local day ends;
progress is rounded done divided by visible minus skipped, or null when uncountable.

## JSON and exits

`--json` keeps successful data on stdout and diagnostics on stderr. Help remains human-readable.
The response shapes are:

| Command | JSON response |
|---|---|
| Default/list, review, each watch line | `date`, `timezone`, `blocks`, `now`, `next`, `progress`, `done`, `skipped`, `unfinished`, `missed`, `overruns` |
| now/next | A block with an `overrun` boolean, or null |
| add/edit/delete/tracking/extend/copy | The saved block |
| events | Event array in increasing `seq` order |
| doctor/backup | `{"ok":true,"path":"..."}` |
| doctor --repair | The same object with `recovery` naming preserved originals |
| --version | `{"version":"..."}` |

Block fields are `id`, `day_id`, `title`, `status`, `archived`, `copied_from_id`,
`planned_start`, `planned_end`, `actual_start`, `actual_end`, `created_at` and `updated_at`.
Times are UTC RFC 3339 with fractional seconds retained; absent optional fields are null.
Events carry `seq`, `block_id`, `from_status`, `to_status`, `occurred_at` and `reason`.
Collections are arrays, including when empty. `missed` and `overruns` contain block ids.
Progress is an integer percentage or null, never a fabricated zero for an uncountable day.

Exit 0 means success; 2 is input/usage failure; 1 is operational failure. Cancelled one-shot
commands exit 130; interrupting watch exits 0. Input codes are `ERR_USAGE`, `ERR_TITLE` and
`ERR_TIME`. Operational codes are `ERR_OVERLAP`, `ERR_ILLEGAL_TRANSITION`, `ERR_CARRYOVER`,
`ERR_STATE`, `ERR_NOT_FOUND`, `ERR_LOCKED`, `ERR_CORRUPT`, `ERR_VERSION`, `ERR_DISK_FULL`,
`ERR_PERMISSION`, `ERR_REPAIR` and `ERR_IO`; cancellation uses `ERR_CANCELLED`.
A success-output failure returns nonzero even when its database save already committed.

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
  nonblocking OS lock, covering the whole command and offline repair. Never delete it on
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

CLI watch builds one fresh snapshot, closes storage, writes the frame, then waits about one second.
Other commands can own the file between refreshes. A busy later refresh is retried without output
flooding; an initially busy watch exits with the lock error. Each refresh captures the current clock
so jumps recalculate once without replaying missed ticks. Implicit dates follow local midnight;
explicit dates stay pinned. Only terminal text uses redraw controls; pipes are plain and JSON is
one snapshot per line. Ctrl-C cancels the wait and exits cleanly.

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
- `backup` uses SQLite `VACUUM INTO` to include committed WAL changes, not a raw main-file copy.
  It writes a private temporary snapshot, syncs it, replaces `.bak`, then syncs the parent
  directory. Publication also owns the backup path and refuses leftover backup journals, rather
  than replacing an active snapshot or allowing stale journals to replay into a new one.
  Pre-publication failure retains the old backup. A directory-sync error after replacement reports
  that publication already happened. Full integrity and foreign-key checks use a separate
  read-only connection, never normal schema initialization or permission tightening.
- Only `doctor --repair` restores `.bak`. It validates the snapshot before moving any original,
  prepares and syncs a replacement, then preserves old main/WAL/SHM/rollback files in a private
  `.recovery-*` directory. Restoring an older snapshot intentionally removes newer live changes.
  A synced `.repair` marker records the recovery directory, replacement and publication stage
  before journals move. Normal opens refuse while it exists; explicit repair resumes safely.
  Publication and marker removal sync the parent directory. Earlier recovery directories are
  never reused or deleted. External SQLite tools must be closed during this operation.

## When things break

| What happens | How it shows | What the app does | How to test it |
|---|---|---|---|
| Kill mid keypress with `SIGKILL` | A save is atomic | Last saved state stays whole, at most one key lost | Loop writes with `kill -9`, reopen and diff |
| Disk full | Write fails | That save fails, old data stays, plain `ERR_DISK_FULL` shows | Fill a small tmpfs and try a write |
| Clock jumps | Jump past 30s | One recalc, no flood | Step a fake clock |
| SQLite broken | Open or check fails | Print `ERR_CORRUPT` with path, point at doctor, never reset data | Flip bytes in a copy and open it |
| Second copy open | Ownership lock | Second copy exits with `ERR_LOCKED` | Open twice in a test |
| Schema newer than app | Version check | Refuse with `ERR_VERSION` and an upgrade hint | Bump a version fixture |

## Checks

Run `go test ./... -count=1`, `go test -race ./... -count=1`, `go vet ./...` and
`go build ./...`. Built-executable tests use temporary files, exercise the complete workflow and
send an actual interrupt to watch. A subprocess kill test checks committed and unfinished
block/event changes; staged repair tests exercise resumption after journal movement.

`go test ./internal/cli -run TestRepresentativeHistory -v -count=1` creates 730 days and 2000
blocks, inspects the store and records snapshot timings without a machine-independent threshold.
On this macOS arm64 host with Go 1.27.0, the fixture database was 598,016 bytes. One run measured
11.27ms mean in-process snapshots and 40.70ms mean / 486.73ms maximum across 20 built processes.
Those observations do not establish the cold-open or TUI keypress targets.

The no-cgo arm64 binary built with `-trimpath -ldflags='-s -w -X main.version=readiness'` was
7,954,674 bytes, including timezone data. macOS amd64 and Linux amd64/arm64 cross-builds passed; Linux runtime, quiet memory,
physical full-disk and power-loss checks were not performed. SQLite capacity-limit and synthetic
ENOSPC tests are narrower write-failure evidence, not physical-disk tests.

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
