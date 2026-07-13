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
	fmt.Fprintf(cmdCtx.Writer, "Nodex %s\n", version.Version)
	fmt.Fprintf(cmdCtx.Writer, "Go: %s\n", version.GoVersion)
	fmt.Fprintf(cmdCtx.Writer, "Commit: %s\n", version.Commit)
	fmt.Fprintf(cmdCtx.Writer, "Built: %s\n", version.BuildDate)
	return nil
}
