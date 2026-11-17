# Domain

What days and blocks are, and the rules they always follow.

## Words we use

- Block. One planned piece of time, like `09:00 to 10:00 Study`.
- Planned time is the schedule. Actual time is what really happened.
- Overrun is a flag, not a status. It means a block is still running or paused after its planned end.
- Archived means hidden from the plan but still stored.
- Carryover means copying a block to tomorrow on purpose.

## Day

- A day has an id, a date like `2026-10-12` and the timezone it was created in.
- There is only one day per date.
- A day has no status of its own. Everything about it comes from its blocks.

## Block

- An id, the day it belongs to and a title of 1 to 120 characters.
- A planned start and a planned end, with the end after the start.
- An actual start, set the first time it starts and never changed after that.
- An actual end, set when it is finished and cleared if it is reopened.
- A status, which is one of todo, in progress, paused, done or skipped.
- Whether it is archived, which block it was copied from, and when it was created and updated.

## Events

- Every change adds one event in the same save as the change itself.
- An event records the block, the old and new status, the time and the reason.
- The reasons are user, auto pause, carryover, reopen and extend.
- Events are never edited or deleted.

## What can happen to a block

| From | Action | To |
| --- | --- | --- |
| todo | start | in progress |
| in progress | pause | paused |
| paused | resume | in progress |
| in progress or paused | finish | done |
| done | reopen, on the same day only | in progress |
| todo, in progress or paused | skip | skipped |
| any visible block | delete | archived, status unchanged |
| todo, in progress or paused | copy | a new todo block tomorrow |

Stretch is not a status change. It moves the planned end 15 minutes later.

## Rules

- Overlap. A planned block covers its start but not its end, so `09:00 to 10:00` and
  `10:00 to 11:00` do not overlap. Hidden blocks are ignored.
- One runner. Only one block can run at a time. Starting another one pauses it automatically.
- Time. Times are stored in UTC and shown in local time.
- Delete. Deleting only hides a block. Nothing is ever removed.
- Copy. Done, skipped and hidden blocks cannot be copied. A copy that would overlap is refused.
- Score. The done blocks divided by the visible blocks that were not skipped, as a rounded
  percentage. An empty day shows a dash instead of a score.

## An example day

1. The plan has `09:00 to 10:00 Study` and `10:00 to 11:00 Code`.
2. Study starts at 09:02.
3. At 10:15 it is still running, so it is flagged as an overrun.
4. Code starts at 10:16 and Study pauses on its own.
5. Code is finished at 10:50.
6. Study is skipped at 11:00.
7. The review shows one block done and one skipped, so the score is 100%.
