package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"golang.org/x/term"
)

func runVMConsole(ctx context.Context, cmdCtx *Context, args []string) error {
	return runConsole(ctx, cmdCtx, args, true)
}

func runContainerConsole(ctx context.Context, cmdCtx *Context, args []string) error {
	return runConsole(ctx, cmdCtx, args, false)
}

func runConsole(ctx context.Context, cmdCtx *Context, args []string, vm bool) error {
	resource := "vm"
	if !vm {
		resource = "container"
	}
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex %s console <node>/<vmid>", resource), app.ExitUsage)
	}
	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	in := cmdCtx.InteractiveIn
	if in == nil {
		in = cmdCtx.Stdin
	}
	out := cmdCtx.InteractiveOut
	if out == nil {
		out = cmdCtx.Writer
	}
	if !isTTY(in) || !isTTY(out) {
		return app.NewExitError(fmt.Errorf("%s console requires an interactive TTY for stdin and stdout", resource), app.ExitUsage)
	}
	terminal := in.(*os.File)
	state, err := term.MakeRaw(int(terminal.Fd()))
	if err != nil {
		return fmt.Errorf("set terminal raw mode: %w", err)
	}
	defer func() { _ = term.Restore(int(terminal.Fd()), state) }()

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()
	cp, ok := prov.(domain.ConsoleProvider)
	if !ok {
		return app.NewExitError(fmt.Errorf("%w: console commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()), app.ExitUnsupportedCap)
	}
	if vm {
		err = cp.VMConsole(ctx, node, vmid, in, out)
	} else {
		err = cp.ContainerConsole(ctx, node, vmid, in, out)
	}
	if err != nil {
		return fmt.Errorf("%s console %s/%d: %w", resource, node, vmid, err)
	}
	return nil
}

func isTTY(v any) bool {
	file, ok := v.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}
