# flowd tab completion for fish. Install it with:
#   flowd completion fish > ~/.config/fish/completions/flowd.fish
# flowd itself works out the candidates, so this only passes on what was
# typed so far and the word being completed.
complete -c flowd -f -a '(flowd __complete (commandline -opc)[2..-1] "$(commandline -ct)" 2>/dev/null)'
