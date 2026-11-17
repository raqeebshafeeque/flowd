# Requirements

What flowd needs to do. The command line version comes first and the terminal UI follows later
with the same rules.

## What it does

### F1. Plan a day in time blocks

- A block has a title, a planned start and a planned end.
- You can add, edit and remove blocks. Removing a block hides it but keeps it in the file.
- Two visible blocks on the same day can never overlap.
- A block starts and ends on the same day. It cannot run past midnight into the next one.
- You can plan today and future days. Past days are only for looking back.
- `flowd` opens today and `flowd 2026-10-12` opens that date.

### F2. Track with one key

- Start, pause, resume, finish, skip and stretch are one action each.
- The real start and end times are recorded for you.
- Starting a block while another one runs pauses the running one.
- Stretch moves a block's planned end 15 minutes later and checks for overlap again.
- Only today's blocks can be started, paused, finished or skipped.

### F3. See now, next and overrun

- Now is the running block. If nothing runs, it is the block planned for the current time.
- Next is the earliest block still to do after now.
- A block that runs or sits paused past its planned end is flagged as an overrun.

### F4. Carry work over by hand

- Unfinished work only moves to tomorrow when you copy it there yourself.
- The copy keeps the same clock times unless you give new ones. The original stays as it was.

### F5. Review the day

- One screen shows the score, plus the blocks that were done, missed, skipped or overrun.
- The score is the done blocks divided by the planned blocks that were not skipped, as a percentage.

### F6. Never lose a day

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
