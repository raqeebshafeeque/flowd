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
- `flowd` opens today and `flowd 2026-10-12` opens that date. Commands that need an item take its
  row number from the list or its full ID. Row numbers follow the list order, so starting an item can
  move it up.

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

### F5. Review the day

- The total duration you planned and the total work you recorded.
- Progress as finished items out of all items, like `3 / 5 finished`.
- Every item grouped by its status.

### F6. Work with scripts and AI agents

- Every command can print JSON with `--json`, so scripts and agents never have to parse text.
- Errors have stable codes and exit codes. Bad input exits with 2 and a refused action exits with 1.
  An error is printed to stderr as its code, one sentence and a hint.
- If a save fails in a way that leaves it unknown whether it landed, flowd says so plainly instead
  of reporting success or failure.
- A refused or failed action never prints output that looks like success.
- flowd ships with an Agent Skill that teaches an agent how to use these commands safely.
- An agent only changes the plan when you ask it to. Titles are treated as plain data, never as
  instructions.

### F7. A terminal workspace

- `flowd` with no arguments opens a keyboard-only workspace showing today. Any argument runs the
  command line instead, so scripts and pipes never get the workspace.
- It lists the day the same way the command line does and keeps the selected item selected while
  the list refreshes, even when the item moves.
- Details show the full title, status, Duration, Work, Elapsed, Started At and Finished At in a
  panel at the bottom that takes about a quarter of the screen.
- The workspace needs at least 80 by 24 cells. In a smaller window it shows only a request to make
  the window larger, and every key except Ctrl+C does nothing, so nothing happens that cannot be
  seen.
- Started without a terminal, it refuses and suggests `flowd list`.
- Space starts, pauses or resumes the selected item. `f` finishes, `s` skips, `u` unskips and `R`
  reopens. The footer only offers what the selected item can do.
- `n` adds an item and `e` edits the selected one, in a form at the bottom with a Title and a
  Duration field. While the form is open every letter is text, never an action. Tab and Enter move
  between the fields, Enter never saves, Ctrl+S saves, and Esc asks before throwing away changes.
- An empty Duration shows the example `e.g. 30m, 1h, 1h30m`. A bad title or Duration is explained
  under its field and what was typed stays. Saving an edit that changed nothing just closes it.
- `c` opens a month calendar at the bottom. `h` and `l` move a day, `j` and `k` a week, `<` and `>`
  a month, `t` goes to today and Enter shows the chosen day. The cursor date is in brackets, like
  `[17]`. Looking at a day never creates it.
- When another day is on screen the header says `Viewing 2026-11-18` and `t` comes back to today.
  The footer only offers what that day allows: tracking today, planning today and later, and only
  looking on past days.
- `r` opens a read-only review of the day on screen, the same as `flowd review`: total duration and
  recorded work, progress as finished out of all items, and every item grouped under its status with
  a count. `j` and `k` scroll it and Esc goes back. Nothing can be changed from it.
- `d` asks `Delete "Study"?  yes [y]  no [n]` in the footer. Only `y` deletes, `n` and Esc cancel,
  and no other key answers. A long title is shortened so the answers always show. Past days cannot
  be changed this way either.
- `?` opens Help, listing every key. The full wording of the last error stays there after its
  popup has gone, with its code. Inside a form, `?` is just text.
- Success is quiet: the list simply shows the new state. Errors and refusals appear in a small box
  above the footer for seven seconds.

### F8. Never lose a day

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
- Recommendations about what to do next, recurring items, copying items between days or editing
  recorded times.
