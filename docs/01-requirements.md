# Requirements - flowd

What the app must do and how well it must do it.

## Functional

What the app must do, each with a plain check for done.

### F1 Plan a day in time blocks

A day holds blocks with a title and a planned start and end. Blocks can be made, changed and
removed. Exact keys live in the TUI help screen, not here. The screen shows one day at a time with
keys to step back and forward. `flowd` opens today, `flowd YYYY-MM-DD` opens that date.

How to tell it works:
- Adding a block with good times shows it in plan order and it survives a restart.
- Saving a second block that overlaps the first gets a clear refusal. Overlap means half open
  `[start, end)`, so 09:00 to 10:00 beside 10:00 to 11:00 is fine.
- Changing a block into an overlap gets refused too, per the overlap rule in [domain.md](02-domain.md) (D1).
- Removing a block hides it from the plan while it stays in the store. See the delete rule in
  [domain.md](02-domain.md) (D4).

### F2 Track with one keypress

Start, pause and resume, finish, skip and stretch each cost one key. Real times get stamped
automatically. Exact keys live in the TUI help screen.

How to tell it works:
- Starting a todo block turns it running with actual start stamped.
- Starting a second block while one runs pauses the first by itself. Both moves get logged.
- Stretching a running or paused block pushes its planned end by 15 minutes, then overlap gets
  checked again.
- Every move writes one event row and saves before the next key counts.

### F3 See now, next and overrun

Today shows what runs, what comes next and what ran past its planned end.

How to tell it works:
- A running block always wins as now. With nothing running, now is the planned block holding the
  moment. Next is the earliest todo after now. Overrun is a running or paused block past its planned
  end, shown flagged.
- The screen works this out on every key and at least once a second while idle.

### F4 Carry over by hand

Unfinished work moves to tomorrow only by an explicit copy. Nothing slides on its own.

How to tell it works:
- Copying a todo, running or paused block makes a fresh todo tomorrow with `copied_from` pointing
  back. The new block keeps the same clock times by default unless fresh times get given.
- The source stays as it was. Done, skipped and archived blocks refuse to copy.
- Copying twice makes two separate blocks, each its own row.

### F5 Review the day

One screen shows the score plus done, missed, skipped and long runners.

How to tell it works:
- Score is done divided by planned minus skipped, times 100 and rounded. A finished overrun still
  counts as done while showing flagged. With 5 blocks where 1 got skipped and 3 got done with 1
  long, the math is 3 divided by 4, so 75 percent.
- The whole review reads in seconds.

### F6 Close and reopen without loss

Killing or closing never eats a saved day. Reopen brings the same day back.

How to tell it works:
- Saved blocks survive `SIGKILL` and reopen fully.
- At most the key pressed mid kill can go missing.

## Non-functional

How well it must feel. Each item carries a number so testing stays honest.

### Performance

- P1: Cold open to a usable screen in under 50ms on a base M1 Mac or a small 2 core Linux box.
  Checked with `hyperfine` and a warm page cache.
- P2: A keypress shows on screen in under 16ms at p95. Checked with a small TUI benchmark, no
  network involved.
- P3: A day still renders instantly with 2 years of daily data behind it, around 2000 blocks. No
  hard benchmark gate for now.

### Usability

- U1: Planning a day feels quick and review reads in seconds. Judged by feel in daily use.
- U2: All of F1 to F5 works keyboard only. A mouse may exist but is never needed.
- U3: Fully offline. Pulling the network changes nothing.

### Reliability

- R1: One keypress means one SQLite save with `fsync` in WAL mode. This is what makes F6 true.
- R2: A broken store never fails quiet. The app exits non zero, prints `ERR_CORRUPT` with path and
  reason to stderr and points at `flowd doctor`. Old data stays untouched.
- R3: A full disk fails that one save, keeps old data and shows a plain error.

### Footprint

- FP1: Quiet memory under 30MB.
- FP2: Download under 15MB, one static file per OS.

### Support

- S1: One file to install plus `flowd doctor`. Doctor checks the file exists and stays readable and
  writable, runs `PRAGMA integrity_check`, checks schema version and permissions. Prints `OK` or the
  first problem with a fix hint. Exit 0 on OK else 1.
- S2: Data only changes with explicit `flowd doctor --repair`, which brings back the `.bak`.
- S3: Data lives at `$XDG_DATA_HOME/flowd/flowd.sqlite`, else `~/.local/share/flowd/flowd.sqlite`.
  Backups sit beside it as `.bak`.

## Out of scope

- Team work and sync between devices.
- Calendar import and sync, phone or web client.
- Pings while the app stays closed.
- Searching across days. The store keeps history but the screen shows one day at a time with no
  cross day search.

## Assumptions

- A1: Mac and Linux terminals with 256 colors.
- A2: The system clock tells now and next. Big jumps stay rare and get a calm recalc, as
  [architecture.md](03-architecture.md) describes.
- A3: One writer. A second open copy gets a clear lock error. No merging.
- A4: Built in Go, terminal first. Times save as RFC 3339 UTC and show in local zone. Data stays on
  the machine.
