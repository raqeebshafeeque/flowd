#!/bin/sh
# Copies the files a release archive contains into .release/package and
# stamps them with the release version. goreleaser runs this before building.
set -eu
[ "$#" -eq 1 ] || { printf '%s\n' 'usage: stage-release.sh X.Y.Z' >&2; exit 2; }
version=$1
if ! printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
    printf '%s\n' 'the version must look like 1.2.3' >&2
    exit 2
fi
rm -rf .release
mkdir -p .release/package/skills/flowd/references
printf '%s\n' "$version" > .release/package/VERSION
printf '%s\n' "$version" > .release/package/skills/flowd/VERSION
cp skills/flowd/SKILL.md .release/package/skills/flowd/SKILL.md
cp skills/flowd/references/commands.md .release/package/skills/flowd/references/commands.md
