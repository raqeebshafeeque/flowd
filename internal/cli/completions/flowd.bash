# flowd tab completion for bash. Load it from ~/.bashrc with:
#   source <(flowd completion bash)
# flowd itself works out the candidates, so this only passes on what was
# typed so far and the word being completed. bash cannot show the titles
# next to the IDs, so only the values are offered.

_flowd() {
    local IFS=$'\n'
    local cur=${COMP_WORDS[COMP_CWORD]}
    COMPREPLY=($(flowd __complete "${COMP_WORDS[@]:1:COMP_CWORD-1}" "$cur" 2>/dev/null | cut -f1))
}

complete -o nosort -F _flowd flowd 2>/dev/null || complete -F _flowd flowd
