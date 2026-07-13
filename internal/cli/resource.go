package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func runNodeList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex node list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	nodes, err := prov.Nodes(ctx)
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	return writeNodes(cmdCtx, nodes)
}

func writeNodes(cmdCtx *Context, nodes []domain.Node) error {
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, nodes)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, nodes)
	default:
		headers := []string{"NAME", "STATUS", "IP", "ROLE", "UPTIME"}
		rows := make([][]string, 0, len(nodes))
		for _, n := range nodes {
			uptime := ""
			if n.Uptime != nil {
				uptime = n.Uptime.String()
			}
			rows = append(rows, []string{
				n.Name,
				n.Status,
				n.IP,
				n.Role,
				uptime,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runVMList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	vms, err := prov.VMs(ctx)
	if err != nil {
		return fmt.Errorf("list VMs: %w", err)
	}
	return writeVMs(cmdCtx, vms)
}

func writeVMs(cmdCtx *Context, vms []domain.VM) error {
	if vms == nil {
		vms = []domain.VM{}
	}
	sort.Slice(vms, func(i, j int) bool { return vms[i].Name < vms[j].Name })

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, vms)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, vms)
	default:
		headers := []string{"ID", "NAME", "STATUS", "NODE", "CPU", "MEMORY", "DISK"}
		rows := make([][]string, 0, len(vms))
		for _, v := range vms {
			rows = append(rows, []string{
				v.ID,
				v.Name,
				v.Status,
				v.Node,
				fmt.Sprintf("%d", v.CPU),
				formatBytes(v.Memory),
				formatBytes(v.Disk),
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runContainerList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex container list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	containers, err := prov.Containers(ctx)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	return writeContainers(cmdCtx, containers)
}

func writeContainers(cmdCtx *Context, containers []domain.Container) error {
	if containers == nil {
		containers = []domain.Container{}
	}
	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, containers)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, containers)
	default:
		headers := []string{"ID", "NAME", "STATUS", "NODE", "OS", "MEMORY", "DISK"}
		rows := make([][]string, 0, len(containers))
		for _, c := range containers {
			rows = append(rows, []string{
				c.ID,
				c.Name,
				c.Status,
				c.Node,
				c.OS,
				formatBytes(c.Memory),
				formatBytes(c.Disk),
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runStorageList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex storage list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	storages, err := prov.Storage(ctx)
	if err != nil {
		return fmt.Errorf("list storage: %w", err)
	}
	return writeStorages(cmdCtx, storages)
}

func writeStorages(cmdCtx *Context, storages []domain.Storage) error {
	if storages == nil {
		storages = []domain.Storage{}
	}
	sort.Slice(storages, func(i, j int) bool { return storages[i].Name < storages[j].Name })

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, storages)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, storages)
	default:
		headers := []string{"NAME", "TYPE", "STATUS", "NODE", "TOTAL", "USED", "AVAIL"}
		rows := make([][]string, 0, len(storages))
		for _, s := range storages {
			rows = append(rows, []string{
				s.Name,
				s.Type,
				s.Status,
				s.Node,
				formatBytes(s.Total),
				formatBytes(s.Used),
				formatBytes(s.Avail),
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}
