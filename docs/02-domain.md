# Domain

What days and items are, and the rules they always follow.

## Words we use

- Item. Something you want to put effort into today, like `Study` for one hour.
- Duration. How long you want to spend on an item. It is a budget, not a deadline.
- Work. The time an item actually ran.
- Paused. The time an item waited as paused after it first started.
- Archived means hidden from the list but still stored.

## Day

- A day has an id, a date like `2026-10-12` and the timezone it was created in.
- There is only one day per date.
- A day keeps its timezone even if you travel.
- A day ends at midnight in its own timezone. Some days are 23 or 25 hours long because of daylight
  saving time.
- Midnight itself belongs to the next day.
- A few dates in a few timezones have no midnight at all, because the clocks jumped past it. Those
  dates are refused rather than shifted.

## Item

- An id, the day it belongs to and a title of 1 to 120 characters.
- A title is visible text only. Hidden, zero-width, direction-changing, tag, private-use and control
  characters are refused, because they can make a title read differently from what it holds or hide
  instructions from an AI agent. Normal text in any language and real emoji are fine.
- A title saved by an older version that still holds such characters is shown with a visible
  placeholder like `<U+202E>` instead. Nothing on disk is changed until the item is changed.
- A duration in whole minutes, always more than zero.
- A status: todo, in progress, paused, done, skipped or incomplete.
- When it was first started. This never changes once it is set.
- When it was finished. This is cleared if it is reopened.
- When it was closed, by finishing, skipping, archiving or the end of the day.
- Whether it is archived, and when it was created and updated.
- The item it was carried from, if it is a copy of an unfinished item from an earlier day. This is
  set when it is created and never changes.
- When it was marked done later, if a forgotten finish was put right after its day ended.
- Its place in the day's list. This is how the list is laid out, not something that happened to the
  item, so it is not part of the item's history.

## Events

- Every change to an item adds one event in the same save as the change itself. Putting the day in
  a new order is the one exception, because an item's place is not part of its history.
- An event records the item, the old and new status, the time and the reason.
- It also keeps a full copy of the item before and after the change. That lets flowd replay the
  history and refuse any event that no real action could have produced.
- The reasons are user, auto pause, reopen and day end.
- Events are never edited or deleted. Work and paused time are always worked out from them.

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
| incomplete or skipped | carry | unchanged, with a new todo copy on a later day |
| incomplete or skipped, in the last 7 days | mark done later | done, marked as corrected |

## How much of a day is left

- The time left is from now until the day's midnight. A day that has not started yet has all of its
  time, 23 to 25 hours.
- From that, every other unfinished item holds its share: a todo item its whole Duration, a running
  or paused item the part it has not worked yet. Done, skipped, incomplete and deleted items hold
  nothing.
- A new item's Duration, or a larger Duration on an edit, has to fit in what is left. The item being
  edited does not count against itself.
- This only ever refuses a change. It never touches items that are already saved.

## Rules

- One runner. Only one item can run at a time. Starting another one pauses it automatically.
- Today only. Status changes only happen on today's items. Planning is allowed for today and the
  future. Past days cannot be changed, with one exception: an item left incomplete or skipped in
  the last 7 days can be marked done later. Only its status changes, it finishes when it was
  closed, no work or start is made up, and it is marked as corrected so it is never mistaken for a
  finish made at the time. It is put right once.
- End of day. When a day ends, its unfinished items become incomplete at exactly midnight, even if
  that is noticed later. This happens once, no matter how many times the day is opened.
- Incomplete is final. It cannot be started, reopened or brought back.
- Every day stands alone. Nothing is copied, repeated or moved to another day by itself. To keep
  working on something, you carry it: an incomplete or skipped item from the 7 days before is copied
  onto the day as a new todo item with the same title and Duration. The original stays as it was,
  and its copy remembers it, so an item is only carried once while its copy is planned.
- First start. The first start time never changes, even after pauses, skips or reopening.
- Order. A day is listed in the order you set. Nothing changes it by itself: starting, pausing,
  finishing, skipping or editing never moves an item, and new or carried items join the end. Any
  item can be moved, on today and later days.
- Time. Times are stored in UTC and shown in local time.
- Whole seconds. Every time is recorded to the nearest second, so work and paused time are always
  whole seconds and read the same everywhere. A time just before midnight is never rounded up into
  the next day.
- Broken history. If the events of an item do not add up, flowd reports an error instead of
  guessing a number.
- Delete. Deleting only hides an item. Nothing is ever removed.

## An example day

1. The plan has `Study` for 1 hour and `Code` for 2 hours.
2. Study starts at 09:00.
3. Code starts at 09:40, so Study pauses on its own with 40 minutes of work.
4. Code is finished at 11:10 with 1 hour 30 minutes of work.
5. Study resumes at 11:15 and is finished at 11:30.
6. Study ends with 55 minutes of work and 1 hour 35 minutes paused. Both items are done.
