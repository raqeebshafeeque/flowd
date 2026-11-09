# flowd

A local-only terminal day planner. One binary, fully offline; no sync or background service.

## Run

```sh
go build -o flowd ./cmd/flowd
./flowd --help
./flowd
./flowd --date 2026-10-14 add 09:00 10:00 "Study"
./flowd --date 2026-10-14 edit --title "Deep study" 1
./flowd --date 2026-10-14 list
./flowd --json now
```

Global options go before the command. `--db PATH` chooses a database; `--timezone Asia/Kolkata`
chooses a new day's zone. Existing days retain their saved timezone. Empty day views create no
planner rows. `delete BLOCK` archives without erasing history. A block selector is a visible row
number, full id or unique visible-id prefix. Row numbers change; use full ids in scripts.
See `--help` for the commands currently available.

Data lives at `$XDG_DATA_HOME/flowd/flowd.sqlite`, otherwise
`~/.local/share/flowd/flowd.sqlite`.

## Docs

- [Vision](docs/00-vision.md)
- [Requirements](docs/01-requirements.md)
- [Domain rules](docs/02-domain.md)
- [Architecture](docs/03-architecture.md)
