package domain

import (
	"context"
	"io"
)

// NodeDetailProvider exposes detailed per-node information.
type NodeDetailProvider interface {
	NodeStatus(ctx context.Context, node string) (map[string]interface{}, error)
	NodeServices(ctx context.Context, node string) ([]NodeService, error)
	NodeNetwork(ctx context.Context, node string) ([]NodeNetwork, error)
	NodeDNS(ctx context.Context, node string) (*NodeDNS, error)
	NodeTime(ctx context.Context, node string) (*NodeTime, error)
	NodeDisks(ctx context.Context, node string) ([]NodeDisk, error)
	NodeCertificates(ctx context.Context, node string) ([]NodeCertificate, error)
	NodeSubscription(ctx context.Context, node string) (*NodeSubscription, error)
	NodeUpdates(ctx context.Context, node string) ([]NodeUpdate, error)
}

// FirewallProvider exposes firewall management beyond basic rules.
type FirewallProvider interface {
	FirewallAliases(ctx context.Context) ([]FirewallAlias, error)
	FirewallIPSet(ctx context.Context, name string) ([]FirewallIPSetEntry, error)
	FirewallIPSets(ctx context.Context) ([]FirewallIPSet, error)
	FirewallSecurityGroups(ctx context.Context) ([]FirewallSecurityGroup, error)
	FirewallOptions(ctx context.Context) (*FirewallOptions, error)
	NodeFirewallRules(ctx context.Context, node string) ([]FirewallRule, error)
	VMFirewallRules(ctx context.Context, node string, vmid int) ([]FirewallRule, error)
}

// HAProvider exposes HA status beyond basic resources/groups.
type HAProvider interface {
	HAStatus(ctx context.Context) (*HAStatus, error)
	HACurrent(ctx context.Context) ([]HACurrent, error)
}

// BackupProvider exposes backup content beyond task listing.
type BackupProvider interface {
	BackupContent(ctx context.Context, node, storage string) ([]BackupContentItem, error)
}

// SDNProvider exposes SDN topology.
type SDNProvider interface {
	SDNZones(ctx context.Context) ([]SDNZone, error)
	SDNVNets(ctx context.Context) ([]SDNVNet, error)
}

// PoolProvider exposes resource pool management.
type PoolProvider interface {
	Pools(ctx context.Context) ([]Pool, error)
}

// ClusterLogProvider exposes cluster-wide log entries.
type ClusterLogProvider interface {
	ClusterLog(ctx context.Context) ([]ClusterLogEntry, error)
}

// ClusterStatusProvider exposes cluster status for quorum and node health.
type ClusterStatusProvider interface {
	ClusterStatuses(ctx context.Context) ([]ClusterStatusDetail, error)
}

// SnapshotDetailProvider exposes snapshot config information.
type SnapshotDetailProvider interface {
	VMSnapshotConfig(ctx context.Context, node string, vmid int, name string) (map[string]interface{}, error)
	ContainerSnapshotConfig(ctx context.Context, node string, vmid int, name string) (map[string]interface{}, error)
}

// ConfigProvider exposes VM and container config update operations.
type ConfigProvider interface {
	VMConfigUpdate(ctx context.Context, node string, vmid int, params map[string]string) (string, error)
	CTConfigUpdate(ctx context.Context, node string, vmid int, params map[string]string) (string, error)
}

// SnapshotMutationProvider exposes snapshot create, delete, and rollback operations.
type SnapshotMutationProvider interface {
	VMSnapshotCreate(ctx context.Context, node string, vmid int, name, description string) (string, error)
	VMSnapshotDelete(ctx context.Context, node string, vmid int, name string) (string, error)
	VMSnapshotRollback(ctx context.Context, node string, vmid int, name string) (string, error)
	CTSnapshotCreate(ctx context.Context, node string, vmid int, name, description string) (string, error)
	CTSnapshotDelete(ctx context.Context, node string, vmid int, name string) (string, error)
	CTSnapshotRollback(ctx context.Context, node string, vmid int, name string) (string, error)
}

// DeleteProvider exposes VM and container deletion operations.
type DeleteProvider interface {
	VMDelete(ctx context.Context, node string, vmid int) (string, error)
	CTDelete(ctx context.Context, node string, vmid int) (string, error)
}

// TemplateProvider exposes VM and container template conversion operations.
type TemplateProvider interface {
	VMTemplate(ctx context.Context, node string, vmid int) (string, error)
	CTTemplate(ctx context.Context, node string, vmid int) (string, error)
}

// CloudInitProvider exposes cloud-init regeneration operations.
type CloudInitProvider interface {
	VMCloudInit(ctx context.Context, node string, vmid int) (string, error)
}

// BackupMutationProvider exposes backup creation and backup job management operations.
// All mutation methods return a UPID string that can be followed with task polling.
type BackupMutationProvider interface {
	CreateBackup(ctx context.Context, node string, vmid int, storage, mode string) (string, error)
	RestoreVM(ctx context.Context, node string, vmid int, archive, storage string) (string, error)
	GetBackupSchedules(ctx context.Context) ([]BackupSchedule, error)
	GetBackupSchedule(ctx context.Context, id string) (*BackupSchedule, error)
	CreateBackupSchedule(ctx context.Context, schedule BackupScheduleCreateParams) (string, error)
	UpdateBackupSchedule(ctx context.Context, id string, schedule BackupScheduleCreateParams) error
	DeleteBackupSchedule(ctx context.Context, id string) error
}

// StorageMutationProvider exposes storage content upload/download/delete operations.
type StorageMutationProvider interface {
	UploadContent(ctx context.Context, node, storage, localPath string) (string, error)
	DownloadContentBody(ctx context.Context, node, storage, volumeID string, w io.Writer) error
	DeleteContent(ctx context.Context, node, storage, volumeID string) (string, error)
}

// MigrationProvider exposes VM and container migration operations.
type MigrationProvider interface {
	VMMigrate(ctx context.Context, node string, vmid int, target string, online bool) (string, error)
	CTMigrate(ctx context.Context, node string, vmid int, target string) (string, error)
}

// CloneProvider exposes VM and container clone operations.
type CloneProvider interface {
	VMClone(ctx context.Context, node string, vmid, newVmid int, name, storage string) (string, error)
	CTClone(ctx context.Context, node string, vmid, newVmid int, hostname, storage string) (string, error)
}

// DiskProvider exposes VM disk resize and move operations.
type DiskProvider interface {
	VMDiskResize(ctx context.Context, node string, vmid int, disk, size string) (string, error)
	VMDiskMove(ctx context.Context, node string, vmid int, disk, storage string) (string, error)
}

// LifecycleProvider exposes VM and container lifecycle mutation operations.
// All methods return a UPID string that can be followed with task polling.
type LifecycleProvider interface {
	// VM lifecycle operations.
	VMStart(ctx context.Context, node string, vmid int) (string, error)
	VMStop(ctx context.Context, node string, vmid int) (string, error)
	VMShutdown(ctx context.Context, node string, vmid int) (string, error)
	VMReset(ctx context.Context, node string, vmid int) (string, error)
	VMReboot(ctx context.Context, node string, vmid int) (string, error)
	VMSuspend(ctx context.Context, node string, vmid int) (string, error)
	VMResume(ctx context.Context, node string, vmid int) (string, error)
	VMPause(ctx context.Context, node string, vmid int) (string, error)
	VMUnpause(ctx context.Context, node string, vmid int) (string, error)

	// Container lifecycle operations.
	CTStart(ctx context.Context, node string, vmid int) (string, error)
	CTStop(ctx context.Context, node string, vmid int) (string, error)
	CTShutdown(ctx context.Context, node string, vmid int) (string, error)
	CTReboot(ctx context.Context, node string, vmid int) (string, error)
	CTSuspend(ctx context.Context, node string, vmid int) (string, error)
	CTResume(ctx context.Context, node string, vmid int) (string, error)
}

// --- Domain types for optional capabilities ---

// NodeService represents a system service on a node.
type NodeService struct {
	Name   string `json:"name" yaml:"name"`
	State  string `json:"state" yaml:"state"`
	Active bool   `json:"active" yaml:"active"`
}

// NodeNetwork represents a network interface on a node.
type NodeNetwork struct {
	Name   string `json:"name" yaml:"name"`
	Type   string `json:"type" yaml:"type"`
	Status string `json:"status" yaml:"status"`
	IP     string `json:"ip,omitempty" yaml:"ip,omitempty"`
	MAC    string `json:"mac,omitempty" yaml:"mac,omitempty"`
}

// NodeDNS represents DNS configuration for a node.
type NodeDNS struct {
	DNS1         string `json:"dns1,omitempty" yaml:"dns1,omitempty"`
	DNS2         string `json:"dns2,omitempty" yaml:"dns2,omitempty"`
	SearchDomain string `json:"search_domain,omitempty" yaml:"search_domain,omitempty"`
}

// NodeTime represents time configuration for a node.
type NodeTime struct {
	TimeZone string `json:"timezone" yaml:"timezone"`
	Epoch    int64  `json:"epoch" yaml:"epoch"`
	Local    string `json:"local,omitempty" yaml:"local,omitempty"`
}

// NodeDisk represents a physical disk on a node.
type NodeDisk struct {
	Name   string `json:"name" yaml:"name"`
	Path   string `json:"path" yaml:"path"`
	Size   int64  `json:"size" yaml:"size"`
	Type   string `json:"type,omitempty" yaml:"type,omitempty"`
	Model  string `json:"model,omitempty" yaml:"model,omitempty"`
	Health string `json:"health,omitempty" yaml:"health,omitempty"`
}

// NodeCertificate represents a TLS certificate on a node.
type NodeCertificate struct {
	Fingerprint string `json:"fingerprint" yaml:"fingerprint"`
	Subject     string `json:"subject" yaml:"subject"`
	Issuer      string `json:"issuer,omitempty" yaml:"issuer,omitempty"`
	NotBefore   string `json:"not_before,omitempty" yaml:"not_before,omitempty"`
	NotAfter    string `json:"not_after,omitempty" yaml:"not_after,omitempty"`
}

// NodeSubscription represents the subscription status for a node.
type NodeSubscription struct {
	Status  string `json:"status" yaml:"status"`
	Key     string `json:"key,omitempty" yaml:"key,omitempty"`
	Expires string `json:"expires,omitempty" yaml:"expires,omitempty"`
}

// NodeUpdate represents available updates for a node.
type NodeUpdate struct {
	Package string `json:"package" yaml:"package"`
	Version string `json:"version" yaml:"version"`
}

// FirewallAlias represents a named address group.
type FirewallAlias struct {
	Name    string `json:"name" yaml:"name"`
	CIDR    string `json:"cidr" yaml:"cidr"`
	Comment string `json:"comment,omitempty" yaml:"comment,omitempty"`
}

// FirewallIPSet represents an IP set.
type FirewallIPSet struct {
	Name    string `json:"name" yaml:"name"`
	Comment string `json:"comment,omitempty" yaml:"comment,omitempty"`
}

// FirewallIPSetEntry represents a single entry in an IP set.
type FirewallIPSetEntry struct {
	CIDR    string `json:"cidr" yaml:"cidr"`
	Comment string `json:"comment,omitempty" yaml:"comment,omitempty"`
}

// FirewallSecurityGroup represents a firewall security group.
type FirewallSecurityGroup struct {
	Name    string         `json:"name" yaml:"name"`
	Comment string         `json:"comment,omitempty" yaml:"comment,omitempty"`
	Rules   []FirewallRule `json:"rules" yaml:"rules"`
}

// FirewallOptions represents cluster-level firewall options.
type FirewallOptions struct {
	Enable int `json:"enable" yaml:"enable"`
	Log    int `json:"log_in_drop" yaml:"log_in_drop"`
}

// HAStatus represents cluster HA status.
type HAStatus struct {
	Quorum int    `json:"quorum" yaml:"quorum"`
	Status string `json:"status" yaml:"status"`
}

// HACurrent represents the current state of an HA resource.
type HACurrent struct {
	ID     string `json:"id" yaml:"id"`
	Type   string `json:"type" yaml:"type"`
	State  string `json:"state" yaml:"state"`
	Node   string `json:"node,omitempty" yaml:"node,omitempty"`
	Status string `json:"status,omitempty" yaml:"status,omitempty"`
}

// BackupContentItem represents content available for backup.
type BackupContentItem struct {
	Content string `json:"content" yaml:"content"`
	Volid   string `json:"volid" yaml:"volid"`
	Size    int64  `json:"size,omitempty" yaml:"size,omitempty"`
	Format  string `json:"format,omitempty" yaml:"format,omitempty"`
}

// SDNZone represents an SDN zone.
type SDNZone struct {
	Name   string `json:"name" yaml:"name"`
	Type   string `json:"type" yaml:"type"`
	Status string `json:"status,omitempty" yaml:"status,omitempty"`
	VNets  int    `json:"vnets,omitempty" yaml:"vnets,omitempty"`
}

// SDNVNet represents an SDN virtual network.
type SDNVNet struct {
	Name  string `json:"name" yaml:"name"`
	Zone  string `json:"zone" yaml:"zone"`
	VLAN  int    `json:"vlan,omitempty" yaml:"vlan,omitempty"`
	Alias string `json:"alias,omitempty" yaml:"alias,omitempty"`
}

// Capability constants for optional features.
const (
	CapabilityNodeDetail       Capability = "node_detail"
	CapabilityFirewallAdvanced Capability = "firewall_advanced"
	CapabilityHAStatus         Capability = "ha_status"
	CapabilityBackupContent    Capability = "backup_content"
	CapabilitySDN              Capability = "sdn"
	CapabilitySnapshotDetail   Capability = "snapshot_detail"
	CapabilityPools            Capability = "pools"
	CapabilityClusterLog       Capability = "cluster_log"
	CapabilityLifecycle        Capability = "lifecycle"
	CapabilityConfig           Capability = "config"
	CapabilitySnapshotMutation Capability = "snapshot_mutation"
	CapabilityDelete           Capability = "delete"
	CapabilityTemplate         Capability = "template"
	CapabilityCloudInit        Capability = "cloud_init"
	CapabilityBackupMutation   Capability = "backup_mutation"
	CapabilityStorageMutation  Capability = "storage_mutation"
	CapabilityMigration        Capability = "migration"
	CapabilityClone            Capability = "clone"
	CapabilityDisk             Capability = "disk"
)
