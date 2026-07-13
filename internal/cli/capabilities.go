package cli

import (
	"fmt"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
)

// requireNodeDetail checks if the provider supports NodeDetailProvider
// and returns an error if not.
func requireNodeDetail(prov domain.Provider) (domain.NodeDetailProvider, error) {
	p, ok := prov.(domain.NodeDetailProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: node detail commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireFirewallAdvanced checks if the provider supports FirewallProvider.
func requireFirewallAdvanced(prov domain.Provider) (domain.FirewallProvider, error) {
	p, ok := prov.(domain.FirewallProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: advanced firewall commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireHAStatus checks if the provider supports HAProvider.
func requireHAStatus(prov domain.Provider) (domain.HAProvider, error) {
	p, ok := prov.(domain.HAProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: HA status commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireBackupContent checks if the provider supports BackupProvider.
func requireBackupContent(prov domain.Provider) (domain.BackupProvider, error) {
	p, ok := prov.(domain.BackupProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: backup content commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireSDN checks if the provider supports SDNProvider.
func requireSDN(prov domain.Provider) (domain.SDNProvider, error) {
	p, ok := prov.(domain.SDNProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: SDN commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireSnapshotDetail checks if the provider supports SnapshotDetailProvider.
func requireSnapshotDetail(prov domain.Provider) (domain.SnapshotDetailProvider, error) {
	p, ok := prov.(domain.SnapshotDetailProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: snapshot detail commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePools checks if the provider supports PoolProvider.
func requirePools(prov domain.Provider) (domain.PoolProvider, error) {
	p, ok := prov.(domain.PoolProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pool commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireClusterLog checks if the provider supports ClusterLogProvider.
func requireClusterLog(prov domain.Provider) (domain.ClusterLogProvider, error) {
	p, ok := prov.(domain.ClusterLogProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: cluster log commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireNetworkMutation checks if the provider supports NetworkMutationProvider.
func requireNetworkMutation(prov domain.Provider) (domain.NetworkMutationProvider, error) {
	p, ok := prov.(domain.NetworkMutationProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: network mutation commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireFirewallMutation checks if the provider supports FirewallMutationProvider.
func requireFirewallMutation(prov domain.Provider) (domain.FirewallMutationProvider, error) {
	p, ok := prov.(domain.FirewallMutationProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: firewall mutation commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireAccess checks if the provider supports AccessProvider.
func requireAccess(prov domain.Provider) (domain.AccessProvider, error) {
	p, ok := prov.(domain.AccessProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: access commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}
