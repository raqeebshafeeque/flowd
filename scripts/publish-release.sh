#!/bin/sh
# Trusted CI/owner entrypoint. Never call this from an untrusted PR job.
set -eu
[ "$#" -eq 1 ] || { printf '%s\n' 'usage: publish-release.sh vX.Y.Z' >&2; exit 2; }
: "${GH_TOKEN:?Configure the scoped publication token in the trusted job}"
tag=$(sh scripts/check-release-tag.sh "$1")
version=${tag#v}
repository=raqeebshafeeque/flowd
staging=$(mktemp -d "${TMPDIR:-/tmp}/flowd-publish.XXXXXX")
trap 'rm -rf "$staging"' EXIT

gh release view "$tag" --repo "$repository" --json tagName,isDraft,isPrerelease > "$staging/release.json"
[ "$(jq -er '.tagName' "$staging/release.json")" = "$tag" ] || { printf '%s\n' 'release tag mismatch' >&2; exit 1; }
[ "$(jq -r '.isPrerelease' "$staging/release.json")" = false ] || { printf '%s\n' 'prerelease publication is not supported' >&2; exit 1; }
draft=$(jq -r '.isDraft' "$staging/release.json")
case "$draft" in
    true) goreleaser release --clean ;;
    false) printf '%s\n' 'Release already published; verifying without rebuilding or replacing assets.' ;;
    *) printf '%s\n' 'invalid release draft state' >&2; exit 1 ;;
esac

mkdir "$staging/assets"
gh release download "$tag" --repo "$repository" --dir "$staging/assets" \
    --pattern checksums.txt --pattern install.sh --pattern "flowd_${version}_*.tar.gz"
go run ./scripts/releasecheck "$version" "$staging/assets"

if [ "$draft" = true ]; then
    gh release edit "$tag" --repo "$repository" --draft=false --latest
    printf 'Published verified release %s\n' "$tag"
else
    printf 'Verified existing release %s; no assets or tags changed.\n' "$tag"
fi
