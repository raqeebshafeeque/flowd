# Requirements

What flowd needs to do. The command line and the terminal workspace follow the same rules.

## What it does

### F1. Plan the day as a list of efforts

- Each item has a title and a Duration, which is how long you want to spend on it today. It is a
  budget, not a time slot.
- Durations are whole minutes or hours, written as `30`, `30m`, `2h` or `1h30m`.
- A new or longer Duration has to fit in what is left of the day, after what the other unfinished
  items still hold. The refusal says how much is left, like `Only 2h30m left on this day`.
- Titles are visible text. A hidden or direction-changing character is refused, and the error names
  it and its position.
- You can add, edit and delete items on today and future days. Deleting hides an item for good, so
  it asks first at a terminal and needs `--yes` everywhere else.

### F2. Put the day in your order

- The day is listed in the order you set. Nothing reorders it by itself, and new or carried items
  join the end.
- `o` in the workspace, or `flowd order` at a terminal, arranges the day: `j` and `k` select, `J`
  and `K` move and Ctrl+S saves. `flowd order <ID>...` saves an order from a script.
- If the day changed while it was being arranged, nothing is saved.

### F3. Carry unfinished work forward

- Nothing moves to another day by itself. You choose what to carry.
- `C` in the workspace, or `flowd carry` at a terminal, lists the incomplete items from the 7 days
  before the day with their title, date, Duration and status. Tick them, or all with `a`, and save
  with Ctrl+S. Scripts use `flowd carry <ID>...` or `flowd carry --all`.
- A carried item is a new todo item with the same title and Duration, and it remembers where it
  came from. The original does not change.
- An item is carried once while its copy is planned. Everything picked is carried in one save, and
  if it does not all fit, nothing is carried.

### F4. Track with one key

- Start, pause and finish are one action each. Starting a paused item resumes it, and starting a
  done one reopens it.
- Any item can be started at any time. Starting one pauses whatever was running.
- Going past the Duration is fine. Nothing stops and nothing nags you.
- Only today's items can be tracked.
- A forgotten finish can be put right. An item left incomplete in the last 7 days can be marked done
  with `f` in the workspace, which asks first, or `flowd finish --date <DATE> <ID>`. Its work stays
  what was tracked, and its details say it was marked done later.

### F5. Show what really happened

- Work is the time an item actually ran. Paused is the time it waited as paused after it started.
- Ten minutes running, twenty paused and five more running is 15 minutes of work and 20 paused.

### F6. Close the day at midnight

- At midnight anything not done becomes incomplete, even if the app was closed.
- Incomplete is final. Tomorrow starts empty until you plan it or carry something over.

### F7. Review the day

- The planned total, the recorded work, progress like `3 / 5 finished`, and every item grouped by
  status.

### F8. The command line

- `flowd` opens the workspace, and a command runs the command line instead.
- Commands take an item's full ID, which `flowd list` shows and every change prints. A position in
  the list is never accepted, because the order can change.
- At a terminal, a command run without an ID lets you pick the item with the arrow keys. Without a
  terminal, or with `--json`, a missing ID is refused, so nothing waits for an answer.
- Items are shown in a table like the workspace, with ID, title, Duration and status, under the
  date. `flowd list` shows the day, and a change shows only the item it changed.
- Options can go before or after the command, each only once.
- A person sees an error as one sentence and a hint.
- Tab completion for fish, zsh and bash completes commands, options and item IDs with their titles.
  It takes no lock, never writes and stays quiet on any problem.

### F9. Work with scripts and AI agents

- Every command prints JSON with `--json`. Errors have stable codes, and bad input exits with 2 and
  a refused action with 1.
- If it is unknown whether a save landed, flowd says so and asks you to check with `flowd list`.
- flowd ships an Agent Skill, installed only when asked with `flowd skill --claude`, `--codex` or
  both. A folder there that holds anything else is never replaced.
- An agent only changes the plan when you ask, and treats titles as plain data.

### F10. A terminal workspace

- Keyboard only, at least 80 by 24. A smaller window only asks to be made larger.
- Space starts, pauses, resumes or reopens the selected item, and `f` finishes it. The footer only
  offers what the item and the day allow, with `more [.]` when the keys do not fit.
- `n` adds and `e` edits in a form at the bottom. Tab moves between fields, Ctrl+S saves, and Esc
  asks before throwing changes away. Everything that commits, a form, an order or a carry, saves
  with Ctrl+S.
- `enter` shows details, `r` a read-only review, `c` a month calendar and `t` goes back to today.
  Looking at a day never creates it.
- `d` asks before deleting, and only `y` deletes.
- `?` lists every key and keeps the full wording of the last error.
- Success is quiet. Errors appear in a small box above the footer for seven seconds.

### F11. A status line

- `flowd now --short` prints the running item as one line, like `Deep work on the parser 1h3m/2h`,
  adding how far over past its Duration, like `+10m`. Titles are cut to 30 columns.
- It prints nothing when nothing runs, and nothing on any problem. It takes no lock, never writes
  and never closes a day, so it can run every second.

### F12. Never lose a day

- A saved day survives a crash, a kill or a restart.
- An older file is backed up before it is upgraded, and the upgrade either finishes or changes
  nothing.

## How well it does it

- Speed. flowd opens in under 50 ms and a keypress shows in under 16 ms, even with two years of
  history. Timing tests in CI catch it getting slower.
- Offline. It never uses the network.
- Reliability. Every action is saved before the next one. Every open checks the file, and a broken
  file fails with a clear error and is never replaced.
- Size. Under 30 MB of memory and one file under 15 MB.
- Support. `flowd doctor` checks the file and `flowd doctor --repair` restores the backup.
  `flowd reset` starts over with an empty planner. You type `reset` to confirm, and the old files
  move into a private folder next to the database instead of being deleted.
- Data. The file is `$XDG_DATA_HOME/flowd/flowd.sqlite`, or `~/.local/share/flowd/flowd.sqlite`.

## Not in scope

- Teams, syncing, calendars or notifications.
- Searching across days, recurring items or recommendations.
- Projects, tags, priorities or subtasks. See the principles in the vision.
