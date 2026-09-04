package cli

import (
	"context"
	"fmt"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func runPoolsList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex pools list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	poolsProv, err := requirePools(prov)
	if err != nil {
		return err
	}
	pools, err := poolsProv.Pools(ctx)
	if err != nil {
		return fmt.Errorf("list pools: %w", err)
	}
	return writePools(cmdCtx, applyLimit(pools, cmdCtx.Opts.Limit))
}

func writePools(cmdCtx *Context, pools []domain.Pool) error {
	if pools == nil {
		pools = []domain.Pool{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, pools)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, pools)
	default:
		headers := []string{"POOLID", "COMMENT", "MEMBERS"}
		rows := make([][]string, 0, len(pools))
		for _, p := range pools {
			members := ""
			if len(p.Members) > 0 {
				members = fmt.Sprintf("%d", len(p.Members))
			}
			rows = append(rows, []string{p.PoolID, p.Comment, members})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runClusterLog(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex cluster log"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	logProv, err := requireClusterLog(prov)
	if err != nil {
		return err
	}
	entries, err := logProv.ClusterLog(ctx)
	if err != nil {
		return fmt.Errorf("get cluster log: %w", err)
	}
	return writeClusterLog(cmdCtx, applyLimit(entries, cmdCtx.Opts.Limit))
}

func writeClusterLog(cmdCtx *Context, entries []domain.ClusterLogEntry) error {
	if entries == nil {
		entries = []domain.ClusterLogEntry{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, entries)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, entries)
	default:
		headers := []string{"TIME", "NODE", "MESSAGE"}
		rows := make([][]string, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, []string{
				fmt.Sprintf("%d", e.Time),
				e.Node,
				e.Message,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}
