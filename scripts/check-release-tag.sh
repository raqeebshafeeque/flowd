#!/bin/sh
set -eu
[ "$#" -eq 1 ] || exit 2
tag=$1
single=$(printf '%s' "$tag" | tr -d '\n\r')
if [ "$single" != "$tag" ] || ! printf '%s\n' "$tag" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
    printf '%s\n' 'release tag must be a normal vX.Y.Z tag' >&2
    exit 2
fi
commit=$(git rev-parse --verify "$tag^{commit}")
[ "$(git rev-parse HEAD)" = "$commit" ] || { printf '%s\n' 'checkout does not match release tag' >&2; exit 1; }
git merge-base --is-ancestor "$commit" refs/remotes/origin/main || { printf '%s\n' 'release tag is not on main history' >&2; exit 1; }
if [ -n "${EXPECTED_RELEASE_SHA:-}" ] && [ "$commit" != "$EXPECTED_RELEASE_SHA" ]; then
    printf '%s\n' 'release tag does not match the approved release commit' >&2
    exit 1
fi
version=${tag#v}
record=$(git show "$tag:.release-please-manifest.json" | jq -er '.["."]')
[ "$record" = "$version" ] || { printf '%s\n' 'tag and approved version manifest disagree' >&2; exit 1; }
printf '%s\n' "$tag"
