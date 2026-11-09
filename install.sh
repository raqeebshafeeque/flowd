#!/bin/sh
# Install a verified flowd release and its matching Agent Skill. No build toolchain is used.
set -eu
umask 077

repository=https://github.com/raqeebshafeeque/flowd
version=
force=false
bin_dir=
skills_dir=
stage=

fail() { printf 'flowd install: %s\n' "$*" >&2; exit 1; }
usage_error() { printf 'flowd install: %s\n' "$*" >&2; exit 2; }
help() {
    printf '%s\n' 'Usage: sh install.sh [--version vX.Y.Z] [--bin-dir DIR] [--skills-dir DIR] [--force]' \
        'Downloads a public release; defaults to ~/.local/bin and ~/.agents/skills.' \
        'Requires curl, tar, standard shell utilities and sha256sum or shasum.' \
        'Does not build source, use sudo, edit shell startup files or operate on planner data.'
}

parse_args() {
    seen_version=false seen_bin=false seen_skills=false
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --help|-h) help; exit 0 ;;
            --force) [ "$force" = false ] || usage_error '--force supplied twice'; force=true; shift ;;
            --version|--bin-dir|--skills-dir)
                [ "$#" -ge 2 ] && [ -n "$2" ] || usage_error "$1 needs a value"
                case "$1" in
                    --version) [ "$seen_version" = false ] || usage_error '--version supplied twice'; seen_version=true; version=$2 ;;
                    --bin-dir) [ "$seen_bin" = false ] || usage_error '--bin-dir supplied twice'; seen_bin=true; bin_dir=$2 ;;
                    --skills-dir) [ "$seen_skills" = false ] || usage_error '--skills-dir supplied twice'; seen_skills=true; skills_dir=$2 ;;
                esac
                shift 2 ;;
            *) usage_error "unknown argument: $1" ;;
        esac
    done
}

prerequisites() {
    for utility in curl tar awk cat chmod cmp cp dirname grep mkdir mktemp mv rm sort uname; do
        command -v "$utility" >/dev/null 2>&1 || fail "required utility not found: $utility"
    done
    if command -v sha256sum >/dev/null 2>&1; then checksum_tool=sha256sum
    elif command -v shasum >/dev/null 2>&1; then checksum_tool=shasum
    else fail 'required utility not found: sha256sum or shasum'
    fi
}

# Called indirectly by the EXIT trap installed during preparation.
# shellcheck disable=SC2329
cleanup() { [ -z "$stage" ] || rm -rf "$stage"; }

fetch() {
    curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 2 \
        --connect-timeout 10 --max-time 120 "$@"
}

normal_version() {
    printf '%s\n' "$1" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
}

verify_checksum() {
    expected=$(awk -v asset="$asset" '
        NF != 2 || length($1) != 64 || $1 ~ /[^0-9a-fA-F]/ || $2 ~ /[^a-zA-Z0-9_.-]/ { bad=1 }
        { if (seen[$2]++) bad=1; if ($2 == asset) { count++; digest=tolower($1) } }
        END { if (bad || count != 1) exit 1; print digest }
    ' "$stage/checksums.txt") || fail 'invalid or missing release checksum entry'
    if [ "$checksum_tool" = sha256sum ]; then
        actual=$(sha256sum "$stage/archive.tar.gz" | awk '{print $1}')
    else
        actual=$(shasum -a 256 "$stage/archive.tar.gz" | awk '{print $1}')
    fi
    [ "$actual" = "$expected" ] || fail 'release checksum mismatch; no files installed'
}

validate_payload() {
    payload=$stage/payload
    tar -tzf "$stage/archive.tar.gz" > "$stage/members" || fail 'cannot list release archive'
    printf '%s\n' flowd VERSION LICENSE skills/flowd/SKILL.md skills/flowd/VERSION \
        skills/flowd/references/commands.md skills/flowd/references/safety.md | sort > "$stage/expected"
    sort "$stage/members" > "$stage/actual"
    cmp -s "$stage/expected" "$stage/actual" || fail 'unexpected, duplicate or missing archive members'
    tar -tvzf "$stage/archive.tar.gz" > "$stage/types" || fail 'cannot inspect archive types'
    awk 'substr($0,1,1) != "-" { bad=1 } END { exit bad }' "$stage/types" || fail 'archive links or unsupported entry types are refused'
    mkdir -p "$payload/skills/flowd/references" || fail 'cannot create payload staging'
    for member in flowd VERSION LICENSE skills/flowd/SKILL.md skills/flowd/VERSION \
        skills/flowd/references/commands.md skills/flowd/references/safety.md; do
        tar -xzOf "$stage/archive.tar.gz" "$member" > "$payload/$member" || fail "cannot stage $member"
        [ -s "$payload/$member" ] || fail "empty package member: $member"
    done
    printf '%s\n' "$version" > "$stage/version"
    if ! cmp -s "$stage/version" "$payload/VERSION" || ! cmp -s "$stage/version" "$payload/skills/flowd/VERSION"; then
        fail 'package version records disagree'
    fi
    grep -q '^name: flowd$' "$payload/skills/flowd/SKILL.md" || fail 'invalid skill name'
    grep -q '^description: .' "$payload/skills/flowd/SKILL.md" || fail 'missing skill description'
    for reference in references/commands.md references/safety.md; do
        grep -Fq "($reference)" "$payload/skills/flowd/SKILL.md" || fail "skill reference missing: $reference"
    done
    chmod 755 "$payload/flowd" || fail 'cannot mark staged executable'
    actual_version=$("$payload/flowd" --version) || fail 'release executable cannot run on this host'
    [ "$actual_version" = "flowd $version" ] || fail 'executable version mismatch'
    actual_json=$("$payload/flowd" --json --version) || fail 'release JSON version check failed'
    [ "$actual_json" = "{\"version\":\"$version\"}" ] || fail 'executable JSON version mismatch'
    "$payload/flowd" --help >/dev/null || fail 'release help check failed'
}

prepare_install() {
    parse_args "$@"
    prerequisites
    if [ -z "$version" ]; then
        location=$(fetch -o /dev/null -w '%{url_effective}' "$repository/releases/latest") || fail 'cannot resolve latest public release'
        case "$location" in
            "$repository/releases/tag/v"*) version=${location#"$repository/releases/tag/v"} ;;
            *) fail 'unexpected latest release URL' ;;
        esac
    else version=${version#v}
    fi
    normal_version "$version" || usage_error 'version must be a normal X.Y.Z release'
    case "$(uname -s)" in Darwin) platform=darwin ;; Linux) platform=linux ;; *) fail 'unsupported operating system' ;; esac
    case "$(uname -m)" in
        x86_64|amd64) architecture=amd64 ;;
        aarch64|arm64) architecture=arm64 ;;
        *) fail 'unsupported CPU architecture' ;;
    esac
    asset=flowd_${version}_${platform}_${architecture}.tar.gz
    stage=$(mktemp -d "${TMPDIR:-/tmp}/flowd-install.XXXXXX") || fail 'cannot create private staging'
    trap 'cleanup' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    base=$repository/releases/download/v$version
    fetch -o "$stage/checksums.txt" "$base/checksums.txt" || fail 'release checksums unavailable; no files installed'
    fetch -o "$stage/archive.tar.gz" "$base/$asset" || fail 'release archive unavailable; no files installed'
    verify_checksum
    validate_payload
}

main() {
    prepare_install "$@"
    fail "verified release; publication to ${bin_dir:-\"$HOME/.local/bin\"} and ${skills_dir:-\"$HOME/.agents/skills\"} is not available in this development revision"
}

main "$@"
