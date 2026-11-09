#!/bin/sh
set -eu

[ "$#" -eq 1 ] || { printf '%s\n' 'usage: stage-release.sh X.Y.Z' >&2; exit 2; }
version=$1
single=$(printf '%s' "$version" | tr -d '\n\r')
if [ "$single" != "$version" ] || ! printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
    printf '%s\n' 'release staging requires a normal version' >&2
    exit 2
fi

mkdir -p .release/package/skills/flowd/references
printf '%s\n' "$version" > .release/package/VERSION
printf '%s\n' "$version" > .release/package/skills/flowd/VERSION
cp skills/flowd/SKILL.md .release/package/skills/flowd/SKILL.md
cp skills/flowd/references/commands.md .release/package/skills/flowd/references/commands.md
cp skills/flowd/references/safety.md .release/package/skills/flowd/references/safety.md
