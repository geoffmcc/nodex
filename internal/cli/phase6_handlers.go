package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/safety"
)

// --- Ceph read-only handlers ---

// nodex ceph status <node>
func runCephStatus(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph status <node>"), app.ExitUsage)
	}
	node := args[0]
	if node == "" {
		return app.NewExitError(fmt.Errorf("node name is required"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ceph, err := requireCeph(prov)
	if err != nil {
		return err
	}
	status, err := ceph.CephStatus(ctx, node)
	if err != nil {
		return fmt.Errorf("get ceph status: %w", err)
	}
	return writeCephStatusTable(cmdCtx, status)
}

func writeCephStatusTable(cmdCtx *Context, status *domain.CephStatus) error {
	if status == nil {
		status = &domain.CephStatus{Health: map[string]interface{}{}}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, status)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, status)
	default:
		health := status.Health
		rows := [][]string{
			{"HEALTH", fmt.Sprintf("%v", health["health"])},
			{"TIME", fmt.Sprintf("%v", health["time"])},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

// nodex ceph osd list <node>
func runCephOSDList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph osd list <node>"), app.ExitUsage)
	}
	node := args[0]
	if node == "" {
		return app.NewExitError(fmt.Errorf("node name is required"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ceph, err := requireCeph(prov)
	if err != nil {
		return err
	}
	osds, err := ceph.CephOSDs(ctx, node)
	if err != nil {
		return fmt.Errorf("get ceph osds: %w", err)
	}
	return writeCephOSDsTable(cmdCtx, osds)
}

func writeCephOSDsTable(cmdCtx *Context, osds []domain.CephOSD) error {
	if osds == nil {
		osds = []domain.CephOSD{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, osds)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, osds)
	default:
		headers := []string{"ID", "NAME", "STATUS", "IN", "HOST", "CLASS", "USED"}
		rows := make([][]string, 0, len(osds))
		for _, o := range osds {
			used := ""
			if o.TotalSpace > 0 {
				used = formatBytes(o.BytesUsed) + " / " + formatBytes(o.TotalSpace)
			}
			in := "yes"
			if o.In == 0 {
				in = "no"
			}
			rows = append(rows, []string{
				strconv.Itoa(o.ID), o.Name, o.Status, in, o.Host, o.DeviceClass, used,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// nodex ceph mon list <node>
func runCephMONList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph mon list <node>"), app.ExitUsage)
	}
	node := args[0]
	if node == "" {
		return app.NewExitError(fmt.Errorf("node name is required"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ceph, err := requireCeph(prov)
	if err != nil {
		return err
	}
	mons, err := ceph.CephMONs(ctx, node)
	if err != nil {
		return fmt.Errorf("get ceph mons: %w", err)
	}
	return writeCephMONsTable(cmdCtx, mons)
}

func writeCephMONsTable(cmdCtx *Context, mons []domain.CephMON) error {
	if mons == nil {
		mons = []domain.CephMON{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, mons)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, mons)
	default:
		headers := []string{"NAME", "HOST", "QUORUM", "STATE", "RANK", "VERSION"}
		rows := make([][]string, 0, len(mons))
		for _, m := range mons {
			quorum := "no"
			if m.Quorum {
				quorum = "yes"
			}
			rows = append(rows, []string{m.Name, m.Host, quorum, m.State, strconv.Itoa(m.Rank), m.Version})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// nodex ceph pool list <node>
func runCephPoolList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph pool list <node>"), app.ExitUsage)
	}
	node := args[0]
	if node == "" {
		return app.NewExitError(fmt.Errorf("node name is required"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ceph, err := requireCeph(prov)
	if err != nil {
		return err
	}
	pools, err := ceph.CephPools(ctx, node)
	if err != nil {
		return fmt.Errorf("get ceph pools: %w", err)
	}
	return writeCephPoolsTable(cmdCtx, pools)
}

func writeCephPoolsTable(cmdCtx *Context, pools []domain.CephPool) error {
	if pools == nil {
		pools = []domain.CephPool{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, pools)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, pools)
	default:
		headers := []string{"ID", "NAME", "TYPE", "SIZE/MIN", "PG", "AUTOSCALE", "USED%"}
		rows := make([][]string, 0, len(pools))
		for _, p := range pools {
			sizeInfo := fmt.Sprintf("%d/%d", p.Size, p.MinSize)
			used := ""
			if p.PercentUsed > 0 {
				used = fmt.Sprintf("%.1f%%", p.PercentUsed)
			}
			rows = append(rows, []string{
				strconv.Itoa(p.ID), p.Name, p.Type, sizeInfo,
				strconv.Itoa(p.PGNum), p.PGAutoscaleMode, used,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// --- Ceph mutation handlers ---

// nodex ceph osd create <node> <dev>
func runCephOSDCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph osd create <node> <dev>"), app.ExitUsage)
	}
	node := args[0]
	dev := args[1]
	if node == "" || dev == "" {
		return app.NewExitError(fmt.Errorf("node and dev are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cm, err := requireCephMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("create Ceph OSD on %s dev %s", node, dev)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := cm.CephCreateOSD(ctx, node, dev)
	if err != nil {
		return fmt.Errorf("create ceph osd: %w", err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "ceph osd create", node, "disruptive")
}

// nodex ceph osd out <node> <id>
func runCephOSDOut(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph osd out <node> <id>"), app.ExitUsage)
	}
	node := args[0]
	idStr := args[1]
	osdid, err := strconv.Atoi(idStr)
	if err != nil || osdid < 0 {
		return app.NewExitError(fmt.Errorf("invalid OSD ID: %s", idStr), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cm, err := requireCephMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("mark Ceph OSD %d out on node %s", osdid, node)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := cm.CephOSDOut(ctx, node, osdid); err != nil {
		return fmt.Errorf("ceph osd out: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "OSD %d marked out on %s\n", osdid, node)
	return nil
}

// nodex ceph osd in <node> <id>
func runCephOSDIn(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph osd in <node> <id>"), app.ExitUsage)
	}
	node := args[0]
	idStr := args[1]
	osdid, err := strconv.Atoi(idStr)
	if err != nil || osdid < 0 {
		return app.NewExitError(fmt.Errorf("invalid OSD ID: %s", idStr), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cm, err := requireCephMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("mark Ceph OSD %d in on node %s", osdid, node)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierReversible,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
		return fmt.Errorf("%w: %s", safety.ErrAuthorizationRequired, result.Message)
	}

	if err := cm.CephOSDIn(ctx, node, osdid); err != nil {
		return fmt.Errorf("ceph osd in: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "OSD %d marked in on %s\n", osdid, node)
	return nil
}

// nodex ceph osd destroy <node> <id>
func runCephOSDDestroy(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph osd destroy <node> <id>"), app.ExitUsage)
	}
	node := args[0]
	idStr := args[1]
	osdid, err := strconv.Atoi(idStr)
	if err != nil || osdid < 0 {
		return app.NewExitError(fmt.Errorf("invalid OSD ID: %s", idStr), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cm, err := requireCephMutation(prov)
	if err != nil {
		return err
	}

	targetID := fmt.Sprintf("%s/osd.%d", node, osdid)
	desc := fmt.Sprintf("Ceph OSD %d on %s", osdid, node)
	if err := checkDestructive(cmdCtx, desc, targetID); err != nil {
		return err
	}

	upid, err := cm.CephDestroyOSD(ctx, node, osdid)
	if err != nil {
		return fmt.Errorf("destroy ceph osd: %w", err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "ceph osd destroy", fmt.Sprintf("osd.%d", osdid), "destructive")
}

// nodex ceph pool create <node> <name> [key=value...]
func runCephPoolCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph pool create <node> <name> [key=value...]"), app.ExitUsage)
	}
	node := args[0]
	name := args[1]
	if node == "" || name == "" {
		return app.NewExitError(fmt.Errorf("node and pool name are required"), app.ExitUsage)
	}

	params := map[string]string{}
	for _, arg := range args[2:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return app.NewExitError(fmt.Errorf("invalid parameter: %s (expected key=value)", arg), app.ExitUsage)
		}
		params[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cm, err := requireCephMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("create Ceph pool %q on node %s", name, node)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := cm.CephCreatePool(ctx, node, name, params)
	if err != nil {
		return fmt.Errorf("create ceph pool: %w", err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "ceph pool create", fmt.Sprintf("%s/%s", node, name), "disruptive")
}

// nodex ceph pool destroy <node> <name>
func runCephPoolDestroy(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph pool destroy <node> <name>"), app.ExitUsage)
	}
	node := args[0]
	name := args[1]
	if node == "" || name == "" {
		return app.NewExitError(fmt.Errorf("node and pool name are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cm, err := requireCephMutation(prov)
	if err != nil {
		return err
	}

	targetID := fmt.Sprintf("%s/%s", node, name)
	desc := fmt.Sprintf("Ceph pool %q on %s", name, node)
	if err := checkDestructive(cmdCtx, desc, targetID); err != nil {
		return err
	}

	upid, err := cm.CephDestroyPool(ctx, node, name)
	if err != nil {
		return fmt.Errorf("destroy ceph pool: %w", err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "ceph pool destroy", fmt.Sprintf("%s/%s", node, name), "destructive")
}

// --- SDN mutation handlers ---

// nodex sdn zone create <name> --type <simple|vlan|qinq|vxlan|evpn>
func runSDNZoneCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn zone create <name> --type <type>"), app.ExitUsage)
	}

	zoneName := args[0]
	var zoneType string

	// Parse remaining args for --type flag
	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 && parts[0] == "--type" {
			zoneType = parts[1]
		}
	}

	if zoneType == "" {
		return app.NewExitError(fmt.Errorf("--type flag is required (simple, vlan, qinq, vxlan, evpn)"), app.ExitUsage)
	}

	validTypes := map[string]bool{"simple": true, "vlan": true, "qinq": true, "vxlan": true, "evpn": true}
	if !validTypes[zoneType] {
		return app.NewExitError(fmt.Errorf("invalid zone type %q; must be simple, vlan, qinq, vxlan, or evpn", zoneType), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sm, err := requireSDNMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("create SDN zone %q (type: %s)", zoneName, zoneType)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := sm.SDNCreateZone(ctx, zoneType, zoneName); err != nil {
		return fmt.Errorf("create SDN zone: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "SDN zone %q created\n", zoneName)
	return nil
}

// nodex sdn zone delete <name>
func runSDNZoneDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn zone delete <name>"), app.ExitUsage)
	}
	zoneName := args[0]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sm, err := requireSDNMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("SDN zone %q", zoneName)
	if err := checkDestructive(cmdCtx, desc, zoneName); err != nil {
		return err
	}

	if err := sm.SDNDeleteZone(ctx, zoneName); err != nil {
		return fmt.Errorf("delete SDN zone: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "SDN zone %q deleted\n", zoneName)
	return nil
}

// nodex sdn vnet create <name> --zone <zone>
func runSDNVNetCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn vnet create <name> --zone <zone>"), app.ExitUsage)
	}

	vnetName := args[0]
	var zone string

	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 && parts[0] == "--zone" {
			zone = parts[1]
		}
	}

	if zone == "" {
		return app.NewExitError(fmt.Errorf("--zone flag is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sm, err := requireSDNMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("create SDN VNet %q (zone: %s)", vnetName, zone)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := sm.SDNCreateVNet(ctx, vnetName, zone); err != nil {
		return fmt.Errorf("create SDN VNet: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "SDN VNet %q created\n", vnetName)
	return nil
}

// nodex sdn vnet delete <name>
func runSDNVNetDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn vnet delete <name>"), app.ExitUsage)
	}
	vnetName := args[0]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sm, err := requireSDNMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("SDN VNet %q", vnetName)
	if err := checkDestructive(cmdCtx, desc, vnetName); err != nil {
		return err
	}

	if err := sm.SDNDeleteVNet(ctx, vnetName); err != nil {
		return fmt.Errorf("delete SDN VNet: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "SDN VNet %q deleted\n", vnetName)
	return nil
}

// nodex sdn subnet create <vnet> <cidr> --gateway <gw>
func runSDNSubnetCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn subnet create <vnet> <cidr> --gateway <gw>"), app.ExitUsage)
	}

	vnet := args[0]
	cidr := args[1]
	var gateway string

	for _, arg := range args[2:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 && parts[0] == "--gateway" {
			gateway = parts[1]
		}
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sm, err := requireSDNMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("create SDN subnet %s in VNet %s", cidr, vnet)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := sm.SDNCreateSubnet(ctx, vnet, cidr, gateway); err != nil {
		return fmt.Errorf("create SDN subnet: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "SDN subnet %s created in VNet %s\n", cidr, vnet)
	return nil
}

// nodex sdn subnet delete <vnet> <subnet>
func runSDNSubnetDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn subnet delete <vnet> <subnet>"), app.ExitUsage)
	}

	vnet := args[0]
	subnet := args[1]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sm, err := requireSDNMutation(prov)
	if err != nil {
		return err
	}

	targetID := fmt.Sprintf("%s/%s", vnet, subnet)
	desc := fmt.Sprintf("SDN subnet %s in VNet %s", subnet, vnet)
	if err := checkDestructive(cmdCtx, desc, targetID); err != nil {
		return err
	}

	if err := sm.SDNDeleteSubnet(ctx, vnet, subnet); err != nil {
		return fmt.Errorf("delete SDN subnet: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "SDN subnet %s deleted from VNet %s\n", subnet, vnet)
	return nil
}

// nodex sdn controller create <name>
func runSDNControllerCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn controller create <name>"), app.ExitUsage)
	}
	ctrlName := args[0]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sm, err := requireSDNMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("create SDN controller %q", ctrlName)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := sm.SDNCreateController(ctx, ctrlName); err != nil {
		return fmt.Errorf("create SDN controller: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "SDN controller %q created\n", ctrlName)
	return nil
}

// nodex sdn controller delete <name>
func runSDNControllerDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn controller delete <name>"), app.ExitUsage)
	}
	ctrlName := args[0]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sm, err := requireSDNMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("SDN controller %q", ctrlName)
	if err := checkDestructive(cmdCtx, desc, ctrlName); err != nil {
		return err
	}

	if err := sm.SDNDeleteController(ctx, ctrlName); err != nil {
		return fmt.Errorf("delete SDN controller: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "SDN controller %q deleted\n", ctrlName)
	return nil
}

// --- Replication handlers ---

// nodex replication list
func runReplicationList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex replication list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	rp, err := requireReplication(prov)
	if err != nil {
		return err
	}
	jobs, err := rp.ReplicationList(ctx)
	if err != nil {
		return fmt.Errorf("get replication jobs: %w", err)
	}
	return writeReplicationJobsTable(cmdCtx, jobs)
}

func writeReplicationJobsTable(cmdCtx *Context, jobs []domain.ReplicationJob) error {
	if jobs == nil {
		jobs = []domain.ReplicationJob{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, jobs)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, jobs)
	default:
		headers := []string{"ID", "GUEST", "TYPE", "SOURCE", "TARGET", "SCHEDULE", "ENABLED", "FAILS"}
		rows := make([][]string, 0, len(jobs))
		for _, j := range jobs {
			enabled := "yes"
			if j.Enabled == 0 {
				enabled = "no"
			}
			fails := ""
			if j.FailCount > 0 {
				fails = strconv.Itoa(j.FailCount)
			}
			rows = append(rows, []string{
				j.ID, strconv.Itoa(j.Guest), j.Type, j.Source, j.Target,
				j.Schedule, enabled, fails,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// nodex replication show <id>
func runReplicationShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex replication show <id>"), app.ExitUsage)
	}
	id := args[0]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	rp, err := requireReplication(prov)
	if err != nil {
		return err
	}
	job, err := rp.ReplicationGet(ctx, id)
	if err != nil {
		return fmt.Errorf("get replication job: %w", err)
	}
	return writeReplicationJobDetail(cmdCtx, job)
}

func writeReplicationJobDetail(cmdCtx *Context, job *domain.ReplicationJob) error {
	if job == nil {
		return app.NewExitError(fmt.Errorf("replication job not found"), app.ExitUsage)
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, job)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, job)
	default:
		enabled := "yes"
		if job.Enabled == 0 {
			enabled = "no"
		}
		rows := [][]string{
			{"ID", job.ID},
			{"GUEST", strconv.Itoa(job.Guest)},
			{"TYPE", job.Type},
			{"SOURCE", job.Source},
			{"TARGET", job.Target},
			{"SCHEDULE", job.Schedule},
			{"ENABLED", enabled},
			{"COMMENT", job.Comment},
		}
		if job.FailCount > 0 {
			rows = append(rows, []string{"FAIL_COUNT", strconv.Itoa(job.FailCount)})
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

// nodex replication create <id> <guest> <type> <target> [key=value...]
func runReplicationCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 4 {
		return app.NewExitError(fmt.Errorf("usage: nodex replication create <id> <guest> <type> <target> [key=value...]"), app.ExitUsage)
	}

	id := args[0]
	guest, err := strconv.Atoi(args[1])
	if err != nil || guest <= 0 {
		return app.NewExitError(fmt.Errorf("invalid guest ID: %s", args[1]), app.ExitUsage)
	}
	jobType := args[2]
	target := args[3]

	input := domain.ReplicationCreateInput{
		ID:     id,
		Guest:  guest,
		Type:   jobType,
		Target: target,
	}

	// Parse optional key=value parameters
	for _, arg := range args[4:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return app.NewExitError(fmt.Errorf("invalid parameter: %s (expected key=value)", arg), app.ExitUsage)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "schedule":
			input.Schedule = val
		case "comment":
			input.Comment = val
		case "rate":
			rate, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return app.NewExitError(fmt.Errorf("invalid rate: %s", val), app.ExitUsage)
			}
			input.Rate = rate
		case "source":
			input.Source = val
		}
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	rp, err := requireReplication(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("create replication job %q for guest %d", id, guest)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := rp.ReplicationCreate(ctx, input); err != nil {
		return fmt.Errorf("create replication job: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Replication job %s created\n", id)
	return nil
}

// nodex replication update <id> [key=value...]
func runReplicationUpdate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex replication update <id> [key=value...]"), app.ExitUsage)
	}

	id := args[0]
	input := domain.ReplicationUpdateInput{}

	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return app.NewExitError(fmt.Errorf("invalid parameter: %s (expected key=value)", arg), app.ExitUsage)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "target":
			input.Target = val
		case "schedule":
			input.Schedule = val
		case "comment":
			input.Comment = val
		case "rate":
			rate, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return app.NewExitError(fmt.Errorf("invalid rate: %s", val), app.ExitUsage)
			}
			input.Rate = rate
		case "source":
			input.Source = val
		case "enable":
			if val == "1" || val == "true" {
				input.Enable = 1
			}
		}
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	rp, err := requireReplication(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("update replication job %q", id)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := rp.ReplicationUpdate(ctx, id, input); err != nil {
		return fmt.Errorf("update replication job: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Replication job %s updated\n", id)
	return nil
}

// nodex replication delete <id>
func runReplicationDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex replication delete <id>"), app.ExitUsage)
	}
	id := args[0]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	rp, err := requireReplication(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("replication job %q", id)
	if err := checkDestructive(cmdCtx, desc, id); err != nil {
		return err
	}

	if err := rp.ReplicationDelete(ctx, id); err != nil {
		return fmt.Errorf("delete replication job: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Replication job %s deleted\n", id)
	return nil
}

// nodex replication schedule <node> <id>
func runReplicationSchedule(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex replication schedule <node> <id>"), app.ExitUsage)
	}
	node := args[0]
	id := args[1]
	if node == "" || id == "" {
		return app.NewExitError(fmt.Errorf("node and job id are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	rp, err := requireReplication(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("schedule replication job %q", id)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierReversible,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
		return fmt.Errorf("%w: %s", safety.ErrAuthorizationRequired, result.Message)
	}

	if err := rp.ReplicationSchedule(ctx, node, id); err != nil {
		return fmt.Errorf("schedule replication job: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Replication job %s scheduled on %s\n", id, node)
	return nil
}

// --- SDN read-only already registered in root.go ---

func runSDNZoneDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn zone <create|delete> [args]"), app.ExitUsage)
	}
	switch args[0] {
	case "create":
		return runSDNZoneCreate(ctx, cmdCtx, args[1:])
	case "delete":
		return runSDNZoneDelete(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(fmt.Errorf("unknown sdn zone subcommand: %s", args[0]), app.ExitUsage)
	}
}

func runSDNVNetDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn vnet <create|delete> [args]"), app.ExitUsage)
	}
	switch args[0] {
	case "create":
		return runSDNVNetCreate(ctx, cmdCtx, args[1:])
	case "delete":
		return runSDNVNetDelete(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(fmt.Errorf("unknown sdn vnet subcommand: %s", args[0]), app.ExitUsage)
	}
}

func runSDNSubnetDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn subnet <create|delete> [args]"), app.ExitUsage)
	}
	switch args[0] {
	case "create":
		return runSDNSubnetCreate(ctx, cmdCtx, args[1:])
	case "delete":
		return runSDNSubnetDelete(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(fmt.Errorf("unknown sdn subnet subcommand: %s", args[0]), app.ExitUsage)
	}
}

func runSDNControllerDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn controller <create|delete> [args]"), app.ExitUsage)
	}
	switch args[0] {
	case "create":
		return runSDNControllerCreate(ctx, cmdCtx, args[1:])
	case "delete":
		return runSDNControllerDelete(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(fmt.Errorf("unknown sdn controller subcommand: %s", args[0]), app.ExitUsage)
	}
}

// --- Replication handlers already registered in root.go ---

func runCephOSDDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph osd <list|create|out|in|destroy> [args]"), app.ExitUsage)
	}
	switch args[0] {
	case "list":
		return runCephOSDList(ctx, cmdCtx, args[1:])
	case "create":
		return runCephOSDCreate(ctx, cmdCtx, args[1:])
	case "out":
		return runCephOSDOut(ctx, cmdCtx, args[1:])
	case "in":
		return runCephOSDIn(ctx, cmdCtx, args[1:])
	case "destroy":
		return runCephOSDDestroy(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown ceph osd subcommand: %s (use list, create, out, in, or destroy)", args[0]),
			app.ExitUsage,
		)
	}
}

func runCephMonDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph mon <list> [args]"), app.ExitUsage)
	}
	switch args[0] {
	case "list":
		return runCephMONList(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown ceph mon subcommand: %s (use list)", args[0]),
			app.ExitUsage,
		)
	}
}

func runCephPoolDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex ceph pool <list|create|destroy> [args]"), app.ExitUsage)
	}
	switch args[0] {
	case "list":
		return runCephPoolList(ctx, cmdCtx, args[1:])
	case "create":
		return runCephPoolCreate(ctx, cmdCtx, args[1:])
	case "destroy":
		return runCephPoolDestroy(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown ceph pool subcommand: %s (use list, create, or destroy)", args[0]),
			app.ExitUsage,
		)
	}
}
