package credentials

import (
	"context"

	"github.com/geoffmcc/nodex/internal/domain"
)

// Backend is the interface for credential storage backends.
type Backend interface {
	// Name returns the backend name (keyring, file, env, stdin).
	Name() string

	// Get retrieves credentials for the given profile.
	Get(ctx context.Context, profile string) (*domain.Credentials, error)

	// Store saves credentials for the given profile.
	Store(ctx context.Context, profile string, creds *domain.Credentials) error

	// Delete removes credentials for the given profile.
	Delete(ctx context.Context, profile string) error

	// List returns all profile names with stored credentials.
	List(ctx context.Context) ([]string, error)
}
