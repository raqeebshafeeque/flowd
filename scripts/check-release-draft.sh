#!/bin/sh
# Makes sure a release tag has exactly one draft, so the build and the
# install test always use the same one.
#
# wait   Before the build. Waits up to a minute for GitHub to list the draft
#        release-please made, so goreleaser adds to it instead of making a
#        second one. Refuses if there is more than one.
# built  After the build. Refuses unless there is exactly one release for the
#        tag, still a draft, with files on it.
#
# usage: check-release-draft.sh wait|built TAG
# GITHUB_REPOSITORY names the repository. CHECK_DRAFT_TRIES and
# CHECK_DRAFT_DELAY change how long wait keeps looking.
set -eu
[ "$#" -eq 2 ] || { printf '%s\n' 'usage: check-release-draft.sh wait|built TAG' >&2; exit 2; }
mode=$1 tag=$2
repo=${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}
tries=${CHECK_DRAFT_TRIES:-12}
delay=${CHECK_DRAFT_DELAY:-5}

# releases prints one line per release for the tag: draft and file count.
releases() {
    gh api --paginate "repos/$repo/releases" --jq ".[] | select(.tag_name == \"$tag\") | \"\(.draft) \(.assets | length)\""
}

case $mode in
    wait)
        try=1
        while :; do
            found=$(releases)
            count=$(printf '%s' "$found" | grep -c . || true)
            if [ "$count" -gt 1 ]; then
                printf '%s has %s releases. Delete the extra ones and run this again.\n' "$tag" "$count" >&2
                exit 1
            fi
            if [ "$count" -eq 1 ]; then
                [ "${found%% *}" = true ] || { printf '%s is already published\n' "$tag" >&2; exit 1; }
                printf 'Found the draft for %s\n' "$tag"
                exit 0
            fi
            [ "$try" -lt "$tries" ] || { printf 'No draft for %s was listed in time\n' "$tag" >&2; exit 1; }
            try=$((try + 1))
            sleep "$delay"
        done
        ;;
    built)
        found=$(releases)
        count=$(printf '%s' "$found" | grep -c . || true)
        [ "$count" -eq 1 ] || { printf '%s has %s releases, expected one draft\n' "$tag" "$count" >&2; exit 1; }
        [ "${found%% *}" = true ] || { printf '%s is already published\n' "$tag" >&2; exit 1; }
        [ "${found#* }" -gt 0 ] || { printf 'The draft for %s has no files\n' "$tag" >&2; exit 1; }
        printf 'The draft for %s has %s files\n' "$tag" "${found#* }"
        ;;
    *)
        printf '%s\n' 'usage: check-release-draft.sh wait|built TAG' >&2
        exit 2
        ;;
esac
