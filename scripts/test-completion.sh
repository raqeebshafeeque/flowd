#!/bin/sh
# Runs the tab completion scripts in real shells against a built flowd and a
# fresh database: start offers only todo items, pause only the running one,
# and a partial command is completed. A shell that is not installed is
# skipped, and so is everything when no shell is found.
# usage: test-completion.sh FLOWD
set -eu
[ "$#" -eq 1 ] || { printf '%s\n' 'usage: test-completion.sh FLOWD' >&2; exit 2; }
work=$(mktemp -d "${TMPDIR:-/tmp}/flowd-completion-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir "$work/bin"
cp "$1" "$work/bin/flowd"
PATH="$work/bin:$PATH"
db=$work/flowd.sqlite
flowd --db "$db" add 1h "Deep work" > /dev/null
flowd --db "$db" add 30m "Inbox" > /dev/null
running=$(flowd --db "$db" --json list | sed -n 's/.*"id":"\(it_[0-9A-Z]*\)","day_id".*"title":"Deep work".*/\1/p' | head -n 1)
[ -n "$running" ] || { printf '%s\n' 'could not find the item to start' >&2; exit 1; }
flowd --db "$db" start "$running" > /dev/null
waiting=$(flowd --db "$db" --json list | tr '{' '\n' | sed -n 's/^"id":"\(it_[0-9A-Z]*\)".*"title":"Inbox".*/\1/p')
failed=0

# check SHELL WHAT GOT WANT
check() {
    if [ "$3" != "$4" ]; then
        printf '%s, %s: got [%s], want [%s]\n' "$1" "$2" "$3" "$4" >&2
        failed=1
    fi
}

flowd completion fish > "$work/flowd.fish"
flowd completion bash > "$work/flowd.bash"
flowd completion zsh > "$work/_flowd"

if command -v fish > /dev/null; then
    check fish start "$(fish --no-config -c "source $work/flowd.fish; complete -C 'flowd --db $db start '")" "$waiting	Inbox"
    check fish pause "$(fish --no-config -c "source $work/flowd.fish; complete -C 'flowd --db $db pause '")" "$running	Deep work"
    check fish command "$(fish --no-config -c "source $work/flowd.fish; complete -C 'flowd fin'" | cut -f1)" "finish"
fi
if command -v bash > /dev/null; then
    complete_bash() {
        bash --norc -c ". $work/flowd.bash; COMP_WORDS=($1); COMP_CWORD=$2; _flowd; printf '%s\n' \"\${COMPREPLY[@]}\""
    }
    check bash start "$(complete_bash "flowd --db $db start ''" 4)" "$waiting"
    check bash pause "$(complete_bash "flowd --db $db pause ''" 4)" "$running"
    check bash command "$(complete_bash "flowd fin" 1)" "finish"
fi
if command -v zsh > /dev/null; then
    complete_zsh() {
        zsh -f -c "compdef() { :; }; _describe() { print -l -- \"\${(@P)4}\"; }; . $work/_flowd; words=($1); CURRENT=$2; _flowd"
    }
    check zsh start "$(complete_zsh "flowd --db $db start ''" 5)" "$waiting:Inbox"
    check zsh pause "$(complete_zsh "flowd --db $db pause ''" 5)" "$running:Deep work"
    check zsh command "$(complete_zsh "flowd fin" 2 | cut -d: -f1)" "finish"
fi
[ "$failed" -eq 0 ] && printf '%s\n' 'completion checks passed'
exit "$failed"
