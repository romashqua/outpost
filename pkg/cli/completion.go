package cli

import "fmt"

// BashCompletion returns a bash completion script for the given binary.
func BashCompletion(binary string, commands []string) string {
	return fmt.Sprintf(`# bash completion for %[1]s
_%[1]s() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    commands="%[2]s"

    case "${prev}" in
        %[1]s)
            COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
            return 0
            ;;
        users|networks|devices|gateways)
            COMPREPLY=( $(compgen -W "list get create delete" -- "${cur}") )
            return 0
            ;;
        audit)
            COMPREPLY=( $(compgen -W "--action --user --limit" -- "${cur}") )
            return 0
            ;;
        compliance)
            COMPREPLY=( $(compgen -W "report soc2 iso27001 gdpr" -- "${cur}") )
            return 0
            ;;
    esac
}
complete -F _%[1]s %[1]s
`, binary, joinWords(commands))
}

// ZshCompletion returns a zsh completion script for the given binary.
func ZshCompletion(binary string, commands []CommandDef) string {
	s := fmt.Sprintf(`#compdef %s

_%[1]s() {
    local -a commands
    commands=(
`, binary)
	for _, cmd := range commands {
		s += fmt.Sprintf("        '%s:%s'\n", cmd.Name, cmd.Description)
	}
	s += fmt.Sprintf(`    )

    _arguments \
        '1:command:->cmds' \
        '*::arg:->args'

    case "$state" in
        cmds)
            _describe -t commands 'command' commands
            ;;
        args)
            case $words[1] in
                users|networks|devices|gateways)
                    _values 'subcommand' list get create delete
                    ;;
                compliance)
                    _values 'subcommand' report soc2 iso27001 gdpr
                    ;;
            esac
            ;;
    esac
}

_%[1]s "$@"
`, binary)
	return s
}

// CommandDef describes a command for completion generation.
type CommandDef struct {
	Name        string
	Description string
}

func joinWords(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += " "
		}
		result += s
	}
	return result
}
