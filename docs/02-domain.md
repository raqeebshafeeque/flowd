# Domain

What days and items are, and the rules they always follow.

## Words we use

- Item. Something you want to put effort into today, like `Study` for one hour.
- Duration. How long you want to spend on an item. It is a budget, not a deadline.
- Work. The time an item actually ran.
- Elapsed. The time from an item's first start until now or until it was closed.
- Archived means hidden from the list but still stored.

## Day

- A day has an id, a date like `2026-10-12` and the timezone it was created in.
- There is only one day per date.
- A day keeps its timezone even if you travel.
- A day ends at midnight in its own timezone. Some days are 23 or 25 hours long because of daylight
  saving time.

## Item

- An id, the day it belongs to and a title of 1 to 120 characters.
- A duration in whole minutes, always more than zero.
- A status: todo, in progress, paused, done, skipped or incomplete.
- When it was first started. This never changes once it is set.
- When it was finished. This is cleared if it is reopened.
- When it was closed, by finishing, skipping, archiving or the end of the day.
- Whether it is archived, and when it was created and updated.

## Events

- Every change adds one event in the same save as the change itself.
- An event records the item, the old and new status, the time and the reason.
- It also keeps a full copy of the item before and after the change. That lets flowd replay the
  history and refuse any event that no real action could have produced.
- The reasons are user, auto pause, reopen and day end.
- Events are never edited or deleted. Work and elapsed are always worked out from them.

## What can happen to an item

| From | Action | To |
| --- | --- | --- |
| todo | start | in progress |
| in progress | pause | paused |
| paused | resume | in progress |
| in progress or paused | finish | done |
| todo, in progress or paused | skip | skipped |
| skipped | unskip | todo if never started, otherwise paused |
| done | reopen | in progress |
| todo, in progress or paused | end of the day | incomplete |
| any visible item | delete | archived, status unchanged |

## Rules

- One runner. Only one item can run at a time. Starting another one pauses it automatically.
- Today only. Status changes only happen on today's items. Planning is allowed for today and the
  future. Past days cannot be changed.
- End of day. When a day ends, its unfinished items become incomplete at exactly midnight, even if
  that is noticed later. This happens once, no matter how many times the day is opened.
- Incomplete is final. It cannot be started, reopened or brought back.
- Every day stands alone. Nothing is copied, repeated or moved to another day. If you want to keep
  working on something tomorrow, you plan it again tomorrow.
- First start. The first start time never changes, even after pauses, skips or reopening.
- Order. Started items come first, ordered by their first start. Items not started yet follow, in
  the order they were created. Pausing, finishing or editing never moves an item.
- Time. Times are stored in UTC and shown in local time.
- Whole seconds. Every time is recorded to the nearest second, so work and elapsed are always whole
  seconds and read the same everywhere.
- Broken history. If the events of an item do not add up, flowd reports an error instead of
  guessing a number.
- Delete. Deleting only hides an item. Nothing is ever removed.

## An example day

1. The plan has `Study` for 1 hour and `Code` for 2 hours.
2. Study starts at 09:00.
3. Code starts at 09:40, so Study pauses on its own with 40 minutes of work.
4. Code is finished at 11:10 with 1 hour 30 minutes of work.
5. Study resumes at 11:15 and is finished at 11:30.
6. Study ends with 55 minutes of work and 2 hours 30 minutes elapsed. Both items are done.
