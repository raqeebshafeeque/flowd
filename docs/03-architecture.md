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
| `internal/tui` | The terminal UI, added later |

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
- When flowd opens a file that already exists, it first checks it read only: SQLite looks for
  damage and flowd checks the schema version and tables. An empty, damaged or foreign file is
  refused and left exactly as it was.
- Hard links and special files are refused. A symbolic link to a real database is followed.

## Errors

- An error is a short code plus a plain sentence, printed to stderr. Examples are
  `ERR_ILLEGAL_TRANSITION`, `ERR_CORRUPT`, `ERR_LOCKED` and `ERR_DISK_FULL`.
- The store hands errors back with their cause and never prints anything itself.
- A damaged or unknown file is refused and left alone, never replaced.

## Doctor and backup

- `flowd backup` writes a consistent copy next to the database, ending in `.bak`.
- `flowd doctor` checks the file, its permissions, its schema version and its integrity, then
  prints `OK` or the first problem it found.
- `flowd doctor --repair` is the only command that restores the backup.

## Agents

- Agents use the same command line as people. There is no separate interface for them.
- The JSON output is a contract. Its fields only change with a new schema version.
- The Agent Skill lives in `skills/flowd/` and is installed together with the program, so its
  instructions always match the installed version.

## Distribution

- Each release has an archive per operating system and processor, with checksums.
- An `install.sh` script downloads, checks and installs the program and the Agent Skill without
  needing Go.
