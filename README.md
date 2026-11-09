# flowd

A local-only terminal day planner. One binary, fully offline; no sync or background service.

## Run

```sh
go build -o flowd ./cmd/flowd
./flowd --help
./flowd
./flowd --date 2026-10-14 list
./flowd --json now
```

Global options go before the command. `--db PATH` chooses a database; `--timezone Asia/Kolkata`
chooses a new day's zone. Existing days retain their saved timezone. Empty day views create no
planner rows. See `--help` for the commands currently available.

Data lives at `$XDG_DATA_HOME/flowd/flowd.sqlite`, otherwise
`~/.local/share/flowd/flowd.sqlite`.

## Docs

- [Vision](docs/00-vision.md)
- [Requirements](docs/01-requirements.md)
- [Domain rules](docs/02-domain.md)
- [Architecture](docs/03-architecture.md)
