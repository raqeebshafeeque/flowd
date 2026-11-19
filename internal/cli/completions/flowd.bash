# flowd tab completion for bash. Load it from ~/.bashrc with:
#   source <(flowd completion bash)
# flowd itself works out the candidates, so this only passes on what was
# typed so far and the word being completed. bash cannot show the titles
# next to the IDs, so only the values are offered.

_flowd() {
    local cur=${COMP_WORDS[COMP_CWORD]}
    # The words are copied before IFS changes, because bash 3.2, the one
    # macOS ships, joins a sliced array with IFS inside quotes.
    local -a typed=("${COMP_WORDS[@]:1:COMP_CWORD-1}")
    local IFS=$'\n'
    COMPREPLY=($(flowd __complete "${typed[@]}" "$cur" 2>/dev/null | cut -f1))
}

complete -o nosort -F _flowd flowd 2>/dev/null || complete -F _flowd flowd
