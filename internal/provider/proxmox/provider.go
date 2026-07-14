package proxmox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"

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
		domain.CapabilityNodeDetail,
		domain.CapabilityFirewallAdvanced,
		domain.CapabilityHAStatus,
		domain.CapabilityBackupContent,
		domain.CapabilitySDN,
		domain.CapabilitySnapshotDetail,
		domain.CapabilityPools,
		domain.CapabilityClusterLog,
		domain.CapabilityLifecycle,
		domain.CapabilityConfig,
		domain.CapabilitySnapshotMutation,
		domain.CapabilityDelete,
		domain.CapabilityTemplate,
		domain.CapabilityCloudInit,
		domain.CapabilityBackupMutation,
		domain.CapabilityStorageMutation,
		domain.CapabilityMigration,
		domain.CapabilityClone,
		domain.CapabilityDisk,
		domain.CapabilityNetworkMutation,
		domain.CapabilityFirewallMutation,
		domain.CapabilityAccess,
		domain.CapabilityCeph,
		domain.CapabilityCephMutation,
		domain.CapabilitySDNMutation,
		domain.CapabilityReplication,
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

// Tasks returns all tasks for a specific node.
func (p *Provider) Tasks(ctx context.Context, node string) ([]domain.Task, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetTasks(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get tasks: %w", err)
	}
	result := make([]domain.Task, 0, len(items))
	for _, item := range items {
		result = append(result, domain.Task{
			UPID:      item.UPID,
			Type:      item.Type,
			State:     item.State,
			StartTime: item.StartTime,
			EndTime:   item.EndTime,
			Status:    item.Status,
			Node:      node,
		})
	}
	return result, nil
}

// Task returns details for a specific task by UPID.
func (p *Provider) Task(ctx context.Context, node, upid string) (*domain.Task, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	item, err := p.client.GetTask(ctx, node, upid)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	return &domain.Task{
		UPID:      item.UPID,
		Type:      item.Type,
		State:     item.State,
		StartTime: item.StartTime,
		EndTime:   item.EndTime,
		Status:    item.Status,
		Node:      node,
	}, nil
}

// VMSnapshots returns snapshots for a VM.
func (p *Provider) VMSnapshots(ctx context.Context, node string, vmid int) ([]domain.Snapshot, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetVMSnapshots(ctx, node, vmid)
	if err != nil {
		return nil, fmt.Errorf("get vm snapshots: %w", err)
	}
	result := make([]domain.Snapshot, 0, len(items))
	for _, item := range items {
		result = append(result, domain.Snapshot{
			Name:   item.Name,
			VMID:   item.VMID,
			Ctime:  item.Ctime,
			Parent: item.Parent,
			Node:   node,
			Target: fmt.Sprintf("%s/%d", node, vmid),
		})
	}
	return result, nil
}

// ContainerSnapshots returns snapshots for a container.
func (p *Provider) ContainerSnapshots(ctx context.Context, node string, vmid int) ([]domain.Snapshot, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetContainerSnapshots(ctx, node, vmid)
	if err != nil {
		return nil, fmt.Errorf("get container snapshots: %w", err)
	}
	result := make([]domain.Snapshot, 0, len(items))
	for _, item := range items {
		result = append(result, domain.Snapshot{
			Name:   item.Name,
			VMID:   item.VMID,
			Ctime:  item.Ctime,
			Parent: item.Parent,
			Node:   node,
			Target: fmt.Sprintf("%s/%d", node, vmid),
		})
	}
	return result, nil
}

// Events returns cluster events.
func (p *Provider) Events(ctx context.Context) ([]domain.Event, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("get events: %w", err)
	}
	result := make([]domain.Event, 0, len(items))
	for _, item := range items {
		result = append(result, domain.Event{
			Type:    item.Type,
			Time:    item.Time,
			Node:    item.Node,
			ID:      item.ID,
			Message: item.Message,
		})
	}
	return result, nil
}

// Syslog returns syslog entries for a specific node.
func (p *Provider) Syslog(ctx context.Context, node string) ([]domain.SyslogEntry, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetSyslog(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get syslog: %w", err)
	}
	result := make([]domain.SyslogEntry, 0, len(items))
	for _, item := range items {
		result = append(result, domain.SyslogEntry{
			Time:    item.Time,
			Node:    item.Node,
			Level:   item.SyslogLevel,
			Message: item.Message,
		})
	}
	return result, nil
}

// Backups returns backup tasks for a specific node.
func (p *Provider) Backups(ctx context.Context, node string) ([]domain.Backup, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetBackupStatus(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get backup status: %w", err)
	}
	result := make([]domain.Backup, 0, len(items))
	for _, item := range items {
		result = append(result, domain.Backup{
			UPID:      item.UPID,
			Type:      item.Type,
			State:     item.State,
			StartTime: item.StartTime,
			EndTime:   item.EndTime,
			Status:    item.Status,
			Node:      item.Node,
			Storage:   item.Storage,
		})
	}
	return result, nil
}

// FirewallRules returns cluster firewall rules.
func (p *Provider) FirewallRules(ctx context.Context) ([]domain.FirewallRule, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetFirewallRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("get firewall rules: %w", err)
	}
	result := make([]domain.FirewallRule, 0, len(items))
	for _, item := range items {
		result = append(result, domain.FirewallRule{
			Type:     item.Type,
			Action:   item.Action,
			Enable:   item.Enable,
			Pos:      item.Pos,
			Proto:    item.Proto,
			Dest:     item.Dest,
			Dport:    item.Dport,
			Source:   item.Source,
			Sport:    item.Sport,
			ICMPType: item.ICMPType,
			Log:      item.Log,
			Comment:  item.Comment,
		})
	}
	return result, nil
}

// HAResources returns HA resources.
func (p *Provider) HAResources(ctx context.Context) ([]domain.HAResource, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetHAResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("get HA resources: %w", err)
	}
	result := make([]domain.HAResource, 0, len(items))
	for _, item := range items {
		result = append(result, domain.HAResource{
			ID:       item.ID,
			Type:     item.Type,
			State:    item.State,
			Node:     item.Node,
			Group:    item.Group,
			MaxRelay: item.MaxRelay,
		})
	}
	return result, nil
}

// HAGroups returns HA groups.
func (p *Provider) HAGroups(ctx context.Context) ([]domain.HAGroup, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetHAGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("get HA groups: %w", err)
	}
	result := make([]domain.HAGroup, 0, len(items))
	for _, item := range items {
		result = append(result, domain.HAGroup{
			ID:         item.ID,
			Type:       item.Type,
			Nodes:      item.Nodes,
			Comment:    item.Comment,
			NoFailback: item.NoFailback,
		})
	}
	return result, nil
}

// NodeStatus returns detailed status for a specific node.
func (p *Provider) NodeStatus(ctx context.Context, node string) (map[string]interface{}, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	status, err := p.client.GetNodeStatus(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node status: %w", err)
	}
	m := map[string]interface{}{
		"cpu":     status.CPU,
		"maxcpu":  status.MaxCPU,
		"mem":     status.Mem,
		"maxmem":  status.MaxMem,
		"disk":    status.Disk,
		"maxdisk": status.MaxDisk,
		"uptime":  status.Uptime,
		"level":   status.Level,
		"id":      status.ID,
		"node":    status.Node,
		"type":    status.Type,
		"status":  status.Status,
	}
	if status.KVersion != "" {
		m["kversion"] = status.KVersion
	}
	if status.PVEVersion != "" {
		m["pveversion"] = status.PVEVersion
	}
	if len(status.LoadAvg) > 0 {
		m["loadavg"] = status.LoadAvg
	}
	return m, nil
}

// NodeServices returns services on a specific node.
func (p *Provider) NodeServices(ctx context.Context, node string) ([]domain.NodeService, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetNodeServices(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node services: %w", err)
	}
	result := make([]domain.NodeService, 0, len(items))
	for _, item := range items {
		result = append(result, domain.NodeService{
			Name:   item.Name,
			State:  item.State,
			Active: item.Active,
		})
	}
	return result, nil
}

// NodeNetwork returns network interfaces on a specific node.
func (p *Provider) NodeNetwork(ctx context.Context, node string) ([]domain.NodeNetwork, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetNodeNetwork(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node network: %w", err)
	}
	result := make([]domain.NodeNetwork, 0, len(items))
	for _, item := range items {
		result = append(result, domain.NodeNetwork{
			Name:   item.Name,
			Type:   item.Type,
			Status: item.Status,
			IP:     item.IP,
			MAC:    item.MAC,
		})
	}
	return result, nil
}

// NodeDNS returns DNS configuration for a specific node.
func (p *Provider) NodeDNS(ctx context.Context, node string) (*domain.NodeDNS, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	data, err := p.client.GetNodeDNS(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node DNS: %w", err)
	}
	return &domain.NodeDNS{
		DNS1:         data.DNS1,
		DNS2:         data.DNS2,
		SearchDomain: data.SearchDomain,
	}, nil
}

// NodeTime returns time configuration for a specific node.
func (p *Provider) NodeTime(ctx context.Context, node string) (*domain.NodeTime, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	data, err := p.client.GetNodeTime(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node time: %w", err)
	}
	return &domain.NodeTime{
		TimeZone: data.TimeZone,
		Epoch:    data.Epoch,
		Local:    data.Local,
	}, nil
}

// NodeDisks returns disk inventory for a specific node.
func (p *Provider) NodeDisks(ctx context.Context, node string) ([]domain.NodeDisk, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetNodeDisks(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node disks: %w", err)
	}
	result := make([]domain.NodeDisk, 0, len(items))
	for _, item := range items {
		result = append(result, domain.NodeDisk{
			Name:   item.Name,
			Path:   item.Path,
			Size:   item.Size,
			Type:   item.Type,
			Model:  item.Model,
			Health: item.Health,
		})
	}
	return result, nil
}

// NodeCertificates returns TLS certificates for a specific node.
func (p *Provider) NodeCertificates(ctx context.Context, node string) ([]domain.NodeCertificate, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetNodeCertificates(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node certificates: %w", err)
	}
	result := make([]domain.NodeCertificate, 0, len(items))
	for _, item := range items {
		result = append(result, domain.NodeCertificate{
			Fingerprint: item.Fingerprint,
			Subject:     item.Subject,
			Issuer:      item.Issuer,
			NotBefore:   item.NotBefore,
			NotAfter:    item.NotAfter,
		})
	}
	return result, nil
}

// NodeSubscription returns subscription status for a specific node.
func (p *Provider) NodeSubscription(ctx context.Context, node string) (*domain.NodeSubscription, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	data, err := p.client.GetNodeSubscription(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node subscription: %w", err)
	}
	return &domain.NodeSubscription{
		Status:  data.Status,
		Key:     data.Key,
		Expires: data.Expires,
	}, nil
}

// NodeUpdates returns available updates for a specific node.
func (p *Provider) NodeUpdates(ctx context.Context, node string) ([]domain.NodeUpdate, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetNodeUpdates(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node updates: %w", err)
	}
	result := make([]domain.NodeUpdate, 0, len(items))
	for _, item := range items {
		result = append(result, domain.NodeUpdate{
			Package: item.Package,
			Version: item.Version,
		})
	}
	return result, nil
}

// FirewallAliases returns cluster firewall aliases.
func (p *Provider) FirewallAliases(ctx context.Context) ([]domain.FirewallAlias, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetFirewallAliases(ctx)
	if err != nil {
		return nil, fmt.Errorf("get firewall aliases: %w", err)
	}
	result := make([]domain.FirewallAlias, 0, len(items))
	for _, item := range items {
		result = append(result, domain.FirewallAlias{
			Name:    item.Name,
			CIDR:    item.CIDR,
			Comment: item.Comment,
		})
	}
	return result, nil
}

// FirewallIPSet returns entries for a specific IP set.
func (p *Provider) FirewallIPSet(ctx context.Context, name string) ([]domain.FirewallIPSetEntry, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetFirewallIPSetEntries(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get firewall IP set entries: %w", err)
	}
	result := make([]domain.FirewallIPSetEntry, 0, len(items))
	for _, item := range items {
		result = append(result, domain.FirewallIPSetEntry{
			CIDR:    item.CIDR,
			Comment: item.Comment,
		})
	}
	return result, nil
}

// FirewallIPSets returns cluster firewall IP sets.
func (p *Provider) FirewallIPSets(ctx context.Context) ([]domain.FirewallIPSet, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetFirewallIPSets(ctx)
	if err != nil {
		return nil, fmt.Errorf("get firewall IP sets: %w", err)
	}
	result := make([]domain.FirewallIPSet, 0, len(items))
	for _, item := range items {
		result = append(result, domain.FirewallIPSet{
			Name:    item.Name,
			Comment: item.Comment,
		})
	}
	return result, nil
}

// FirewallSecurityGroups returns cluster firewall security groups.
func (p *Provider) FirewallSecurityGroups(ctx context.Context) ([]domain.FirewallSecurityGroup, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetFirewallSecurityGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("get firewall security groups: %w", err)
	}
	result := make([]domain.FirewallSecurityGroup, 0, len(items))
	for _, item := range items {
		group := domain.FirewallSecurityGroup{
			Name:    item.Name,
			Comment: item.Comment,
			Rules:   make([]domain.FirewallRule, 0, len(item.Rules)),
		}
		for _, r := range item.Rules {
			group.Rules = append(group.Rules, domain.FirewallRule{
				Type:     r.Type,
				Action:   r.Action,
				Enable:   r.Enable,
				Pos:      r.Pos,
				Proto:    r.Proto,
				Dest:     r.Dest,
				Dport:    r.Dport,
				Source:   r.Source,
				Sport:    r.Sport,
				ICMPType: r.ICMPType,
				Log:      r.Log,
				Comment:  r.Comment,
			})
		}
		result = append(result, group)
	}
	return result, nil
}

// FirewallOptions returns cluster firewall options.
func (p *Provider) FirewallOptions(ctx context.Context) (*domain.FirewallOptions, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	data, err := p.client.GetFirewallOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("get firewall options: %w", err)
	}
	return &domain.FirewallOptions{
		Enable: data.Enable,
		Log:    data.Log,
	}, nil
}

// NodeFirewallRules returns firewall rules for a specific node.
func (p *Provider) NodeFirewallRules(ctx context.Context, node string) ([]domain.FirewallRule, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetNodeFirewallRules(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get node firewall rules: %w", err)
	}
	result := make([]domain.FirewallRule, 0, len(items))
	for _, item := range items {
		result = append(result, domain.FirewallRule{
			Type:     item.Type,
			Action:   item.Action,
			Enable:   item.Enable,
			Pos:      item.Pos,
			Proto:    item.Proto,
			Dest:     item.Dest,
			Dport:    item.Dport,
			Source:   item.Source,
			Sport:    item.Sport,
			ICMPType: item.ICMPType,
			Log:      item.Log,
			Comment:  item.Comment,
		})
	}
	return result, nil
}

// VMFirewallRules returns firewall rules for a specific VM.
func (p *Provider) VMFirewallRules(ctx context.Context, node string, vmid int) ([]domain.FirewallRule, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetVMFirewallRules(ctx, node, vmid)
	if err != nil {
		return nil, fmt.Errorf("get VM firewall rules: %w", err)
	}
	result := make([]domain.FirewallRule, 0, len(items))
	for _, item := range items {
		result = append(result, domain.FirewallRule{
			Type:     item.Type,
			Action:   item.Action,
			Enable:   item.Enable,
			Pos:      item.Pos,
			Proto:    item.Proto,
			Dest:     item.Dest,
			Dport:    item.Dport,
			Source:   item.Source,
			Sport:    item.Sport,
			ICMPType: item.ICMPType,
			Log:      item.Log,
			Comment:  item.Comment,
		})
	}
	return result, nil
}

// HAStatus returns cluster HA status.
func (p *Provider) HAStatus(ctx context.Context) (*domain.HAStatus, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	data, err := p.client.GetHAStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("get HA status: %w", err)
	}
	return &domain.HAStatus{
		Quorum: data.Quorum,
		Status: data.Status,
	}, nil
}

// HACurrent returns current HA resource states.
func (p *Provider) HACurrent(ctx context.Context) ([]domain.HACurrent, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetHACurrent(ctx)
	if err != nil {
		return nil, fmt.Errorf("get HA current: %w", err)
	}
	result := make([]domain.HACurrent, 0, len(items))
	for _, item := range items {
		result = append(result, domain.HACurrent{
			ID:     item.ID,
			Type:   item.Type,
			State:  item.State,
			Node:   item.Node,
			Status: item.Status,
		})
	}
	return result, nil
}

// BackupContent returns content items for backup on a specific storage.
func (p *Provider) BackupContent(ctx context.Context, node, storage string) ([]domain.BackupContentItem, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetStorageContent(ctx, node, storage)
	if err != nil {
		return nil, fmt.Errorf("get backup content: %w", err)
	}
	result := make([]domain.BackupContentItem, 0, len(items))
	for _, item := range items {
		result = append(result, domain.BackupContentItem{
			Content: item.Content,
			Volid:   item.Volid,
			Size:    item.Size,
			Format:  item.Format,
		})
	}
	return result, nil
}

// SDNZones returns SDN zones.
func (p *Provider) SDNZones(ctx context.Context) ([]domain.SDNZone, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetSDNZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("get SDN zones: %w", err)
	}
	result := make([]domain.SDNZone, 0, len(items))
	for _, item := range items {
		result = append(result, domain.SDNZone{
			Name:   item.Name,
			Type:   item.Type,
			Status: item.Status,
			VNets:  item.VNets,
		})
	}
	return result, nil
}

// SDNVNets returns SDN virtual networks.
func (p *Provider) SDNVNets(ctx context.Context) ([]domain.SDNVNet, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetSDNVNets(ctx)
	if err != nil {
		return nil, fmt.Errorf("get SDN vnets: %w", err)
	}
	result := make([]domain.SDNVNet, 0, len(items))
	for _, item := range items {
		result = append(result, domain.SDNVNet{
			Name:  item.Name,
			Zone:  item.Zone,
			VLAN:  item.VLAN,
			Alias: item.Alias,
		})
	}
	return result, nil
}

// --- Phase 6: Ceph, SDN Mutation, Replication ---

// CephStatus returns Ceph cluster health status.
func (p *Provider) CephStatus(ctx context.Context, node string) (*domain.CephStatus, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	health, err := p.client.GetCephStatus(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get ceph status: %w", err)
	}
	return &domain.CephStatus{Health: health}, nil
}

// CephOSDs returns Ceph OSD inventory.
func (p *Provider) CephOSDs(ctx context.Context, node string) ([]domain.CephOSD, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	resp, err := p.client.GetCephOSDs(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get ceph osds: %w", err)
	}
	return flattenOSDs(resp.Data.Root.Children), nil
}

// flattenOSDs recursively extracts OSD leaves from the tree.
func flattenOSDs(nodes []client.CephOSDTreeNode) []domain.CephOSD {
	var result []domain.CephOSD
	for _, n := range nodes {
		if n.Leaf == 1 && n.Type == "osd" {
			result = append(result, domain.CephOSD{
				ID:          n.ID,
				Name:        n.Name,
				Type:        n.Type,
				Status:      n.Status,
				In:          n.In,
				Host:        n.Host,
				DeviceClass: n.DeviceClass,
				TotalSpace:  n.TotalSpace,
				BytesUsed:   n.BytesUsed,
				PercentUsed: n.PercentUsed,
			})
		}
		if n.Children != nil {
			for i := range n.Children {
				n.Children[i].Host = n.Name
			}
			result = append(result, flattenOSDs(n.Children)...)
		}
	}
	return result
}

// CephMONs returns Ceph monitor status.
func (p *Provider) CephMONs(ctx context.Context, node string) ([]domain.CephMON, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetCephMONs(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get ceph mons: %w", err)
	}
	result := make([]domain.CephMON, 0, len(items))
	for _, item := range items {
		result = append(result, domain.CephMON{
			Name:    item.Name,
			Host:    item.Host,
			Quorum:  item.Quorum != 0,
			State:   item.State,
			Rank:    item.Rank,
			Version: item.CephVersionShort,
		})
	}
	return result, nil
}

// CephPools returns Ceph pool listing.
func (p *Provider) CephPools(ctx context.Context, node string) ([]domain.CephPool, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetCephPools(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get ceph pools: %w", err)
	}
	result := make([]domain.CephPool, 0, len(items))
	for _, item := range items {
		result = append(result, domain.CephPool{
			ID:              item.Pool,
			Name:            item.PoolName,
			Size:            item.Size,
			MinSize:         item.MinSize,
			PGNum:           item.PGNum,
			CrushRule:       item.CrushRule,
			CrushRuleName:   item.CrushRuleName,
			Type:            item.Type,
			PGNumFinal:      item.PGNumFinal,
			PercentUsed:     item.PercentUsed,
			BytesUsed:       item.BytesUsed,
			PGAutoscaleMode: item.PGAutoscaleMode,
		})
	}
	return result, nil
}

// CephCreateOSD creates a new Ceph OSD and returns the task UPID.
func (p *Provider) CephCreateOSD(ctx context.Context, node, dev string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CreateOSD(ctx, node, dev)
}

// CephOSDOut marks an OSD as out.
func (p *Provider) CephOSDOut(ctx context.Context, node string, osdid int) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.OSDOut(ctx, node, osdid)
}

// CephOSDIn marks an OSD as in.
func (p *Provider) CephOSDIn(ctx context.Context, node string, osdid int) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.OSDIn(ctx, node, osdid)
}

// CephDestroyOSD destroys a Ceph OSD and returns the task UPID.
func (p *Provider) CephDestroyOSD(ctx context.Context, node string, osdid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.DestroyOSD(ctx, node, osdid)
}

// CephCreatePool creates a new Ceph pool and returns the task UPID.
func (p *Provider) CephCreatePool(ctx context.Context, node, name string, params map[string]string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	body := url.Values{}
	body.Set("name", name)
	for k, v := range params {
		body.Set(k, v)
	}
	return p.client.CreatePool(ctx, node, body)
}

// CephDestroyPool destroys a Ceph pool and returns the task UPID.
func (p *Provider) CephDestroyPool(ctx context.Context, node, name string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.DestroyPool(ctx, node, name)
}

// --- SDN Mutation ---

// SDNCreateZone creates an SDN zone.
func (p *Provider) SDNCreateZone(ctx context.Context, zoneType, zone string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.CreateSDNZone(ctx, zoneType, zone)
}

// SDNDeleteZone deletes an SDN zone.
func (p *Provider) SDNDeleteZone(ctx context.Context, zone string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteSDNZone(ctx, zone)
}

// SDNCreateVNet creates an SDN virtual network.
func (p *Provider) SDNCreateVNet(ctx context.Context, vnet, zone string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.CreateSDNVNet(ctx, vnet, zone)
}

// SDNDeleteVNet deletes an SDN virtual network.
func (p *Provider) SDNDeleteVNet(ctx context.Context, vnet string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteSDNVNet(ctx, vnet)
}

// SDNCreateSubnet creates an SDN subnet.
func (p *Provider) SDNCreateSubnet(ctx context.Context, vnet, cidr, gateway string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.CreateSDNSubnet(ctx, vnet, cidr, gateway)
}

// SDNDeleteSubnet deletes an SDN subnet.
func (p *Provider) SDNDeleteSubnet(ctx context.Context, vnet, subnet string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteSDNSubnet(ctx, vnet, subnet)
}

// SDNCreateController creates an SDN controller.
func (p *Provider) SDNCreateController(ctx context.Context, ctrl string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.CreateSDNController(ctx, ctrl)
}

// SDNDeleteController deletes an SDN controller.
func (p *Provider) SDNDeleteController(ctx context.Context, ctrl string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteSDNController(ctx, ctrl)
}

// --- Replication ---

// ReplicationList returns all replication jobs.
func (p *Provider) ReplicationList(ctx context.Context) ([]domain.ReplicationJob, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetReplicationJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("get replication jobs: %w", err)
	}
	result := make([]domain.ReplicationJob, 0, len(items))
	for _, item := range items {
		result = append(result, domain.ReplicationJob{
			ID:        item.ID,
			Guest:     item.Guest,
			Type:      item.Type,
			Source:    item.Source,
			Target:    item.Target,
			Schedule:  item.Schedule,
			Comment:   item.Comment,
			Enabled:   1 - item.Disable,
			Rate:      item.Rate,
			JobNum:    item.JobNum,
			LastSync:  item.LastSync,
			FailCount: item.FailCount,
		})
	}
	return result, nil
}

// ReplicationGet returns a single replication job.
func (p *Provider) ReplicationGet(ctx context.Context, id string) (*domain.ReplicationJob, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	item, err := p.client.GetReplicationJob(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get replication job: %w", err)
	}
	return &domain.ReplicationJob{
		ID:        item.ID,
		Guest:     item.Guest,
		Type:      item.Type,
		Source:    item.Source,
		Target:    item.Target,
		Schedule:  item.Schedule,
		Comment:   item.Comment,
		Enabled:   1 - item.Disable,
		Rate:      item.Rate,
		JobNum:    item.JobNum,
		LastSync:  item.LastSync,
		FailCount: item.FailCount,
	}, nil
}

// ReplicationCreate creates a new replication job.
func (p *Provider) ReplicationCreate(ctx context.Context, params domain.ReplicationCreateInput) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.CreateReplication(ctx, client.ReplicationCreateRequest{
		ID:       params.ID,
		Guest:    params.Guest,
		Type:     params.Type,
		Target:   params.Target,
		Schedule: params.Schedule,
		Comment:  params.Comment,
		Rate:     params.Rate,
		Source:   params.Source,
	})
}

// ReplicationUpdate updates a replication job.
func (p *Provider) ReplicationUpdate(ctx context.Context, id string, params domain.ReplicationUpdateInput) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.UpdateReplication(ctx, id, client.ReplicationUpdateRequest{
		Target:   params.Target,
		Schedule: params.Schedule,
		Comment:  params.Comment,
		Rate:     params.Rate,
		Source:   params.Source,
		Delete:   params.Delete,
	})
}

// ReplicationDelete deletes a replication job.
func (p *Provider) ReplicationDelete(ctx context.Context, id string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteReplication(ctx, id)
}

// ReplicationSchedule schedules a replication job to run now.
func (p *Provider) ReplicationSchedule(ctx context.Context, node, id string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.ScheduleReplication(ctx, node, id)
}

// VMSnapshotConfig returns configuration for a specific VM snapshot.
func (p *Provider) VMSnapshotConfig(ctx context.Context, node string, vmid int, name string) (map[string]interface{}, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	config, err := p.client.GetVMSnapshotConfig(ctx, node, vmid, name)
	if err != nil {
		return nil, fmt.Errorf("get VM snapshot config: %w", err)
	}
	return config, nil
}

// ContainerSnapshotConfig returns configuration for a specific container snapshot.
func (p *Provider) ContainerSnapshotConfig(ctx context.Context, node string, vmid int, name string) (map[string]interface{}, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	config, err := p.client.GetContainerSnapshotConfig(ctx, node, vmid, name)
	if err != nil {
		return nil, fmt.Errorf("get container snapshot config: %w", err)
	}
	return config, nil
}

// Pools returns all resource pools.
func (p *Provider) Pools(ctx context.Context) ([]domain.Pool, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetPools(ctx)
	if err != nil {
		return nil, fmt.Errorf("get pools: %w", err)
	}
	result := make([]domain.Pool, 0, len(items))
	for _, item := range items {
		pool := domain.Pool{
			PoolID:  item.PoolID,
			Comment: item.Comment,
			Members: make([]string, 0, len(item.Members)),
		}
		for _, m := range item.Members {
			pool.Members = append(pool.Members, m.ID)
		}
		result = append(result, pool)
	}
	return result, nil
}

// ClusterLog returns cluster-wide log entries.
func (p *Provider) ClusterLog(ctx context.Context) ([]domain.ClusterLogEntry, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetClusterLog(ctx)
	if err != nil {
		return nil, fmt.Errorf("get cluster log: %w", err)
	}
	result := make([]domain.ClusterLogEntry, 0, len(items))
	for _, item := range items {
		result = append(result, domain.ClusterLogEntry{
			N:       item.N,
			Message: item.T,
		})
	}
	return result, nil
}

// ClusterStatuses returns detailed cluster status including quorum info.
func (p *Provider) ClusterStatuses(ctx context.Context) ([]domain.ClusterStatusDetail, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetClusterStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("get cluster status: %w", err)
	}
	result := make([]domain.ClusterStatusDetail, 0, len(items))
	for _, item := range items {
		result = append(result, domain.ClusterStatusDetail{
			Type:    item.Type,
			ID:      item.ID,
			Name:    item.Name,
			Status:  item.Status,
			Level:   item.Level,
			IP:      item.IP,
			Quorate: item.Quorate,
			Version: item.Version,
		})
	}
	return result, nil
}

// --- LifecycleProvider methods ---

func (p *Provider) VMStart(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMStart(ctx, node, vmid)
}

func (p *Provider) VMStop(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMStop(ctx, node, vmid)
}

func (p *Provider) VMShutdown(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMShutdown(ctx, node, vmid, 60)
}

func (p *Provider) VMReset(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMReset(ctx, node, vmid)
}

func (p *Provider) VMReboot(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMReboot(ctx, node, vmid)
}

func (p *Provider) VMSuspend(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMSuspend(ctx, node, vmid)
}

func (p *Provider) VMResume(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMResume(ctx, node, vmid)
}

func (p *Provider) VMPause(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMPause(ctx, node, vmid)
}

func (p *Provider) VMUnpause(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMUnpause(ctx, node, vmid)
}

func (p *Provider) CTStart(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTStart(ctx, node, vmid)
}

func (p *Provider) CTStop(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTStop(ctx, node, vmid)
}

func (p *Provider) CTShutdown(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTShutdown(ctx, node, vmid, 60)
}

func (p *Provider) CTReboot(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTReboot(ctx, node, vmid)
}

func (p *Provider) CTSuspend(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTSuspend(ctx, node, vmid)
}

func (p *Provider) CTResume(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTResume(ctx, node, vmid)
}

// --- ConfigProvider methods ---

func (p *Provider) VMConfigUpdate(ctx context.Context, node string, vmid int, params map[string]string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMConfigUpdate(ctx, node, vmid, mapToValues(params))
}

func (p *Provider) CTConfigUpdate(ctx context.Context, node string, vmid int, params map[string]string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTConfigUpdate(ctx, node, vmid, mapToValues(params))
}

// --- SnapshotMutationProvider methods ---

func (p *Provider) VMSnapshotCreate(ctx context.Context, node string, vmid int, name, description string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMSnapshotCreate(ctx, node, vmid, name, description)
}

func (p *Provider) VMSnapshotDelete(ctx context.Context, node string, vmid int, name string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMSnapshotDelete(ctx, node, vmid, name)
}

func (p *Provider) VMSnapshotRollback(ctx context.Context, node string, vmid int, name string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMSnapshotRollback(ctx, node, vmid, name)
}

func (p *Provider) CTSnapshotCreate(ctx context.Context, node string, vmid int, name, description string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTSnapshotCreate(ctx, node, vmid, name, description)
}

func (p *Provider) CTSnapshotDelete(ctx context.Context, node string, vmid int, name string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTSnapshotDelete(ctx, node, vmid, name)
}

func (p *Provider) CTSnapshotRollback(ctx context.Context, node string, vmid int, name string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTSnapshotRollback(ctx, node, vmid, name)
}

// --- DeleteProvider methods ---

func (p *Provider) VMDelete(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMDelete(ctx, node, vmid)
}

func (p *Provider) CTDelete(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTDelete(ctx, node, vmid)
}

// --- TemplateProvider methods ---

func (p *Provider) VMTemplate(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMTemplate(ctx, node, vmid)
}

func (p *Provider) CTTemplate(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTTemplate(ctx, node, vmid)
}

// --- CloudInitProvider methods ---

func (p *Provider) VMCloudInit(ctx context.Context, node string, vmid int) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMCloudInit(ctx, node, vmid)
}

// --- BackupMutationProvider methods ---

func (p *Provider) CreateBackup(ctx context.Context, node string, vmid int, storage, mode string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CreateBackup(ctx, node, vmid, storage, mode)
}

func (p *Provider) RestoreVM(ctx context.Context, node string, vmid int, archive, storage string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.RestoreVM(ctx, node, vmid, archive, storage)
}

func (p *Provider) GetBackupSchedules(ctx context.Context) ([]domain.BackupSchedule, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetBackupSchedules(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.BackupSchedule, 0, len(items))
	for _, item := range items {
		result = append(result, backupScheduleToDomain(item))
	}
	return result, nil
}

func (p *Provider) GetBackupSchedule(ctx context.Context, id string) (*domain.BackupSchedule, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	item, err := p.client.GetBackupSchedule(ctx, id)
	if err != nil {
		return nil, err
	}
	s := backupScheduleToDomain(*item)
	return &s, nil
}

func (p *Provider) CreateBackupSchedule(ctx context.Context, params domain.BackupScheduleCreateParams) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	req := client.BackupScheduleCreateRequest{
		Node:             params.Node,
		Storage:          params.Storage,
		VMID:             params.VMID,
		All:              params.All,
		Dow:              params.Dow,
		Starttime:        params.Starttime,
		Mode:             params.Mode,
		Enabled:          params.Enabled,
		Compress:         params.Compress,
		Comment:          params.Comment,
		Bwlimit:          params.Bwlimit,
		Ionice:           params.Ionice,
		MailNotification: params.MailNotification,
		Mailto:           params.Mailto,
		Maxfiles:         params.Maxfiles,
		PruneBackups:     params.PruneBackups,
		Quiet:            params.Quiet,
		Remove:           params.Remove,
		Pool:             params.Pool,
		Tmpdir:           params.Tmpdir,
	}
	return p.client.CreateBackupSchedule(ctx, req)
}

func (p *Provider) UpdateBackupSchedule(ctx context.Context, id string, params domain.BackupScheduleCreateParams) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	req := client.BackupScheduleCreateRequest{
		Node:             params.Node,
		Storage:          params.Storage,
		VMID:             params.VMID,
		All:              params.All,
		Dow:              params.Dow,
		Starttime:        params.Starttime,
		Mode:             params.Mode,
		Enabled:          params.Enabled,
		Compress:         params.Compress,
		Comment:          params.Comment,
		Bwlimit:          params.Bwlimit,
		Ionice:           params.Ionice,
		MailNotification: params.MailNotification,
		Mailto:           params.Mailto,
		Maxfiles:         params.Maxfiles,
		PruneBackups:     params.PruneBackups,
		Quiet:            params.Quiet,
		Remove:           params.Remove,
		Pool:             params.Pool,
		Tmpdir:           params.Tmpdir,
	}
	return p.client.UpdateBackupSchedule(ctx, id, req)
}

func (p *Provider) DeleteBackupSchedule(ctx context.Context, id string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteBackupSchedule(ctx, id)
}

func backupScheduleToDomain(item client.BackupScheduleItem) domain.BackupSchedule {
	return domain.BackupSchedule{
		ID:               item.ID,
		Node:             item.Node,
		Storage:          item.Storage,
		VMID:             item.VMID,
		All:              item.All,
		Dow:              item.Dow,
		Starttime:        item.Starttime,
		Mode:             item.Mode,
		Enabled:          item.Enabled,
		Compress:         item.Compress,
		Comment:          item.Comment,
		MailNotification: item.MailNotification,
		Mailto:           item.Mailto,
		Maxfiles:         item.Maxfiles,
		PruneBackups:     item.PruneBackups,
		Pool:             item.Pool,
	}
}

// --- StorageMutationProvider methods ---

func (p *Provider) UploadContent(ctx context.Context, node, storage, localPath string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.UploadContent(ctx, node, storage, localPath)
}

func (p *Provider) DownloadContentBody(ctx context.Context, node, storage, volumeID string, w io.Writer) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DownloadContentBody(ctx, node, storage, volumeID, w)
}

func (p *Provider) DeleteContent(ctx context.Context, node, storage, volumeID string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.DeleteContent(ctx, node, storage, volumeID)
}

// --- MigrationProvider methods ---

func (p *Provider) VMMigrate(ctx context.Context, node string, vmid int, target string, online bool) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMMigrate(ctx, node, vmid, target, online)
}

func (p *Provider) CTMigrate(ctx context.Context, node string, vmid int, target string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTMigrate(ctx, node, vmid, target)
}

// --- CloneProvider methods ---

func (p *Provider) VMClone(ctx context.Context, node string, vmid, newVmid int, name, storage string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMClone(ctx, node, vmid, newVmid, name, storage)
}

func (p *Provider) CTClone(ctx context.Context, node string, vmid, newVmid int, hostname, storage string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.CTClone(ctx, node, vmid, newVmid, hostname, storage)
}

// --- DiskProvider methods ---

func (p *Provider) VMDiskResize(ctx context.Context, node string, vmid int, disk, size string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMDiskResize(ctx, node, vmid, disk, size)
}

func (p *Provider) VMDiskMove(ctx context.Context, node string, vmid int, disk, storage string) (string, error) {
	if p.client == nil {
		return "", errors.New(errNotConnected)
	}
	return p.client.VMDiskMove(ctx, node, vmid, disk, storage)
}

// --- NetworkMutationProvider methods ---

func (p *Provider) ApplyNodeNetwork(ctx context.Context, node string, config map[string]string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	cfg := make(map[string]interface{})
	for k, v := range config {
		cfg[k] = v
	}
	return p.client.ApplyNodeNetwork(ctx, node, cfg)
}

func (p *Provider) RevertNodeNetwork(ctx context.Context, node string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.RevertNodeNetwork(ctx, node)
}

// --- FirewallMutationProvider methods ---

func (p *Provider) CreateFirewallRule(ctx context.Context, rule domain.FirewallRuleCreateInput) (*domain.FirewallRule, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	item, err := p.client.CreateFirewallRule(ctx, firewallRuleInputToRequest(rule))
	if err != nil {
		return nil, err
	}
	return &domain.FirewallRule{
		Type:     item.Type,
		Action:   item.Action,
		Enable:   item.Enable,
		Pos:      item.Pos,
		Proto:    item.Proto,
		Dest:     item.Dest,
		Dport:    item.Dport,
		Source:   item.Source,
		Sport:    item.Sport,
		ICMPType: item.ICMPType,
		Log:      item.Log,
		Comment:  item.Comment,
	}, nil
}

func (p *Provider) UpdateFirewallRule(ctx context.Context, pos int, rule domain.FirewallRuleCreateInput) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.UpdateFirewallRule(ctx, pos, firewallRuleInputToRequest(rule))
}

func (p *Provider) DeleteFirewallRule(ctx context.Context, pos int) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteFirewallRule(ctx, pos)
}

func (p *Provider) CreateNodeFirewallRule(ctx context.Context, node string, rule domain.FirewallRuleCreateInput) (*domain.FirewallRule, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	item, err := p.client.CreateNodeFirewallRule(ctx, node, firewallRuleInputToRequest(rule))
	if err != nil {
		return nil, err
	}
	return &domain.FirewallRule{
		Type:     item.Type,
		Action:   item.Action,
		Enable:   item.Enable,
		Pos:      item.Pos,
		Proto:    item.Proto,
		Dest:     item.Dest,
		Dport:    item.Dport,
		Source:   item.Source,
		Sport:    item.Sport,
		ICMPType: item.ICMPType,
		Log:      item.Log,
		Comment:  item.Comment,
	}, nil
}

func (p *Provider) UpdateNodeFirewallRule(ctx context.Context, node string, pos int, rule domain.FirewallRuleCreateInput) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.UpdateNodeFirewallRule(ctx, node, pos, firewallRuleInputToRequest(rule))
}

func (p *Provider) DeleteNodeFirewallRule(ctx context.Context, node string, pos int) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteNodeFirewallRule(ctx, node, pos)
}

func (p *Provider) CreateVMFirewallRule(ctx context.Context, node string, vmid int, rule domain.FirewallRuleCreateInput) (*domain.FirewallRule, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	item, err := p.client.CreateVMFirewallRule(ctx, node, vmid, firewallRuleInputToRequest(rule))
	if err != nil {
		return nil, err
	}
	return &domain.FirewallRule{
		Type:     item.Type,
		Action:   item.Action,
		Enable:   item.Enable,
		Pos:      item.Pos,
		Proto:    item.Proto,
		Dest:     item.Dest,
		Dport:    item.Dport,
		Source:   item.Source,
		Sport:    item.Sport,
		ICMPType: item.ICMPType,
		Log:      item.Log,
		Comment:  item.Comment,
	}, nil
}

func (p *Provider) UpdateVMFirewallRule(ctx context.Context, node string, vmid int, pos int, rule domain.FirewallRuleCreateInput) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.UpdateVMFirewallRule(ctx, node, vmid, pos, firewallRuleInputToRequest(rule))
}

func (p *Provider) DeleteVMFirewallRule(ctx context.Context, node string, vmid int, pos int) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteVMFirewallRule(ctx, node, vmid, pos)
}

func (p *Provider) CreateCTFirewallRule(ctx context.Context, node string, vmid int, rule domain.FirewallRuleCreateInput) (*domain.FirewallRule, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	item, err := p.client.CreateCTFirewallRule(ctx, node, vmid, firewallRuleInputToRequest(rule))
	if err != nil {
		return nil, err
	}
	return &domain.FirewallRule{
		Type:     item.Type,
		Action:   item.Action,
		Enable:   item.Enable,
		Pos:      item.Pos,
		Proto:    item.Proto,
		Dest:     item.Dest,
		Dport:    item.Dport,
		Source:   item.Source,
		Sport:    item.Sport,
		ICMPType: item.ICMPType,
		Log:      item.Log,
		Comment:  item.Comment,
	}, nil
}

func (p *Provider) UpdateCTFirewallRule(ctx context.Context, node string, vmid int, pos int, rule domain.FirewallRuleCreateInput) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.UpdateCTFirewallRule(ctx, node, vmid, pos, firewallRuleInputToRequest(rule))
}

func (p *Provider) DeleteCTFirewallRule(ctx context.Context, node string, vmid int, pos int) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteCTFirewallRule(ctx, node, vmid, pos)
}

func (p *Provider) CreateFirewallAlias(ctx context.Context, name, cidr, comment string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.CreateFirewallAlias(ctx, name, cidr, comment)
}

func (p *Provider) DeleteFirewallAlias(ctx context.Context, name string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteFirewallAlias(ctx, name)
}

func (p *Provider) CreateFirewallIPSet(ctx context.Context, name, comment string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.CreateFirewallIPSet(ctx, name, comment)
}

func (p *Provider) AddFirewallIPSetEntry(ctx context.Context, name, cidr, comment string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.AddFirewallIPSetEntry(ctx, name, cidr, comment)
}

func (p *Provider) RemoveFirewallIPSetEntry(ctx context.Context, name, cidr string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.RemoveFirewallIPSetEntry(ctx, name, cidr)
}

func (p *Provider) DeleteFirewallIPSet(ctx context.Context, name string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteFirewallIPSet(ctx, name)
}

func (p *Provider) CreateFirewallGroup(ctx context.Context, name, comment string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.CreateFirewallGroup(ctx, name, comment)
}

func (p *Provider) DeleteFirewallGroup(ctx context.Context, name string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteFirewallGroup(ctx, name)
}

func (p *Provider) UpdateFirewallOptions(ctx context.Context, opts domain.FirewallOptionsUpdateInput) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	req := client.FirewallOptionsUpdateRequest{
		Enable:       opts.Enable,
		PolicyIn:     opts.PolicyIn,
		PolicyOut:    opts.PolicyOut,
		LogInDrop:    opts.LogInDrop,
		LogRateLimit: opts.LogRateLimit,
		NFConntrack:  opts.NFConntrack,
		Digest:       opts.Digest,
	}
	return p.client.UpdateFirewallOptions(ctx, req)
}

// --- AccessProvider methods ---

func (p *Provider) Users(ctx context.Context) ([]domain.AccessUser, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("get users: %w", err)
	}
	result := make([]domain.AccessUser, 0, len(items))
	for _, item := range items {
		tokens := 0
		if item.Tokens != nil {
			tokens = *item.Tokens
		}
		result = append(result, domain.AccessUser{
			UserID:    item.UserID,
			Comment:   item.Comment,
			Email:     item.Email,
			Enable:    item.Enable,
			Expire:    item.Expire,
			FirstName: item.FirstName,
			LastName:  item.LastName,
			Tokens:    tokens,
		})
	}
	return result, nil
}

func (p *Provider) Groups(ctx context.Context) ([]domain.AccessGroup, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("get groups: %w", err)
	}
	result := make([]domain.AccessGroup, 0, len(items))
	for _, item := range items {
		members := item.Members
		if members == nil {
			members = []string{}
		}
		result = append(result, domain.AccessGroup{
			GroupID: item.GroupID,
			Comment: item.Comment,
			Members: members,
		})
	}
	return result, nil
}

func (p *Provider) Roles(ctx context.Context) ([]domain.AccessRole, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("get roles: %w", err)
	}
	result := make([]domain.AccessRole, 0, len(items))
	for _, item := range items {
		result = append(result, domain.AccessRole{
			RoleID:  item.RoleID,
			Privs:   item.Privs,
			Special: item.Special,
		})
	}
	return result, nil
}

func (p *Provider) ACL(ctx context.Context) ([]domain.AccessACLEntry, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetACL(ctx)
	if err != nil {
		return nil, fmt.Errorf("get ACL: %w", err)
	}
	result := make([]domain.AccessACLEntry, 0, len(items))
	for _, item := range items {
		userID := item.UserID
		groupID := item.GroupID
		result = append(result, domain.AccessACLEntry{
			Path:      item.Path,
			Type:      item.Type,
			RoleID:    item.RoleID,
			Propagate: item.Propagate,
			UserID:    userID,
			GroupID:   groupID,
		})
	}
	return result, nil
}

func (p *Provider) Domains(ctx context.Context) ([]domain.AccessDomain, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("get domains: %w", err)
	}
	result := make([]domain.AccessDomain, 0, len(items))
	for _, item := range items {
		result = append(result, domain.AccessDomain{
			Realm:   item.Realm,
			Type:    item.Type,
			Comment: item.Comment,
			Default: item.Default,
			TFA:     item.TFA,
		})
	}
	return result, nil
}

func (p *Provider) Tokens(ctx context.Context, user string) ([]domain.AccessToken, error) {
	if p.client == nil {
		return nil, errors.New(errNotConnected)
	}
	items, err := p.client.GetTokens(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("get tokens for user %s: %w", user, err)
	}
	result := make([]domain.AccessToken, 0, len(items))
	for _, item := range items {
		result = append(result, domain.AccessToken{
			TokenID:  item.TokenID,
			Comment:  item.Comment,
			Expire:   item.Expire,
			Privsep:  item.Privsep,
			Created:  item.Created,
			UserID:   item.UserID,
			Disabled: item.Disabled,
		})
	}
	return result, nil
}

func (p *Provider) CreateUser(ctx context.Context, userid, password, email, firstname, lastname, comment string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.CreateUser(ctx, userid, password, email, firstname, lastname, comment)
}

func (p *Provider) DeleteUser(ctx context.Context, userid string) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.DeleteUser(ctx, userid)
}

func (p *Provider) AddACL(ctx context.Context, path, role, user, group string, propagate int) error {
	if p.client == nil {
		return errors.New(errNotConnected)
	}
	return p.client.AddACL(ctx, path, role, user, group, propagate)
}

// firewallRuleInputToRequest converts a domain.FirewallRuleCreateInput to a client request.
func firewallRuleInputToRequest(input domain.FirewallRuleCreateInput) client.FirewallRuleCreateRequest {
	return client.FirewallRuleCreateRequest{
		Type:     input.Type,
		Action:   input.Action,
		Enable:   input.Enable,
		Pos:      input.Pos,
		Proto:    input.Proto,
		Dest:     input.Dest,
		Dport:    input.Dport,
		Source:   input.Source,
		Sport:    input.Sport,
		ICMPType: input.ICMPType,
		Log:      input.Log,
		Comment:  input.Comment,
		IFace:    input.IFace,
		Macro:    input.Macro,
	}
}

// mapToValues converts a map[string]string to url.Values.
func mapToValues(m map[string]string) url.Values {
	v := url.Values{}
	for key, val := range m {
		v.Set(key, val)
	}
	return v
}
