package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// completionCommands is used by the generated shell completion scripts. Keep
// it in sync with the Usage section in cf.go.
var completionCommands = []string{
	"config", "submit", "list", "ncontests", "parse", "gen", "test", "watch",
	"open", "stand", "sid", "detail", "race", "pull", "sync",
	"start", "end",
	"clone", "upgrade", "statement", "material", "completion",
	"db",
	"lpractice", "lrepractice", "lnrepractice", "opractice",
}

const bashCompletion = `# bash completion for cf
# Load it with: source <(cf completion bash)
_cf() {
    local cur prev
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    case "$prev" in
        -f|--file)
            COMPREPLY=( $(compgen -f -- "$cur") )
            return 0
            ;;
        -d|--dir)
            COMPREPLY=( $(compgen -d -- "$cur") )
            return 0
            ;;
    esac

    if [[ $COMP_CWORD -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "COMMANDS" -- "$cur") )
        return 0
    fi

    case "${COMP_WORDS[1]}" in
        pull|clone)
            [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "ac" -- "$cur") )
            ;;
        watch)
            [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "all" -- "$cur") )
            ;;
        db)
            [[ $COMP_CWORD -eq 2 ]] && COMPREPLY=( $(compgen -W "sql time stats sync path" -- "$cur") )
            ;;
    esac
}
complete -F _cf cf
`

const zshCompletion = `#compdef cf
# zsh completion for cf
# Load it with: eval "$(cf completion zsh)" in ~/.zshrc
_cf() {
    local -a commands
    commands=(
        'config:configure the tool'
        'submit:submit code'
        'list:list problems of a contest'
        'ncontests:list recent contests'
        'parse:fetch samples'
        'gen:generate code from a template'
        'test:run the template scripts and test samples'
        'watch:watch submissions'
        'open:open a problem page'
        'stand:open the standings page'
        'sid:open a submission page'
        'detail:show submission details'
        'race:start a contest'
        'pull:pull your submissions'
        'sync:sync samples, statement and latest submission'
        'start:start timing a problem or contest'
        'end:stop timing a running problem'
        'clone:clone all submissions of a user'
        'upgrade:upgrade cf'
        'statement:fetch problem statements'
        'material:fetch contest materials'
        'completion:print a shell completion script'
        'db:query the local timer database'
        'lpractice:pick random local problems'
        'lrepractice:pick random accepted local problems'
        'lnrepractice:pick random untimed local problems'
        'opractice:pick random Codeforces problemset problems'
    )

    if (( CURRENT == 2 )); then
        _describe 'command' commands
        return
    fi

    case "${words[2]}" in
        pull|clone)
            _values 'option' ac
            ;;
        watch)
            _values 'scope' all
            ;;
        db)
            _values 'db command' sql time stats sync path
            ;;
    esac
}
compdef _cf cf
`

const fishCompletion = `# fish completion for cf
# Load it with: cf completion fish > ~/.config/fish/completions/cf.fish
complete -c cf -f -n "not __fish_seen_subcommand_from COMMANDS" -a "COMMANDS"
complete -c cf -n "__fish_seen_subcommand_from pull clone" -a ac
complete -c cf -n "__fish_seen_subcommand_from watch" -a all
complete -c cf -n "__fish_seen_subcommand_from db" -a "sql time stats sync path"
`

// Completion prints a shell completion script.
func Completion() (err error) {
	shell := strings.TrimSpace(Args.Shell)
	if shell == "" {
		shell = filepath.Base(os.Getenv("SHELL"))
	}
	commands := strings.Join(completionCommands, " ")
	switch shell {
	case "bash":
		fmt.Print(strings.ReplaceAll(bashCompletion, "COMMANDS", commands))
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(strings.ReplaceAll(fishCompletion, "COMMANDS", commands))
	default:
		return fmt.Errorf("Unsupported shell %q. Use bash, zsh or fish", shell)
	}
	return nil
}
