# flowd commands

Options work before or after the command, like `flowd --json list --date 2026-11-18`. Each one can
be given only once. Put `--` before a title that starts with a dash.

- `--db <PATH>` uses another database file.
- `--date <YYYY-MM-DD>` picks a day other than today.
- `--json` prints JSON instead of text.
- `--version`, `--help` and `--licenses` print the version, the help or the licenses of the code
  flowd is built with. They touch no files.

Always name a command. `flowd` with nothing after it opens the terminal workspace, which needs a
person at a terminal, and fails with exit 2 when run by an agent.

`<ID>` is the full ID of an item, such as `it_01JD8K6Z2KAS98N1W1N1PMM62G`, from the JSON of `list`
or of any change. Nothing shorter and no position in the list is accepted.
`<DURATION>` is whole minutes or hours: `30`, `30m`, `2h` or `1h30m`. It cannot be longer than
what is left of the day after the other unfinished items, otherwise flowd refuses with
`ERR_DURATION_LIMIT` and says how much is left.

### list

Shows the day.

JSON:

```json
{
  "schema_version": 5,
  "date": "2026-11-17",
  "timezone": "Asia/Kolkata",
  "at": "2026-11-17T04:30:00Z",
  "items": [ITEM, ...],
  "running": ITEM or null,
  "status_counts": {"todo": 1, "in_progress": 0, "paused": 0, "done": 0, "incomplete": 0},
  "duration_minutes": 90,
  "work_ns": 0
}
```

Each ITEM has `id`, `day_id`, `title`, `status`, `duration_minutes`, `started_at`, `finished_at`,
`closed_at`, `archived`, `created_at`, `updated_at`, `carried_from`, `corrected_at`, `work_ns` and
`paused_ns`. Times are UTC. `carried_from` is the ID of the item it was carried from, or null.
`corrected_at` is when a forgotten finish was put right, or null.
`work_ns` and `paused_ns` are whole seconds written in nanoseconds. `work_ns` is the time the item
ran and `paused_ns` the time it waited as paused after it started. `paused_ns` is null until the
item first starts.

### add

`flowd add <DURATION> <TITLE>` plans a new item. JSON is the new ITEM. The commands that change one
item also print the saved ITEM as JSON, so there is no need to read the day again after each one.

### edit

`flowd edit [--title <TITLE>] [--duration <DURATION>] <ID>` changes the title, the Duration or both.

### delete

`flowd delete --yes <ID>` deletes an item. Deleted items are gone from every list and cannot be
brought back. Without `--yes` flowd asks a person at a terminal, and refuses with `ERR_USAGE` and
exit 2 everywhere else, including with `--json`. Only pass `--yes` when the user asked to delete
that exact item.

### order

`flowd order <ID> <ID> ...` puts the day in this order. It has to name every visible item of the day
exactly once, in the order they should be listed. Changing an item's status never moves it, and new
or carried items go to the end, so the order only changes when it is saved. JSON is the day, like
`list`. If the day changed since it was read, for example because an item was added, flowd refuses
with `ERR_STALE` and nothing changes. Read the day again and send the whole order. Only reorder
when the user asks.

### carry

`flowd carry <ID> [<ID>...]` copies incomplete items from the 7 days before the day onto
it, today unless `--date` names a later day. Each becomes a new todo item with the same title and
Duration, and `carried_from` set to the original's ID. The originals do not change. JSON is the list
of new items. `flowd carry --all` carries everything that can be carried. Everything is carried in
one save, and if it does not all fit in what is left of the day, nothing is carried and flowd
refuses with `ERR_DURATION_LIMIT`. An item that is not incomplete, is too old, or still
has a planned copy is refused with `ERR_NOT_CARRYABLE`. Only carry what the user asked for.

### start

`flowd start <ID>` starts a todo item, resumes a paused one or reopens a done one, and pauses
whatever was running.

### pause

`flowd pause <ID>` pauses the running item.

### finish

`flowd finish <ID>` marks a running or paused item done for today. On one of the last 7 days,
`flowd finish --date <DATE> <ID>` marks an item that was left incomplete done after all, for a
forgotten finish. Its work stays what was tracked and `corrected_at` says when it was put right.
Only do this when the user says the item was really finished.

### now

`flowd now` shows the running item. JSON is that ITEM or null. `flowd now --short` prints it as one
line for a person's status bar, and nothing when nothing runs. Agents use `flowd --json now`.

### review

`flowd review` shows the total Duration, the recorded work, how many items are finished and every
item grouped by status. JSON is the same as `list`.

### backup

`flowd backup` writes a backup next to the database. JSON is `{"backup": "PATH"}`.

### doctor

`flowd doctor` checks the database and prints `OK` or the first problem. JSON is
`{"status": "ok"}`. `flowd doctor --repair` replaces the database with
its backup and moves the current files into a recovery folder. Only run it when the user asks.

### reset

`flowd reset --yes` starts over with an empty planner. The current files move into a private
recovery folder next to the database, unless `--no-backup` is added, which deletes them. JSON is
`{"status": "reset", "recovery": "PATH"}` with an empty path when nothing was kept. Without `--yes`
flowd asks a person y or n to go on and y or n to keep a backup, and refuses with `ERR_USAGE` and exit
2 everywhere else. Only run it when the user clearly asked to empty the whole planner.

### completion

`flowd completion bash|fish|zsh` prints the tab completion script for a shell, for a person to load.
Agents never need it. A command that needs an ID and gets none opens an arrow key picker at a
terminal, but an agent is never at one, so it must always pass the full ID. Without a terminal, or
with `--json`, a missing ID is refused with `ERR_USAGE`.

### skill

`flowd skill --claude --codex` installs this skill for a person, into `~/.claude/skills/flowd` for
Claude Code and `~/.agents/skills/flowd` for Codex. Only run it when the user asks. A folder of
that name that holds anything other than the flowd skill is refused, never replaced.

## Exit status

`0` success, `1` a refused action or a problem with the database, `2` bad input, `130` stopped.
With `--json`, an error is printed to stderr as
`{"error": {"code": "ERR_NOT_FOUND", "message": "...", "hint": "..."}}` and nothing is printed to
stdout. Without `--json` it is a sentence and a hint for a person, with no code, so always use
`--json`.
