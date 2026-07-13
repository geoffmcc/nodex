package cli

import (
	"context"
	"fmt"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/version"
)

func runVersion(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex version"), app.ExitUsage)
	}
	info := version.Current()
	fmt.Fprintf(cmdCtx.Writer, "Nodex %s\n", info.Version)
	fmt.Fprintf(cmdCtx.Writer, "Go: %s\n", info.GoVersion)
	fmt.Fprintf(cmdCtx.Writer, "Commit: %s\n", info.Commit)
	fmt.Fprintf(cmdCtx.Writer, "Built: %s\n", info.BuildDate)
	if info.Dirty {
		fmt.Fprintln(cmdCtx.Writer, "Dirty: true")
	}
	return nil
}
