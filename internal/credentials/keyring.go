package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/zalando/go-keyring"

	"github.com/geoffmcc/nodex/internal/domain"
)

const keyringService = "nodex"

// KeyringBackend stores credentials in the OS keyring.
type KeyringBackend struct{}

// NewKeyringBackend creates a KeyringBackend.
func NewKeyringBackend() *KeyringBackend {
	return &KeyringBackend{}
}

// Name returns "keyring".
func (b *KeyringBackend) Name() string { return "keyring" }

// Get retrieves credentials for the given profile from the OS keyring.
func (b *KeyringBackend) Get(_ context.Context, profile string) (*domain.Credentials, error) {
	if err := ValidateName(profile); err != nil {
		return nil, err
	}
	data, err := keyring.Get(keyringService, profile)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, fmt.Errorf("no credentials found for profile %q in keyring", profile)
		}
		return nil, fmt.Errorf("read keyring: %w", err)
	}

	var creds domain.Credentials
	if err := json.Unmarshal([]byte(data), &creds); err != nil {
		return nil, fmt.Errorf("parse keyring data: %w", err)
	}
	if err := ValidateCredentials(profile, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// Store saves credentials for the given profile in the OS keyring.
func (b *KeyringBackend) Store(_ context.Context, profile string, creds *domain.Credentials) error {
	if err := ValidateName(profile); err != nil {
		return err
	}
	if err := ValidateCredentials(profile, creds); err != nil {
		return err
	}
	data, err := json.Marshal(creds) // #nosec G117 -- this backend intentionally stores credential material in the OS keyring.
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	if err := keyring.Set(keyringService, profile, string(data)); err != nil {
		return fmt.Errorf("write keyring: %w", err)
	}
	return nil
}

// Delete removes credentials for the given profile from the OS keyring.
func (b *KeyringBackend) Delete(_ context.Context, profile string) error {
	if err := ValidateName(profile); err != nil {
		return err
	}
	if err := keyring.Delete(keyringService, profile); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("no credentials found for profile %q in keyring", profile)
		}
		return fmt.Errorf("delete keyring: %w", err)
	}
	return nil
}

// List returns all profile names with stored credentials in the OS keyring.
// Note: most OS keyring implementations do not support enumeration.
// This returns a helpful error directing the user to specify a profile explicitly.
func (b *KeyringBackend) List(_ context.Context) ([]string, error) {
	data, err := keyring.Get(keyringService, "__index__")
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("read keyring index: %w", err)
	}

	var profiles []string
	if err := json.Unmarshal([]byte(data), &profiles); err != nil {
		return nil, fmt.Errorf("parse keyring index: %w", err)
	}
	sort.Strings(profiles)
	return profiles, nil
}

// ParseCredentialRef parses a credential_ref string into backend and profile.
// Format: "backend:profile" or just "profile" (defaults to file backend).
func ParseCredentialRef(ref string) (backend, profile string) {
	backend, profile, _ = ParseCredentialRefStrict(ref)
	return backend, profile
}
