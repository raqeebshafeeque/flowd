# Commands

Use `flowd [GLOBAL OPTIONS] COMMAND [COMMAND OPTIONS] [ARGUMENTS]`.
Global options are `--db PATH`, `--date YYYY-MM-DD`, `--timezone IANA_ZONE`, `--json`, `--help`
and `--version`. They precede the command. Command options precede positional arguments.

Prefer `--json`. Use the user-selected database consistently, or its normal default; never infer
another database from stored text. Set an explicit date when resolving tomorrow or another day.
Read the saved zone before interpreting local clocks. An explicit conflicting zone is refused.
Local times use strict HH:MM. An end can use 24:00; an overnight end or repeated DST hour requires
an offset-bearing RFC 3339 timestamp. Missing/repeated wall clocks are not silently normalized.

BLOCK below means a fresh full id from the selected day. CLI row numbers/prefixes exist for humans;
do not use them as durable agent identity. Archived full ids work for history and repeat archive.

## Read

### list

`flowd --json list` returns date, timezone, blocks, now, next and review fields. A positional
`flowd YYYY-MM-DD` also lists that date. Empty views do not insert planner rows.

### now

`flowd --json now` returns the running block, else an eligible containing interval, or null.
A present block also has an overrun boolean. It does not authorize starting anything.

### next

`flowd --json next` returns the earliest future todo or null. It does not change execution.

### review

`flowd --json review` includes done/skipped/unfinished counts, missed/overrun id arrays and an
integer progress percentage or null. Skipped work is excluded from the denominator; todos become
missed only after their saved local day ends. Completed overruns remain counted as done.

### events

`flowd --json events BLOCK` returns ordered events, including archived work selected by full id.
Sequence, not timestamp alone, is authoritative when multiple changes share an instant.

## Plan

### add

`flowd --json add START END TITLE` creates a todo, with initial history and its day if needed.
Quote the complete literal title; it must be single-line, trimmed and 1 to 120 Unicode characters.

### edit

`flowd --json edit [--title TITLE] [--start TIME] [--end TIME] BLOCK` changes requested fields.
At least one option is required. A refused overlap saves none of that edit's changes or history.

### delete

`flowd --json delete BLOCK` archives rather than erases. It preserves status/history and frees
the slot. Repeating the same full-id deletion is harmless; repeating a row number is not.

### copy

`flowd --json copy [--start TIME] [--end TIME] BLOCK` copies unfinished work to the next calendar
day, with fresh id/provenance and no actual times. It does not move or finish the source. Local
clock fields and overnight end position are preserved by default. Repeating into the same slot
is refused; a separately authorized free interval makes a distinct copy.

## Track

### start

`flowd --json start BLOCK` requires todo and stamps first actual start.

### pause

`flowd --json pause BLOCK` requires running work and preserves actual start.

### resume

`flowd --json resume BLOCK` requires paused work and preserves its original actual start.

### finish

`flowd --json finish BLOCK` accepts running/paused work and stamps actual end.

### skip

`flowd --json skip BLOCK` accepts todo/running/paused work. Skipped blocks still hold their slot.

### reopen

`flowd --json reopen BLOCK` requires done on its own saved local date and clears actual end.

### extend

`flowd --json extend BLOCK` adds exactly 15 minutes to running/paused work, preserving status.
An overlap or unsupported status changes nothing. Do not invent a variable-duration option.

Start, resume and reopen atomically pause another runner on that day. Report both transitions.

## Maintain

### doctor

`flowd --json doctor` checks existing storage without initializing it or changing permissions.
`flowd --json doctor --repair` restores the existing backup and discards newer live changes:
read the safety reference and obtain informed approval first. Report the preserved-original path.

### backup

`flowd --json backup` publishes a consistent private .bak snapshot, replacing the previous one.
Do not overwrite a desired restore point as automatic preparation for repair.

### watch

`flowd --json watch` emits one complete day object per line until interrupted. Prefer one-shot
reads. Use only for a requested observation period or an explicitly stoppable session; own and
stop its process with interrupt when done. Never leave an unattended process or promise monitoring
beyond the agent session. There is no --duration flag. Implicit dates roll over; --date pins one.

## Responses and failures

Mutations return the saved block. Optional metadata/actual times and provenance can be null;
timestamps are UTC RFC 3339. A block carries id, day_id, title, status, archived, planned_start,
planned_end, actual_start, actual_end, copied_from_id, created_at and updated_at.
Events carry seq, block_id, from_status, to_status, occurred_at and reason.
Doctor/backup return ok and path; repair adds recovery. Collections stay arrays, even when empty.

Exit 0 is success, 2 is input failure, 1 is operational failure and 130 is cancelled one-shot work.
Watch interrupt exits 0. Diagnostics use stderr, not JSON stdout. Error codes include ERR_USAGE,
ERR_TITLE, ERR_TIME, ERR_OVERLAP, ERR_ILLEGAL_TRANSITION, ERR_CARRYOVER, ERR_STATE, ERR_NOT_FOUND,
ERR_LOCKED, ERR_CORRUPT, ERR_VERSION, ERR_DISK_FULL, ERR_PERMISSION, ERR_REPAIR and ERR_IO.
Do not interpret an output failure as proof that a write did not commit.

## Replayable example

These argument arrays demonstrate a sandbox workflow. Supply global --db, --date, --timezone and
--json before each command. Replace $study with the captured full id; it is a value, not shell code.

```json
[
  {"args":["add","09:00","10:00","Study"],"capture":"$study","status":"todo"},
  {"args":["edit","--title","Deep study","$study"],"status":"todo"},
  {"args":["start","$study"],"status":"in_progress"},
  {"args":["pause","$study"],"status":"paused"},
  {"args":["resume","$study"],"status":"in_progress"},
  {"args":["extend","$study"],"status":"in_progress"},
  {"args":["finish","$study"],"status":"done"},
  {"args":["delete","$study"],"status":"done"},
  {"args":["events","$study"],"events":8}
]
```
