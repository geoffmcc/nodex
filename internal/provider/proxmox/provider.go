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
		domain.CapabilityNodeDetail,
		domain.CapabilityFirewallAdvanced,
		domain.CapabilityHAStatus,
		domain.CapabilityBackupContent,
		domain.CapabilitySDN,
		domain.CapabilitySnapshotDetail,
		domain.CapabilityPools,
		domain.CapabilityClusterLog,
		domain.CapabilityLifecycle,
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
