# Domain - flowd

What blocks and days are and the rules they always follow. Code follows this
file, not the other way round.

## Words used here

- Block: one planned chunk of time, like `09:00 to 10:00 Study`.
- Planned time: the schedule. Actual time: what really happened.
- Overrun: not a status, just a flag. A running block past its planned end.
- Archived: soft deleted. Hidden from the plan but kept around.
- Carryover: an explicit copy of a block to tomorrow. The source stays put.

## Entities

### Block

| Field | Type | Notes |
|---|---|---|
| `id` | TEXT (ULID) PK | Never reused, stable across restarts |
| `day_id` | TEXT FK to `days.id` | The day it belongs to |
| `title` | TEXT 1 to 120 chars | Required, trimmed |
| `planned_start_utc` | TEXT RFC3339 UTC | Required |
| `planned_end_utc` | TEXT RFC3339 UTC | Required, after planned start |
| `actual_start_utc` | TEXT nullable | Stamped on first start |
| `actual_end_utc` | TEXT nullable | Stamped on finish, cleared on reopen |
| `status` | enum | `todo, in_progress, paused, done, skipped` |
| `archived` | INTEGER 0 or 1 | 1 means hidden, default 0 |
| `copied_from` | TEXT nullable FK to `blocks.id` | Only set by a carryover copy |
| `created_at_utc`, `updated_at_utc` | TEXT RFC3339 | Kept by the store |

### Day

| Field | Type | Notes |
|---|---|---|
| `id` | TEXT (ULID) PK | Never reused, same scheme as blocks |
| `date` | TEXT `YYYY-MM-DD` UNIQUE | Local calendar date, one row per day |
| `timezone` | TEXT IANA | Saved at creation, like `Asia/Kolkata` |
| `created_at_utc` | TEXT | |

Blocks hang off the day through `day_id`. The day itself carries no status; review gets worked out
from its blocks.

### BlockEvent, a diary that only grows

| Field | Type | Notes |
|---|---|---|
| `seq` | INTEGER PK AUTOINCREMENT | Order |
| `block_id` | TEXT FK | Which block moved |
| `from_status`, `to_status` | enum | Before and after |
| `at_utc` | TEXT RFC3339 | When it moved |
| `reason` | TEXT | `user, auto_pause, carryover, reopen, extend` |

Every status move writes exactly one diary row in the same save as the block. Stretching writes one
too, with before and after matching and reason `extend`.

## Statuses in plain words

- `todo`: planned, not started.
- `in_progress`: running now. At most one per day; starting a new one pauses the old one by itself.
- `paused`: was running, can resume.
- `done`: finished. Reopen works only on the same local day.
- `skipped`: dropped. Left out of the score math.
- `archived` sits apart. It hides any status while changing none. No unhide path in this version.

## Moves allowed

| From | To | Trigger | Effect |
|---|---|---|---|
| `todo` | `in_progress` | start key | Stamp actual start if empty, pause whoever runs |
| `in_progress` | `paused` | pause key, or auto when another starts | Keep actual start |
| `paused` | `in_progress` | resume key | Back to running |
| `in_progress`, `paused` | `done` | finish key | Stamp actual end with now |
| `done` | `in_progress` | reopen key, same local day only | Clear actual end |
| `todo`, `in_progress`, `paused` | `skipped` | skip key | Keep actuals as they are |
| any visible | archived | delete key | Set hidden flag, keep status |
| `todo`, `in_progress`, `paused` | `todo` tomorrow | copy to tomorrow key | New row with `copied_from` set |

Stretching is not a move. It pushes `planned_end` by 15 minutes and checks overlap again.

## House rules

- D1 Overlap: planned chunks are half open `[start, end)`, so 09:00 to 10:00 beside 10:00 to 11:00
  is fine. Two visible blocks on one day never overlap; hidden ones are ignored. Real life may
  overlap and run long freely.
- D2 One runner: at most one `in_progress` per day. Starting a new one pauses the old one with
  reason `auto_pause`.
- D3 Time: save UTC in RFC 3339, show local time. The day line follows the local date.
- D4 Delete: delete only hides. Nothing gets hard removed here.
- D5 Reopen: a finished block runs again only while today still matches its own day. Else the app
  says no.
- D6 Real times: actual start lands on first start and stays. Actual end lands on finish and clears
  on reopen. A skipped block stays skipped with frozen actuals.
- D7 Carryover: copy makes a fresh todo for tomorrow, keeping the same clock times unless fresh ones
  get given. The source stays. Finished, skipped and hidden ones refuse to copy. A copy that would
  overlap tomorrow gets refused with `ERR_OVERLAP`. After a zone flight the date stays `YYYY-MM-DD`
  while stored UTC simply renders in the new local zone.
- D8 Score: `round(100 times done divided by visible minus skipped)`. An empty day shows a dash. A
  finished overrun still counts as done while showing flagged.

## Reading the day

- Now: the running block wins. With none running, the planned chunk holding the moment.
- Next: the earliest visible todo starting after now.
- Overrun: running or paused past planned end. A flag only, the app never moves it alone.
- Missed: a todo the day ends without ever starting.

## An example day

Day `2026-10-12` in `Asia/Kolkata`:
1. Plan holds `09:00 to 10:00 Study` and `10:00 to 11:00 Code`.
2. Study starts at 09:02. Actual start reads 09:02, running.
3. At 10:15 it still runs, so it shows flagged as overrun.
4. Code starts at 10:16. Study pauses by itself, Code runs.
5. Code finishes at 10:50. Actual end reads 10:50, done.
6. Study never resumes and gets skipped at 11:00. Skipped is allowed from paused,
   it just means abandoned.
7. Review reads 2 total with 1 skipped and 1 done, so the score is 1 divided by 1, 100 percent with
   1 skipped noted.
