package cli

import (
	"context"
	"fmt"

	"github.com/geoffmcc/nodex/internal/version"
)

func runVersion(_ context.Context, cmdCtx *Context, _ []string) error {
	fmt.Fprintf(cmdCtx.Writer, "Nodex %s\n", version.Version)
	fmt.Fprintf(cmdCtx.Writer, "Go: %s\n", version.GoVersion)
	fmt.Fprintf(cmdCtx.Writer, "Commit: %s\n", version.Commit)
	fmt.Fprintf(cmdCtx.Writer, "Built: %s\n", version.BuildDate)
	return nil
}
