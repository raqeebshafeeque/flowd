# Architecture

How the parts fit together.

## Overall shape

- One Go program for macOS and Linux. There is no server and no network.
- One SQLite file. Every action is a single transaction that is written to disk before it returns.
- The command line version comes first. The terminal UI comes later and uses exactly the same
  operations.

## Packages

| Package | Job |
| --- | --- |
| `cmd/flowd` | Starts the program |
| `internal/domain` | Items, status changes, time and rules, with no file or network access |
| `internal/store` | SQLite tables, transactions and backups |
| `internal/planner` | The operations every frontend shares |
| `internal/cli` | Commands and their output |
| `internal/tui` | The terminal workspace |

- Packages only depend on the ones below them. The frontends use the planner, the planner uses the
  store, and the store uses the domain.
- All rules live in the domain, so the command line and the terminal UI can never disagree.

## Saving

- A change to an item and its event are written in the same transaction.
- Work and elapsed are never stored. They are worked out from the events when they are needed.
- Closing a day at midnight happens the next time the file is opened, so it works even if flowd
  was not running. Every operation does this first.
- Times are stored as UTC text in RFC 3339 format.
- Before a transaction commits, the whole day is checked against the domain rules and every item's
  history is replayed. A change that breaks a rule never reaches the disk.
- A change carries a copy of the item as it was read. If the saved item has changed since then, the
  change is refused instead of overwriting newer work.
- Only one process uses the file at a time. A second one gets a clear message that the file is
  busy. The lock lives in a separate `.lock` file next to the database, which is never deleted.
- The database is created private to you, and its folder too. A file that others can read is
  refused.
- When flowd opens a file that already exists, it first checks it read only. The schema version and
  tables are checked every time. SQLite's full scan for damage reads the whole file, so it runs only
  when it can tell something: when the file changed since flowd last closed it cleanly, when the last
  passed scan is more than a day old, when a crash left a journal behind, at the start of a workspace
  session, and always for `flowd backup` and `flowd doctor`. An empty, damaged or foreign file is
  refused and left exactly as it was.
- After each clean close flowd writes a private `flowd.sqlite.check` file with the database's size,
  modification time and SQLite change counter. Anything wrong with that file only causes a scan,
  never a skip. On two years of daily use, this makes opening take a few milliseconds.
- Hard links and special files are refused. A symbolic link to a real database is followed.

## Shared operations

- Every operation opens the file, reads the clock once, closes any day whose midnight has passed,
  does its work and closes the file again. Nothing holds the database while a person is thinking.
- What counts as today comes from this computer's timezone. Choosing another timezone for a day
  cannot reopen a day that has already passed here.
- A change names the revision of the item it was based on, a fingerprint of the item and its whole
  history. If the item has changed since, the change is refused.
- Starting, resuming or reopening an item pauses the running one in the same save, at the same
  instant.
- Every change ends in one of three outcomes. Refused means nothing was saved. Confirmed means it is
  on disk. Unconfirmed means the save failed in a way that leaves it unknown whether it landed.
- An unconfirmed change is never retried blindly. flowd looks for it in the saved history first and
  only then says whether it happened. A day closing at midnight afterwards does not hide that it did.

## Text safety

- One policy in the domain decides which characters a title may hold. Titles are checked against it
  when they are added or edited, and any older title is cleaned with it when it is read, so every
  output shows the same visible text, JSON included.
- A repository test fails if a hidden character appears in the source or the docs. Tests that need
  one write it as an escape such as `\u202e`.

## Errors

- An error is a short code plus a plain sentence, printed to stderr. Examples are
  `ERR_ILLEGAL_TRANSITION`, `ERR_CORRUPT`, `ERR_LOCKED` and `ERR_DISK_FULL`.
- The store hands errors back with their cause and never prints anything itself.
- A damaged or unknown file is refused and left alone, never replaced.

## Doctor and backup

- `flowd backup` writes a consistent copy next to the database, ending in `.bak`. It includes
  changes that SQLite has not yet moved out of its WAL file. The old backup is only replaced once the
  new one is complete and on disk.
- `flowd doctor` checks the file, its permissions, its schema version and its integrity, and
  whether another flowd is using it. It prints `OK` or the first problem it found and changes nothing.
  It also warns about titles that hold hidden characters, listing the items, without treating that
  as damage.
- `flowd doctor --repair` is the only command that restores the backup. It checks the backup first.
  The current files are never deleted. They move into a private recovery folder next to the
  database, so changes made after the backup can still be recovered by hand.
- Repair writes its plan to a `.repair` file before moving anything. If it is interrupted, flowd
  refuses to open the database until `flowd doctor --repair` is run again to finish the job.

## Terminal workspace

- The workspace never holds the database between keys. Every second it compares a fingerprint of
  the database file, taken with stat alone, with the one from its last read. It reads again only when
  that changed, after its own change, when the day on screen moved on at midnight, and at least
  every thirty seconds. In between it measures the running work again from what it already has, so
  the counters keep moving without touching the file. A change made from the command line shows up
  within a second or two.
- If a read fails, the last good day stays on screen with the error under it.
- The workspace remembers which date is on screen, or simply "today" so it moves on by itself at
  midnight. A read that comes back for a date no longer on screen is ignored.
- Only one change is in flight at a time. While it saves, other changes are ignored.
- If a change ends unconfirmed, the workspace asks the planner to reconcile it, again on every tick
  until the history answers, and no other change can start until then.
- Durations, work and statuses are written by the same planner functions in the workspace and the
  command line, so they always read the same.
- Every row is drawn exactly as wide as the window, so resizing never leaves stray characters.

## Agents

- Agents use the same command line as people. There is no separate interface for them.
- The JSON output is a contract. Its fields only change with a new schema version.
- The Agent Skill lives in `skills/flowd/` and is installed together with the program, so its
  instructions always match the installed version.

## Distribution

- Each release has an archive per operating system and processor, with checksums. The archive holds
  the program, the Agent Skill, a VERSION file and the licenses.
- An `install.sh` script downloads, checks and installs the program and the Agent Skill without
  needing Go. It refuses an archive whose checksum or contents do not match, runs the new program
  once in an empty private home to check its version, and only then puts the files in place.
- If installing fails halfway, the previous version is put back. Two installers cannot run at once.
- `install.sh --from DIR` installs from release files that are already downloaded.
- Every release signs build provenance for its archives, checksums and installer, proving they were
  built by this repository's release workflow. The checksum alone only proves a file matches another
  file from the same place.
- When the GitHub CLI is installed and signed in, `install.sh` verifies that provenance before it
  runs anything it downloaded and refuses on a mismatch. Without it, the installer says so and relies
  on the checksum. `--require-attestation` refuses to install without the proof.
- The release workflow only gets permission to sign in the job that publishes, and it installs each
  draft with `--require-attestation` before publishing it.
- The programs are fully static, with no system libraries, so they run on any Linux, including
  minimal ones like Alpine. Building them as position independent would make them depend on the
  system's loader, so that is not done.
- Releases are cut by release-please from the commit history. Merging its release PR tags the
  version. CI then builds the archives as a draft, installs that draft with the real installer and
  only publishes it if that works.
- CI builds a full release and installs it with the real installer on every pull request, on Linux
  and macOS.
