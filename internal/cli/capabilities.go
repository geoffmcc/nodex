package cli

import (
	"fmt"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
)

// --- Core inspection helpers ---

// requireNodeInspector asserts the provider has NodeInspector.
func requireNodeInspector(prov domain.Provider) (domain.NodeInspector, error) {
	p, ok := prov.(domain.NodeInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: node listing not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireVMInspector asserts the provider has VMInspector.
func requireVMInspector(prov domain.Provider) (domain.VMInspector, error) {
	p, ok := prov.(domain.VMInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: VM listing not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireContainerInspector asserts the provider has ContainerInspector.
func requireContainerInspector(prov domain.Provider) (domain.ContainerInspector, error) {
	p, ok := prov.(domain.ContainerInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: container listing not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireStorageInspector asserts the provider has StorageInspector.
func requireStorageInspector(prov domain.Provider) (domain.StorageInspector, error) {
	p, ok := prov.(domain.StorageInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: storage listing not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireClusterInspector asserts the provider has ClusterInspector.
func requireClusterInspector(prov domain.Provider) (domain.ClusterInspector, error) {
	p, ok := prov.(domain.ClusterInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: cluster info not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireTaskInspector asserts the provider has TaskInspector.
func requireTaskInspector(prov domain.Provider) (domain.TaskInspector, error) {
	p, ok := prov.(domain.TaskInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: task operations not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireSnapshotInspector asserts the provider has SnapshotInspector.
func requireSnapshotInspector(prov domain.Provider) (domain.SnapshotInspector, error) {
	p, ok := prov.(domain.SnapshotInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: snapshot listing not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireEventInspector asserts the provider has EventInspector.
func requireEventInspector(prov domain.Provider) (domain.EventInspector, error) {
	p, ok := prov.(domain.EventInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: event listing not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireSyslogInspector asserts the provider has SyslogInspector.
func requireSyslogInspector(prov domain.Provider) (domain.SyslogInspector, error) {
	p, ok := prov.(domain.SyslogInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: syslog not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireBackupInspector asserts the provider has BackupInspector.
func requireBackupInspector(prov domain.Provider) (domain.BackupInspector, error) {
	p, ok := prov.(domain.BackupInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: backup listing not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireFirewallInspector asserts the provider has FirewallInspector.
func requireFirewallInspector(prov domain.Provider) (domain.FirewallInspector, error) {
	p, ok := prov.(domain.FirewallInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: firewall rule listing not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireHAInspector asserts the provider has HAInspector.
func requireHAInspector(prov domain.Provider) (domain.HAInspector, error) {
	p, ok := prov.(domain.HAInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: HA resource listing not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// --- Optional capability helpers ---

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

// requireCeph checks if the provider supports CephProvider.
func requireCeph(prov domain.Provider) (domain.CephProvider, error) {
	p, ok := prov.(domain.CephProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: ceph commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireCephMutation checks if the provider supports CephMutationProvider.
func requireCephMutation(prov domain.Provider) (domain.CephMutationProvider, error) {
	p, ok := prov.(domain.CephMutationProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: ceph mutation commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireSDNMutation checks if the provider supports SDNMutationProvider.
func requireSDNMutation(prov domain.Provider) (domain.SDNMutationProvider, error) {
	p, ok := prov.(domain.SDNMutationProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: SDN mutation commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireReplication checks if the provider supports ReplicationProvider.
func requireReplication(prov domain.Provider) (domain.ReplicationProvider, error) {
	p, ok := prov.(domain.ReplicationProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: replication commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePBSSystem checks if the provider supports PBSSystemInspector.
func requirePBSSystem(prov domain.Provider) (domain.PBSSystemInspector, error) {
	p, ok := prov.(domain.PBSSystemInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pbs commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePBSDatastores checks if the provider supports PBSDatastoreInspector.
func requirePBSDatastores(prov domain.Provider) (domain.PBSDatastoreInspector, error) {
	p, ok := prov.(domain.PBSDatastoreInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pbs datastore commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePBSSnapshots checks if the provider supports PBSSnapshotInspector.
func requirePBSSnapshots(prov domain.Provider) (domain.PBSSnapshotInspector, error) {
	p, ok := prov.(domain.PBSSnapshotInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pbs snapshot commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePBSTasks checks if the provider supports PBSTaskInspector.
func requirePBSTasks(prov domain.Provider) (domain.PBSTaskInspector, error) {
	p, ok := prov.(domain.PBSTaskInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pbs task commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePBSJobs checks if the provider supports PBSJobInspector.
func requirePBSJobs(prov domain.Provider) (domain.PBSJobInspector, error) {
	p, ok := prov.(domain.PBSJobInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pbs job commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePBSGC checks if the provider supports PBSGCInspector.
func requirePBSGC(prov domain.Provider) (domain.PBSGCInspector, error) {
	p, ok := prov.(domain.PBSGCInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pbs garbage-collection commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePBSVerifyRun checks if the provider supports PBSVerifyRunner.
func requirePBSVerifyRun(prov domain.Provider) (domain.PBSVerifyRunner, error) {
	p, ok := prov.(domain.PBSVerifyRunner)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pbs verify run not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePBSSyncRun checks if the provider supports PBSSyncRunner.
func requirePBSSyncRun(prov domain.Provider) (domain.PBSSyncRunner, error) {
	p, ok := prov.(domain.PBSSyncRunner)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pbs sync run not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePBSPruneRun checks if the provider supports PBSPruneRunner.
func requirePBSPruneRun(prov domain.Provider) (domain.PBSPruneRunner, error) {
	p, ok := prov.(domain.PBSPruneRunner)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pbs prune run not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requirePBSGCRun checks if the provider supports PBSGCRunner.
func requirePBSGCRun(prov domain.Provider) (domain.PBSGCRunner, error) {
	p, ok := prov.(domain.PBSGCRunner)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: pbs garbage-collection run not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}
