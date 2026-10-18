package credentials

import (
	"context"
	"errors"
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

// Resolve loads credentials from an explicit credential_ref when present.
// Without a reference, it tries environment credentials and then the same-name
// file backend, but only treats an absent environment value as eligible for
// fallback. It never prompts; interactive credential entry is handled by the
// CLI's explicit credential workflow.
func (r *Resolver) Resolve(ctx context.Context, profile, credentialRef string) (*domain.Credentials, error) {
	if credentialRef != "" {
		return r.resolveFromRef(ctx, profile, credentialRef)
	}
	if err := ValidateName(profile); err != nil {
		return nil, app.NewExitError(
			fmt.Errorf("%w: profile name: %w", app.ErrCredential, err),
			app.ExitCredential,
		)
	}

	// Try environment credentials first. Continue to file credentials only
	// when the environment backend reports that no credential was supplied;
	// malformed partial environment values must not silently select a stale,
	// potentially more privileged file identity.
	if creds, err := r.backends["env"].Get(ctx, profile); err == nil {
		return creds, nil
	} else if !errors.Is(err, ErrNoEnvironmentCredentials) {
		return nil, app.NewExitError(
			fmt.Errorf("%w: environment backend: %w", app.ErrCredential, err),
			app.ExitCredential,
		)
	}

	// Try file backend.
	credPath := filepath.Join(r.credDir, profile+".json")
	if _, err := os.Lstat(credPath); err == nil {
		creds, err := r.backends["file"].Get(ctx, profile)
		if err != nil {
			return nil, app.NewExitError(
				fmt.Errorf("%w: file backend: %w", app.ErrCredential, err),
				app.ExitCredential,
			)
		}
		return creds, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, app.NewExitError(
			fmt.Errorf("%w: inspect file credential: %w", app.ErrCredential, err),
			app.ExitCredential,
		)
	}

	return nil, app.NewExitError(
		fmt.Errorf("%w: no credentials found for profile %q (set credential_ref in profile, or set NODEX_<PROFILE>_TOKEN_ID and NODEX_<PROFILE>_TOKEN_SECRET)",
			app.ErrCredential, profile),
		app.ExitCredential,
	)
}

// resolveFromRef resolves credentials from an explicit credential_ref.
func (r *Resolver) resolveFromRef(ctx context.Context, profile, ref string) (*domain.Credentials, error) {
	backendName, refProfile, err := ParseCredentialRefStrict(ref)
	if err != nil {
		return nil, app.NewExitError(
			fmt.Errorf("%w: profile %q credential_ref: %w", app.ErrCredential, profile, err),
			app.ExitCredential,
		)
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
			fmt.Errorf("%w: backend %q: %w",
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
