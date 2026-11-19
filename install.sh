#!/bin/sh
# Install a flowd release. Nothing is built from source. The Agent Skill is
# built into flowd and installed only when asked, with flowd skill.
set -eu
umask 077

repository=https://github.com/raqeebshafeeque/flowd
version=
force=false
from=
require_attestation=false
attestation_note=
bin_dir=
stage=
binary_stage=
binary_lock=
publication=false
new_binary=false
refreshed_skills=
install_complete=false
interactive=false
progress_active=false

progress_init() {
    if [ -t 1 ] && [ -n "${TERM:-}" ] && [ "$TERM" != dumb ] && [ -z "${NO_COLOR:-}" ]; then
        interactive=true
    fi
}

progress_start() {
    if [ "$interactive" = true ]; then
        printf '  \033[36m›\033[0m %s... ' "$2"
        progress_active=true
    else
        printf '[%s/4] %s...\n' "$1" "$2"
    fi
}

progress_pause() {
    if [ "$progress_active" = true ]; then
        printf '\r\033[2K'
        progress_active=false
    fi
}

progress_end() {
    if [ "$progress_active" = true ]; then
        printf '\033[0m\n'
        progress_active=false
    fi
}

progress_done() {
    if [ "$interactive" = true ]; then
        progress_pause
        printf '  \033[32m✓ %s\033[0m\n' "$1"
    fi
}

# say prints one line of the closing message, bold when color is allowed.
say() {
    if [ "$interactive" = true ]; then printf '\033[1m%s\033[0m\n' "$1"; else printf '%s\n' "$1"; fi
}

show_success() {
    printf '\n'
    if [ "$interactive" = true ]; then
        printf '\033[1;32m%s\033[0m\n' "Wohoo! Installed flowd $version!"
    else
        printf '%s\n' "Wohoo! Installed flowd $version!"
    fi
    printf '%s\n' "It's time to plan and lock in 🔒"
    case :${PATH:-}: in
        *:"$bin_dir":*) ;;
        *) printf '\n%s\n' "Add $bin_dir to your PATH, then open a new terminal." ;;
    esac
    [ -z "$attestation_note" ] || printf '\n%s\n' "$attestation_note"
    if [ -n "$refreshed_skills" ]; then
        printf '\n%s\n' "Updated the agent skill for$refreshed_skills."
    else
        printf '\n'
        say 'To install the agent skill 🤖, run:'
        printf '%s\n' '$ flowd skill --claude --codex'
    fi
    show_completion
}

# show_completion prints how to set up tab completion for the shell in use,
# unless it is set up already. The installer only reads shell files, and
# never edits them.
show_completion() {
    config=${XDG_CONFIG_HOME:-$HOME/.config}
    case ${SHELL##*/} in
        fish)
            [ ! -e "$config/fish/completions/flowd.fish" ] || return 0
            set -- '$ mkdir -p ~/.config/fish/completions' '$ flowd completion fish > ~/.config/fish/completions/flowd.fish' ;;
        zsh)
            ! grep -qs 'flowd completion zsh' "${ZDOTDIR:-$HOME}/.zshrc" || return 0
            set -- "$ echo 'source <(flowd completion zsh)' >> ~/.zshrc" ;;
        bash)
            ! grep -qs 'flowd completion bash' "$HOME/.bashrc" || return 0
            set -- "$ echo 'source <(flowd completion bash)' >> ~/.bashrc" ;;
        *) set -- '$ flowd completion bash|fish|zsh' ;;
    esac
    printf '\n'
    say 'To add tab completion, run:'
    printf '%s\n' "$@"
}

fail() { progress_end; printf 'flowd install: %s\n' "$*" >&2; exit 1; }
usage_error() { progress_end; printf 'flowd install: %s\n' "$*" >&2; exit 2; }
help() {
    printf '%s\n' 'Usage: sh install.sh [--version vX.Y.Z] [--from DIR] [--bin-dir DIR] [--force] [--require-attestation]' \
        'Downloads a public release and installs flowd to ~/.local/bin.' \
        'The Agent Skill is installed only when asked, with flowd skill --claude --codex.' \
        '--from DIR installs from release files already downloaded into DIR instead.' \
        'Needs curl, tar, gzip, standard shell tools and sha256sum or shasum.' \
        'Checks the release checksum before installing anything, and the signed build provenance' \
        'too when the GitHub CLI (gh) is installed and signed in. --require-attestation refuses to' \
        'install without that proof.' \
        'Never builds source, uses sudo, edits shell startup files or touches your planner data.'
}

parse_args() {
    seen_version=false seen_bin=false
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --help|-h) help; exit 0 ;;
            --force) [ "$force" = false ] || usage_error '--force supplied twice'; force=true; shift ;;
            --require-attestation) [ "$require_attestation" = false ] || usage_error '--require-attestation supplied twice'; require_attestation=true; shift ;;
            --version|--bin-dir|--from)
                [ "$#" -ge 2 ] && [ -n "$2" ] || usage_error "$1 needs a value"
                case "$1" in
                    --version) [ "$seen_version" = false ] || usage_error '--version supplied twice'; seen_version=true; version=$2 ;;
                    --bin-dir) [ "$seen_bin" = false ] || usage_error '--bin-dir supplied twice'; seen_bin=true; bin_dir=$2 ;;
                    --from) [ -z "$from" ] || usage_error '--from supplied twice'; from=$2 ;;
                esac
                shift 2 ;;
            *) usage_error "unknown argument: $1" ;;
        esac
    done
}

prerequisites() {
    for utility in curl tar awk cat chmod cmp cp dirname env grep gzip mkdir mktemp mv rm rmdir sed sort uname; do
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
    progress_pause
    restored=true
    if [ "$publication" = true ]; then
        if [ "$new_binary" = true ]; then rm -f "$binary_target" || restored=false; fi
        if [ -n "$binary_stage" ] && [ -e "$binary_stage/old" ]; then
            if [ -e "$binary_target" ] || [ -L "$binary_target" ]; then restored=false
            elif ! mv "$binary_stage/old" "$binary_target"; then restored=false
            fi
        fi
        if [ "$restored" = true ]; then
            printf '%s\n' 'flowd install: prior installation restored after failed publication' >&2
        else
            printf 'flowd install: restoration incomplete; inspect %s\nrecovery: %s\n' \
                "$binary_target" "$binary_stage" >&2
            status=1
        fi
    fi
    [ -z "$stage" ] || rm -rf "$stage" || status=1
    if [ "$restored" = true ]; then
        [ -z "$binary_stage" ] || rm -rf "$binary_stage" || status=1
    fi
    if [ -n "$binary_lock" ]; then
        if ! rm -f "$binary_lock/owner" || ! rmdir "$binary_lock"; then
            printf 'flowd install: cannot release install lock: %s\n' "$binary_lock" >&2
            status=1
        fi
    fi
    if [ "$status" -eq 0 ] && [ "$install_complete" = true ]; then
        progress_done 'Installed flowd'
        show_success
    fi
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

# The checksum only proves the archive matches a file from the same place.
# Provenance proves it was built by this repository's release workflow. It is
# checked before anything downloaded runs. gh is optional unless
# --require-attestation is given.
verify_attestation() {
    attestation_repository=${repository#https://github.com/}
    if command -v gh >/dev/null 2>&1 && gh attestation verify --help >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
        if gh attestation verify "$stage/archive.tar.gz" --repo "$attestation_repository" \
            --signer-workflow "$attestation_repository/.github/workflows/release.yml" >"$stage/attestation.log" 2>&1; then
            attestation_note=
            return 0
        fi
        fail 'build provenance verification failed; no files installed'
    fi
    if [ "$require_attestation" = true ]; then
        fail 'build provenance verification is required but needs the GitHub CLI (gh), installed and signed in; no files installed'
    fi
    attestation_note='Checked the download with its checksum. Sign in to the GitHub CLI (gh) to also check it was built by this repository.'
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
    printf '%s\n' flowd VERSION LICENSE | sort > "$stage/expected"
    sort "$stage/members" > "$stage/actual"
    cmp -s "$stage/expected" "$stage/actual" || fail 'unexpected, duplicate or missing archive members'
    tar -tvzf "$stage/archive.tar.gz" > "$stage/types" || fail 'cannot inspect archive types'
    awk 'substr($0,1,1) != "-" { bad=1 } END { exit bad }' "$stage/types" || fail 'archive links or unsupported entry types are refused'
    mkdir -p "$payload" || fail 'cannot create payload staging'
    for member in flowd VERSION LICENSE; do
        tar -xzOf "$stage/archive.tar.gz" "$member" > "$payload/$member" || fail "cannot stage $member"
        [ -s "$payload/$member" ] || fail "empty package member: $member"
    done
    printf '%s\n' "$version" > "$stage/version"
    cmp -s "$stage/version" "$payload/VERSION" || fail 'package version records disagree'
    chmod 755 "$payload/flowd" || fail 'cannot mark staged executable'
    mkdir "$stage/home" || fail 'cannot create private preflight home'
    actual_version=$(run_staged --version) || fail 'release executable cannot run on this host'
    [ "$actual_version" = "flowd $version" ] || fail 'executable version mismatch'
    run_staged --help >/dev/null || fail 'release help check failed'
}

prepare_install() {
    parse_args "$@"
    prerequisites
    if [ -n "$version" ]; then
        version=${version#v}
        normal_version "$version" || usage_error 'version must be a normal X.Y.Z release'
    fi
    progress_init
    progress_start 1 'Selecting release'
    if [ -n "$from" ]; then
        [ -d "$from" ] || usage_error "--from directory not found: $from"
        from=$(CDPATH='' cd "$from" && pwd -P)
        [ -n "$version" ] || usage_error '--from needs --version too'
    fi
    if [ -z "$version" ]; then
        location=$(fetch -o /dev/null -w '%{url_effective}' "$repository/releases/latest") || fail 'cannot resolve latest public release'
        case "$location" in
            "$repository/releases/tag/v"*) version=${location#"$repository/releases/tag/v"} ;;
            *) fail 'unexpected latest release URL' ;;
        esac
    fi
    normal_version "$version" || usage_error 'version must be a normal X.Y.Z release'
    case "$(uname -s)" in
        Darwin) platform=darwin; platform_name=macOS ;;
        Linux) platform=linux; platform_name=Linux ;;
        *) fail 'unsupported operating system' ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64) architecture=amd64 ;;
        aarch64|arm64) architecture=arm64 ;;
        *) fail 'unsupported CPU architecture' ;;
    esac
    progress_done "Selected v$version for $platform_name $architecture"
    asset=flowd_${version}_${platform}_${architecture}.tar.gz
    stage=$(mktemp -d "${TMPDIR:-/tmp}/flowd-install.XXXXXX") || fail 'cannot create private staging'
    trap 'cleanup' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    stage=$(CDPATH='' cd "$stage" && pwd -P) || fail 'cannot resolve private staging'
    base=$repository/releases/download/v$version
    progress_start 2 "Downloading flowd $version for $platform_name $architecture"
    if [ -n "$from" ]; then
        cp "$from/checksums.txt" "$stage/checksums.txt" || fail "checksums.txt not found in $from; no files installed"
        cp "$from/$asset" "$stage/archive.tar.gz" || fail "$asset not found in $from; no files installed"
    else
        fetch -o "$stage/checksums.txt" "$base/checksums.txt" || fail 'release checksums unavailable; no files installed'
        fetch -o "$stage/archive.tar.gz" "$base/$asset" || fail 'release archive unavailable; no files installed'
    fi
    progress_done 'Downloaded release'
    progress_start 3 'Verifying checksum and package'
    verify_checksum
    verify_attestation
    validate_payload
    progress_done 'Verified checksum and package'
}

publish_payload() {
    [ -n "${HOME:-}" ] || fail 'HOME must be set for user installation'
    bin_dir=${bin_dir:-"$HOME/.local/bin"}
    mkdir -p "$bin_dir" || fail 'cannot create install directory'
    bin_dir=$(CDPATH='' cd "$bin_dir" && pwd -P)
    binary_target=$bin_dir/flowd
    # Cooperating installers must own the directory before inspecting or moving the target.
    # Never override an existing lock, including after a crash or with --force.
    if mkdir "$bin_dir/.flowd-install.lock" 2>/dev/null; then binary_lock=$bin_dir/.flowd-install.lock
    else fail "cannot acquire install lock: $bin_dir/.flowd-install.lock; inspect for another or interrupted installer"
    fi
    printf '%s\n' "$$" > "$binary_lock/owner" || fail 'cannot record install lock owner'
    if [ -L "$binary_target" ]; then fail 'destination symlinks are refused, even with --force'; fi
    if [ -e "$binary_target" ] && [ ! -f "$binary_target" ]; then fail 'executable destination is not a regular file'; fi
    # The record says this installer put the binary there. Older installers
    # kept it in the skill folder, which is still accepted.
    record=$bin_dir/.flowd-install
    printf 'flowd-install-v2\n%s\n' "$binary_target" > "$stage/record"
    managed=false
    if [ -f "$record" ] && [ ! -L "$record" ] && cmp -s "$stage/record" "$record"; then managed=true; fi
    legacy=$HOME/.agents/skills/flowd/.flowd-install
    if [ "$managed" = false ] && [ -f "$legacy" ] && [ ! -L "$legacy" ] &&
        [ "$(sed -n 1p "$legacy")" = flowd-install-v1 ] && [ "$(sed -n 3p "$legacy")" = "$binary_target" ]; then
        managed=true
    fi
    if [ "$force" = false ] && [ "$managed" = false ] && [ -e "$binary_target" ]; then
        fail 'unmanaged flowd destination exists; review it before using --force'
    fi
    binary_stage=$(mktemp -d "$bin_dir/.flowd-bin.XXXXXX") || fail 'cannot stage executable beside destination'
    cp "$payload/flowd" "$binary_stage/new" || fail 'cannot stage executable'
    chmod 755 "$binary_stage/new" || fail 'cannot set executable mode'
    publication=true
    if [ -e "$binary_target" ]; then mv "$binary_target" "$binary_stage/old" || fail 'cannot retain prior executable'; fi
    new_binary=true
    mv "$binary_stage/new" "$binary_target" || fail 'cannot publish executable'
    publication=false
    cp "$stage/record" "$record" || fail 'cannot record installation ownership'
    chmod 600 "$record" || fail 'cannot set ownership record mode'
    refresh_skills
    install_complete=true
}

# refresh_skills updates the Agent Skill where it was installed before, so it
# always matches the new flowd. It is never installed anywhere new, and a
# failure here only means the old skill stays.
refresh_skills() {
    for pair in "claude:$HOME/.claude/skills/flowd:Claude Code" "codex:$HOME/.agents/skills/flowd:Codex"; do
        flag=${pair%%:*} rest=${pair#*:}
        folder=${rest%%:*} name=${rest#*:}
        [ -f "$folder/SKILL.md" ] && [ "$(sed -n 2p "$folder/SKILL.md")" = 'name: flowd' ] || continue
        if "$binary_target" skill "--$flag" > /dev/null 2>&1; then
            if [ -z "$refreshed_skills" ]; then refreshed_skills=" $name"; else refreshed_skills="$refreshed_skills and $name"; fi
        fi
    done
}

main() {
    prepare_install "$@"
    progress_start 4 'Installing flowd'
    publish_payload
}

main "$@"
