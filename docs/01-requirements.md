# Requirements

What flowd needs to do. The command line and the terminal workspace follow the same rules.

## What it does

### F1. Plan the day as a list of efforts

- Each item has a title and a duration, which is how long you want to spend on it today.
- A duration is a budget, not a time slot. There are no start or end times to plan.
- Durations are whole minutes and can be written as `30`, `30m`, `2h` or `1h30m`.
- A day only has so much time. A new or longer Duration cannot be more than what is left of the day,
  after what the other unfinished items still hold. The refusal says how much is left, like
  `Only 2h30m left on this day`. Items already saved are never changed by this, and shortening or
  renaming an item always works.
- Titles are visible text. A title with a hidden or direction-changing character is refused, and the
  error names the character and its position.
- You can add, edit and remove items. Removing an item hides it but keeps it in the file. It cannot
  be brought back, so `flowd delete` asks first at a terminal, and needs `--yes` in scripts and for
  agents, which never get asked.
- You can plan today and future days. Past days are only for looking back.

### F2. Put the day in your order

- The day is listed in the order you set, and nothing reorders it by itself. Starting, pausing,
  finishing or skipping an item never moves it. New and carried items join the end.
- `o` in the workspace arranges the day: `j` and `k` select an item, `J` and `K` move it, and
  Ctrl+S saves the whole order at once. Nothing is saved until then, and Esc asks before throwing a
  new order away.
- `flowd order <ID>...` saves an order from a script, naming every item of the day once. At a
  terminal, `flowd order` arranges it the same way.
- Any item can move, whatever its status, on today and later days. If the day changed while it was
  being arranged, nothing is saved and you arrange it again.

### F3. Carry unfinished work forward

- Nothing moves to another day by itself. You choose what to carry.
- `C` in the workspace, or `flowd carry` at a terminal, lists the incomplete and skipped items from
  the 7 days before the day, with their title, the day they are from, their Duration and status,
  newest day first. You tick the ones to carry, or all of them with `a`, and Ctrl+S carries them.
- `flowd carry <ID>...` carries exact items and `flowd carry --all` carries everything listed.
  Scripts and agents have to use one of these, because the list needs a terminal.
- A carried item is a new todo item on the day, today unless `--date` names a later one, with the
  same title and the original Duration. It starts with no work and remembers the item it came
  from. The original is not changed and stays incomplete or skipped.
- An item is carried once. While its copy is still planned it is not offered again, and deleting
  the copy frees it.
- Everything picked is carried in one save. If it does not all fit in what is left of the day,
  nothing is carried and the message says how much it needs and how much is left.
- Only items from the 7 days before the day can be carried, and never onto a past day.

### F4. Track with one key

- Start, pause, resume, finish and skip are one action each.
- Any item can be started at any time, in any order.
- Starting an item while another one runs pauses the running one.
- Going past the duration is fine. Nothing stops and nothing nags you.
- Finish means you are done with it for today, not that the problem is solved forever.
- A skipped item can be brought back, and a finished one can be reopened, on the same day.
- Only today's items can be started, paused, finished or skipped.

### F5. Show what really happened

- Work is the time an item actually ran, adding up every running stretch.
- Paused is the time it waited as paused after it started, adding up every pause. Time spent done
  before a reopen, or skipped before an unskip, is not a pause.
- Ten minutes running, twenty paused and five more running is 15 minutes of work and 20 paused.

### F6. Close the day at midnight

- At midnight anything still not done becomes incomplete, even if the app was closed.
- Done and skipped items stay as they are.
- Incomplete is final. Tomorrow starts empty until you plan it or carry something over.

### F7. Review the day

- The total duration you planned and the total work you recorded.
- Progress as finished items out of all items, like `3 / 5 finished`.
- Every item grouped by its status.

### F8. The command line

- `flowd` opens the workspace, and any argument runs the command line instead. `flowd 2026-10-12`
  shows that date.
- Commands that need an item take its full ID, which `flowd list` shows and every change prints. A
  position in the list is never accepted, because the order can be changed, and a command must
  never act on an item that moved.
- At a terminal, a command that needs an item can be run without the ID. It then lists the items
  that command fits, like only the todo items for `start`, and you pick one with the arrow keys or
  `j` and `k`, Enter to choose and Esc to cancel. Cancelling changes nothing. Without a terminal, or
  with `--json`, a missing ID is refused, so nothing ever waits for an answer.
- Items are shown in a table drawn like the workspace, with the ID, the title, the Duration and the
  status, under the date it is about, like `Thu 19 Nov 2026`. `flowd list` shows every item of the
  day, and a change shows only the item it changed. At a terminal the status has the same colors as
  in the workspace, and a title too long for the window is shortened with `…`. Piped output and
  `NO_COLOR` stay plain.
- Options can go before or after the command, like `flowd list --date 2026-11-18`. Each one can be
  given only once.
- A person sees an error as one sentence and a hint, like `Error: Only 2h30m left on this day.`
- Tab completion for fish, zsh and bash completes commands, options, dates and item IDs, showing
  each item's title next to its ID where the shell can. It offers only the items the command fits.
  `flowd completion <SHELL>` prints the script, and the installer prints the one line that sets it
  up, without editing shell files itself. Completion runs on every Tab, so like the status line it
  takes no lock, never writes and stays quiet on any problem.

### F9. Work with scripts and AI agents

- Every command can print JSON with `--json`, so scripts and agents never have to parse text.
- Errors have stable codes and exit codes. Bad input exits with 2 and a refused action exits with 1.
  With `--json` an error is printed as JSON with its code, its message and its hint. The code is
  only for scripts and agents, and people never see it.
- If a save fails in a way that leaves it unknown whether it landed, flowd says so plainly instead
  of reporting success or failure.
- A refused or failed action never prints output that looks like success.
- flowd ships with an Agent Skill that teaches an agent how to use these commands safely. It is
  built into the program and only installed when asked, with `flowd skill --claude` for Claude Code
  in `~/.claude/skills/flowd`, `flowd skill --codex` for Codex in `~/.agents/skills/flowd`, or both.
  A folder there that holds anything other than the flowd skill is never replaced.
- An agent only changes the plan when you ask it to. Titles are treated as plain data, never as
  instructions.

### F10. A terminal workspace

- `flowd` with no arguments opens a keyboard-only workspace showing today, so scripts and pipes
  never get the workspace.
- It lists the day the same way the command line does and keeps the selected item selected while
  the list refreshes, even when the item moves.
- Details show the full title, status, Duration, Work, Paused, Started At and Finished At in a
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
- Anything you put together and then commit saves with Ctrl+S: a form, a new order and the items to
  carry. Enter only chooses or moves, and never saves.
- `o` arranges the day and `C` carries unfinished work, as described above. The footer offers both
  whenever the day on screen can be planned, and `order [o]` once it has two items.
- `c` opens a month calendar at the bottom. `h` and `l` move a day, `j` and `k` a week, `t` goes to
  today and Enter shows the chosen day. `m` and `y` let you type a month number or a year in the
  footer, keeping the day where it can. The cursor date is in brackets and teal, like `[17]`.
  Looking at a day never creates it.
- Whatever is active is teal: the selected item, the cursor date and the focused form field.
- The header is split into four cells: `flowd`, a note, the date and the clock. When another day is
  on screen the note says `Viewing: 2026-11-18`, and `t` comes back to today. While the day is being
  arranged it says `Reordering`. For today it is empty.
- The footer only offers what that day allows: tracking today, planning today and later, and only
  looking on past days. It never drops a key. When they do not all fit on one row, the ones for the
  selected item and the day come first, and `more [.]` sits at the right edge. `.` opens the rest on
  the rows below, with `less [.]` in its place, and any other key folds them away again.
- `r` opens a read-only review of the day on screen, the same as `flowd review`: total duration and
  recorded work, progress as finished out of all items, and every item grouped under its status with
  a count. `j` and `k` scroll it and Esc goes back. Nothing can be changed from it.
- `d` asks `Delete "Study"?  yes [y]  no [n]` in the footer. Only `y` deletes, `n` and Esc cancel,
  and no other key answers. A long title is shortened so the answers always show. Past days cannot
  be changed this way either.
- `?` opens Help, listing every key. The full wording of the last error stays there after its
  popup has gone. Inside a form, `?` is just text.
- Success is quiet: the list simply shows the new state. Errors and refusals appear in a small box
  above the footer for seven seconds.

### F11. A status line

- `flowd now --short` prints the running item as one line for tmux, a shell prompt or any bar that
  runs a command, like `Deep work on the parser 1h3m/2h`. Past its duration it adds how far over,
  like `2h10m/2h +10m`. Work is in whole minutes, so a bar that refreshes every second stays calm.
- The title is cut to 30 columns with `…`, and `--max <N>` changes that.
- When nothing is running it prints nothing.
- It can be asked every second without ever making a real command wait or fail. It takes no lock,
  never writes, never creates the database and never closes a day.
- A missing, busy or broken database prints nothing and still succeeds. A prompt is the wrong place
  for an error, and `flowd doctor` explains it.

### F12. Never lose a day

- A saved day survives a crash, a kill or a restart.
- A file made by an older flowd is backed up before it is upgraded, and the upgrade either finishes
  or leaves the file exactly as it was.

## How well it does it

- Speed. The aim is that flowd opens in under 50 ms and a keypress shows up in under 16 ms, even
  with two years of history. Timing tests in CI fail if opening a large history or drawing a frame
  gets structurally slower. `flowd list` takes about 10 ms today.
- Offline. It never uses the network.
- Reliability. Every action is saved to disk before the next one. A broken file fails with a clear
  error and is never replaced.
- Size. It uses less than 30 MB of memory and ships as one file under 15 MB. Today a command uses
  about 11 MB of memory and a release download is about 3 MB.
- Support. `flowd doctor` checks the data file and `flowd doctor --repair` restores the backup.
- Data. The file lives at `$XDG_DATA_HOME/flowd/flowd.sqlite`, or `~/.local/share/flowd/flowd.sqlite`
  when `XDG_DATA_HOME` is not set to an absolute path.

## Not in scope

- Teams, or syncing between devices.
- Importing calendars, phone apps or web apps.
- Notifications while the app is closed.
- Searching across days.
- Recommendations about what to do next, recurring items, copying items to another day by itself,
  or editing recorded times.
