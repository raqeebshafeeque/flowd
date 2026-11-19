#!/bin/sh
# Runs check-release-draft.sh against a fake GitHub CLI that lists the
# releases written in a file, one JSON array per call, to check that a
# missing, late, doubled, published or empty draft is handled.
set -eu
work=$(mktemp -d "${TMPDIR:-/tmp}/flowd-draft-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir "$work/bin"
# The fake prints the next line of $work/answers on each call, and keeps
# repeating the last one.
cat > "$work/bin/gh" <<SCRIPT
#!/bin/sh
n=\$(cat "$work/calls" 2>/dev/null || echo 0)
n=\$((n + 1))
echo "\$n" > "$work/calls"
total=\$(wc -l < "$work/answers")
[ "\$n" -le "\$total" ] || n=\$total
line=\$(sed -n "\${n}p" "$work/answers")
printf '%s\n' "\$line" | jq -r '.[] | select(.tag_name == "v1.0.0") | "\(.draft) \(.assets | length)"'
SCRIPT
chmod 755 "$work/bin/gh"

draft='{"tag_name":"v1.0.0","draft":true,"assets":[]}'
built='{"tag_name":"v1.0.0","draft":true,"assets":[{},{}]}'
published='{"tag_name":"v1.0.0","draft":false,"assets":[{}]}'
other='{"tag_name":"v0.9.0","draft":false,"assets":[{}]}'
failed=0

# expect NAME MODE OUTCOME ANSWER...
expect() {
    name=$1 mode=$2 outcome=$3
    shift 3
    : > "$work/answers"
    for answer in "$@"; do printf '%s\n' "$answer" >> "$work/answers"; done
    rm -f "$work/calls"
    if PATH="$work/bin:$PATH" GITHUB_REPOSITORY=o/r CHECK_DRAFT_TRIES=3 CHECK_DRAFT_DELAY=0 \
        sh scripts/check-release-draft.sh "$mode" v1.0.0 > "$work/out" 2>&1; then
        result=ok
    else
        result=refused
    fi
    if [ "$result" != "$outcome" ]; then
        printf '%s: expected %s, got %s\n' "$name" "$outcome" "$result" >&2
        cat "$work/out" >&2
        failed=1
    fi
}

expect "draft is listed"            wait  ok      "[$other,$draft]"
expect "draft is listed late"       wait  ok      "[$other]" "[$other]" "[$other,$draft]"
expect "draft never shows up"       wait  refused "[$other]"
expect "two drafts"                 wait  refused "[$draft,$draft]"
expect "already published"          wait  refused "[$published]"
expect "one built draft"            built ok      "[$built,$other]"
expect "built draft has no files"   built refused "[$draft]"
expect "build made a second draft"  built refused "[$draft,$built]"
expect "nothing to check"           built refused "[$other]"

[ "$failed" -eq 0 ] && printf '%s\n' 'release draft checks passed'
exit "$failed"
