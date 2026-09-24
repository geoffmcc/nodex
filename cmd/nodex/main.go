package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/cli"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	var signalExit atomic.Int32
	go func() {
		sig := <-sigCh
		if sig == syscall.SIGTERM {
			signalExit.Store(app.ExitSigterm)
		} else {
			signalExit.Store(app.ExitInterrupted)
		}
		cancel()
	}()

	if err := run(ctx); err != nil {
		code := emitError(err, wantsJSON(os.Args[1:]), os.Stderr)
		if sc := signalExit.Load(); sc != 0 {
			os.Exit(int(sc))
		}
		os.Exit(code)
	}
	if code := signalExit.Load(); code != 0 {
		os.Exit(int(code))
	}
}

// emitError writes err to w and returns the process exit code implied by err.
// In JSON mode an error that has already been emitted inside an operation
// result envelope (app.IsEmitted) is suppressed so the stream stays valid
// JSON; any other error is rendered as a JSON error document. In text mode the
// error is always printed as a single "Error:" line.
func emitError(err error, jsonMode bool, w io.Writer) int {
	msg := output.SanitizeTerminal(redact.String(err.Error()))
	code := app.ExitCodeFromError(err)
	if jsonMode {
		if !app.IsEmitted(err) {
			_ = output.WriteErrorJSON(w, msg, "", code)
		}
	} else {
		fmt.Fprintf(w, "Error: %s\n", msg)
	}
	return code
}

func run(ctx context.Context) error {
	return cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

// wantsJSON checks if --output json appears in the args.
func wantsJSON(args []string) bool {
	for i, a := range args {
		if a == "--output" && i+1 < len(args) && strings.EqualFold(args[i+1], "json") {
			return true
		}
		if strings.HasPrefix(a, "--output=") && strings.EqualFold(strings.TrimPrefix(a, "--output="), "json") {
			return true
		}
	}
	return false
}
