package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
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

func runNodeStatus(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex node status <name>"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	// Get detailed node status via typed client method
	status, err := getNodeStatus(ctx, prov, args[0])
	if err != nil {
		return err
	}
	return writeNodeStatus(cmdCtx, status)
}

func getNodeStatus(ctx context.Context, prov domain.Provider, nodeName string) (*client.NodeStatusData, error) {
	// For now, use the existing Nodes method and filter
	// In the future, we can add a typed method to the provider interface
	nodes, err := prov.Nodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	for _, node := range nodes {
		if node.Name == nodeName || node.ID == nodeName {
			// Return a basic NodeStatusData from the existing node info
			return &client.NodeStatusData{
				ID:         node.ID,
				Node:       node.Name,
				Status:     node.Status,
				Type:       node.Role,
				Uptime:     0,
				PVEVersion: node.Version,
			}, nil
		}
	}
	return nil, app.NewExitError(fmt.Errorf("node %q not found", nodeName), app.ExitProvider)
}

func writeNodeStatus(cmdCtx *Context, status *client.NodeStatusData) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, status)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, status)
	default:
		uptime := ""
		if status.Uptime > 0 {
			uptime = formatDuration(status.Uptime)
		}
		loadAvg := ""
		if len(status.LoadAvg) > 0 {
			parts := make([]string, len(status.LoadAvg))
			for i, v := range status.LoadAvg {
				parts[i] = fmt.Sprintf("%.2f", v)
			}
			loadAvg = strings.Join(parts, " ")
		}
		rows := [][]string{
			{"NODE", status.Node},
			{"STATUS", status.Status},
			{"CPU", fmt.Sprintf("%.2f%%", status.CPU*100)},
			{"MAX CPU", fmt.Sprintf("%d", status.MaxCPU)},
			{"MEMORY", formatBytes(status.Mem)},
			{"MAX MEMORY", formatBytes(status.MaxMem)},
			{"DISK", formatBytes(status.Disk)},
			{"MAX DISK", formatBytes(status.MaxDisk)},
			{"UPTIME", uptime},
			{"LEVEL", status.Level},
			{"KVERSION", status.KVersion},
			{"PVEVERSION", status.PVEVersion},
			{"LOAD AVG", loadAvg},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

func formatDuration(seconds int) string {
	d := time.Duration(seconds) * time.Second
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, secs)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, secs)
	}
	return fmt.Sprintf("%ds", secs)
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

func runClusterStatus(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex cluster status"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cluster, err := prov.Cluster(ctx)
	if err != nil {
		return fmt.Errorf("get cluster status: %w", err)
	}
	return writeClusterStatus(cmdCtx, cluster)
}

func writeClusterStatus(cmdCtx *Context, cluster *domain.Cluster) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, cluster)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, cluster)
	default:
		rows := [][]string{
			{"NAME", cluster.Name},
			{"VERSION", cluster.Version},
			{"NODES", fmt.Sprintf("%d", cluster.Nodes)},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

func runVMConfig(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm config <node/vmid>"), app.ExitUsage)
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex vm config <node/vmid>"), app.ExitUsage)
	}
	node := parts[0]
	vmid, err := strconv.Atoi(parts[1])
	if err != nil || vmid <= 0 {
		return app.NewExitError(fmt.Errorf("invalid VMID: %s", parts[1]), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	config, err := prov.VMConfig(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("get vm config: %w", err)
	}
	return writeConfig(cmdCtx, config)
}

func runContainerConfig(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex container config <node/vmid>"), app.ExitUsage)
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex container config <node/vmid>"), app.ExitUsage)
	}
	node := parts[0]
	vmid, err := strconv.Atoi(parts[1])
	if err != nil || vmid <= 0 {
		return app.NewExitError(fmt.Errorf("invalid VMID: %s", parts[1]), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	config, err := prov.ContainerConfig(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("get container config: %w", err)
	}
	return writeConfig(cmdCtx, config)
}

func writeConfig(cmdCtx *Context, config map[string]interface{}) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, config)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, config)
	default:
		keys := make([]string, 0, len(config))
		for k := range config {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		rows := make([][]string, 0, len(keys))
		for _, k := range keys {
			rows = append(rows, []string{strings.ToUpper(k), fmt.Sprintf("%v", config[k])})
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}
