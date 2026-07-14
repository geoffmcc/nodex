package domain

import "context"

// Provider is the minimal interface that all infrastructure providers must implement.
// It covers lifecycle (connect, disconnect, health), identity (name, version),
// and capability discovery. Resource inspection and mutation are exposed through
// narrow optional capability interfaces that a provider may choose to implement.
//
// Nodex is Proxmox-first and provider-extensible. A future provider (e.g. VMware)
// need only implement the minimal Provider interface plus the specific capability
// interfaces it supports.
type Provider interface {
	// Name returns the provider name (e.g., "proxmox").
	Name() string

	// Version returns the provider version.
	Version() string

	// Connect initializes the provider with the given endpoint and credentials.
	Connect(ctx context.Context, endpoint string, credentials *Credentials) error

	// Close releases any resources held by the provider.
	Close() error

	// Health returns nil if the provider is connected and responsive.
	// An error indicates the provider is unreachable or misconfigured.
	Health(ctx context.Context) error

	// Capabilities returns the list of capabilities this provider supports.
	Capabilities() []Capability
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
