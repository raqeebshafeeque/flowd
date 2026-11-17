#!/bin/sh
# Runs install.sh against a locally built release (goreleaser --snapshot)
# with a fake GitHub CLI, to check how it handles build provenance:
# verified, failed, not available, and required.
# usage: test-installer.sh DIST_DIR
set -eu
[ "$#" -eq 1 ] || { printf '%s\n' 'usage: test-installer.sh DIST_DIR' >&2; exit 2; }
dist=$(CDPATH='' cd "$1" && pwd -P)
work=$(mktemp -d "${TMPDIR:-/tmp}/flowd-installer-test.XXXXXX")
trap 'rm -rf "$work"' EXIT

# fake_gh writes a gh that is signed in and whose verify succeeds or fails,
# or one that is not signed in at all.
fake_gh() {
    mkdir -p "$work/fake-$1"
    cat > "$work/fake-$1/gh" <<SCRIPT
#!/bin/sh
case "\$1 \$2" in
    "attestation verify") [ "\$3" = --help ] && exit 0; [ "$1" = verified ] && exit 0; exit 1 ;;
    "auth status") [ "$1" = signed-out ] && exit 1; exit 0 ;;
esac
exit 1
SCRIPT
    chmod 755 "$work/fake-$1/gh"
    printf '%s\n' "$work/fake-$1"
}

# expect NAME FAKE OUTCOME [installer options]
expect() {
    name=$1 fake=$2 outcome=$3
    shift 3
    target=$work/$name
    if PATH="$fake:$PATH" sh install.sh --from "$dist" --version 0.0.0 --bin-dir "$target/bin" --skills-dir "$target/skills" "$@" > "$work/$name.out" 2>&1; then
        result=installed
    else
        result=refused
    fi
    if [ "$result" != "$outcome" ]; then
        printf '%s: expected %s, got %s\n' "$name" "$outcome" "$result" >&2
        cat "$work/$name.out" >&2
        exit 1
    fi
    if [ "$outcome" = refused ] && [ -e "$target/bin/flowd" ]; then
        printf '%s: refused but installed files anyway\n' "$name" >&2
        exit 1
    fi
    printf 'ok  %s\n' "$name"
}

verified=$(fake_gh verified)
failing=$(fake_gh failing)
signed_out=$(fake_gh signed-out)

expect provenance-verified "$verified" installed
grep -q 'Verified (signed build provenance)' "$work/provenance-verified.out"
expect provenance-failed "$failing" refused
grep -q 'build provenance verification failed' "$work/provenance-failed.out"
expect no-gh-checksum-only "$signed_out" installed
grep -q 'Checksum verified' "$work/no-gh-checksum-only.out"
expect no-gh-but-required "$signed_out" refused --require-attestation
expect verified-and-required "$verified" installed --require-attestation
