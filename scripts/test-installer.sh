#!/bin/sh
# Runs install.sh against a locally built release (goreleaser --snapshot)
# with a fake GitHub CLI, to check how it handles build provenance:
# verified, failed, not available, and required. It also checks the closing
# message, that the Agent Skill is only refreshed where it already was, and
# that an older install is still recognised. Every run gets its own HOME, so
# nothing outside the test folder is touched.
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
# The run's HOME is $work/NAME/home, which a test can prepare beforehand.
# XDG_CONFIG_HOME and ZDOTDIR point inside it too, because CI runners set
# their own.
expect() {
    name=$1 fake=$2 outcome=$3
    shift 3
    target=$work/$name
    mkdir -p "$target/home"
    existed=false
    [ ! -e "$target/bin/flowd" ] || existed=true
    if HOME="$target/home" XDG_CONFIG_HOME="$target/home/.config" ZDOTDIR="$target/home" PATH="$fake:$PATH" sh install.sh --from "$dist" --version 0.0.0 --bin-dir "$target/bin" "$@" > "$work/$name.out" 2>&1; then
        result=installed
    else
        result=refused
    fi
    if [ "$result" != "$outcome" ]; then
        printf '%s: expected %s, got %s\n' "$name" "$outcome" "$result" >&2
        cat "$work/$name.out" >&2
        exit 1
    fi
    if [ "$outcome" = refused ] && [ "$existed" = false ] && [ -e "$target/bin/flowd" ]; then
        printf '%s: refused but installed files anyway\n' "$name" >&2
        exit 1
    fi
    printf 'ok  %s\n' "$name"
}

verified=$(fake_gh verified)
failing=$(fake_gh failing)
signed_out=$(fake_gh signed-out)

expect provenance-verified "$verified" installed
grep -q 'Wohoo! Installed flowd 0.0.0!' "$work/provenance-verified.out"
if grep -q 'Checked the download' "$work/provenance-verified.out"; then printf '%s\n' 'verified: should not warn about provenance' >&2; exit 1; fi
expect provenance-failed "$failing" refused
grep -q 'build provenance verification failed' "$work/provenance-failed.out"
expect no-gh-checksum-only "$signed_out" installed
grep -q 'Checked the download with its checksum' "$work/no-gh-checksum-only.out"
expect no-gh-but-required "$signed_out" refused --require-attestation
expect verified-and-required "$verified" installed --require-attestation

# The skill is never installed on its own. The message says how to.
expect no-skill-installed "$verified" installed
grep -q 'flowd skill' "$work/no-skill-installed.out"
if [ -e "$work/no-skill-installed/home/.agents" ] || [ -e "$work/no-skill-installed/home/.claude" ]; then printf '%s\n' 'no-skill-installed: the skill was installed' >&2; exit 1; fi

# A skill installed before is updated to match, and only there.
mkdir -p "$work/refresh-skill/home/.claude/skills/flowd"
printf -- '---\nname: flowd\ndescription: old\n---\n' > "$work/refresh-skill/home/.claude/skills/flowd/SKILL.md"
expect refresh-skill "$verified" installed
grep -q 'Updated the agent skill for Claude Code.' "$work/refresh-skill.out"
grep -q '^0.0.0$' "$work/refresh-skill/home/.claude/skills/flowd/VERSION"
if [ -e "$work/refresh-skill/home/.agents" ]; then printf '%s\n' 'refresh-skill: installed somewhere new' >&2; exit 1; fi

# A binary from an older installer, which kept its record in the skill
# folder, is still replaced without --force.
mkdir -p "$work/older-install/home/.agents/skills/flowd" "$work/older-install/bin"
cp "$work/provenance-verified/bin/flowd" "$work/older-install/bin/flowd"
bin=$(CDPATH='' cd "$work/older-install/bin" && pwd -P)
printf 'flowd-install-v1\n0.0.0\n%s\n' "$bin/flowd" > "$work/older-install/home/.agents/skills/flowd/.flowd-install"
printf -- '---\nname: flowd\n---\n' > "$work/older-install/home/.agents/skills/flowd/SKILL.md"
expect older-install "$verified" installed
grep -q 'Updated the agent skill for Codex, OpenCode, Pi and Gemini CLI.' "$work/older-install.out"

# A flowd that no installer put there is never replaced without --force.
mkdir -p "$work/unmanaged/bin"
printf 'mine\n' > "$work/unmanaged/bin/flowd"
expect unmanaged "$verified" refused
grep -q '^mine$' "$work/unmanaged/bin/flowd"

# Tab completion is suggested for the shell in use, and only until it is
# set up. The installer never edits shell files itself.
user_shell=${SHELL:-}
for shell in fish zsh bash; do
    SHELL=/bin/$shell
    export SHELL
    expect "completion-hint-$shell" "$verified" installed
    grep -q "flowd completion $shell" "$work/completion-hint-$shell.out"
done
mkdir -p "$work/completion-done/home/.config/fish/completions"
: > "$work/completion-done/home/.config/fish/completions/flowd.fish"
SHELL=/bin/fish
expect completion-done "$verified" installed
if grep -q 'tab completion' "$work/completion-done.out"; then printf '%s\n' 'completion-done: should not suggest completion again' >&2; exit 1; fi
SHELL=$user_shell
