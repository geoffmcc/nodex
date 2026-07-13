package domain

import "context"

// Provider is the interface that all infrastructure providers must implement.
type Provider interface {
	// Name returns the provider name (e.g., "proxmox", "vmware").
	Name() string

	// Version returns the provider version.
	Version() string

	// Connect initializes the provider with the given endpoint and credentials.
	Connect(ctx context.Context, endpoint string, credentials *Credentials) error

	// Close releases any resources held by the provider.
	Close() error

	// Capabilities returns the list of capabilities this provider supports.
	Capabilities() []Capability

	// Nodes returns all nodes managed by this provider.
	Nodes(ctx context.Context) ([]Node, error)

	// VMs returns all VMs managed by this provider.
	VMs(ctx context.Context) ([]VM, error)

	// Containers returns all containers managed by this provider.
	Containers(ctx context.Context) ([]Container, error)

	// Storage returns all storage pools managed by this provider.
	Storage(ctx context.Context) ([]Storage, error)

	// Cluster returns cluster information.
	Cluster(ctx context.Context) (*Cluster, error)

	// VMConfig returns configuration for a specific VM.
	VMConfig(ctx context.Context, node string, vmid int) (map[string]interface{}, error)

	// ContainerConfig returns configuration for a specific container.
	ContainerConfig(ctx context.Context, node string, vmid int) (map[string]interface{}, error)

	// StorageContent returns content items for a specific storage.
	StorageContent(ctx context.Context, node, storage string) ([]StorageContentItem, error)

	// Tasks returns all tasks for a specific node.
	Tasks(ctx context.Context, node string) ([]Task, error)

	// Task returns details for a specific task by UPID.
	Task(ctx context.Context, node, upid string) (*Task, error)

	// VMSnapshots returns snapshots for a VM.
	VMSnapshots(ctx context.Context, node string, vmid int) ([]Snapshot, error)

	// ContainerSnapshots returns snapshots for a container.
	ContainerSnapshots(ctx context.Context, node string, vmid int) ([]Snapshot, error)

	// Events returns cluster events.
	Events(ctx context.Context) ([]Event, error)

	// Syslog returns syslog entries for a specific node.
	Syslog(ctx context.Context, node string) ([]SyslogEntry, error)
}

// Credentials holds authentication information for a provider.
type Credentials struct {
	// Type identifies the credential type (token, password, etc.).
	Type string

	// Token is the API token (for token-based auth).
	Token string

	// Username is the username (for password-based auth).
	Username string

	// Password is the password (for password-based auth).
	Password string

	// TokenID is the token ID (for Proxmox PVEAPIToken).
	TokenID string

	// TokenSecret is the token secret (for Proxmox PVEAPIToken).
	TokenSecret string
}

// Capability represents a provider capability.
type Capability string

const (
	// CapabilityNodes indicates the provider can list nodes.
	CapabilityNodes Capability = "nodes"

	// CapabilityVMs indicates the provider can list VMs.
	CapabilityVMs Capability = "vms"

	// CapabilityContainers indicates the provider can list containers.
	CapabilityContainers Capability = "containers"

	// CapabilityStorage indicates the provider can list storage.
	CapabilityStorage Capability = "storage"

	// CapabilityCluster indicates the provider can list cluster info.
	CapabilityCluster Capability = "cluster"
)
