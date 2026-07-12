package proxmox

import (
	"context"
	"fmt"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider"
	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

const (
	ProviderName    = "proxmox"
	ProviderVersion = "0.1.0"
)

func init() {
	provider.Register(ProviderName, func() domain.Provider {
		return &Provider{}
	})
}

// Provider implements domain.Provider for Proxmox VE.
type Provider struct {
	client *client.Client
}

// Name returns "proxmox".
func (p *Provider) Name() string { return ProviderName }

// Version returns the provider version.
func (p *Provider) Version() string { return ProviderVersion }

// Connect initializes the provider with the given endpoint and credentials.
func (p *Provider) Connect(_ context.Context, endpoint string, creds *domain.Credentials) error {
	opts := []httpclient.Option{}
	if creds.Type == "insecure" {
		opts = append(opts, httpclient.WithInsecureTLS())
	}
	p.client = client.New(endpoint, creds, opts...)
	return nil
}

// Close releases resources held by the provider.
func (p *Provider) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

// Capabilities returns the list of capabilities this provider supports.
func (p *Provider) Capabilities() []domain.Capability {
	return []domain.Capability{
		domain.CapabilityNodes,
		domain.CapabilityVMs,
		domain.CapabilityContainers,
		domain.CapabilityStorage,
		domain.CapabilityCluster,
	}
}

const errNotConnected = "provider not connected: call Connect() first"

// Nodes returns all Proxmox nodes.
func (p *Provider) Nodes(ctx context.Context) ([]domain.Node, error) {
	if p.client == nil {
		return nil, fmt.Errorf(errNotConnected)
	}
	items, err := p.client.Nodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	return MapNodes(items), nil
}

// VMs returns all VMs across the cluster.
func (p *Provider) VMs(ctx context.Context) ([]domain.VM, error) {
	if p.client == nil {
		return nil, fmt.Errorf(errNotConnected)
	}
	resources, err := p.client.ClusterResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cluster resources: %w", err)
	}
	var vms []domain.VM
	for _, r := range resources {
		if r.Type == "qemu" {
			vms = append(vms, MapVM(r))
		}
	}
	return vms, nil
}

// Containers returns all containers across the cluster.
func (p *Provider) Containers(ctx context.Context) ([]domain.Container, error) {
	if p.client == nil {
		return nil, fmt.Errorf(errNotConnected)
	}
	resources, err := p.client.ClusterResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cluster resources: %w", err)
	}
	var containers []domain.Container
	for _, r := range resources {
		if r.Type == "lxc" {
			containers = append(containers, MapContainer(r))
		}
	}
	return containers, nil
}

// Storage returns all storage pools across the cluster.
func (p *Provider) Storage(ctx context.Context) ([]domain.Storage, error) {
	if p.client == nil {
		return nil, fmt.Errorf(errNotConnected)
	}
	resources, err := p.client.ClusterResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cluster resources: %w", err)
	}
	var storages []domain.Storage
	for _, r := range resources {
		if r.Type == "storage" {
			storages = append(storages, MapStorage(r))
		}
	}
	return storages, nil
}

// Cluster returns cluster information.
func (p *Provider) Cluster(ctx context.Context) (*domain.Cluster, error) {
	if p.client == nil {
		return nil, fmt.Errorf(errNotConnected)
	}
	version, err := p.client.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	nodes, err := p.client.Nodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	return MapCluster(version, len(nodes)), nil
}

// TestConnectivity checks if the provider can connect to the endpoint.
func (p *Provider) TestConnectivity(ctx context.Context) (*client.VersionData, error) {
	if p.client == nil {
		return nil, fmt.Errorf(errNotConnected)
	}
	version, err := p.client.Version(ctx)
	if err != nil {
		return nil, err
	}
	return version, nil
}
