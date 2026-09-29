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
		format := wantsFormat(os.Args[1:])
		code := emitError(err, format, os.Stderr)
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
// In a structured output mode an error that has already been emitted inside an
// operation result envelope (app.IsEmitted) is suppressed so the stream stays
// valid; any other error is rendered as a structured error document in the
// requested format. In text mode the error is always printed as a single
// "Error:" line.
func emitError(err error, format output.Format, w io.Writer) int {
	msg := output.SanitizeTerminal(redact.String(err.Error()))
	code := app.ExitCodeFromError(err)
	switch format {
	case output.FormatJSON:
		if !app.IsEmitted(err) {
			_ = output.WriteErrorJSON(w, msg, "", code)
		}
	case output.FormatYAML:
		if !app.IsEmitted(err) {
			_ = output.WriteErrorYAML(w, msg, "", code)
		}
	default:
		fmt.Fprintf(w, "Error: %s\n", msg)
	}
	return code
}

func run(ctx context.Context) error {
	return cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

// wantsFormat determines the requested output format from the args, defaulting
// to text so that an unrecognised value keeps the plain "Error:" line rather
// than being forced into a structured envelope.
func wantsFormat(args []string) output.Format {
	for i, a := range args {
		var v string
		switch {
		case a == "--output" && i+1 < len(args):
			v = args[i+1]
		case strings.HasPrefix(a, "--output="):
			v = strings.TrimPrefix(a, "--output=")
		default:
			continue
		}
		switch strings.ToLower(v) {
		case "json":
			return output.FormatJSON
		case "yaml", "yml":
			return output.FormatYAML
		}
	}
	return output.FormatTable
}
