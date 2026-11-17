#!/bin/sh
# Fails when a function cannot be reached from the program or its tests.
set -eu
report=$(go run golang.org/x/tools/cmd/deadcode@v0.52.0 -test ./... 2>&1) || {
    printf '%s\n' "$report" >&2
    exit 1
}
unreachable=$(printf '%s\n' "$report" | grep -v '^go: ' | grep -v '^$' || true)
if [ -n "$unreachable" ]; then
    printf '%s\n' 'Unreachable functions, remove them:' "$unreachable" >&2
    exit 1
fi
printf '%s\n' 'No unreachable functions.'
