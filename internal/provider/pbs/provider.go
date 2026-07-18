// Package pbs implements the Proxmox Backup Server provider. It is a
// separate first-class provider with its own typed client; it shares Nodex's
// transport, credential, redaction, and safety infrastructure with the
// Proxmox VE provider but none of its client code.
package pbs

import (
	"context"
	"errors"
	"fmt"

	"github.com/geoffmcc/nodex/internal/credentials"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider"
	"github.com/geoffmcc/nodex/internal/provider/pbs/client"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

const (
	ProviderName    = "pbs"
	ProviderVersion = "0.1.0"
)

func init() {
	provider.Register(ProviderName, func() domain.Provider {
		return &Provider{}
	})
}

// Provider implements domain.Provider for Proxmox Backup Server.
type Provider struct {
	client *client.Client
}

// Name returns "pbs".
func (p *Provider) Name() string { return ProviderName }

// Version returns the provider version.
func (p *Provider) Version() string { return ProviderVersion }

// APIVersion returns the connected PBS API version after Health has queried
// /version.
func (p *Provider) APIVersion() string {
	if p.client == nil || p.client.VersionData() == nil {
		return ""
	}
	return p.client.VersionData().Version
}

// Connect initializes the provider with the given endpoint and credentials.
func (p *Provider) Connect(_ context.Context, endpoint string, creds *domain.Credentials) error {
	return p.ConnectWithOptions(endpoint, creds)
}

// ConnectWithOptions initializes the provider with explicit transport options.
func (p *Provider) ConnectWithOptions(endpoint string, creds *domain.Credentials, opts ...httpclient.Option) error {
	if err := credentials.ValidateCredentials("profile", creds); err != nil {
		return err
	}
	c, err := client.New(endpoint, creds, opts...)
	if err != nil {
		return err
	}
	p.client = c
	return nil
}

// Close releases resources held by the provider.
func (p *Provider) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

// Health returns nil if the provider is connected and the endpoint responds.
func (p *Provider) Health(ctx context.Context) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	_, err := p.client.Version(ctx)
	return err
}

// Capabilities returns the list of capabilities this provider supports.
func (p *Provider) Capabilities() []domain.Capability {
	return []domain.Capability{
		domain.CapabilityPBSSystem,
		domain.CapabilityPBSDatastores,
		domain.CapabilityPBSSnapshots,
		domain.CapabilityPBSTasks,
		domain.CapabilityPBSJobs,
		domain.CapabilityPBSGC,
	}
}

const errNotConnected = "provider not connected: call Connect() first"

// PBSVersionInfo returns the PBS server version.
func (p *Provider) PBSVersionInfo(ctx context.Context) (*domain.PBSVersionInfo, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	v, err := p.client.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	return MapVersion(v), nil
}

// PBSNodeStatus returns the PBS host status.
func (p *Provider) PBSNodeStatus(ctx context.Context) (*domain.PBSNodeStatus, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	s, err := p.client.NodeStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("get node status: %w", err)
	}
	return MapNodeStatus(s), nil
}

// PBSSubscription returns subscription state.
func (p *Provider) PBSSubscription(ctx context.Context) (*domain.PBSSubscription, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	s, err := p.client.Subscription(ctx)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return MapSubscription(s), nil
}

// PBSCertificates returns certificate information.
func (p *Provider) PBSCertificates(ctx context.Context) ([]domain.PBSCertificate, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.Certificates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	return MapCertificates(items), nil
}

// PBSDatastores lists datastore configurations.
func (p *Provider) PBSDatastores(ctx context.Context) ([]domain.PBSDatastore, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.Datastores(ctx)
	if err != nil {
		return nil, fmt.Errorf("list datastores: %w", err)
	}
	return MapDatastores(items), nil
}

// PBSDatastore returns one datastore configuration.
func (p *Provider) PBSDatastore(ctx context.Context, name string) (*domain.PBSDatastore, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	item, err := p.client.Datastore(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get datastore %q: %w", name, err)
	}
	mapped := MapDatastore(*item)
	return &mapped, nil
}

// PBSDatastoreStatus returns datastore usage.
func (p *Provider) PBSDatastoreStatus(ctx context.Context, store string) (*domain.PBSDatastoreStatus, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	s, err := p.client.DatastoreStatus(ctx, store)
	if err != nil {
		return nil, fmt.Errorf("get datastore %q status: %w", store, err)
	}
	return MapDatastoreStatus(store, s), nil
}

// PBSDatastoreUsages returns usage summaries for all datastores.
func (p *Provider) PBSDatastoreUsages(ctx context.Context) ([]domain.PBSDatastoreUsage, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.DatastoreUsages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list datastore usage: %w", err)
	}
	return MapDatastoreUsages(items), nil
}

// PBSSnapshots lists backup snapshots in a datastore.
func (p *Provider) PBSSnapshots(ctx context.Context, store string, filter domain.PBSSnapshotFilter) ([]domain.PBSSnapshot, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.Snapshots(ctx, store, filter)
	if err != nil {
		return nil, fmt.Errorf("list snapshots in %q: %w", store, err)
	}
	return MapSnapshots(store, filter.Namespace, items), nil
}

// PBSTasks lists tasks.
func (p *Provider) PBSTasks(ctx context.Context, filter domain.PBSTaskFilter) ([]domain.PBSTask, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.Tasks(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return MapTasks(items), nil
}

// PBSTaskStatus returns detailed task state.
func (p *Provider) PBSTaskStatus(ctx context.Context, upid string) (*domain.PBSTaskStatus, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	s, err := p.client.TaskStatus(ctx, upid)
	if err != nil {
		return nil, fmt.Errorf("get task status: %w", err)
	}
	return MapTaskStatus(s), nil
}

// PBSTaskLog returns the task log.
func (p *Provider) PBSTaskLog(ctx context.Context, upid string) ([]domain.PBSTaskLogLine, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	lines, err := p.client.TaskLog(ctx, upid)
	if err != nil {
		return nil, fmt.Errorf("get task log: %w", err)
	}
	return MapTaskLog(lines), nil
}

// PBSVerifyJobs lists verification jobs.
func (p *Provider) PBSVerifyJobs(ctx context.Context) ([]domain.PBSVerifyJob, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.VerifyJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list verify jobs: %w", err)
	}
	return MapVerifyJobs(items), nil
}

// PBSPruneJobs lists prune jobs.
func (p *Provider) PBSPruneJobs(ctx context.Context) ([]domain.PBSPruneJob, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.PruneJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list prune jobs: %w", err)
	}
	return MapPruneJobs(items), nil
}

// PBSSyncJobs lists sync jobs.
func (p *Provider) PBSSyncJobs(ctx context.Context) ([]domain.PBSSyncJob, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.SyncJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sync jobs: %w", err)
	}
	return MapSyncJobs(items), nil
}

// PBSGCStatuses returns garbage-collection status for all datastores.
func (p *Provider) PBSGCStatuses(ctx context.Context) ([]domain.PBSGCStatus, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GCStatuses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list garbage-collection status: %w", err)
	}
	return MapGCStatuses(items), nil
}

// PBSGCStatus returns garbage-collection status for one datastore.
func (p *Provider) PBSGCStatus(ctx context.Context, store string) (*domain.PBSGCStatus, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	s, err := p.client.GCStatus(ctx, store)
	if err != nil {
		return nil, fmt.Errorf("get garbage-collection status for %q: %w", store, err)
	}
	mapped := MapGCStatus(*s)
	return &mapped, nil
}
