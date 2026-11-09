#!/bin/sh
set -eu
[ "$#" -eq 1 ] || exit 2
title=$1
case "$title" in *'
'*) printf '%s\n' 'PR title must be one line' >&2; exit 1 ;; esac
if printf '%s' "$title" | LC_ALL=C grep -q '[[:cntrl:]]'; then
    printf '%s\n' 'PR title must not contain control characters' >&2
    exit 1
fi
if ! printf '%s\n' "$title" | LC_ALL=C grep -Eq '^(feat|fix|perf|refactor|docs|style|test|build|ci|chore|revert)(\([^()]+\))?!?: [^[:space:]].*$'; then
    printf '%s\n' 'Use a Conventional Commit title, for example: feat(cli): add review' >&2
    exit 1
fi
