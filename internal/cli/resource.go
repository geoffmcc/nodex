package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

func runNodeShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex node show <name>"), app.ExitUsage)
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
	node, ok := findNode(nodes, args[0])
	if !ok {
		return app.NewExitError(fmt.Errorf("node %q not found", args[0]), app.ExitProvider)
	}
	return writeNode(cmdCtx, node)
}

func findNode(nodes []domain.Node, name string) (domain.Node, bool) {
	for _, node := range nodes {
		if node.Name == name || node.ID == name {
			return node, true
		}
	}
	return domain.Node{}, false
}

func writeNode(cmdCtx *Context, node domain.Node) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, node)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, node)
	default:
		uptime := ""
		if node.Uptime != nil {
			uptime = node.Uptime.String()
		}
		rows := [][]string{
			{"ID", node.ID},
			{"NAME", node.Name},
			{"STATUS", node.Status},
			{"ROLE", node.Role},
			{"IP", node.IP},
			{"PLATFORM", node.Platform},
			{"VERSION", node.Version},
			{"UPTIME", uptime},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
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

func runVMShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm show <id>"), app.ExitUsage)
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
	vm, ok := findVM(vms, args[0])
	if !ok {
		return app.NewExitError(fmt.Errorf("VM %q not found", args[0]), app.ExitProvider)
	}
	return writeVM(cmdCtx, vm)
}

func findVM(vms []domain.VM, id string) (domain.VM, bool) {
	for _, vm := range vms {
		if vm.ID == id {
			return vm, true
		}
	}
	return domain.VM{}, false
}

func writeVM(cmdCtx *Context, vm domain.VM) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, vm)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, vm)
	default:
		rows := [][]string{
			{"ID", vm.ID},
			{"NAME", vm.Name},
			{"STATUS", vm.Status},
			{"NODE", vm.Node},
			{"CPU", fmt.Sprintf("%d", vm.CPU)},
			{"MEMORY", formatBytes(vm.Memory)},
			{"DISK", formatBytes(vm.Disk)},
			{"IP", vm.IP},
			{"OS", vm.OS},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
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

func runContainerShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex container show <id>"), app.ExitUsage)
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
	container, ok := findContainer(containers, args[0])
	if !ok {
		return app.NewExitError(fmt.Errorf("container %q not found", args[0]), app.ExitProvider)
	}
	return writeContainer(cmdCtx, container)
}

func findContainer(containers []domain.Container, id string) (domain.Container, bool) {
	for _, container := range containers {
		if container.ID == id {
			return container, true
		}
	}
	return domain.Container{}, false
}

func writeContainer(cmdCtx *Context, container domain.Container) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, container)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, container)
	default:
		rows := [][]string{
			{"ID", container.ID},
			{"NAME", container.Name},
			{"STATUS", container.Status},
			{"NODE", container.Node},
			{"OS", container.OS},
			{"MEMORY", formatBytes(container.Memory)},
			{"DISK", formatBytes(container.Disk)},
			{"IP", container.IP},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
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

func runStorageShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex storage show <name>"), app.ExitUsage)
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
	storage, ok := findStorage(storages, args[0])
	if !ok {
		return app.NewExitError(fmt.Errorf("storage %q not found", args[0]), app.ExitProvider)
	}
	return writeStorage(cmdCtx, storage)
}

func findStorage(storages []domain.Storage, name string) (domain.Storage, bool) {
	for _, storage := range storages {
		if storage.Name == name || storage.ID == name {
			return storage, true
		}
	}
	return domain.Storage{}, false
}

func writeStorage(cmdCtx *Context, storage domain.Storage) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, storage)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, storage)
	default:
		rows := [][]string{
			{"ID", storage.ID},
			{"NAME", storage.Name},
			{"TYPE", storage.Type},
			{"STATUS", storage.Status},
			{"NODE", storage.Node},
			{"TOTAL", formatBytes(storage.Total)},
			{"USED", formatBytes(storage.Used)},
			{"AVAIL", formatBytes(storage.Avail)},
			{"CONTENT", strings.Join(storage.Content, ",")},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
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
