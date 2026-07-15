package cli

import (
	"context"
	"fmt"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
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
		return fmt.Errorf("get node services: %w", err)
	}
	return writeNodeServices(cmdCtx, services)
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
		headers := []string{"NAME", "STATE", "ACTIVE"}
		rows := make([][]string, 0, len(services))
		for _, s := range services {
			active := ""
			if s.Active {
				active = "yes"
			}
			rows = append(rows, []string{s.Name, s.State, active})
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
		return fmt.Errorf("get node network: %w", err)
	}
	return writeNodeNetwork(cmdCtx, interfaces)
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
		headers := []string{"NAME", "TYPE", "STATUS", "IP", "MAC"}
		rows := make([][]string, 0, len(interfaces))
		for _, iface := range interfaces {
			rows = append(rows, []string{iface.Name, iface.Type, iface.Status, iface.IP, iface.MAC})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runNodeDNS(ctx context.Context, cmdCtx *Context, args []string) error {
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
		return fmt.Errorf("get node time: %w", err)
	}
	return writeNodeTime(cmdCtx, nodeTime)
}

func writeNodeTime(cmdCtx *Context, nodeTime *domain.NodeTime) error {
	if nodeTime == nil {
		nodeTime = &domain.NodeTime{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, nodeTime)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, nodeTime)
	default:
		rows := [][]string{
			{"TIMEZONE", nodeTime.TimeZone},
			{"LOCAL", nodeTime.Local},
			{"EPOCH", fmt.Sprintf("%d", nodeTime.Epoch)},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
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
		return fmt.Errorf("get node disks: %w", err)
	}
	return writeNodeDisks(cmdCtx, disks)
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
		return fmt.Errorf("get node certificates: %w", err)
	}
	return writeNodeCertificates(cmdCtx, certs)
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
		headers := []string{"NAME"}
		rows := make([][]string, 0, len(certs))
		for _, c := range certs {
			rows = append(rows, []string{c.Name})
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
		return fmt.Errorf("get node subscription: %w", err)
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
		return fmt.Errorf("get node updates: %w", err)
	}
	return writeNodeUpdates(cmdCtx, updates)
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
