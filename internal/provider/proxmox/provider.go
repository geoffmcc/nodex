package proxmox

import (
	"context"
	"errors"
	"fmt"

	"github.com/geoffmcc/nodex/internal/credentials"
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
		return nil, errors.New(errNotConnected)
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
		return nil, errors.New(errNotConnected)
	}
	resources, err := p.client.ClusterResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cluster resources: %w", err)
	}
	vms := make([]domain.VM, 0)
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
		return nil, errors.New(errNotConnected)
	}
	resources, err := p.client.ClusterResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cluster resources: %w", err)
	}
	containers := make([]domain.Container, 0)
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
		return nil, errors.New(errNotConnected)
	}
	resources, err := p.client.ClusterResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cluster resources: %w", err)
	}
	storages := make([]domain.Storage, 0)
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
		return nil, errors.New(errNotConnected)
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
		return nil, errors.New(errNotConnected)
	}
	version, err := p.client.Version(ctx)
	if err != nil {
		return nil, err
	}
	return version, nil
}

// VMConfig returns configuration for a specific VM.
func (p *Provider) VMConfig(ctx context.Context, node string, vmid int) (map[string]interface{}, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	config, err := p.client.GetVMConfig(ctx, node, vmid)
	if err != nil {
		return nil, fmt.Errorf("get vm config: %w", err)
	}
	return vmConfigToMap(config), nil
}

// ContainerConfig returns configuration for a specific container.
func (p *Provider) ContainerConfig(ctx context.Context, node string, vmid int) (map[string]interface{}, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	config, err := p.client.GetContainerConfig(ctx, node, vmid)
	if err != nil {
		return nil, fmt.Errorf("get container config: %w", err)
	}
	return containerConfigToMap(config), nil
}

func vmConfigToMap(c *client.VMConfigData) map[string]interface{} {
	m := map[string]interface{}{
		"vmid": c.VMID,
	}
	if c.Name != "" {
		m["name"] = c.Name
	}
	if c.CPU > 0 {
		m["cores"] = c.CPU
	}
	if c.Memory > 0 {
		m["memory"] = c.Memory
	}
	if c.Net0 != "" {
		m["net0"] = c.Net0
	}
	if c.Scsi0 != "" {
		m["scsi0"] = c.Scsi0
	}
	if c.Boot != "" {
		m["boot"] = c.Boot
	}
	if c.OnBoot != 0 {
		m["onboot"] = c.OnBoot
	}
	if c.Agent != 0 {
		m["agent"] = c.Agent
	}
	if c.OSType != "" {
		m["ostype"] = c.OSType
	}
	if c.Description != "" {
		m["description"] = c.Description
	}
	if c.Protection != 0 {
		m["protection"] = c.Protection
	}
	if c.Tags != "" {
		m["tags"] = c.Tags
	}
	if c.ScsiHW != "" {
		m["scsihw"] = c.ScsiHW
	}
	if c.Bios != "" {
		m["bios"] = c.Bios
	}
	if c.IDE2 != "" {
		m["ide2"] = c.IDE2
	}
	if c.Args != "" {
		m["args"] = c.Args
	}
	if c.VMGenID != "" {
		m["vmgenid"] = c.VMGenID
	}
	if c.SMBIOS1 != "" {
		m["smbios1"] = c.SMBIOS1
	}
	if c.Numa != 0 {
		m["numa"] = c.Numa
	}
	return m
}

func containerConfigToMap(c *client.ContainerConfigData) map[string]interface{} {
	m := map[string]interface{}{
		"vmid": c.VMID,
	}
	if c.Hostname != "" {
		m["hostname"] = c.Hostname
	}
	if c.CPU > 0 {
		m["cores"] = c.CPU
	}
	if c.Memory > 0 {
		m["memory"] = c.Memory
	}
	if c.Swap > 0 {
		m["swap"] = c.Swap
	}
	if c.Rootfs != "" {
		m["rootfs"] = c.Rootfs
	}
	if c.MP0 != "" {
		m["mp0"] = c.MP0
	}
	if c.Net0 != "" {
		m["net0"] = c.Net0
	}
	if c.OnBoot != 0 {
		m["onboot"] = c.OnBoot
	}
	if c.OSType != "" {
		m["ostype"] = c.OSType
	}
	if c.Description != "" {
		m["description"] = c.Description
	}
	if c.Protection != 0 {
		m["protection"] = c.Protection
	}
	if c.Tags != "" {
		m["tags"] = c.Tags
	}
	if c.Features != "" {
		m["features"] = c.Features
	}
	if c.Architecture != "" {
		m["architecture"] = c.Architecture
	}
	if c.Nameserver != "" {
		m["nameserver"] = c.Nameserver
	}
	if c.SearchDomain != "" {
		m["searchdomain"] = c.SearchDomain
	}
	if c.Fstab != "" {
		m["fstab"] = c.Fstab
	}
	if c.Hookscript != "" {
		m["hookscript"] = c.Hookscript
	}
	return m
}

// StorageContent returns content items for a specific storage.
func (p *Provider) StorageContent(ctx context.Context, node, storage string) ([]domain.StorageContentItem, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetStorageContent(ctx, node, storage)
	if err != nil {
		return nil, fmt.Errorf("get storage content: %w", err)
	}
	result := make([]domain.StorageContentItem, 0, len(items))
	for _, item := range items {
		result = append(result, domain.StorageContentItem{
			Content: item.Content,
			Ctime:   item.Ctime,
			Format:  item.Format,
			Volid:   item.Volid,
			Size:    item.Size,
			Subtype: item.Subtype,
			VMID:    item.VMID,
		})
	}
	return result, nil
}
