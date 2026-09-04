package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/safety"
)

func requireClusterAdministration(prov domain.Provider) (domain.ClusterAdministrationProvider, error) {
	p, ok := prov.(domain.ClusterAdministrationProvider)
	if !ok {
		return nil, app.NewExitError(fmt.Errorf("%w: cluster administration is not supported by provider %q", app.ErrUnsupportedCap, prov.Name()), app.ExitUnsupportedCap)
	}
	return p, nil
}

func checkExpertDestructive(cmdCtx *Context, desc, target string) error {
	if !cmdCtx.Opts.Expert {
		return app.NewExitError(fmt.Errorf("%w: cluster administration requires --expert", safety.ErrExpertRequired), app.ExitUsage)
	}
	return checkDestructive(cmdCtx, desc, target)
}

func runClusterInit(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex cluster init <name> <bind-address>"), app.ExitUsage)
	}
	name, bindAddress := args[0], args[1]
	if err := checkExpertDestructive(cmdCtx, fmt.Sprintf("initialize cluster %s on %s", name, bindAddress), name); err != nil {
		return err
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()
	admin, err := requireClusterAdministration(prov)
	if err != nil {
		return err
	}
	upid, err := admin.ClusterInit(ctx, domain.ClusterInitParams{Name: name, BindAddress: bindAddress})
	if err != nil {
		return fmt.Errorf("initialize cluster: %w", err)
	}
	profileName, _ := resolveProfileName(cmdCtx)
	result := output.NewOperationResult("cluster init", prov.Name(), profileName)
	result.Target = name
	result.Safety = safety.TierSecurityAdmin.String()
	result.UPID = upid
	result.Submitted = true
	result.Success = true
	return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result)
}

func runClusterJoin(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex cluster join <node-address> <fingerprint>"), app.ExitUsage)
	}
	nodeAddress, fingerprint := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
	if err := checkExpertDestructive(cmdCtx, fmt.Sprintf("join cluster through %s", nodeAddress), nodeAddress); err != nil {
		return err
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()
	admin, err := requireClusterAdministration(prov)
	if err != nil {
		return err
	}
	_, err = admin.ClusterJoin(ctx, domain.ClusterJoinParams{NodeAddress: nodeAddress, Fingerprint: fingerprint})
	if err == nil {
		return app.NewExitError(fmt.Errorf("cluster join unexpectedly produced no refusal"), app.ExitAmbiguousOutcome)
	}
	return app.NewExitError(err, app.ExitValidationError)
}
