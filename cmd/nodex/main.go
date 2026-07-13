package main

import (
	"context"
	"fmt"
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
		msg := output.SanitizeTerminal(redact.String(err.Error()))
		if wantsJSON(os.Args[1:]) {
			_ = output.WriteErrorJSON(os.Stderr, msg, "", app.ExitCodeFromError(err))
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
		}
		if code := signalExit.Load(); code != 0 {
			os.Exit(int(code))
		}
		os.Exit(app.ExitCodeFromError(err))
	}
	if code := signalExit.Load(); code != 0 {
		os.Exit(int(code))
	}
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
