package cli

import (
	"context"
	"fmt"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/version"
)

func runVersionCompare(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex version compare <v1> <v2>"), app.ExitUsage)
	}
	result, err := version.Compare(args[0], args[1])
	if err != nil {
		return app.NewExitError(fmt.Errorf("comparison failed: %w", err), app.ExitUsage)
	}
	switch {
	case result < 0:
		fmt.Fprintf(cmdCtx.Writer, "%s < %s\n", args[0], args[1])
	case result > 0:
		fmt.Fprintf(cmdCtx.Writer, "%s > %s\n", args[0], args[1])
	default:
		fmt.Fprintf(cmdCtx.Writer, "%s == %s\n", args[0], args[1])
	}
	return nil
}

func runVersionParse(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex version parse <version>"), app.ExitUsage)
	}
	sv, err := version.ParseSemVer(args[0])
	if err != nil {
		return app.NewExitError(fmt.Errorf("parse failed: %w", err), app.ExitUsage)
	}
	fmt.Fprintf(cmdCtx.Writer, "Major:      %d\n", sv.Major)
	fmt.Fprintf(cmdCtx.Writer, "Minor:      %d\n", sv.Minor)
	fmt.Fprintf(cmdCtx.Writer, "Patch:      %d\n", sv.Patch)
	if sv.Prerelease != "" {
		fmt.Fprintf(cmdCtx.Writer, "Prerelease: %s\n", sv.Prerelease)
	}
	if sv.BuildMeta != "" {
		fmt.Fprintf(cmdCtx.Writer, "Build meta: %s\n", sv.BuildMeta)
	}
	return nil
}
