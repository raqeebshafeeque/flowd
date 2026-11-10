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
binary_stage=
skill_stage=
binary_lock=
skill_lock=
publication=false
new_binary=false
new_skill=false

fail() { printf 'flowd install: %s\n' "$*" >&2; exit 1; }
usage_error() { printf 'flowd install: %s\n' "$*" >&2; exit 2; }
help() {
    printf '%s\n' 'Usage: sh install.sh [--version vX.Y.Z] [--bin-dir DIR] [--skills-dir DIR] [--force]' \
        'Downloads a public release; defaults to ~/.local/bin and ~/.agents/skills.' \
        'Requires curl, tar, gzip, standard shell utilities and sha256sum or shasum.' \
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
    for utility in curl tar awk cat chmod cmp cp dirname env grep gzip mkdir mktemp mv rm rmdir sort uname; do
        command -v "$utility" >/dev/null 2>&1 || fail "required utility not found: $utility"
    done
    if command -v sha256sum >/dev/null 2>&1; then checksum_tool=sha256sum
    elif command -v shasum >/dev/null 2>&1; then checksum_tool=shasum
    else fail 'required utility not found: sha256sum or shasum'
    fi
}

# Called indirectly by the EXIT trap installed during preparation.
# shellcheck disable=SC2329
cleanup() {
    status=$?
    trap - EXIT INT TERM
    restored=true
    if [ "$publication" = true ]; then
        if [ "$new_skill" = true ]; then rm -rf "$skill_target" || restored=false; fi
        if [ -n "$skill_stage" ] && [ -e "$skill_stage/old" ]; then
            if [ -e "$skill_target" ] || [ -L "$skill_target" ]; then restored=false
            elif ! mv "$skill_stage/old" "$skill_target"; then restored=false
            fi
        fi
        if [ "$new_binary" = true ]; then rm -f "$binary_target" || restored=false; fi
        if [ -n "$binary_stage" ] && [ -e "$binary_stage/old" ]; then
            if [ -e "$binary_target" ] || [ -L "$binary_target" ]; then restored=false
            elif ! mv "$binary_stage/old" "$binary_target"; then restored=false
            fi
        fi
        if [ "$restored" = true ]; then
            printf '%s\n' 'flowd install: prior installation restored after failed publication' >&2
        else
            printf 'flowd install: restoration incomplete; inspect binary %s and skill %s\nrecovery: %s %s\n' \
                "$binary_target" "$skill_target" "$binary_stage" "$skill_stage" >&2
            status=1
        fi
    fi
    [ -z "$stage" ] || rm -rf "$stage" || status=1
    if [ "$restored" = true ]; then
        [ -z "$binary_stage" ] || rm -rf "$binary_stage" || status=1
        [ -z "$skill_stage" ] || rm -rf "$skill_stage" || status=1
    fi
    for lock in "$binary_lock" "$skill_lock"; do
        [ -n "$lock" ] || continue
        if ! rm -f "$lock/owner" || ! rmdir "$lock"; then
            printf 'flowd install: cannot release install lock: %s\n' "$lock" >&2
            status=1
        fi
    done
    exit "$status"
}

fetch() {
    curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 2 \
        --connect-timeout 10 --max-time 120 "$@"
}

normal_version() {
    case "$1" in ''|*[!0-9.]*) return 1 ;; esac
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

# Environment isolation avoids incidental planner access and credential inheritance.
# It is not an OS sandbox for a malicious publisher's executable.
run_staged() (
    CDPATH='' cd "$stage/home" || exit 1
    env -i PATH="${PATH:-/usr/bin:/bin}" HOME="$stage/home" \
        XDG_DATA_HOME="$stage/home/data" TMPDIR="$stage/home" TZ=UTC "$payload/flowd" "$@"
)

validate_payload() {
    payload=$stage/payload
    tar -tzf "$stage/archive.tar.gz" > "$stage/members" || fail 'cannot list release archive'
    printf '%s\n' flowd VERSION LICENSE THIRD_PARTY_NOTICES skills/flowd/SKILL.md skills/flowd/VERSION \
        skills/flowd/references/commands.md skills/flowd/references/safety.md | sort > "$stage/expected"
    sort "$stage/members" > "$stage/actual"
    cmp -s "$stage/expected" "$stage/actual" || fail 'unexpected, duplicate or missing archive members'
    tar -tvzf "$stage/archive.tar.gz" > "$stage/types" || fail 'cannot inspect archive types'
    awk 'substr($0,1,1) != "-" { bad=1 } END { exit bad }' "$stage/types" || fail 'archive links or unsupported entry types are refused'
    mkdir -p "$payload/skills/flowd/references" || fail 'cannot create payload staging'
    for member in flowd VERSION LICENSE THIRD_PARTY_NOTICES skills/flowd/SKILL.md skills/flowd/VERSION \
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
    mkdir "$stage/home" || fail 'cannot create private preflight home'
    actual_version=$(run_staged --version) || fail 'release executable cannot run on this host'
    [ "$actual_version" = "flowd $version" ] || fail 'executable version mismatch'
    actual_json=$(run_staged --json --version) || fail 'release JSON version check failed'
    [ "$actual_json" = "{\"version\":\"$version\"}" ] || fail 'executable JSON version mismatch'
    run_staged --help >/dev/null || fail 'release help check failed'
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
    stage=$(CDPATH='' cd "$stage" && pwd -P) || fail 'cannot resolve private staging'
    base=$repository/releases/download/v$version
    fetch -o "$stage/checksums.txt" "$base/checksums.txt" || fail 'release checksums unavailable; no files installed'
    fetch -o "$stage/archive.tar.gz" "$base/$asset" || fail 'release archive unavailable; no files installed'
    verify_checksum
    validate_payload
}

publish_payload() {
    [ -n "${HOME:-}" ] || fail 'HOME must be set for user installation'
    bin_dir=${bin_dir:-"$HOME/.local/bin"}
    skills_dir=${skills_dir:-"$HOME/.agents/skills"}
    mkdir -p "$bin_dir" "$skills_dir" || fail 'cannot create install directories'
    bin_dir=$(CDPATH='' cd "$bin_dir" && pwd -P)
    skills_dir=$(CDPATH='' cd "$skills_dir" && pwd -P)
    binary_target=$bin_dir/flowd
    skill_target=$skills_dir/flowd
    [ "$binary_target" != "$skill_target" ] || fail 'binary and skill destinations must be distinct'
    case "$bin_dir/" in "$skill_target/"*) fail 'binary directory must not be inside the managed skill destination' ;; esac
    # Cooperating installers must own both parents before inspecting or moving targets.
    # Never override an existing lock, including after a crash or with --force.
    if mkdir "$bin_dir/.flowd-install.lock" 2>/dev/null; then binary_lock=$bin_dir/.flowd-install.lock
    else fail "cannot acquire install lock: $bin_dir/.flowd-install.lock; inspect for another or interrupted installer"
    fi
    if mkdir "$skills_dir/.flowd-install.lock" 2>/dev/null; then skill_lock=$skills_dir/.flowd-install.lock
    else fail "cannot acquire install lock: $skills_dir/.flowd-install.lock; inspect for another or interrupted installer"
    fi
    for lock in "$binary_lock" "$skill_lock"; do
        printf '%s\n' "$$" > "$lock/owner" || fail 'cannot record install lock owner'
    done
    if [ -L "$binary_target" ] || [ -L "$skill_target" ]; then fail 'destination symlinks are refused, even with --force'; fi
    if [ -e "$binary_target" ] && [ ! -f "$binary_target" ]; then fail 'executable destination is not a regular file'; fi
    if [ -e "$skill_target" ] && [ ! -d "$skill_target" ]; then fail 'skill destination is not a real directory'; fi
    managed=false
    record=$skill_target/.flowd-install
    if [ -f "$record" ] && [ ! -L "$record" ] && [ -f "$skill_target/VERSION" ] && [ ! -L "$skill_target/VERSION" ]; then
        previous_version=$(cat "$skill_target/VERSION")
        if normal_version "$previous_version"; then
            printf 'flowd-install-v1\n%s\n%s\n' "$previous_version" "$binary_target" > "$stage/previous-record"
            if cmp -s "$stage/previous-record" "$record"; then managed=true; fi
        fi
    fi
    if [ "$force" = false ] && [ "$managed" = false ]; then
        if [ -e "$binary_target" ] || [ -e "$skill_target" ]; then fail 'unmanaged flowd destination exists; review it before using --force'; fi
    fi
    binary_stage=$(mktemp -d "$bin_dir/.flowd-bin.XXXXXX") || fail 'cannot stage executable beside destination'
    skill_stage=$(mktemp -d "$skills_dir/.flowd-skill.XXXXXX") || fail 'cannot stage skill beside destination'
    cp "$payload/flowd" "$binary_stage/new" || fail 'cannot stage executable'
    chmod 755 "$binary_stage/new" || fail 'cannot set executable mode'
    cp -R "$payload/skills/flowd" "$skill_stage/new" || fail 'cannot stage skill'
    for notice in LICENSE THIRD_PARTY_NOTICES; do
        cp "$payload/$notice" "$skill_stage/new/$notice" || fail "cannot retain $notice"
    done
    chmod -R u=rwX,go=rX "$skill_stage/new" || fail 'cannot set skill modes'
    printf 'flowd-install-v1\n%s\n%s\n' "$version" "$binary_target" > "$skill_stage/new/.flowd-install" || fail 'cannot record installation ownership'
    chmod 600 "$skill_stage/new/.flowd-install" || fail 'cannot set ownership record mode'
    publication=true
    if [ -e "$binary_target" ]; then mv "$binary_target" "$binary_stage/old" || fail 'cannot retain prior executable'; fi
    if [ -e "$skill_target" ]; then mv "$skill_target" "$skill_stage/old" || fail 'cannot retain prior skill'; fi
    new_binary=true
    mv "$binary_stage/new" "$binary_target" || fail 'cannot publish executable'
    new_skill=true
    mv "$skill_stage/new" "$skill_target" || fail 'cannot publish skill'
    publication=false
    printf 'Installed flowd %s\nbinary: %s\nskill: %s\n' "$version" "$binary_target" "$skill_target"
    case :${PATH:-}: in
        *:"$bin_dir":*) ;;
        *) printf "Add %s to your shell's PATH.\n" "$bin_dir" ;;
    esac
    printf '%s\n' 'Refresh your Agent Skills discovery (Pi: /reload, then /skill:flowd).'
}

main() {
    prepare_install "$@"
    publish_payload
}

main "$@"
