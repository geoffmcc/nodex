package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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
	data, err := keyring.Get(keyringService, profile)
	if err != nil {
		if err == keyring.ErrNotFound {
			return nil, fmt.Errorf("no credentials found for profile %q in keyring", profile)
		}
		return nil, fmt.Errorf("read keyring: %w", err)
	}

	var creds domain.Credentials
	if err := json.Unmarshal([]byte(data), &creds); err != nil {
		return nil, fmt.Errorf("parse keyring data: %w", err)
	}
	return &creds, nil
}

// Store saves credentials for the given profile in the OS keyring.
func (b *KeyringBackend) Store(_ context.Context, profile string, creds *domain.Credentials) error {
	data, err := json.Marshal(creds)
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
	if err := keyring.Delete(keyringService, profile); err != nil {
		if err == keyring.ErrNotFound {
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
		if err == keyring.ErrNotFound {
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

// updateIndex adds or removes a profile name from the keyring index.
func updateIndex(profile string, add bool) error {
	data, err := keyring.Get(keyringService, "__index__")
	var profiles []string
	if err == nil {
		_ = json.Unmarshal([]byte(data), &profiles)
	}

	if add {
		if !contains(profiles, profile) {
			profiles = append(profiles, profile)
			sort.Strings(profiles)
		}
	} else {
		var filtered []string
		for _, p := range profiles {
			if p != profile {
				filtered = append(filtered, p)
			}
		}
		profiles = filtered
	}

	indexData, err := json.Marshal(profiles)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	return keyring.Set(keyringService, "__index__", string(indexData))
}

// ParseCredentialRef parses a credential_ref string into backend and profile.
// Format: "backend:profile" or just "profile" (defaults to file backend).
func ParseCredentialRef(ref string) (backend, profile string) {
	if ref == "" {
		return "", ""
	}
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "file", ref
}
