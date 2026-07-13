package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
)

func runCompletion(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex completion <bash|zsh|fish>"), app.ExitUsage)
	}

	switch args[0] {
	case "bash":
		writeBashCompletion(cmdCtx)
	case "zsh":
		writeZshCompletion(cmdCtx)
	case "fish":
		writeFishCompletion(cmdCtx)
	default:
		return app.NewExitError(fmt.Errorf("usage: nodex completion <bash|zsh|fish>"), app.ExitUsage)
	}
	return nil
}

func writeBashCompletion(cmdCtx *Context) {
	commandsList := strings.Join(commandNames(), " ")
	fmt.Fprintln(cmdCtx.Writer, `# bash completion for nodex
_nodex_completion() {
  local cur prev cmd
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  case "$prev" in
    --output)
      COMPREPLY=( $(compgen -W "table json yaml" -- "$cur") )
      return 0
      ;;
    --timeout|--profile)
      return 0
      ;;
  esac

  if [[ "$cur" == --* ]]; then
    COMPREPLY=( $(compgen -W "--profile --output --timeout --no-color --non-interactive --quiet --verbose --debug" -- "$cur") )
    return 0
  fi

  cmd="${COMP_WORDS[1]}"
  case "$cmd" in`)
	for _, name := range commandNames() {
		cmd := commands[name]
		if len(cmd.sub) == 0 {
			continue
		}
		fmt.Fprintf(cmdCtx.Writer, "    %s)\n", name)
		fmt.Fprintf(cmdCtx.Writer, "      COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") )\n", strings.Join(subcommandNames(cmd), " "))
		fmt.Fprintln(cmdCtx.Writer, "      return 0")
		fmt.Fprintln(cmdCtx.Writer, "      ;;")
	}
	fmt.Fprintf(cmdCtx.Writer, `  esac

  if [[ $COMP_CWORD -le 1 ]]; then
    COMPREPLY=( $(compgen -W "%s help" -- "$cur") )
  fi
}
complete -F _nodex_completion nodex
`, commandsList)
}

func writeZshCompletion(cmdCtx *Context) {
	fmt.Fprintln(cmdCtx.Writer, `#compdef nodex

_nodex() {
local -a commands
commands=(`)
	for _, name := range commandNames() {
		fmt.Fprintf(cmdCtx.Writer, "  '%s:%s'\n", name, commands[name].short)
	}
	fmt.Fprintln(cmdCtx.Writer, `)

if (( CURRENT == 2 )); then
    _describe 'command' commands
    return
fi

case $words[2] in`)
	for _, name := range commandNames() {
		cmd := commands[name]
		if len(cmd.sub) == 0 {
			continue
		}
		fmt.Fprintf(cmdCtx.Writer, "  %s)\n    local -a subcommands\n    subcommands=(\n", name)
		for _, subName := range subcommandNames(cmd) {
			fmt.Fprintf(cmdCtx.Writer, "      '%s:%s'\n", subName, cmd.sub[subName].short)
		}
		fmt.Fprintln(cmdCtx.Writer, "    )\n    _describe 'subcommand' subcommands\n    ;;")
	}
	fmt.Fprintln(cmdCtx.Writer, `esac
}

_nodex "$@"`)
}

func writeFishCompletion(cmdCtx *Context) {
	fmt.Fprintln(cmdCtx.Writer, "# fish completion for nodex")
	fmt.Fprintln(cmdCtx.Writer, "complete -c nodex -f")
	for _, flag := range []string{"profile", "output", "timeout", "no-color", "non-interactive", "quiet", "verbose", "debug"} {
		fmt.Fprintf(cmdCtx.Writer, "complete -c nodex -l %s\n", flag)
	}
	for _, name := range commandNames() {
		fmt.Fprintf(cmdCtx.Writer, "complete -c nodex -n '__fish_use_subcommand' -a %s -d '%s'\n", name, commands[name].short)
		for _, subName := range subcommandNames(commands[name]) {
			fmt.Fprintf(cmdCtx.Writer, "complete -c nodex -n '__fish_seen_subcommand_from %s' -a %s -d '%s'\n", name, subName, commands[name].sub[subName].short)
		}
	}
	fmt.Fprintln(cmdCtx.Writer, "complete -c nodex -n '__fish_use_subcommand' -a help -d 'Show help for a command'")
}

func commandNames() []string {
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func subcommandNames(cmd *command) []string {
	names := make([]string, 0, len(cmd.sub))
	for name := range cmd.sub {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
