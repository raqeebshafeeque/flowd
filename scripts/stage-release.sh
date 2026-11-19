#!/bin/sh
# Writes the VERSION file a release archive contains into .release/package.
# The Agent Skill is not in the archive, because it is built into flowd and
# installed with flowd skill. goreleaser runs this before building.
set -eu
[ "$#" -eq 1 ] || { printf '%s\n' 'usage: stage-release.sh X.Y.Z' >&2; exit 2; }
version=$1
if ! printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
    printf '%s\n' 'the version must look like 1.2.3' >&2
    exit 2
fi
rm -rf .release
mkdir -p .release/package
printf '%s\n' "$version" > .release/package/VERSION
