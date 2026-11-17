# Requirements

What flowd needs to do. The command line version comes first and the terminal UI follows later
with the same rules.

## What it does

### F1. Plan the day as a list of efforts

- Each item has a title and a duration, which is how long you want to spend on it today.
- A duration is a budget, not a time slot. There are no start or end times to plan.
- Durations are whole minutes and can be written as `30`, `30m`, `2h` or `1h30m`.
- You can add, edit and remove items. Removing an item hides it but keeps it in the file.
- You can plan today and future days. Past days are only for looking back.
- `flowd` opens today and `flowd 2026-10-12` opens that date.

### F2. Track with one key

- Start, pause, resume, finish and skip are one action each.
- Any item can be started at any time, in any order.
- Starting an item while another one runs pauses the running one.
- Going past the duration is fine. Nothing stops and nothing nags you.
- Finish means you are done with it for today, not that the problem is solved forever.
- A skipped item can be brought back, and a finished one can be reopened, on the same day.
- Only today's items can be started, paused, finished or skipped.

### F3. Show what really happened

- Work is the time an item actually ran, adding up every running stretch.
- Elapsed is the time from its first start until now or until it was closed, pauses included.
- Ten minutes running, twenty paused and five more running is 15 minutes of work and 35 elapsed.
- Items you started come first, in the order you first started them. Items not started yet follow.

### F4. Close the day at midnight

- At midnight anything still not done becomes incomplete, even if the app was closed.
- Done and skipped items stay as they are.
- Incomplete is final. Tomorrow starts empty until you plan it.

### F5. Carry work over by hand

- Unfinished work only moves to another day when you copy it there yourself.
- The original stays as it was.

### F6. Review the day

- The total duration you planned and the total work you recorded.
- Progress as finished items out of all items, like `3 / 5 finished`.
- Every item grouped by its status.

### F7. Never lose a day

- A saved day survives a crash, a kill or a restart.

## How well it does it

- Speed. It opens in under 50 ms, a keypress shows up in under 16 ms, and a day with two years of
  history behind it still opens instantly.
- Offline. It never uses the network.
- Reliability. Every action is saved to disk before the next one. A broken file fails with a clear
  error and is never replaced.
- Size. It uses less than 30 MB of memory and ships as one file under 15 MB.
- Support. `flowd doctor` checks the data file and `flowd doctor --repair` restores the backup.
- Data. The file lives at `$XDG_DATA_HOME/flowd/flowd.sqlite`, or `~/.local/share/flowd/flowd.sqlite`
  when that variable is not set.

## Not in scope

- Teams, or syncing between devices.
- Importing calendars, phone apps or web apps.
- Notifications while the app is closed.
- Searching across days.
- Recommendations about what to do next, recurring items or editing recorded times.
