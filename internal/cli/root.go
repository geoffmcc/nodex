package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/logging"
	"github.com/geoffmcc/nodex/internal/output"
)

// Options holds parsed global flags.
type Options struct {
	Profile        string
	Output         output.Format
	Timeout        time.Duration
	NoColor        bool
	NonInteractive bool
	Quiet          bool
	Verbose        bool
	Debug          bool
}

// Context carries global state through command execution.
type Context struct {
	Opts   Options
	Logger *logging.Logger
	Writer io.Writer // stdout
	ErrW   io.Writer // stderr
	Config io.Reader // optional, for testing
}

// CommandFunc is the signature for command handlers.
type CommandFunc func(ctx context.Context, cmdCtx *Context, args []string) error

type command struct {
	name  string
	short string
	run   CommandFunc
	sub   map[string]*command
}

var commands = map[string]*command{}

func register(name, short string, run CommandFunc, subs ...*command) {
	c := &command{name: name, short: short, run: run}
	if len(subs) > 0 {
		c.sub = make(map[string]*command)
		for _, s := range subs {
			c.sub[s.name] = s
		}
	}
	commands[name] = c
}

func init() {
	register("version", "Print version information", runVersion)
	register("init", "Initialize nodex configuration", runInit)
	register("completion", "Generate shell completion scripts", runCompletion)
	register("profile", "Manage connection profiles", nil,
		&command{name: "add", short: "Add a new profile", run: runProfileAdd},
		&command{name: "list", short: "List all profiles", run: runProfileList},
		&command{name: "show", short: "Show profile details", run: runProfileShow},
		&command{name: "set-credentials", short: "Set profile credentials", run: runProfileSetCredentials},
		&command{name: "use", short: "Set the current profile", run: runProfileUse},
		&command{name: "current", short: "Show the current profile", run: runProfileCurrent},
		&command{name: "test", short: "Test profile connectivity", run: runProfileTest},
		&command{name: "remove", short: "Remove a profile", run: runProfileRemove},
	)
	register("provider", "Manage providers", nil,
		&command{name: "list", short: "List available providers", run: runProviderList},
		&command{name: "capabilities", short: "Show provider capabilities", run: runProviderCapabilities},
	)
	register("node", "Manage nodes", nil,
		&command{name: "list", short: "List all nodes", run: runNodeList},
		&command{name: "show", short: "Show node details", run: runNodeShow},
		&command{name: "status", short: "Show detailed node status", run: runNodeStatus},
	)
	register("vm", "Manage virtual machines", nil,
		&command{name: "list", short: "List all VMs", run: runVMList},
		&command{name: "show", short: "Show VM details", run: runVMShow},
	)
	register("container", "Manage containers", nil,
		&command{name: "list", short: "List all containers", run: runContainerList},
		&command{name: "show", short: "Show container details", run: runContainerShow},
	)
	register("storage", "Manage storage", nil,
		&command{name: "list", short: "List all storage pools", run: runStorageList},
		&command{name: "show", short: "Show storage details", run: runStorageShow},
	)
	register("doctor", "Check system health", runDoctor)
}

// Run parses global flags and dispatches to the appropriate command.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	opts, remaining, err := parseGlobal(args)
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	level := logging.LevelError
	if opts.Debug {
		level = logging.LevelDebug
	} else if opts.Verbose {
		level = logging.LevelInfo
	} else if opts.Quiet {
		level = logging.LevelSilent
	}

	cmdCtx := &Context{
		Opts:   opts,
		Logger: logging.NewStderr(level, opts.Debug),
		Writer: stdout,
		ErrW:   stderr,
	}

	if len(remaining) == 0 {
		printUsage(stdout)
		return nil
	}

	name := remaining[0]
	args = remaining[1:]

	if name == "help" {
		if len(args) > 1 {
			return app.NewExitError(fmt.Errorf("usage: nodex help [command]"), app.ExitUsage)
		}
		if len(args) == 1 {
			printCommandHelp(stdout, args[0])
		} else {
			printUsage(stdout)
		}
		return nil
	}

	cmd, ok := commands[name]
	if !ok {
		return app.NewExitError(
			fmt.Errorf("unknown command: %s", name),
			app.ExitUsage,
		)
	}

	// Handle subcommands.
	if cmd.run == nil && cmd.sub != nil {
		if len(args) == 0 {
			printSubcommandUsage(stdout, cmd)
			return nil
		}
		subName := args[0]
		args = args[1:]
		sub, ok := cmd.sub[subName]
		if !ok {
			return app.NewExitError(
				fmt.Errorf("unknown %s subcommand: %s", name, subName),
				app.ExitUsage,
			)
		}
		return sub.run(ctx, cmdCtx, args)
	}

	return cmd.run(ctx, cmdCtx, args)
}

func parseGlobal(args []string) (Options, []string, error) {
	var opts Options
	fs := flag.NewFlagSet("nodex", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&opts.Profile, "profile", "", "")
	outFmt := fs.String("output", "", "")
	fs.DurationVar(&opts.Timeout, "timeout", 30*time.Second, "")
	fs.BoolVar(&opts.NoColor, "no-color", false, "")
	fs.BoolVar(&opts.NonInteractive, "non-interactive", false, "")
	fs.BoolVar(&opts.Quiet, "quiet", false, "")
	fs.BoolVar(&opts.Verbose, "verbose", false, "")
	fs.BoolVar(&opts.Debug, "debug", false, "")

	if err := fs.Parse(args); err != nil {
		return opts, nil, err
	}
	if opts.Timeout <= 0 {
		return opts, nil, fmt.Errorf("timeout must be greater than zero")
	}

	// Resolve output format.
	if *outFmt != "" {
		switch strings.ToLower(*outFmt) {
		case "table":
			opts.Output = output.FormatTable
		case "json":
			opts.Output = output.FormatJSON
		case "yaml":
			opts.Output = output.FormatYAML
		default:
			return opts, nil, fmt.Errorf("invalid output format: %s (use table, json, or yaml)", *outFmt)
		}
	} else {
		opts.Output = output.DefaultFormat()
	}

	remaining := fs.Args()
	// Skip "--" separator if present.
	if len(remaining) > 0 && remaining[0] == "--" {
		remaining = remaining[1:]
	}

	return opts, remaining, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Nodex — open infrastructure management for self-hosters")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  nodex [global-flags] <command> [command-flags] [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cmd := commands[name]
		fmt.Fprintf(w, "  %-14s %s\n", name, cmd.short)
	}
	fmt.Fprintf(w, "  %-14s %s\n", "help", "Show help for a command")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Global Flags:")
	fmt.Fprintln(w, "  --profile <name>     Override current profile")
	fmt.Fprintln(w, "  --output <format>    Output format: table, json, yaml (default: table/tty, json/non-tty)")
	fmt.Fprintln(w, "  --timeout <duration> Request timeout (default: 30s)")
	fmt.Fprintln(w, "  --no-color           Disable color output")
	fmt.Fprintln(w, "  --non-interactive    Disable interactive prompts")
	fmt.Fprintln(w, "  --quiet              Suppress non-essential output")
	fmt.Fprintln(w, "  --verbose            Info-level stderr output")
	fmt.Fprintln(w, "  --debug              Debug-level stderr output (redacted)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'nodex help <command>' for details on a specific command.")
}

func printCommandHelp(w io.Writer, name string) {
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(w, "Unknown command: %s\n", name)
		return
	}
	fmt.Fprintf(w, "nodex %s — %s\n", name, cmd.short)
	fmt.Fprintln(w)
	if cmd.sub != nil {
		fmt.Fprintln(w, "Subcommands:")
		names := make([]string, 0, len(cmd.sub))
		for subName := range cmd.sub {
			names = append(names, subName)
		}
		sort.Strings(names)
		for _, subName := range names {
			sub := cmd.sub[subName]
			fmt.Fprintf(w, "  %-14s %s\n", subName, sub.short)
		}
	}
}

func printSubcommandUsage(w io.Writer, cmd *command) {
	fmt.Fprintf(w, "Usage: nodex %s <subcommand> [args]\n", cmd.name)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	names := make([]string, 0, len(cmd.sub))
	for name := range cmd.sub {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sub := cmd.sub[name]
		fmt.Fprintf(w, "  %-14s %s\n", name, sub.short)
	}
}

// Exported for testing.
func GetCommand(name string) (*command, bool) {
	c, ok := commands[name]
	return c, ok
}
