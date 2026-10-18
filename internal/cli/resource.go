package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func runNodeList(ctx context.Context, cmdCtx *Context, args []string) error {
	if cmdCtx.Opts.All {
		return runNodesAll(ctx, cmdCtx, args)
	}
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex node list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ni, err := requireNodeInspector(prov)
	if err != nil {
		return err
	}
	nodes, err := ni.Nodes(ctx)
	if err != nil {
		return err
	}

	sortByField(nodes, func(n domain.Node) string { return n.Name + "\x00" + n.ID })
	return writeNodes(cmdCtx, applyLimit(nodes, cmdCtx.Opts.Limit))
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

	ni, err := requireNodeInspector(prov)
	if err != nil {
		return err
	}
	nodes, err := ni.Nodes(ctx)
	if err != nil {
		return err
	}
	node, ok := findNode(nodes, args[0])
	if !ok {
		return app.NotFoundError("node %q", args[0])
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

	detail, err := requireNodeDetail(prov)
	if err != nil {
		return err
	}
	status, err := detail.NodeStatus(ctx, args[0])
	if err != nil {
		return err
	}
	return writeNodeStatusMap(cmdCtx, status)
}

func writeNodeStatusMap(cmdCtx *Context, status map[string]interface{}) error {
	uptimeSec := int64(toFloat(status["uptime"]))
	status["uptime_seconds"] = uptimeSec
	status["uptime_human"] = formatUptime(time.Duration(uptimeSec) * time.Second)
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, status)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, status)
	default:
		rows := [][]string{
			{"NODE", fmt.Sprintf("%v", status["node"])},
			{"STATUS", fmt.Sprintf("%v", status["status"])},
			{"CPU", fmt.Sprintf("%.2f%%", toFloat(status["cpu"])*100)},
			{"MAX CPU", fmt.Sprintf("%v", status["maxcpu"])},
			{"MEMORY", formatBytes(toInt64(status["mem"]))},
			{"MAX MEMORY", formatBytes(toInt64(status["maxmem"]))},
			{"DISK", formatBytes(toInt64(status["disk"]))},
			{"MAX DISK", formatBytes(toInt64(status["maxdisk"]))},
			{"UPTIME", formatUptime(time.Duration(uptimeSec) * time.Second)},
			{"LEVEL", fmt.Sprintf("%v", status["level"])},
			{"KVERSION", fmt.Sprintf("%v", status["kversion"])},
			{"PVEVERSION", fmt.Sprintf("%v", status["pveversion"])},
			{"LOAD AVG", fmt.Sprintf("%v", status["loadavg"])},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	case int:
		return int64(val)
	default:
		return 0
	}
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
	node = decorateNode(node)
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
	nodes = decorateNodes(nodes)

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
	if cmdCtx.Opts.All {
		return runVMsAll(ctx, cmdCtx, args)
	}
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	vi, err := requireVMInspector(prov)
	if err != nil {
		return err
	}
	vms, err := vi.VMs(ctx)
	if err != nil {
		return fmt.Errorf("list VMs: %w", err)
	}
	sortGuestsByID(vms, func(v domain.VM) string { return v.ID })
	return writeVMs(cmdCtx, applyLimit(vms, cmdCtx.Opts.Limit))
}

func runVMShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm show <id>"), app.ExitUsage)
	}
	if _, _, err := parseNodeVMID(args[0]); err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	vi, err := requireVMInspector(prov)
	if err != nil {
		return err
	}
	vms, err := vi.VMs(ctx)
	if err != nil {
		return fmt.Errorf("list VMs: %w", err)
	}
	vm, ok := findVM(vms, args[0])
	if !ok {
		return app.NotFoundError("VM %q", args[0])
	}
	enrichVMRuntimeStatus(ctx, prov, cmdCtx, &vm)
	return writeVM(cmdCtx, vm)
}

// enrichVMRuntimeStatus refreshes one VM from the authoritative per-guest read
// when the provider exposes it.
//
// The aggregate listing only knows that the QEMU process is "running", which
// makes a VM suspended in memory look like it is executing. The per-guest read
// also carries the QMP execution state, so both process status and execution
// state are reported. A failed read leaves the aggregate values in place rather
// than inventing a runtime state, and the absence of QMPStatus then means "not
// observed", not "running".
func enrichVMRuntimeStatus(ctx context.Context, prov domain.Provider, cmdCtx *Context, vm *domain.VM) {
	rti, ok := prov.(domain.GuestRuntimeInspector)
	if !ok {
		return
	}
	node, vmid, err := parseNodeVMID(vm.ID)
	if err != nil || node == "" || vmid <= 0 {
		return
	}
	status, err := rti.GuestRuntimeStatus(ctx, node, "vm", vmid)
	if err != nil {
		// The aggregate view is still a truthful statement of what was read;
		// note that the authoritative refresh failed rather than dropping it.
		fmt.Fprintf(cmdCtx.ErrW, "warning: authoritative VM status unavailable, reporting aggregate state: %v\n", err)
		return
	}
	if status == nil {
		return
	}
	vm.Status = status.Status
	vm.QMPStatus = status.QMPStatus
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
	vm = decorateVM(vm)
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
		// The execution state is a distinct axis from the process status: a
		// guest suspended in memory keeps a running QEMU process with paused
		// CPUs. Only shown when it was actually observed.
		if vm.QMPStatus != "" {
			rows = append(rows, []string{"EXECUTION", vm.QMPStatus})
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

func writeVMs(cmdCtx *Context, vms []domain.VM) error {
	if vms == nil {
		vms = []domain.VM{}
	}
	sort.Slice(vms, func(i, j int) bool { return vms[i].Name < vms[j].Name })
	vms = decorateVMs(vms)

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, vms)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, vms)
	default:
		if len(vms) == 0 {
			_, err := fmt.Fprintln(cmdCtx.Writer, "No VMs found.")
			return err
		}
		headers := []string{"ID", "NAME", "STATUS", "NODE", "CPU-LIMIT", "MEMORY-LIMIT", "DISK-LIMIT"}
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
	if cmdCtx.Opts.All {
		return runContainersAll(ctx, cmdCtx, args)
	}
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex container list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ci, err := requireContainerInspector(prov)
	if err != nil {
		return err
	}
	containers, err := ci.Containers(ctx)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	sortGuestsByID(containers, func(c domain.Container) string { return c.ID })
	containers = applyLimit(containers, cmdCtx.Opts.Limit)
	enrichContainerConfig(ctx, ci, containers, false)
	return writeContainers(cmdCtx, containers)
}

func runContainerShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex container show <id>"), app.ExitUsage)
	}
	if _, _, err := parseNodeVMID(args[0]); err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ci, err := requireContainerInspector(prov)
	if err != nil {
		return err
	}
	containers, err := ci.Containers(ctx)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	container, ok := findContainer(containers, args[0])
	if !ok {
		return app.NotFoundError("container %q", args[0])
	}
	// The listing aggregates guest status and can lag a completed lifecycle
	// task, so a single-container read is corrected from the authoritative
	// per-guest endpoint when one is available.
	if insp, supported := prov.(domain.GuestStatusInspector); supported {
		if node, vmid, parsed := parseNodeVMID(args[0]); parsed == nil {
			if status, err := insp.GuestStatus(ctx, node, "container", vmid); err == nil && status != "" {
				container.Status = status
			}
		}
	}
	selected := []domain.Container{container}
	enrichContainerConfig(ctx, ci, selected, true)
	return writeContainer(cmdCtx, selected[0])
}

// Aggregate inventory can lag restored guests and omit config-backed fields.
// Enrich only returned rows from read-only configuration, with one total
// budget rather than one timeout per guest. No persistent cache or guest
// execution is used. Missing/denied reads leave aggregate values intact.
func enrichContainerConfig(ctx context.Context, inspector domain.ContainerInspector, containers []domain.Container, refresh bool) {
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	for i := range containers {
		if bounded.Err() != nil {
			return
		}
		container := &containers[i]
		if !refresh && container.OS != "" && container.Name != "" && container.CPU != 0 && container.Memory != 0 && container.Disk != 0 {
			continue
		}
		node, vmid, err := parseNodeVMID(container.ID)
		if err != nil {
			continue
		}
		cfg, err := inspector.ContainerConfig(bounded, node, vmid)
		if err != nil {
			continue
		}
		if hostname, ok := cfg["hostname"].(string); ok && hostname != "" && (refresh || container.Name == "") {
			container.Name = hostname
		}
		if cores, ok := cfg["cores"]; ok && (refresh || container.CPU == 0) {
			container.CPU = int(toInt64(cores))
		}
		if memory, ok := cfg["memory"]; ok && (refresh || container.Memory == 0) {
			container.Memory = toInt64(memory) * 1024 * 1024
		}
		if disk := parseContainerRootfsSize(cfg["rootfs"]); disk > 0 && (refresh || container.Disk == 0) {
			container.Disk = disk
		}
		if osType, ok := cfg["ostype"].(string); ok && osType != "" && (refresh || container.OS == "") {
			container.OS = osType
		}
	}
}

func parseContainerRootfsSize(value any) int64 {
	rootfs, ok := value.(string)
	if !ok {
		return 0
	}
	for _, property := range strings.Split(rootfs, ",") {
		key, size, found := strings.Cut(strings.TrimSpace(property), "=")
		if found && key == "size" {
			return parseStorageByteSize(size)
		}
	}
	return 0
}

func parseStorageByteSize(value string) int64 {
	value = strings.TrimSpace(value)
	upper := strings.ToUpper(value)
	units := []struct {
		suffix string
		factor int64
	}{
		{"KIB", 1 << 10}, {"KB", 1 << 10}, {"K", 1 << 10},
		{"MIB", 1 << 20}, {"MB", 1 << 20}, {"M", 1 << 20},
		{"GIB", 1 << 30}, {"GB", 1 << 30}, {"G", 1 << 30},
		{"TIB", 1 << 40}, {"TB", 1 << 40}, {"T", 1 << 40},
	}
	factor := int64(1)
	for _, unit := range units {
		if strings.HasSuffix(upper, unit.suffix) {
			factor = unit.factor
			value = strings.TrimSpace(value[:len(value)-len(unit.suffix)])
			break
		}
	}
	amount, err := strconv.ParseInt(value, 10, 64)
	const maxInt64 = int64(^uint64(0) >> 1)
	if err != nil || amount < 0 || amount > maxInt64/factor {
		return 0
	}
	return amount * factor
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
	container = decorateContainer(container)
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
			{"CPU", fmt.Sprintf("%d", container.CPU)},
			{"OS", containerOSLabel(container.OS)},
			{"MEMORY", formatBytes(container.Memory)},
			{"DISK", formatBytes(container.Disk)},
			{"IP", container.IP},
			{"TEMPLATE", yesNo(container.Template)},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

func writeContainers(cmdCtx *Context, containers []domain.Container) error {
	if containers == nil {
		containers = []domain.Container{}
	}
	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
	containers = decorateContainers(containers)

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, containers)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, containers)
	default:
		if len(containers) == 0 {
			_, err := fmt.Fprintln(cmdCtx.Writer, "No containers found.")
			return err
		}
		headers := []string{"ID", "NAME", "STATUS", "NODE", "CPU-LIMIT", "OS", "MEMORY-LIMIT", "DISK-LIMIT", "TEMPLATE"}
		rows := make([][]string, 0, len(containers))
		for _, c := range containers {
			rows = append(rows, []string{
				c.ID,
				c.Name,
				c.Status,
				c.Node,
				fmt.Sprintf("%d", c.CPU),
				containerOSLabel(c.OS),
				formatBytes(c.Memory),
				formatBytes(c.Disk),
				yesNo(c.Template),
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

	si, err := requireStorageInspector(prov)
	if err != nil {
		return err
	}
	storages, err := si.Storage(ctx)
	if err != nil {
		return fmt.Errorf("list storage: %w", err)
	}
	sortByField(storages, func(s domain.Storage) string { return s.Name + "\x00" + s.ID + "\x00" + s.Node })
	return writeStorages(cmdCtx, applyLimit(storages, cmdCtx.Opts.Limit))
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

	si, err := requireStorageInspector(prov)
	if err != nil {
		return err
	}
	storages, err := si.Storage(ctx)
	if err != nil {
		return fmt.Errorf("list storage: %w", err)
	}
	storage, err := resolveStorage(storages, args[0])
	if err != nil {
		return err
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

// resolveStorage resolves a storage target given as <name>, <node>/<name>, or
// the fully-qualified id form storage/<node>/<name>.
func resolveStorage(storages []domain.Storage, target string) (domain.Storage, error) {
	notFound := app.NotFoundError("storage %q", target)
	if strings.Count(target, "/") == 1 {
		node, name, _ := strings.Cut(target, "/")
		for _, storage := range storages {
			if storage.Node == node && storage.Name == name {
				return storage, nil
			}
		}
		return domain.Storage{}, notFound
	}
	storage, ok := findStorage(storages, target)
	if !ok {
		return domain.Storage{}, notFound
	}
	return storage, nil
}

func writeStorage(cmdCtx *Context, storage domain.Storage) error {
	storage = decorateStorage(storage)
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
	storages = decorateStorages(storages)

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, storages)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, storages)
	default:
		entries, capacity := buildStatusStorage(storages, "available", time.Now())
		groups := map[string]string{}
		for i, group := range capacity.Groups {
			groups[group.ID] = fmt.Sprintf("%s:%d", group.Scope, i+1)
		}
		headers := []string{"NAME", "TYPE", "STATUS", "NODE", "TOTAL", "USED", "AVAIL", "NAMESPACE", "CAPACITY-GROUP"}
		rows := make([][]string, 0, len(entries))
		for _, s := range entries {
			namespace, group := "-", "unresolved"
			if s.Identity != nil && s.Identity.Scope == "pbs_datastore" {
				namespace = s.Identity.Namespace
				if namespace == "" {
					namespace = "(root)"
				}
			}
			if label, ok := groups[s.CapacityGroup]; ok {
				group = label
			}
			rows = append(rows, []string{s.Name, s.Type, s.Status, s.Node, formatBytes(s.Total), formatBytes(s.Used), formatBytes(s.Avail), namespace, group})
		}
		if err := output.WriteTable(cmdCtx.Writer, headers, rows); err != nil {
			return err
		}
		if len(entries) > 0 {
			_, err := fmt.Fprintln(cmdCtx.Writer, "Capacity groups identify configured views, not independent physical pools.\nPer-entry/group capacities may overlap; do not sum.")
			return err
		}
		return nil
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

	cl, err := requireClusterInspector(prov)
	if err != nil {
		return err
	}
	cluster, err := cl.Cluster(ctx)
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
		if cluster.Standalone {
			rows = append(rows, []string{"STANDALONE", "true"})
		}
		if cluster.Quorate != nil {
			rows = append(rows, []string{"QUORATE", fmt.Sprintf("%t", *cluster.Quorate)})
		} else if cluster.Standalone {
			rows = append(rows, []string{"QUORATE", "unknown (standalone node)"})
		}
		for _, node := range cluster.NodeDetail {
			name := node.Name
			if name == "" {
				name = node.ID
			}
			value := node.Status
			if node.IP != "" {
				value += " (" + node.IP + ")"
			}
			rows = append(rows, []string{"NODE " + name, value})
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

	vi, err := requireVMInspector(prov)
	if err != nil {
		return err
	}
	config, err := vi.VMConfig(ctx, node, vmid)
	if err != nil {
		return err
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

	ci, err := requireContainerInspector(prov)
	if err != nil {
		return err
	}
	config, err := ci.ContainerConfig(ctx, node, vmid)
	if err != nil {
		return err
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

func runStorageContent(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex storage content <node> <storage>"), app.ExitUsage)
	}
	node := args[0]
	storage := args[1]
	if node == "" || storage == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex storage content <node> <storage>"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	si, err := requireStorageInspector(prov)
	if err != nil {
		return err
	}
	items, err := si.StorageContent(ctx, node, storage)
	if err != nil {
		return err
	}
	return writeStorageContent(cmdCtx, applyLimit(items, cmdCtx.Opts.Limit))
}

func writeStorageContent(cmdCtx *Context, items []domain.StorageContentItem) error {
	if items == nil {
		items = []domain.StorageContentItem{}
	}
	items = decorateStorageContent(items)
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, items)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, items)
	default:
		headers := []string{"VOLID", "CONTENT", "FORMAT", "SIZE", "VMID"}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			vmid := ""
			if item.VMID > 0 {
				vmid = fmt.Sprintf("%d", item.VMID)
			}
			rows = append(rows, []string{
				item.Volid,
				item.Content,
				item.Format,
				formatBytes(item.Size),
				vmid,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runTaskList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex task list <node>"), app.ExitUsage)
	}
	node := args[0]
	if node == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex task list <node>"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ti, err := requireTaskInspector(prov)
	if err != nil {
		return err
	}
	tasks, err := ti.Tasks(ctx, node)
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}
	return writeTaskList(cmdCtx, applyLimit(tasks, cmdCtx.Opts.Limit))
}

func writeTaskList(cmdCtx *Context, tasks []domain.Task) error {
	if tasks == nil {
		tasks = []domain.Task{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, tasks)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, tasks)
	default:
		headers := []string{"UPID", "TYPE", "STATE", "STATUS", "STARTED"}
		rows := make([][]string, 0, len(tasks))
		for _, t := range tasks {
			rows = append(rows, []string{
				t.UPID,
				t.Type,
				t.State,
				t.Status,
				fmt.Sprintf("%d", t.StartTime),
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runTaskShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex task show <node> <upid>"), app.ExitUsage)
	}
	node := args[0]
	upid := args[1]
	if node == "" || upid == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex task show <node> <upid>"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ti, err := requireTaskInspector(prov)
	if err != nil {
		return err
	}
	task, err := ti.Task(ctx, node, upid)
	if err != nil {
		return err
	}
	return writeTaskDetail(cmdCtx, task)
}

func writeTaskDetail(cmdCtx *Context, task *domain.Task) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, task)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, task)
	default:
		rows := [][]string{
			{"UPID", task.UPID},
			{"TYPE", task.Type},
			{"STATE", task.State},
			{"STATUS", task.Status},
			{"NODE", task.Node},
			{"STARTED", fmt.Sprintf("%d", task.StartTime)},
			{"ENDED", fmt.Sprintf("%d", task.EndTime)},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

func runTaskLog(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex task log <node> <upid>"), app.ExitUsage)
	}
	node := args[0]
	upid := args[1]
	if node == "" || upid == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex task log <node> <upid>"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	tl, err := requireTaskLog(prov)
	if err != nil {
		return err
	}
	lines, err := tl.TaskLog(ctx, node, upid, cmdCtx.Opts.Limit)
	if err != nil {
		return err
	}
	return writeTaskLog(cmdCtx, lines)
}

func writeTaskLog(cmdCtx *Context, lines []domain.TaskLogLine) error {
	if lines == nil {
		lines = []domain.TaskLogLine{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, lines)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, lines)
	default:
		for _, l := range lines {
			fmt.Fprintf(cmdCtx.Writer, "%s\n", l.Text)
		}
		return nil
	}
}

func runVMSnapshots(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm snapshots <node/vmid>"), app.ExitUsage)
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex vm snapshots <node/vmid>"), app.ExitUsage)
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

	sp, err := requireSnapshotInspector(prov)
	if err != nil {
		return err
	}
	snaps, err := sp.VMSnapshots(ctx, node, vmid)
	if err != nil {
		return err
	}
	return writeSnapshotList(cmdCtx, applyLimit(snaps, cmdCtx.Opts.Limit))
}

func runContainerSnapshots(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex container snapshots <node/vmid>"), app.ExitUsage)
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex container snapshots <node/vmid>"), app.ExitUsage)
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

	sp, err := requireSnapshotInspector(prov)
	if err != nil {
		return err
	}
	snaps, err := sp.ContainerSnapshots(ctx, node, vmid)
	if err != nil {
		return err
	}
	return writeSnapshotList(cmdCtx, applyLimit(snaps, cmdCtx.Opts.Limit))
}

func writeSnapshotList(cmdCtx *Context, snaps []domain.Snapshot) error {
	if snaps == nil {
		snaps = []domain.Snapshot{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, snaps)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, snaps)
	default:
		headers := []string{"NAME", "PARENT", "CREATED", "DESCRIPTION"}
		rows := make([][]string, 0, len(snaps))
		for _, s := range snaps {
			rows = append(rows, []string{
				s.Name,
				s.Parent,
				formatSnapshotTime(s.Ctime),
				s.Description,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// formatSnapshotTime renders a snapshot epoch as a readable UTC timestamp. The
// Proxmox VE "current" pseudo-snapshot carries no time, so an unset value is
// left blank rather than shown as the Unix epoch.
func formatSnapshotTime(epoch int) string {
	if epoch <= 0 {
		return ""
	}
	return time.Unix(int64(epoch), 0).UTC().Format(time.RFC3339)
}

type statusOverview struct {
	Cluster     string `json:"cluster" yaml:"cluster"`
	Version     string `json:"version" yaml:"version"`
	Nodes       int    `json:"nodes" yaml:"nodes"`
	Quorum      int    `json:"quorum" yaml:"quorum"`
	QuorumKnown bool   `json:"quorum_known" yaml:"quorum_known"`
	// Standalone reports that the provider confirmed no cluster exists, so a
	// zero quorum is expected rather than a fault (nit #36).
	Standalone         bool                   `json:"standalone" yaml:"standalone"`
	NodesDetail        []statusNode           `json:"nodes_detail" yaml:"nodes_detail"`
	VMs                int                    `json:"vms" yaml:"vms"`
	VMsRunning         int                    `json:"vms_running" yaml:"vms_running"`
	VMsStopped         int                    `json:"vms_stopped" yaml:"vms_stopped"`
	VMDetails          []statusGuest          `json:"vm_details,omitempty" yaml:"vm_details,omitempty"`
	Containers         int                    `json:"containers" yaml:"containers"`
	CTsRunning         int                    `json:"cts_running" yaml:"cts_running"`
	CTsStopped         int                    `json:"cts_stopped" yaml:"cts_stopped"`
	ContainerDetails   []statusGuest          `json:"container_details,omitempty" yaml:"container_details,omitempty"`
	Storage            []statusStorage        `json:"storage" yaml:"storage"`
	StorageCapacity    *statusStorageCapacity `json:"storage_capacity,omitempty" yaml:"storage_capacity,omitempty"`
	DiskInventory      *statusDiskInventory   `json:"disk_inventory,omitempty" yaml:"disk_inventory,omitempty"`
	HA                 *statusHA              `json:"ha,omitempty" yaml:"ha,omitempty"`
	VMsUnknown         int                    `json:"vms_unknown,omitempty" yaml:"vms_unknown,omitempty"`
	CTsUnknown         int                    `json:"cts_unknown,omitempty" yaml:"cts_unknown,omitempty"`
	Sections           map[string]string      `json:"sections,omitempty" yaml:"sections,omitempty"`
	Warnings           []string               `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	RecentTasks        []domain.Task          `json:"recent_tasks,omitempty" yaml:"recent_tasks,omitempty"`
	TaskWarningDetails map[string]string      `json:"task_warning_details,omitempty" yaml:"task_warning_details,omitempty"`
}

type statusHA struct {
	Status      string `json:"status" yaml:"status"`
	Quorum      int    `json:"quorum" yaml:"quorum"`
	QuorumKnown bool   `json:"quorum_known" yaml:"quorum_known"`
	// Standalone mirrors statusOverview.Standalone so a single HA block is
	// unambiguous on its own.
	Standalone bool `json:"standalone" yaml:"standalone"`
}

type statusNode struct {
	Name          string `json:"name" yaml:"name"`
	Status        string `json:"status" yaml:"status"`
	Uptime        string `json:"uptime,omitempty" yaml:"uptime,omitempty"`
	UptimeSeconds int64  `json:"uptime_seconds,omitempty" yaml:"uptime_seconds,omitempty"`
	UptimeHuman   string `json:"uptime_human,omitempty" yaml:"uptime_human,omitempty"`
}

type statusGuest struct {
	ID     string `json:"id" yaml:"id"`
	Name   string `json:"name" yaml:"name"`
	Node   string `json:"node" yaml:"node"`
	Status string `json:"status" yaml:"status"`
}

type statusStorage struct {
	Name          string                          `json:"name" yaml:"name"`
	Type          string                          `json:"type" yaml:"type"`
	Total         int64                           `json:"total" yaml:"total"`
	Used          int64                           `json:"used" yaml:"used"`
	Avail         int64                           `json:"avail" yaml:"avail"`
	TotalHuman    string                          `json:"total_human,omitempty" yaml:"total_human,omitempty"`
	UsedHuman     string                          `json:"used_human,omitempty" yaml:"used_human,omitempty"`
	AvailHuman    string                          `json:"avail_human,omitempty" yaml:"avail_human,omitempty"`
	ID            string                          `json:"id" yaml:"id"`
	Node          string                          `json:"node" yaml:"node"`
	Status        string                          `json:"status" yaml:"status"`
	Identity      *domain.StorageCapacityIdentity `json:"identity" yaml:"identity"`
	Backing       *domain.StorageBacking          `json:"backing,omitempty" yaml:"backing,omitempty"`
	CapacityGroup string                          `json:"capacity_group,omitempty" yaml:"capacity_group,omitempty"`
}

type statusDiskInventory struct {
	State         string           `json:"state" yaml:"state"`
	Nodes         int              `json:"nodes" yaml:"nodes"`
	NodesReported int              `json:"nodes_reported" yaml:"nodes_reported"`
	DeviceCount   int              `json:"device_count" yaml:"device_count"`
	TotalBytes    int64            `json:"total_bytes" yaml:"total_bytes"`
	TotalHuman    string           `json:"total_human,omitempty" yaml:"total_human,omitempty"`
	Devices       []statusNodeDisk `json:"devices" yaml:"devices"`
}

type statusNodeDisk struct {
	Node   string `json:"node" yaml:"node"`
	Name   string `json:"name" yaml:"name"`
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`
	Size   int64  `json:"size" yaml:"size"`
	Type   string `json:"type,omitempty" yaml:"type,omitempty"`
	Model  string `json:"model,omitempty" yaml:"model,omitempty"`
	Health string `json:"health,omitempty" yaml:"health,omitempty"`
}

func runStatus(ctx context.Context, cmdCtx *Context, args []string) error {
	watch, interval, err := parseStatusFlags(args)
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	if watch {
		return runStatusWatch(ctx, cmdCtx, interval)
	}
	if cmdCtx.Opts.All {
		return runStatusAll(ctx, cmdCtx, nil)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()
	overview := collectStatusSnapshot(ctx, prov, false)
	return writeStatus(cmdCtx, overview)
}

// collectStatusSnapshot is shared by one-shot output and watch. It only calls
// inspection capabilities, never mutation or repair methods. Query failures are
// section states rather than falsely healthy zero counts.
func collectStatusSnapshot(ctx context.Context, prov domain.Provider, includeTasks bool) *statusOverview {
	return collectStatusSnapshotWithTaskWarningCache(ctx, prov, includeTasks, nil)
}

func collectStatusSnapshotWithTaskWarningCache(ctx context.Context, prov domain.Provider, includeTasks bool, warningCache *statusTaskWarningCache) *statusOverview {
	overview := statusOverview{}
	overview.Sections = map[string]string{}
	for _, section := range []string{"nodes", "vms", "containers", "storage", "cluster", "quorum", "ha"} {
		overview.Sections[section] = "unsupported"
	}

	if ni, ok := prov.(domain.NodeInspector); ok {
		overview.Sections["nodes"] = "unavailable"
		if nodes, err := ni.Nodes(ctx); err == nil {
			overview.Sections["nodes"] = "available"
			overview.Nodes = len(nodes)
			for _, n := range nodes {
				sn := statusNode{Name: n.Name, Status: n.Status}
				if n.Uptime != nil {
					sn.Uptime = n.Uptime.String()
					sn.UptimeSeconds = int64(*n.Uptime / time.Second)
					sn.UptimeHuman = formatUptime(*n.Uptime)
				}
				overview.NodesDetail = append(overview.NodesDetail, sn)
			}
		}
	}

	if vi, ok := prov.(domain.VMInspector); ok {
		overview.Sections["vms"] = "unavailable"
		if vms, err := vi.VMs(ctx); err == nil {
			overview.Sections["vms"] = "available"
			overview.VMs = len(vms)
			sort.Slice(vms, func(i, j int) bool { return vms[i].ID < vms[j].ID })
			for _, v := range vms {
				overview.VMDetails = append(overview.VMDetails, statusGuest{ID: v.ID, Name: v.Name, Node: v.Node, Status: v.Status})
			}
			for _, v := range vms {
				if v.Status == "running" {
					overview.VMsRunning++
				} else if v.Status == "stopped" {
					overview.VMsStopped++
				} else {
					overview.VMsUnknown++
				}
			}
		}
	}

	if ci, ok := prov.(domain.ContainerInspector); ok {
		overview.Sections["containers"] = "unavailable"
		if cts, err := ci.Containers(ctx); err == nil {
			overview.Sections["containers"] = "available"
			overview.Containers = len(cts)
			sort.Slice(cts, func(i, j int) bool { return cts[i].ID < cts[j].ID })
			for _, c := range cts {
				overview.ContainerDetails = append(overview.ContainerDetails, statusGuest{ID: c.ID, Name: c.Name, Node: c.Node, Status: c.Status})
			}
			for _, c := range cts {
				if c.Status == "running" {
					overview.CTsRunning++
				} else if c.Status == "stopped" {
					overview.CTsStopped++
				} else {
					overview.CTsUnknown++
				}
			}
		}
	}

	collectStatusStorage(ctx, prov, &overview)
	overview.Sections["storage"] = overview.StorageCapacity.State
	collectStatusDiskInventory(ctx, prov, &overview)

	if cl, ok := prov.(domain.ClusterInspector); ok {
		overview.Sections["cluster"] = "unavailable"
		if cluster, err := cl.Cluster(ctx); err == nil && cluster != nil {
			overview.Sections["cluster"] = "available"
			overview.Cluster = cluster.Name
			overview.Version = cluster.Version
		}
	}

	// Query cluster status for quorum information.
	quorum := detectQuorum(ctx, prov)
	overview.Quorum = quorum.Quorum
	overview.QuorumKnown = quorum.Known
	overview.Standalone = quorum.Standalone
	if _, ok := prov.(domain.ClusterStatusProvider); ok {
		overview.Sections["quorum"] = "unavailable"
		if quorum.Known || quorum.Standalone {
			overview.Sections["quorum"] = "available"
		}
	}

	// Query HA status if available.
	if hp, ok := prov.(domain.HAProvider); ok {
		overview.Sections["ha"] = "unavailable"
		if ha, err := hp.HAStatus(ctx); err == nil && ha != nil {
			overview.Sections["ha"] = "available"
			overview.HA = &statusHA{
				Status: ha.Status,
				// Cluster quorum is authoritative for the cluster overview. The
				// HA manager endpoint can report an unknown lifecycle status while
				// cluster status still provides a reliable quorum value.
				Quorum:      quorum.Quorum,
				QuorumKnown: quorum.Known,
				Standalone:  quorum.Standalone,
			}
		}
	}

	if includeTasks {
		collectStatusTasksWithWarnings(ctx, prov, &overview, warningCache)
	}
	for _, section := range []string{"nodes", "vms", "containers", "storage", "cluster", "quorum", "ha", "tasks"} {
		if state := overview.Sections[section]; state != "" && state != "available" {
			if section == "ha" && overview.Standalone {
				continue
			}
			overview.Warnings = append(overview.Warnings, section+": "+state)
		}
	}
	return &overview
}

func writeStatus(cmdCtx *Context, overview *statusOverview) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, overview)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, overview)
	default:
		if statusColorEnabled(cmdCtx) {
			var plain bytes.Buffer
			if err := writeStatusTable(output.NewSanitizingWriter(&plain), overview); err != nil {
				return err
			}
			_, err := io.WriteString(cmdCtx.InteractiveOut, statusColorText(plain.String(), true))
			return err
		}
		return writeStatusTable(cmdCtx.Writer, overview)
	}
}

func writeStatusTable(w io.Writer, overview *statusOverview) error {
	fmt.Fprintf(w, "Cluster: %s  Version: %s  Nodes: ", overview.Cluster, overview.Version)
	if state := overview.Sections["nodes"]; state != "" && state != "available" {
		fmt.Fprint(w, state)
	} else {
		fmt.Fprint(w, overview.Nodes)
	}
	switch {
	case overview.QuorumKnown:
		fmt.Fprintf(w, "  Quorum: %d", overview.Quorum)
	case overview.Standalone:
		fmt.Fprint(w, "  Quorum: n/a (standalone host)")
	default:
		fmt.Fprint(w, "  Quorum: unavailable")
	}
	fmt.Fprintln(w)
	writeStatusGuestCounts(w, "VMs", overview.Sections["vms"], overview.VMsRunning, overview.VMsStopped, overview.VMsUnknown)
	writeStatusGuestCounts(w, "Containers", overview.Sections["containers"], overview.CTsRunning, overview.CTsStopped, overview.CTsUnknown)
	for _, warning := range overview.Warnings {
		fmt.Fprintf(w, "Warning: %s\n", warning)
	}
	if overview.HA != nil {
		switch {
		case overview.HA.QuorumKnown:
			fmt.Fprintf(w, "HA: %s (quorum: %d)\n", overview.HA.Status, overview.HA.Quorum)
		case overview.HA.Standalone:
			fmt.Fprintf(w, "HA: %s (quorum: n/a — standalone host)\n", overview.HA.Status)
		default:
			fmt.Fprintf(w, "HA: %s (quorum: unavailable)\n", overview.HA.Status)
		}
	}
	if err := writeStatusStorage(w, overview); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "\nGuests:"); err != nil {
		return err
	}
	if err := writeStatusGuestInventory(w, "  ", "VMs", overview.Sections["vms"], overview.VMs, overview.VMDetails, overview.Nodes > 1); err != nil {
		return err
	}
	if err := writeStatusGuestInventory(w, "  ", "Containers", overview.Sections["containers"], overview.Containers, overview.ContainerDetails, overview.Nodes > 1); err != nil {
		return err
	}
	return nil
}

func runEventList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex event list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ei, err := requireEventInspector(prov)
	if err != nil {
		return err
	}
	events, err := ei.Events(ctx)
	if err != nil {
		return fmt.Errorf("list events: %w", err)
	}
	return writeEventList(cmdCtx, applyLimit(events, cmdCtx.Opts.Limit))
}

func writeEventList(cmdCtx *Context, events []domain.Event) error {
	if events == nil {
		events = []domain.Event{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, events)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, events)
	default:
		headers := []string{"TIME", "TYPE", "NODE", "ID", "MESSAGE"}
		rows := make([][]string, 0, len(events))
		for _, e := range events {
			rows = append(rows, []string{
				fmt.Sprintf("%d", e.Time),
				e.Type,
				e.Node,
				e.ID,
				e.Message,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// defaultSyslogEntries caps `nodex log` output. PVE returns the whole ring
// buffer since boot, which is unusable without a bound.
const defaultSyslogEntries = 50

// maxProxmoxSyslogEntries is PVE's documented maximum `limit` for one syslog
// response. Larger requests are capped and reported rather than silently
// truncated by the server.
const maxProxmoxSyslogEntries = 5000

// syslogFollowInterval is the poll period for `nodex log --follow`.
const syslogFollowInterval = 2 * time.Second

// logOptions holds the flags `nodex log` parses for itself.
type logOptions struct {
	node   string
	last   int
	grep   *regexp.Regexp
	follow bool
}

func logUsage() error {
	return app.NewExitError(
		fmt.Errorf("usage: nodex log <node> [--last <n>] [--grep <regexp>] [--follow]"),
		app.ExitUsage,
	)
}

// parseLogArgs splits the node positional from the handler-owned log flags.
// Proxmox's /nodes/{node}/syslog carries no timestamp (only the line number and
// text), so time-window filters are deliberately absent; --last and --grep
// cover the "show me the recent/relevant part" case instead.
func parseLogArgs(args []string, defaultLast int) (logOptions, error) {
	opts := logOptions{last: defaultLast}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, inline, hasInline := strings.Cut(arg, "=")
		value := func() (string, bool) {
			if hasInline {
				return inline, true
			}
			if i+1 < len(args) {
				i++
				return args[i], true
			}
			return "", false
		}
		switch name {
		case "--last":
			v, ok := value()
			if !ok {
				return opts, logUsage()
			}
			n, perr := strconv.Atoi(v)
			if perr != nil {
				return opts, app.NewExitError(
					fmt.Errorf("invalid value %q for flag --last: expected a non-negative integer", v),
					app.ExitUsage,
				)
			}
			if n < 0 {
				return opts, app.NewExitError(
					fmt.Errorf("invalid value %q for flag --last: must be non-negative", v),
					app.ExitUsage,
				)
			}
			opts.last = n
		case "--grep":
			v, ok := value()
			if !ok {
				return opts, logUsage()
			}
			re, perr := regexp.Compile(v)
			if perr != nil {
				return opts, app.NewExitError(
					fmt.Errorf("invalid value %q for flag --grep: %w", v, perr),
					app.ExitUsage,
				)
			}
			opts.grep = re
		case "--follow":
			opts.follow = true
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, app.NewExitError(
					fmt.Errorf("unknown flag: %s", name),
					app.ExitUsage,
				)
			}
			if opts.node != "" {
				return opts, logUsage()
			}
			opts.node = arg
		}
	}
	if opts.node == "" {
		return opts, logUsage()
	}
	return opts, nil
}

// filterSyslog keeps entries matching the grep pattern. A nil pattern is a
// no-op so callers can pass opts.grep directly.
func filterSyslog(entries []domain.SyslogEntry, re *regexp.Regexp) []domain.SyslogEntry {
	if re == nil {
		return entries
	}
	out := make([]domain.SyslogEntry, 0, len(entries))
	for _, e := range entries {
		if re.MatchString(e.Text) {
			out = append(out, e)
		}
	}
	return out
}

// tailSyslog returns at most n trailing entries. n <= 0 means no cap.
func tailSyslog(entries []domain.SyslogEntry, n int) []domain.SyslogEntry {
	if n <= 0 || len(entries) <= n {
		return entries
	}
	return entries[len(entries)-n:]
}

func loadSyslogEntries(ctx context.Context, inspector domain.SyslogInspector, node string, requested int) ([]domain.SyslogEntry, bool, error) {
	pager, ok := inspector.(domain.PagedSyslogInspector)
	if !ok {
		entries, err := inspector.Syslog(ctx, node)
		return entries, false, err
	}
	_, total, err := pager.SyslogPage(ctx, node, 0, 0)
	if err != nil {
		return nil, false, err
	}
	if total <= 0 {
		// Older providers may not expose the PVE `total` envelope field. Keep
		// their legacy behavior rather than mistaking an unavailable count for
		// an empty log.
		entries, fallbackErr := inspector.Syslog(ctx, node)
		return entries, false, fallbackErr
	}
	limit := requested
	capped := false
	if limit <= 0 || limit > maxProxmoxSyslogEntries {
		limit = maxProxmoxSyslogEntries
		capped = total > maxProxmoxSyslogEntries
	}
	if int64(limit) > total {
		limit = int(total)
	}
	start := int(total) - limit + 1
	if start < 1 {
		start = 1
	}
	entries, _, err := pager.SyslogPage(ctx, node, start, limit)
	return entries, capped, err
}

func runLog(ctx context.Context, cmdCtx *Context, args []string) error {
	// An explicit --limit keeps its global meaning; otherwise fall back to
	// --last, then to the default cap.
	defaultLast := defaultSyslogEntries
	if cmdCtx.Opts.Limit > 0 {
		defaultLast = cmdCtx.Opts.Limit
	}
	opts, err := parseLogArgs(args, defaultLast)
	if err != nil {
		return err
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sl, err := requireSyslogInspector(prov)
	if err != nil {
		return err
	}
	entries, capped, err := loadSyslogEntries(ctx, sl, opts.node, opts.last)
	if err != nil {
		return err
	}
	if capped {
		fmt.Fprintf(cmdCtx.ErrW, "warning: PVE limits syslog output to the most recent %d entries\n", maxProxmoxSyslogEntries)
	}

	shown := filterSyslog(tailSyslog(entries, opts.last), opts.grep)
	if opts.follow {
		// Streaming a table or a JSON document has no coherent meaning, so
		// --follow is text-only regardless of --output.
		return followSyslog(ctx, cmdCtx, sl, opts, shown)
	}
	return writeSyslog(cmdCtx, shown)
}

// followSyslog prints the initial tail then polls for entries newer than the
// highest line number already emitted, until the context is cancelled.
func followSyslog(ctx context.Context, cmdCtx *Context, sl domain.SyslogInspector, opts logOptions, initial []domain.SyslogEntry) error {
	var lastN int64
	for _, e := range initial {
		lastN = e.N
		fmt.Fprintf(cmdCtx.Writer, "%d\t%s\n", e.N, e.Text)
	}
	ticker := time.NewTicker(syslogFollowInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		entries, err := sl.Syslog(ctx, opts.node)
		if err != nil {
			return err
		}
		for _, e := range filterSyslog(entries, opts.grep) {
			if e.N <= lastN {
				continue
			}
			lastN = e.N
			fmt.Fprintf(cmdCtx.Writer, "%d\t%s\n", e.N, e.Text)
		}
	}
}

func writeSyslog(cmdCtx *Context, entries []domain.SyslogEntry) error {
	if entries == nil {
		entries = []domain.SyslogEntry{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, entries)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, entries)
	default:
		headers := []string{"N", "TEXT"}
		rows := make([][]string, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, []string{
				fmt.Sprintf("%d", e.N),
				e.Text,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func containerOSLabel(os string) string {
	if os == "" {
		return "unknown"
	}
	return os
}
