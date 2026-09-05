package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
)

var completionFlags = []string{"--profile", "--output", "--timeout", "--limit", "--yes", "--force", "--wait", "--expert", "--all", "--no-color", "--non-interactive", "--quiet", "--verbose", "--debug", "--password-stdin", "--confirm-target"}

func runCompletion(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex completion <bash|zsh|fish|powershell>"), app.ExitUsage)
	}
	switch args[0] {
	case "bash":
		writeBashCompletion(cmdCtx)
	case "zsh":
		writeZshCompletion(cmdCtx)
	case "fish":
		writeFishCompletion(cmdCtx)
	case "powershell":
		writePowerShellCompletion(cmdCtx)
	default:
		return app.NewExitError(fmt.Errorf("usage: nodex completion <bash|zsh|fish|powershell>"), app.ExitUsage)
	}
	return nil
}

type completionNode struct {
	Path     string
	Children []string
}

func completionNodes() []completionNode {
	childrenByPath := make(map[string]map[string]bool)
	addPath := func(path string) {
		parts := strings.Fields(path)
		for i := 1; i < len(parts); i++ {
			parent := strings.Join(parts[:i], " ")
			if childrenByPath[parent] == nil {
				childrenByPath[parent] = make(map[string]bool)
			}
			childrenByPath[parent][parts[i]] = true
		}
	}
	for _, op := range Operations() {
		addPath(op.Path)
	}
	var out []completionNode
	var walk func(map[string]*command, string)
	walk = func(nodes map[string]*command, prefix string) {
		names := make([]string, 0, len(nodes))
		for name := range nodes {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			c := nodes[name]
			path := strings.TrimSpace(prefix + " " + name)
			children := subcommandNames(c)
			for child := range childrenByPath[path] {
				if !containsString(children, child) {
					children = append(children, child)
				}
			}
			sort.Strings(children)
			if len(children) > 0 {
				out = append(out, completionNode{Path: path, Children: children})
				walk(c.sub, path)
			}
		}
	}
	walk(commands, "")
	seen := make(map[string]bool)
	for _, node := range out {
		seen[node.Path] = true
	}
	for path, childMap := range childrenByPath {
		if seen[path] {
			continue
		}
		children := make([]string, 0, len(childMap))
		for child := range childMap {
			children = append(children, child)
		}
		sort.Strings(children)
		out = append(out, completionNode{Path: path, Children: children})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func shellWords(values []string) string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = shellQuote(value)
	}
	return strings.Join(out, " ")
}
func shellQuote(value string) string                    { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func completionPathCases(shell string) []completionNode { return completionNodes() }

func writeBashCompletion(cmdCtx *Context) {
	root := append(commandNames(), "help")
	fmt.Fprintln(cmdCtx.Writer, "# bash completion for nodex")
	fmt.Fprintln(cmdCtx.Writer, `_nodex_completion() {
  local cur path token i
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  if [[ "$cur" == --* ]]; then
    COMPREPLY=( $(compgen -W "`+strings.Join(completionFlags, " ")+`" -- "$cur") )
    return 0
  fi
  path=""
  for ((i=1; i<COMP_CWORD; i++)); do
    token="${COMP_WORDS[i]}"
    [[ "$token" == --* ]] && continue
    path="${path:+$path }$token"
  done
  case "$path" in`)
	for _, node := range completionPathCases("bash") {
		fmt.Fprintf(cmdCtx.Writer, "    %s) COMPREPLY=( $(compgen -W \"%s\" -- \"$cur\") ); return 0 ;;\n", strings.ReplaceAll(node.Path, " ", "\\ "), strings.Join(node.Children, " "))
	}
	fmt.Fprintf(cmdCtx.Writer, `  esac
  if [[ $COMP_CWORD -le 1 ]]; then COMPREPLY=( $(compgen -W "%s" -- "$cur") ); fi
}
complete -F _nodex_completion nodex
`, strings.Join(root, " "))
}

func writeZshCompletion(cmdCtx *Context) {
	fmt.Fprintln(cmdCtx.Writer, `#compdef nodex
_nodex() {
  local path=""
  local -a values
  if [[ "$words[CURRENT]" == --* ]]; then
    _describe 'flag' '(--profile --output --timeout --limit --yes --force --wait --expert --all --no-color --non-interactive --quiet --verbose --debug --password-stdin --confirm-target)'
    return
  fi
  for ((i=2; i<CURRENT; i++)); do [[ "${words[i]}" == --* ]] && continue; path="${path:+$path }${words[i]}"; done
  case "$path" in`)
	for _, node := range completionPathCases("zsh") {
		fmt.Fprintf(cmdCtx.Writer, "    %s) values=(%s); _describe 'subcommand' values; return ;;\n", strings.ReplaceAll(node.Path, " ", "\\ "), shellWords(node.Children))
	}
	fmt.Fprintln(cmdCtx.Writer, `  esac
  if (( CURRENT == 2 )); then _describe 'command' '(help version init setup certification completion profile provider status node vm task container storage cluster event log doctor monitor backup firewall ha sdn pools network access ceph maintenance environment pbs replication)'; fi
}
_nodex "$@"`)
}

func writeFishCompletion(cmdCtx *Context) {
	fmt.Fprintln(cmdCtx.Writer, "# fish completion for nodex")
	fmt.Fprintln(cmdCtx.Writer, "complete -c nodex -f")
	for _, flag := range completionFlags {
		fmt.Fprintf(cmdCtx.Writer, "complete -c nodex -l %s\n", strings.TrimPrefix(flag, "--"))
	}
	for _, name := range append(commandNames(), "help") {
		fmt.Fprintf(cmdCtx.Writer, "complete -c nodex -n '__fish_use_subcommand' -a %s\n", name)
	}
	for _, node := range completionPathCases("fish") {
		fmt.Fprintf(cmdCtx.Writer, "complete -c nodex -n '__fish_seen_subcommand_from %s' -a '%s'\n", strings.ReplaceAll(node.Path, " ", " "), strings.Join(node.Children, " "))
	}
}

func writePowerShellCompletion(cmdCtx *Context) {
	fmt.Fprintln(cmdCtx.Writer, `# PowerShell completion for nodex
Register-ArgumentCompleter -Native -CommandName nodex -ScriptBlock {
  param($wordToComplete, $commandAst, $cursorPosition)
  $tokens = @($commandAst.CommandElements | Select-Object -Skip 1 | Where-Object { $_.Value -notlike '--*' })
  if ($tokens.Count -gt 0 -and $tokens[-1].Value -eq $wordToComplete) { $tokens = @($tokens | Select-Object -SkipLast 1) }
  $path = ($tokens | Select-Object -ExpandProperty Value) -join ' '
  $values = @()`)
	root := append(commandNames(), "help")
	fmt.Fprintf(cmdCtx.Writer, "  if ($path -eq '') { $values = @(%s) }\n", strings.Join(func() []string {
		out := make([]string, len(root))
		for i, c := range root {
			out[i] = "'" + strings.ReplaceAll(c, "'", "''") + "'"
		}
		return out
	}(), ", "))
	for _, node := range completionPathCases("powershell") {
		fmt.Fprintf(cmdCtx.Writer, "  if ($path -eq '%s') { $values = @(%s) }\n", strings.ReplaceAll(node.Path, "'", "''"), strings.Join(func() []string {
			out := make([]string, len(node.Children))
			for i, c := range node.Children {
				out[i] = "'" + strings.ReplaceAll(c, "'", "''") + "'"
			}
			return out
		}(), ", "))
	}
	fmt.Fprintln(cmdCtx.Writer, `  if ($wordToComplete -like '--*') { $values = @('--profile','--output','--timeout','--limit','--yes','--force','--wait','--expert','--all','--no-color','--non-interactive','--quiet','--verbose','--debug','--password-stdin','--confirm-target') }
  $values | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
}`)
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
