package domain

import (
	"context"
	"io"
)

// --- Core inspection interfaces ---
//
// These interfaces expose the resource-inspection methods that were previously
// part of the base Provider interface. Splitting them into narrow, optional
// interfaces means a provider need only implement inspection for the resource
// families it supports.

// NodeInspector provides basic node listing.
type NodeInspector interface {
	Nodes(ctx context.Context) ([]Node, error)
}

// VMInspector provides VM listing and configuration.
type VMInspector interface {
	VMs(ctx context.Context) ([]VM, error)
	VMConfig(ctx context.Context, node string, vmid int) (map[string]interface{}, error)
}

// ContainerInspector provides container listing and configuration.
type ContainerInspector interface {
	Containers(ctx context.Context) ([]Container, error)
	ContainerConfig(ctx context.Context, node string, vmid int) (map[string]interface{}, error)
}

// StorageInspector provides storage pool listing and content inspection.
type StorageInspector interface {
	Storage(ctx context.Context) ([]Storage, error)
	StorageContent(ctx context.Context, node, storage string) ([]StorageContentItem, error)
}

// ClusterInspector provides cluster-level information.
type ClusterInspector interface {
	Cluster(ctx context.Context) (*Cluster, error)
}

// TaskInspector provides task listing and detail retrieval.
type TaskInspector interface {
	Tasks(ctx context.Context, node string) ([]Task, error)
	Task(ctx context.Context, node, upid string) (*Task, error)
}

// SnapshotInspector provides snapshot listing for VMs and containers.
type SnapshotInspector interface {
	VMSnapshots(ctx context.Context, node string, vmid int) ([]Snapshot, error)
	ContainerSnapshots(ctx context.Context, node string, vmid int) ([]Snapshot, error)
}

// EventInspector provides cluster event listing.
type EventInspector interface {
	Events(ctx context.Context) ([]Event, error)
}

// SyslogInspector provides syslog retrieval per node.
type SyslogInspector interface {
	Syslog(ctx context.Context, node string) ([]SyslogEntry, error)
}

// BackupInspector provides backup task listing per node.
type BackupInspector interface {
	Backups(ctx context.Context, node string) ([]Backup, error)
}

// FirewallInspector provides cluster-level firewall rule listing.
type FirewallInspector interface {
	FirewallRules(ctx context.Context) ([]FirewallRule, error)
}

// HAInspector provides HA resource and group listing.
type HAInspector interface {
	HAResources(ctx context.Context) ([]HAResource, error)
	HAGroups(ctx context.Context) ([]HAGroup, error)
}

// --- Optional capability interfaces (inspection detail and mutation) ---

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

// --- Ceph, SDN Mutation, and Replication provider interfaces ---

// CephProvider exposes Ceph health and inventory (read-only).
type CephProvider interface {
	CephStatus(ctx context.Context, node string) (*CephStatus, error)
	CephOSDs(ctx context.Context, node string) ([]CephOSD, error)
	CephMONs(ctx context.Context, node string) ([]CephMON, error)
	CephPools(ctx context.Context, node string) ([]CephPool, error)
}

// CephMutationProvider exposes Ceph mutation operations.
type CephMutationProvider interface {
	CephCreateOSD(ctx context.Context, node, dev string) (string, error)
	CephOSDOut(ctx context.Context, node string, osdid int) error
	CephOSDIn(ctx context.Context, node string, osdid int) error
	CephDestroyOSD(ctx context.Context, node string, osdid int) (string, error)
	CephCreatePool(ctx context.Context, node, name string, params map[string]string) (string, error)
	CephDestroyPool(ctx context.Context, node, name string) (string, error)
}

// SDNMutationProvider exposes SDN mutation operations.
type SDNMutationProvider interface {
	SDNCreateZone(ctx context.Context, zoneType, zone string) error
	SDNDeleteZone(ctx context.Context, zone string) error
	SDNCreateVNet(ctx context.Context, vnet, zone string) error
	SDNDeleteVNet(ctx context.Context, vnet string) error
	SDNCreateSubnet(ctx context.Context, vnet, cidr, gateway string) error
	SDNDeleteSubnet(ctx context.Context, vnet, subnet string) error
	SDNCreateController(ctx context.Context, ctrl string) error
	SDNDeleteController(ctx context.Context, ctrl string) error
}

// ReplicationProvider exposes replication job operations.
type ReplicationProvider interface {
	ReplicationList(ctx context.Context) ([]ReplicationJob, error)
	ReplicationGet(ctx context.Context, id string) (*ReplicationJob, error)
	ReplicationCreate(ctx context.Context, params ReplicationCreateInput) error
	ReplicationUpdate(ctx context.Context, id string, params ReplicationUpdateInput) error
	ReplicationDelete(ctx context.Context, id string) error
	ReplicationSchedule(ctx context.Context, node, id string) error
}

// --- Domain types for Ceph, SDN, and Replication ---

// CephStatus represents the Ceph cluster health overview.
type CephStatus struct {
	Health map[string]interface{} `json:"health" yaml:"health"`
}

// CephOSD represents a Ceph OSD entry.
type CephOSD struct {
	ID          int     `json:"id" yaml:"id"`
	Name        string  `json:"name" yaml:"name"`
	Type        string  `json:"type" yaml:"type"`
	Status      string  `json:"status" yaml:"status"`
	In          int     `json:"in" yaml:"in"`
	Host        string  `json:"host,omitempty" yaml:"host,omitempty"`
	DeviceClass string  `json:"device_class,omitempty" yaml:"device_class,omitempty"`
	TotalSpace  int64   `json:"total_space,omitempty" yaml:"total_space,omitempty"`
	BytesUsed   int64   `json:"bytes_used,omitempty" yaml:"bytes_used,omitempty"`
	PercentUsed float64 `json:"percent_used,omitempty" yaml:"percent_used,omitempty"`
}

// CephMON represents a Ceph Monitor entry.
type CephMON struct {
	Name    string `json:"name" yaml:"name"`
	Host    string `json:"host,omitempty" yaml:"host,omitempty"`
	Quorum  bool   `json:"quorum" yaml:"quorum"`
	State   string `json:"state,omitempty" yaml:"state,omitempty"`
	Rank    int    `json:"rank,omitempty" yaml:"rank,omitempty"`
	Version string `json:"ceph_version_short,omitempty" yaml:"ceph_version_short,omitempty"`
}

// CephPool represents a Ceph pool entry.
type CephPool struct {
	ID              int     `json:"pool" yaml:"pool"`
	Name            string  `json:"pool_name" yaml:"pool_name"`
	Size            int     `json:"size" yaml:"size"`
	MinSize         int     `json:"min_size" yaml:"min_size"`
	PGNum           int     `json:"pg_num" yaml:"pg_num"`
	CrushRule       int     `json:"crush_rule" yaml:"crush_rule"`
	CrushRuleName   string  `json:"crush_rule_name,omitempty" yaml:"crush_rule_name,omitempty"`
	Type            string  `json:"type" yaml:"type"`
	PGNumFinal      int     `json:"pg_num_final,omitempty" yaml:"pg_num_final,omitempty"`
	PercentUsed     float64 `json:"percent_used,omitempty" yaml:"percent_used,omitempty"`
	BytesUsed       int64   `json:"bytes_used,omitempty" yaml:"bytes_used,omitempty"`
	PGAutoscaleMode string  `json:"pg_autoscale_mode,omitempty" yaml:"pg_autoscale_mode,omitempty"`
}

// SDNZoneCreateInput holds the fields for creating an SDN zone.
type SDNZoneCreateInput struct {
	Type string
	Zone string
}

// SDNVNetCreateInput holds the fields for creating an SDN VNet.
type SDNVNetCreateInput struct {
	VNet string
	Zone string
}

// SDNSubnetCreateInput holds the fields for creating an SDN subnet.
type SDNSubnetCreateInput struct {
	VNet    string
	CIDR    string
	Gateway string
}

// SDNControllerCreateInput holds the fields for creating an SDN controller.
type SDNControllerCreateInput struct {
	Controller string
}

// ReplicationJob represents a replication job.
type ReplicationJob struct {
	ID        string `json:"id" yaml:"id"`
	Guest     int    `json:"guest" yaml:"guest"`
	Type      string `json:"type" yaml:"type"`
	Source    string `json:"source,omitempty" yaml:"source,omitempty"`
	Target    string `json:"target" yaml:"target"`
	Schedule  string `json:"schedule,omitempty" yaml:"schedule,omitempty"`
	Comment   string `json:"comment,omitempty" yaml:"comment,omitempty"`
	Enabled   int    `json:"enabled" yaml:"enabled"`
	Rate      int64  `json:"rate,omitempty" yaml:"rate,omitempty"`
	JobNum    int    `json:"jobnum,omitempty" yaml:"jobnum,omitempty"`
	LastSync  int64  `json:"last_sync,omitempty" yaml:"last_sync,omitempty"`
	FailCount int    `json:"fail_count,omitempty" yaml:"fail_count,omitempty"`
}

// ReplicationCreateInput holds fields for creating a replication job.
type ReplicationCreateInput struct {
	ID       string
	Guest    int
	Type     string
	Target   string
	Schedule string
	Comment  string
	Rate     int64
	Source   string
}

// ReplicationUpdateInput holds fields for updating a replication job.
type ReplicationUpdateInput struct {
	Target   string
	Schedule string
	Comment  string
	Rate     int64
	Enable   int
	Source   string
	Delete   string
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
	CapabilityNetworkMutation  Capability = "network_mutation"
	CapabilityFirewallMutation Capability = "firewall_mutation"
	CapabilityAccess           Capability = "access"
	CapabilityCeph             Capability = "ceph"
	CapabilityCephMutation     Capability = "ceph_mutation"
	CapabilitySDNMutation      Capability = "sdn_mutation"
	CapabilityReplication      Capability = "replication"
)

// NetworkMutationProvider exposes network configuration mutation operations.
type NetworkMutationProvider interface {
	ApplyNodeNetwork(ctx context.Context, node string, config map[string]string) error
	RevertNodeNetwork(ctx context.Context, node string) error
}

// FirewallMutationProvider exposes firewall rule, alias, IP set, group, and options mutations.
type FirewallMutationProvider interface {
	// Rule mutations (cluster level).
	CreateFirewallRule(ctx context.Context, rule FirewallRuleCreateInput) (*FirewallRule, error)
	UpdateFirewallRule(ctx context.Context, pos int, rule FirewallRuleCreateInput) error
	DeleteFirewallRule(ctx context.Context, pos int) error

	// Rule mutations (node level).
	CreateNodeFirewallRule(ctx context.Context, node string, rule FirewallRuleCreateInput) (*FirewallRule, error)
	UpdateNodeFirewallRule(ctx context.Context, node string, pos int, rule FirewallRuleCreateInput) error
	DeleteNodeFirewallRule(ctx context.Context, node string, pos int) error

	// Rule mutations (VM level).
	CreateVMFirewallRule(ctx context.Context, node string, vmid int, rule FirewallRuleCreateInput) (*FirewallRule, error)
	UpdateVMFirewallRule(ctx context.Context, node string, vmid int, pos int, rule FirewallRuleCreateInput) error
	DeleteVMFirewallRule(ctx context.Context, node string, vmid int, pos int) error

	// Rule mutations (CT level).
	CreateCTFirewallRule(ctx context.Context, node string, vmid int, rule FirewallRuleCreateInput) (*FirewallRule, error)
	UpdateCTFirewallRule(ctx context.Context, node string, vmid int, pos int, rule FirewallRuleCreateInput) error
	DeleteCTFirewallRule(ctx context.Context, node string, vmid int, pos int) error

	// Alias mutations.
	CreateFirewallAlias(ctx context.Context, name, cidr, comment string) error
	DeleteFirewallAlias(ctx context.Context, name string) error

	// IP set mutations.
	CreateFirewallIPSet(ctx context.Context, name, comment string) error
	AddFirewallIPSetEntry(ctx context.Context, name, cidr, comment string) error
	RemoveFirewallIPSetEntry(ctx context.Context, name, cidr string) error
	DeleteFirewallIPSet(ctx context.Context, name string) error

	// Security group mutations.
	CreateFirewallGroup(ctx context.Context, name, comment string) error
	DeleteFirewallGroup(ctx context.Context, name string) error

	// Options mutations.
	UpdateFirewallOptions(ctx context.Context, opts FirewallOptionsUpdateInput) error
}

// FirewallRuleCreateInput holds the fields for creating or updating a firewall rule.
type FirewallRuleCreateInput struct {
	Type     string
	Action   string
	Enable   int
	Pos      int
	Proto    string
	Dest     string
	Dport    string
	Source   string
	Sport    string
	ICMPType string
	Log      string
	Comment  string
	IFace    string
	Macro    string
}

// FirewallOptionsUpdateInput holds the fields for updating firewall options.
type FirewallOptionsUpdateInput struct {
	Enable       int
	PolicyIn     string
	PolicyOut    string
	LogInDrop    int
	LogRateLimit string
	NFConntrack  int
	Digest       string
}

// AccessProvider exposes identity management operations.
type AccessProvider interface {
	// Read-only operations.
	Users(ctx context.Context) ([]AccessUser, error)
	Groups(ctx context.Context) ([]AccessGroup, error)
	Roles(ctx context.Context) ([]AccessRole, error)
	ACL(ctx context.Context) ([]AccessACLEntry, error)
	Domains(ctx context.Context) ([]AccessDomain, error)
	Tokens(ctx context.Context, user string) ([]AccessToken, error)

	// Expert-mode mutations (Tier 4).
	CreateUser(ctx context.Context, userid, password, email, firstname, lastname, comment string) error
	DeleteUser(ctx context.Context, userid string) error
	AddACL(ctx context.Context, path, role, user, group string, propagate int) error
}

// --- Identity domain types ---

// AccessUser represents a Proxmox VE user.
type AccessUser struct {
	UserID    string `json:"userid" yaml:"userid"`
	Comment   string `json:"comment,omitempty" yaml:"comment,omitempty"`
	Email     string `json:"email,omitempty" yaml:"email,omitempty"`
	Enable    int    `json:"enable" yaml:"enable"`
	Expire    int64  `json:"expire,omitempty" yaml:"expire,omitempty"`
	FirstName string `json:"firstname,omitempty" yaml:"firstname,omitempty"`
	LastName  string `json:"lastname,omitempty" yaml:"lastname,omitempty"`
	Tokens    int    `json:"tokens,omitempty" yaml:"tokens,omitempty"`
}

// AccessGroup represents a Proxmox VE group.
type AccessGroup struct {
	GroupID string   `json:"groupid" yaml:"groupid"`
	Comment string   `json:"comment,omitempty" yaml:"comment,omitempty"`
	Members []string `json:"members,omitempty" yaml:"members,omitempty"`
}

// AccessRole represents a Proxmox VE role.
type AccessRole struct {
	RoleID  string `json:"roleid" yaml:"roleid"`
	Privs   string `json:"privs,omitempty" yaml:"privs,omitempty"`
	Special int    `json:"special,omitempty" yaml:"special,omitempty"`
}

// AccessACLEntry represents a Proxmox VE ACL entry.
type AccessACLEntry struct {
	Path      string `json:"path" yaml:"path"`
	Type      string `json:"type" yaml:"type"`
	RoleID    string `json:"roleid" yaml:"roleid"`
	Propagate int    `json:"propagate,omitempty" yaml:"propagate,omitempty"`
	UserID    string `json:"userid,omitempty" yaml:"userid,omitempty"`
	GroupID   string `json:"groupid,omitempty" yaml:"groupid,omitempty"`
}

// AccessDomain represents a Proxmox VE authentication realm.
type AccessDomain struct {
	Realm   string `json:"realm" yaml:"realm"`
	Type    string `json:"type" yaml:"type"`
	Comment string `json:"comment,omitempty" yaml:"comment,omitempty"`
	Default int    `json:"default,omitempty" yaml:"default,omitempty"`
	TFA     string `json:"tfa,omitempty" yaml:"tfa,omitempty"`
}

// AccessToken represents a Proxmox VE API token (metadata only).
type AccessToken struct {
	TokenID  string `json:"tokenid" yaml:"tokenid"`
	Comment  string `json:"comment,omitempty" yaml:"comment,omitempty"`
	Expire   int64  `json:"expire,omitempty" yaml:"expire,omitempty"`
	Privsep  int    `json:"privsep,omitempty" yaml:"privsep,omitempty"`
	Created  int64  `json:"created,omitempty" yaml:"created,omitempty"`
	UserID   string `json:"userid,omitempty" yaml:"userid,omitempty"`
	Disabled int    `json:"disabled,omitempty" yaml:"disabled,omitempty"`
}

// --- Capability metadata ---

// CapCategory describes whether a capability is inspection or mutation.
type CapCategory string

const (
	CapInspection CapCategory = "inspection"
	CapMutation   CapCategory = "mutation"
)

// SafetyTier maps to the safety confirmation tiers used throughout Nodex.
type SafetyTier string

const (
	TierObservation   SafetyTier = "observation"
	TierReversible    SafetyTier = "reversible"
	TierDisruptive    SafetyTier = "disruptive"
	TierDestructive   SafetyTier = "destructive"
	TierSecurityAdmin SafetyTier = "security_admin"
)

// CapabilityMeta describes a single capability: its category, safety tier,
// and which optional Go interfaces it corresponds to.
type CapabilityMeta struct {
	Name       string
	Category   CapCategory
	Safety     SafetyTier
	Interfaces []string // Go interface names, e.g. "NodeInspector"
}

// CapabilityMetadata returns the known metadata for every capability constant.
// A provider may implement any subset of these; Capabilities() declares the
// supported subset.
func CapabilityMetadata() map[Capability]CapabilityMeta {
	return map[Capability]CapabilityMeta{
		// --- Core inspection ---
		CapabilityNodes: {
			Name: "Nodes", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"NodeInspector"},
		},
		CapabilityVMs: {
			Name: "VMs", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"VMInspector"},
		},
		CapabilityContainers: {
			Name: "Containers", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"ContainerInspector"},
		},
		CapabilityStorage: {
			Name: "Storage", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"StorageInspector"},
		},
		CapabilityCluster: {
			Name: "Cluster", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"ClusterInspector"},
		},

		// --- Extended inspection ---
		CapabilityNodeDetail: {
			Name: "Node Detail", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"NodeDetailProvider"},
		},
		CapabilityFirewallAdvanced: {
			Name: "Firewall Advanced", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"FirewallProvider"},
		},
		CapabilityHAStatus: {
			Name: "HA Status", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"HAProvider"},
		},
		CapabilityBackupContent: {
			Name: "Backup Content", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"BackupProvider"},
		},
		CapabilitySDN: {
			Name: "SDN", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"SDNProvider"},
		},
		CapabilitySnapshotDetail: {
			Name: "Snapshot Detail", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"SnapshotDetailProvider"},
		},
		CapabilityPools: {
			Name: "Pools", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"PoolProvider"},
		},
		CapabilityClusterLog: {
			Name: "Cluster Log", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"ClusterLogProvider"},
		},
		CapabilityCeph: {
			Name: "Ceph", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"CephProvider"},
		},
		CapabilityReplication: {
			Name: "Replication", Category: CapInspection, Safety: TierObservation,
			Interfaces: []string{"ReplicationProvider"},
		},

		// --- Mutation: reversible ---
		CapabilityLifecycle: {
			Name: "Lifecycle", Category: CapMutation, Safety: TierReversible,
			Interfaces: []string{"LifecycleProvider"},
		},

		// --- Mutation: disruptive ---
		CapabilityConfig: {
			Name: "Config", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"ConfigProvider"},
		},
		CapabilityBackupMutation: {
			Name: "Backup Mutation", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"BackupMutationProvider"},
		},
		CapabilityStorageMutation: {
			Name: "Storage Mutation", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"StorageMutationProvider"},
		},
		CapabilityMigration: {
			Name: "Migration", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"MigrationProvider"},
		},
		CapabilityClone: {
			Name: "Clone", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"CloneProvider"},
		},
		CapabilityDisk: {
			Name: "Disk", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"DiskProvider"},
		},
		CapabilityTemplate: {
			Name: "Template", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"TemplateProvider"},
		},
		CapabilityCloudInit: {
			Name: "Cloud Init", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"CloudInitProvider"},
		},
		CapabilityNetworkMutation: {
			Name: "Network Mutation", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"NetworkMutationProvider"},
		},
		CapabilityFirewallMutation: {
			Name: "Firewall Mutation", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"FirewallMutationProvider"},
		},
		CapabilityCephMutation: {
			Name: "Ceph Mutation", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"CephMutationProvider"},
		},
		CapabilitySDNMutation: {
			Name: "SDN Mutation", Category: CapMutation, Safety: TierDisruptive,
			Interfaces: []string{"SDNMutationProvider"},
		},

		// --- Mutation: destructive ---
		CapabilitySnapshotMutation: {
			Name: "Snapshot Mutation", Category: CapMutation, Safety: TierDestructive,
			Interfaces: []string{"SnapshotMutationProvider"},
		},
		CapabilityDelete: {
			Name: "Delete", Category: CapMutation, Safety: TierDestructive,
			Interfaces: []string{"DeleteProvider"},
		},

		// --- Mutation: security administration ---
		CapabilityAccess: {
			Name: "Access", Category: CapMutation, Safety: TierSecurityAdmin,
			Interfaces: []string{"AccessProvider"},
		},
	}
}
