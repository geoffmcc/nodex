package credentials

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
)

// Resolver loads credentials from a credential_ref and profile name.
type Resolver struct {
	backends map[string]Backend
	credDir  string
}

// NewResolver creates a Resolver with all available backends.
// If credDir is empty, the default ~/.nodex/credentials is used.
func NewResolver(credDir string) *Resolver {
	if credDir == "" {
		credDir = defaultCredDir()
	}
	return &Resolver{
		credDir: credDir,
		backends: map[string]Backend{
			"keyring": NewKeyringBackend(),
			"file":    NewFileBackend(credDir),
			"env":     NewEnvBackend("nodex"),
			"stdin":   NewStdinBackend(nil),
		},
	}
}

// Resolve loads credentials for a profile using its credential_ref.
// credential_ref format: "backend:profile" or just "profile" (defaults to file).
// Falls back through env and stdin if the primary backend fails.
func (r *Resolver) Resolve(ctx context.Context, profile, credentialRef string) (*domain.Credentials, error) {
	if credentialRef != "" {
		return r.resolveFromRef(ctx, profile, credentialRef)
	}

	// Try env first (NODEX_PROFILE_TOKEN pattern).
	if creds, err := r.backends["env"].Get(ctx, profile); err == nil {
		return creds, nil
	}

	// Try file backend.
	credPath := filepath.Join(r.credDir, profile+".json")
	if _, err := os.Stat(credPath); err == nil {
		if creds, err := r.backends["file"].Get(ctx, profile); err == nil {
			return creds, nil
		}
	}

	return nil, app.NewExitError(
		fmt.Errorf("%w: no credentials found for profile %q (set credential_ref in profile or use NODEX_<PROFILE>_TOKEN env var)",
			app.ErrCredential, profile),
		app.ExitCredential,
	)
}

// resolveFromRef resolves credentials from an explicit credential_ref.
func (r *Resolver) resolveFromRef(ctx context.Context, profile, ref string) (*domain.Credentials, error) {
	backendName, refProfile := ParseCredentialRef(ref)
	if refProfile == "" {
		refProfile = profile
	}

	backend, ok := r.backends[backendName]
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: unknown credential backend %q (available: %s)",
				app.ErrCredential, backendName, r.availableBackends()),
			app.ExitCredential,
		)
	}

	creds, err := backend.Get(ctx, refProfile)
	if err != nil {
		return nil, app.NewExitError(
			fmt.Errorf("%w: backend %q: %v",
				app.ErrCredential, backendName, err),
			app.ExitCredential,
		)
	}
	return creds, nil
}

// availableBackends returns a comma-separated list of backend names.
func (r *Resolver) availableBackends() string {
	names := make([]string, 0, len(r.backends))
	for name := range r.backends {
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// defaultCredDir returns the default credential directory.
func defaultCredDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".nodex/credentials"
	}
	return filepath.Join(home, ".nodex", "credentials")
}

// GetBackend returns a specific backend by name.
func (r *Resolver) GetBackend(name string) (Backend, bool) {
	b, ok := r.backends[name]
	return b, ok
}
