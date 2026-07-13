package domain

import "context"

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

// SnapshotDetailProvider exposes snapshot config information.
type SnapshotDetailProvider interface {
	VMSnapshotConfig(ctx context.Context, node string, vmid int, name string) (map[string]interface{}, error)
	ContainerSnapshotConfig(ctx context.Context, node string, vmid int, name string) (map[string]interface{}, error)
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
)
