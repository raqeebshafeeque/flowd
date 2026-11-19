# flowd commands

Options work before or after the command, like `flowd --json list --date 2026-11-18`. Each one can
be given only once. Put `--` before a title that starts with a dash.

- `--db <PATH>` uses another database file.
- `--date <YYYY-MM-DD>` picks a day other than today.
- `--timezone <ZONE>` sets the timezone of a day that does not exist yet, like `Asia/Kolkata`.
- `--json` prints JSON instead of text.
- `--version`, `--help` and `--licenses` print the version, the help or the licenses of the code
  flowd is built with. They touch no files.

Always name a command. `flowd` with nothing after it opens the terminal workspace, which needs a
person at a terminal, and fails with exit 2 when run by an agent.

`<ID>` is the full ID of an item, such as `it_01JD8K6Z2KAS98N1W1N1PMM62G`, from the JSON of `list`
or of any change. Nothing shorter and no position in the list is accepted.
`<DURATION>` is whole minutes or hours: `30`, `30m`, `2h` or `1h30m`. It cannot be longer than what is
left of the day after the other unfinished items, otherwise flowd refuses with `ERR_DURATION_LIMIT`
and says how much is left.

### list

Shows the day.

JSON:

```json
{
  "schema_version": 2,
  "date": "2026-11-17",
  "timezone": "Asia/Kolkata",
  "at": "2026-11-17T04:30:00Z",
  "items": [ITEM, ...],
  "running": ITEM or null,
  "status_counts": {"todo": 1, "in_progress": 0, "paused": 0, "done": 0, "skipped": 0, "incomplete": 0},
  "duration_minutes": 90,
  "work_ns": 0
}
```

Each ITEM has `id`, `day_id`, `title`, `status`, `duration_minutes`, `started_at`, `finished_at`,
`closed_at`, `archived`, `created_at`, `updated_at`, `work_ns` and `paused_ns`. Times are UTC.
`work_ns` and `paused_ns` are whole seconds written in nanoseconds. `work_ns` is the time the item
ran and `paused_ns` the time it waited as paused after it started. `paused_ns` is null until the
item first starts.

### add

`flowd add <DURATION> <TITLE>` plans a new item. JSON is the new ITEM. Every command below that changes
an item also prints the saved ITEM as JSON, so there is no need to read the day again after each one.

### edit

`flowd edit [--title <TITLE>] [--duration <DURATION>] <ID>` changes the title, the Duration or both.

### delete

`flowd delete --yes <ID>` deletes an item. Deleted items are gone from every list and cannot be
brought back. Without `--yes` flowd asks a person at a terminal, and refuses with `ERR_USAGE` and
exit 2 everywhere else, including with `--json`. Only pass `--yes` when the user asked to delete
that exact item.

### start

`flowd start <ID>` starts a todo item and pauses whatever was running.

### pause

`flowd pause <ID>` pauses the running item.

### resume

`flowd resume <ID>` continues a paused item and pauses whatever was running.

### finish

`flowd finish <ID>` marks a running or paused item done for today.

### skip

`flowd skip <ID>` drops a todo, running or paused item for today.

### unskip

`flowd unskip <ID>` brings back a skipped item, as todo if it never started, otherwise paused.

### reopen

`flowd reopen <ID>` starts a done item again and pauses whatever was running.

### now

`flowd now` shows the running item. JSON is that ITEM or null.

### review

`flowd review` shows the total Duration, the recorded work, how many items are finished and every
item grouped by status. JSON is the same as `list`.

### events

`flowd events <ID>` shows the recorded history of an item. JSON is a list of events with `seq`,
`item_id`, `from_status`, `to_status`, `occurred_at`, `recorded_at`, `reason`, `before` and `after`.

### backup

`flowd backup` writes a backup next to the database. JSON is `{"backup": "PATH"}`.

### doctor

`flowd doctor` checks the database and prints `OK` or the first problem. It also warns about item
titles that hold hidden characters, which are shown as placeholders like `<U+202E>`. JSON is
`{"status": "ok", "unsafe_titles": [ID, ...]}`. `flowd doctor --repair` replaces the database with its backup and moves the
current files into a recovery folder. Only run it when the user asks.

### completion

`flowd completion bash|fish|zsh` prints the tab completion script for a shell, for a person to load.
Agents never need it. A command that needs an ID and gets none opens an arrow key picker at a
terminal, but an agent is never at one, so it must always pass the full ID. Without a terminal, or
with `--json`, a missing ID is refused with `ERR_USAGE`.

## Exit status

`0` success, `1` a refused action or a problem with the database, `2` bad input, `130` stopped.
With `--json`, an error is printed to stderr as `{"error": {"code": "ERR_NOT_FOUND", "message": "...",
"hint": "..."}}` and nothing is printed to stdout. Without `--json` it is a sentence and a hint
for a person, with no code, so always use `--json`.
