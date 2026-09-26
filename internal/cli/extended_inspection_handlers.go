package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func runHAStatus(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex ha status"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ha, err := requireHAStatus(prov)
	if err != nil {
		return err
	}
	status, err := ha.HAStatus(ctx)
	if err != nil {
		return fmt.Errorf("get HA status: %w", err)
	}
	// A standalone host has no cluster, so a zero quorum with an "unknown"
	// status is expected rather than a fault. Report the cluster name and
	// whether HA applies at all so an operator does not chase a phantom
	// partition (nit #36).
	quorum := detectQuorum(ctx, prov)
	return writeHAStatusTable(cmdCtx, newHAStatusView(status, quorum))
}

// haStatusView is the CLI projection of HA status. It adds the cluster
// context an operator needs to interpret a zero quorum.
type haStatusView struct {
	Quorum int    `json:"quorum" yaml:"quorum"`
	Status string `json:"status" yaml:"status"`
	// Enabled reports whether the HA subsystem applies to this host at all.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Cluster is the cluster name, empty on a standalone host.
	Cluster string `json:"cluster" yaml:"cluster"`
	// Standalone reports that no cluster exists, so Quorum is not meaningful.
	Standalone bool `json:"standalone" yaml:"standalone"`
	// QuorumKnown reports whether Quorum is a real reading.
	QuorumKnown bool `json:"quorum_known" yaml:"quorum_known"`
}

func newHAStatusView(status *domain.HAStatus, q quorumInfo) *haStatusView {
	if status == nil {
		status = &domain.HAStatus{}
	}
	return &haStatusView{
		Quorum:      status.Quorum,
		Status:      status.Status,
		Enabled:     !q.Standalone && q.Known,
		Cluster:     q.Name,
		Standalone:  q.Standalone,
		QuorumKnown: q.Known,
	}
}

func writeHAStatusTable(cmdCtx *Context, view *haStatusView) error {
	if view == nil {
		view = &haStatusView{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, view)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, view)
	default:
		quorum := strconv.Itoa(view.Quorum)
		switch {
		case view.QuorumKnown:
			// keep the numeric reading
		case view.Standalone:
			quorum = "n/a (standalone host)"
		default:
			quorum = "unavailable"
		}
		cluster := view.Cluster
		if cluster == "" {
			cluster = "-"
		}
		rows := [][]string{
			{"QUORUM", quorum},
			{"STATUS", view.Status},
			{"ENABLED", fmt.Sprintf("%t", view.Enabled)},
			{"CLUSTER", cluster},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

func runHACurrent(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex ha current"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ha, err := requireHAStatus(prov)
	if err != nil {
		return err
	}
	current, err := ha.HACurrent(ctx)
	if err != nil {
		return fmt.Errorf("get HA current: %w", err)
	}
	return writeHACurrentTable(cmdCtx, current)
}

func writeHACurrentTable(cmdCtx *Context, current []domain.HACurrent) error {
	if current == nil {
		current = []domain.HACurrent{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, current)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, current)
	default:
		headers := []string{"ID", "TYPE", "STATE", "NODE", "STATUS"}
		rows := make([][]string, 0, len(current))
		for _, c := range current {
			rows = append(rows, []string{c.ID, c.Type, c.State, c.Node, c.Status})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runBackupContent(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex backup content <node> <storage>"), app.ExitUsage)
	}
	node := args[0]
	storage := args[1]
	if node == "" || storage == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex backup content <node> <storage>"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	bkp, err := requireBackupContent(prov)
	if err != nil {
		return err
	}
	items, err := bkp.BackupContent(ctx, node, storage)
	if err != nil {
		return fmt.Errorf("get backup content: %w", err)
	}
	return writeBackupContentTable(cmdCtx, items)
}

func writeBackupContentTable(cmdCtx *Context, items []domain.BackupContentItem) error {
	if items == nil {
		items = []domain.BackupContentItem{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, items)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, items)
	default:
		headers := []string{"CONTENT", "VOLID", "FORMAT", "SIZE"}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{item.Content, item.Volid, item.Format, formatBytes(item.Size)})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runSDNZones(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn zones"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sdn, err := requireSDN(prov)
	if err != nil {
		return err
	}
	zones, err := sdn.SDNZones(ctx)
	if err != nil {
		return fmt.Errorf("get SDN zones: %w", err)
	}
	return writeSDNZonesTable(cmdCtx, zones)
}

func writeSDNZonesTable(cmdCtx *Context, zones []domain.SDNZone) error {
	if zones == nil {
		zones = []domain.SDNZone{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, zones)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, zones)
	default:
		headers := []string{"NAME", "TYPE", "STATUS", "VNETS"}
		rows := make([][]string, 0, len(zones))
		for _, z := range zones {
			rows = append(rows, []string{z.Name, z.Type, z.Status, fmt.Sprintf("%d", z.VNets)})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runSDNVNets(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn vnets"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sdn, err := requireSDN(prov)
	if err != nil {
		return err
	}
	vnets, err := sdn.SDNVNets(ctx)
	if err != nil {
		return fmt.Errorf("get SDN vnets: %w", err)
	}
	return writeSDNVNetsTable(cmdCtx, vnets)
}

func writeSDNVNetsTable(cmdCtx *Context, vnets []domain.SDNVNet) error {
	if vnets == nil {
		vnets = []domain.SDNVNet{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, vnets)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, vnets)
	default:
		headers := []string{"NAME", "ZONE", "VLAN", "ALIAS"}
		rows := make([][]string, 0, len(vnets))
		for _, v := range vnets {
			vlan := ""
			if v.VLAN > 0 {
				vlan = fmt.Sprintf("%d", v.VLAN)
			}
			rows = append(rows, []string{v.Name, v.Zone, vlan, v.Alias})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runSDNSubnets(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn subnets"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sdn, err := requireSDN(prov)
	if err != nil {
		return err
	}
	subnets, err := sdn.SDNSubnets(ctx)
	if err != nil {
		return fmt.Errorf("get SDN subnets: %w", err)
	}
	return writeSDNSubnetsTable(cmdCtx, subnets)
}

func writeSDNSubnetsTable(cmdCtx *Context, subnets []domain.SDNSubnet) error {
	if subnets == nil {
		subnets = []domain.SDNSubnet{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, subnets)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, subnets)
	default:
		headers := []string{"SUBNET", "TYPE", "VNET", "ZONE", "CIDR", "GATEWAY"}
		rows := make([][]string, 0, len(subnets))
		for _, s := range subnets {
			rows = append(rows, []string{s.Name, s.Type, s.VNet, s.Zone, s.CIDR, s.Gateway})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runSDNControllers(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex sdn controllers"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sdn, err := requireSDN(prov)
	if err != nil {
		return err
	}
	controllers, err := sdn.SDNControllers(ctx)
	if err != nil {
		return fmt.Errorf("get SDN controllers: %w", err)
	}
	return writeSDNControllersTable(cmdCtx, controllers)
}

func writeSDNControllersTable(cmdCtx *Context, controllers []domain.SDNController) error {
	if controllers == nil {
		controllers = []domain.SDNController{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, controllers)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, controllers)
	default:
		headers := []string{"NAME", "TYPE", "STATE", "ASN"}
		rows := make([][]string, 0, len(controllers))
		for _, c := range controllers {
			asn := ""
			if c.ASN > 0 {
				asn = strconv.Itoa(c.ASN)
			}
			rows = append(rows, []string{c.Name, c.Type, c.State, asn})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runVMSnapshotConfig(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm snapshot-config <node>/<vmid> <name>"), app.ExitUsage)
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex vm snapshot-config <node>/<vmid> <name>"), app.ExitUsage)
	}
	node := parts[0]
	vmid, err := strconv.Atoi(parts[1])
	if err != nil || vmid <= 0 {
		return app.NewExitError(fmt.Errorf("invalid VMID: %s", parts[1]), app.ExitUsage)
	}
	name := args[1]
	if name == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex vm snapshot-config <node>/<vmid> <name>"), app.ExitUsage)
	}
	if name == "current" {
		return app.NewExitError(fmt.Errorf("snapshot config is unavailable for the synthetic %q snapshot", name), app.ExitValidationError)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	snap, err := requireSnapshotDetail(prov)
	if err != nil {
		return err
	}
	config, err := snap.VMSnapshotConfig(ctx, node, vmid, name)
	if err != nil {
		return fmt.Errorf("get VM snapshot config: %w", err)
	}
	return writeConfig(cmdCtx, config)
}

func runContainerSnapshotConfig(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex container snapshot-config <node>/<vmid> <name>"), app.ExitUsage)
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex container snapshot-config <node>/<vmid> <name>"), app.ExitUsage)
	}
	node := parts[0]
	vmid, err := strconv.Atoi(parts[1])
	if err != nil || vmid <= 0 {
		return app.NewExitError(fmt.Errorf("invalid VMID: %s", parts[1]), app.ExitUsage)
	}
	name := args[1]
	if name == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex container snapshot-config <node>/<vmid> <name>"), app.ExitUsage)
	}
	if name == "current" {
		return app.NewExitError(fmt.Errorf("snapshot config is unavailable for the synthetic %q snapshot", name), app.ExitValidationError)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	snap, err := requireSnapshotDetail(prov)
	if err != nil {
		return err
	}
	config, err := snap.ContainerSnapshotConfig(ctx, node, vmid, name)
	if err != nil {
		return fmt.Errorf("get container snapshot config: %w", err)
	}
	return writeConfig(cmdCtx, config)
}
