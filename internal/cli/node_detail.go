package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	// Embed the IANA time zone database next to the code that calls
	// time.LoadLocation. Windows ships no system tzdata, so without this,
	// every zone lookup fails and rendered timestamps fall back to UTC.
	_ "time/tzdata"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/safety"
)

func runNodeServices(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex node services <node>"), app.ExitUsage)
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
	services, err := detail.NodeServices(ctx, args[0])
	if err != nil {
		return err
	}
	// Cluster-only services such as corosync always read as dead on a
	// standalone host. Annotate them so the state is not mistaken for a fault
	// (nit #28). A failed cluster query leaves the output untouched.
	annotateStandaloneServices(services, detectQuorum(ctx, prov))
	return writeNodeServices(cmdCtx, applyLimit(services, cmdCtx.Opts.Limit))
}

func writeNodeServices(cmdCtx *Context, services []domain.NodeService) error {
	if services == nil {
		services = []domain.NodeService{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, services)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, services)
	default:
		headers := []string{"NAME", "STATE", "ACTIVE", "NOTE"}
		rows := make([][]string, 0, len(services))
		for _, s := range services {
			active := "no"
			if s.Active {
				active = "yes"
			}
			rows = append(rows, []string{s.Name, s.State, active, s.Note})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runNodeNetwork(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex node network <node>"), app.ExitUsage)
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
	interfaces, err := detail.NodeNetwork(ctx, args[0])
	if err != nil {
		return err
	}
	return writeNodeNetwork(cmdCtx, applyLimit(interfaces, cmdCtx.Opts.Limit))
}

func writeNodeNetwork(cmdCtx *Context, interfaces []domain.NodeNetwork) error {
	if interfaces == nil {
		interfaces = []domain.NodeNetwork{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, interfaces)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, interfaces)
	default:
		headers := []string{"NAME", "TYPE", "STATUS", "IP", "MAC", "PORTS", "VLAN-AWARE", "VLANS"}
		rows := make([][]string, 0, len(interfaces))
		for _, iface := range interfaces {
			ports := iface.BridgePorts
			if ports == "" && iface.Type == "vlan" && iface.VLANDevice != "" {
				ports = "on " + iface.VLANDevice
			}
			vlanAware := ""
			if iface.BridgeVLANAware {
				vlanAware = "yes"
			}
			vlans := iface.BridgeVLANs
			if vlans == "" && iface.Type == "vlan" && iface.VLANID != 0 {
				vlans = strconv.Itoa(iface.VLANID)
			}
			rows = append(rows, []string{iface.Name, iface.Type, iface.Status, iface.IP, iface.MAC, ports, vlanAware, vlans})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runNodeDNS(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) > 0 && args[0] == "set" {
		return runNodeDNSSet(ctx, cmdCtx, args[1:])
	}
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex node dns <node>"), app.ExitUsage)
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
	dns, err := detail.NodeDNS(ctx, args[0])
	if err != nil {
		return fmt.Errorf("get node dns: %w", err)
	}
	return writeNodeDNS(cmdCtx, dns)
}

func runNodeDNSSet(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 || net.ParseIP(args[1]) == nil {
		return app.NewExitError(fmt.Errorf("usage: nodex node dns set <node> <resolver-ip>"), app.ExitUsage)
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
	current, err := detail.NodeDNS(ctx, args[0])
	if err != nil {
		return fmt.Errorf("get current node DNS: %w", err)
	}
	updated := *current
	updated.DNS1 = args[1]
	if err := checkDisruptive(cmdCtx, fmt.Sprintf("set primary DNS for node %s to %s", args[0], args[1])); err != nil {
		return err
	}
	updater, ok := prov.(domain.NodeDNSUpdater)
	if !ok {
		return app.NewExitError(fmt.Errorf("provider does not support node DNS updates"), app.ExitIncompatibility)
	}
	if err := updater.SetNodeDNS(ctx, args[0], &updated); err != nil {
		var providerErr *app.ProviderError
		if errors.As(err, &providerErr) && providerErr.StatusCode != 0 {
			return app.NewExitError(fmt.Errorf("set node DNS: %w", err), app.ExitProvider)
		}
		return app.NewExitError(fmt.Errorf("node DNS update outcome is unknown: %w", err), app.ExitAmbiguousOutcome)
	}
	actual, err := detail.NodeDNS(ctx, args[0])
	if err != nil {
		return app.NewExitError(fmt.Errorf("node DNS update submitted but readback failed: %w", err), app.ExitPartialFailure)
	}
	if actual.DNS1 != updated.DNS1 || actual.DNS2 != updated.DNS2 || actual.SearchDomain != updated.SearchDomain {
		return app.NewExitError(fmt.Errorf("node DNS readback does not match requested configuration"), app.ExitPartialFailure)
	}
	result := output.NewOperationResult("node dns set", prov.Name(), cmdCtx.Opts.Profile)
	result.Target = args[0]
	result.Safety = safety.TierDisruptive.String()
	result.Submitted, result.Synchronous, result.Success = true, true, true
	changed := current.DNS1 != updated.DNS1 || current.DNS2 != updated.DNS2 || current.SearchDomain != updated.SearchDomain
	result.Changed = &changed
	result.Status, result.Verification = "OK", "verified"
	return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result)
}

func writeNodeDNS(cmdCtx *Context, dns *domain.NodeDNS) error {
	if dns == nil {
		dns = &domain.NodeDNS{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, dns)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, dns)
	default:
		rows := [][]string{
			{"DNS1", dns.DNS1},
			{"DNS2", dns.DNS2},
			{"SEARCH DOMAIN", dns.SearchDomain},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

func runNodeTime(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex node time <node>"), app.ExitUsage)
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
	nodeTime, err := detail.NodeTime(ctx, args[0])
	if err != nil {
		return err
	}
	return writeNodeTime(cmdCtx, nodeTime)
}

func writeNodeTime(cmdCtx *Context, nodeTime *domain.NodeTime) error {
	if nodeTime == nil {
		nodeTime = &domain.NodeTime{}
	}
	nodeTime.LocalHuman = nodeTimeLocalHuman(nodeTime)
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, nodeTime)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, nodeTime)
	default:
		rows := [][]string{
			{"TIMEZONE", nodeTime.TimeZone},
			{"EPOCH", fmt.Sprintf("%d", nodeTime.Epoch)},
			{"LOCAL HUMAN", nodeTime.LocalHuman},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

// nodeTimeLocalHuman renders the node's epoch as an RFC3339 timestamp in its
// configured time zone.
//
// When the configured zone cannot be loaded this returns "" rather than a UTC
// timestamp. A UTC value presented as the node's local time is a plausible
// but wrong answer, which is worse for a scheduling surface than an absent
// one: callers can detect the empty field, but cannot detect the skew.
func nodeTimeLocalHuman(nodeTime *domain.NodeTime) string {
	if nodeTime == nil || nodeTime.Epoch == 0 {
		return ""
	}
	loc := time.UTC
	if nodeTime.TimeZone != "" {
		l, err := time.LoadLocation(nodeTime.TimeZone)
		if err != nil {
			return ""
		}
		loc = l
	}
	return time.Unix(nodeTime.Epoch, 0).In(loc).Format(time.RFC3339)
}

func runNodeDisks(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex node disks <node>"), app.ExitUsage)
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
	disks, err := detail.NodeDisks(ctx, args[0])
	if err != nil {
		return err
	}
	return writeNodeDisks(cmdCtx, applyLimit(disks, cmdCtx.Opts.Limit))
}

func writeNodeDisks(cmdCtx *Context, disks []domain.NodeDisk) error {
	if disks == nil {
		disks = []domain.NodeDisk{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, disks)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, disks)
	default:
		headers := []string{"NAME", "PATH", "TYPE", "SIZE", "MODEL", "HEALTH"}
		rows := make([][]string, 0, len(disks))
		for _, d := range disks {
			rows = append(rows, []string{d.Name, d.Path, d.Type, formatBytes(d.Size), d.Model, d.Health})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runNodeCertificates(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex node certificates <node>"), app.ExitUsage)
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
	certs, err := detail.NodeCertificates(ctx, args[0])
	if err != nil {
		return err
	}
	return writeNodeCertificates(cmdCtx, applyLimit(certs, cmdCtx.Opts.Limit))
}

func writeNodeCertificates(cmdCtx *Context, certs []domain.NodeCertificate) error {
	if certs == nil {
		certs = []domain.NodeCertificate{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, certs)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, certs)
	default:
		headers := []string{"FILE", "SUBJECT", "ISSUER", "FINGERPRINT", "NOT BEFORE", "NOT AFTER"}
		rows := make([][]string, 0, len(certs))
		for _, c := range certs {
			rows = append(rows, []string{
				c.Filename,
				c.Subject,
				c.Issuer,
				c.Fingerprint,
				formatEpoch(c.NotBefore),
				formatEpoch(c.NotAfter),
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runNodeSubscription(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex node subscription <node>"), app.ExitUsage)
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
	sub, err := detail.NodeSubscription(ctx, args[0])
	if err != nil {
		return err
	}
	return writeNodeSubscription(cmdCtx, sub)
}

func writeNodeSubscription(cmdCtx *Context, sub *domain.NodeSubscription) error {
	if sub == nil {
		sub = &domain.NodeSubscription{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, sub)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, sub)
	default:
		rows := [][]string{
			{"STATUS", sub.Status},
			{"KEY", sub.Key},
			{"EXPIRES", sub.Expires},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

func runNodeUpdates(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex node updates <node>"), app.ExitUsage)
	}
	policy := safety.ConfirmationPolicy{Tier: safety.TierReversible, ResourceDescription: "package-status cache on node " + args[0]}
	confirmation := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if confirmation.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", confirmation.Message), app.ExitUsage)
		}
		return fmt.Errorf("%w: %s", safety.ErrAuthorizationRequired, confirmation.Message)
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
	updates, err := detail.NodeUpdates(ctx, args[0])
	if err != nil {
		return err
	}
	return writeNodeUpdates(cmdCtx, applyLimit(updates, cmdCtx.Opts.Limit))
}

func writeNodeUpdates(cmdCtx *Context, updates []domain.NodeUpdate) error {
	if updates == nil {
		updates = []domain.NodeUpdate{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, updates)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, updates)
	default:
		headers := []string{"PACKAGE", "VERSION"}
		rows := make([][]string, 0, len(updates))
		for _, u := range updates {
			rows = append(rows, []string{u.Package, u.Version})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}
