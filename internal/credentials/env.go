package credentials

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/geoffmcc/nodex/internal/domain"
)

// EnvBackend reads credentials from environment variables.
type EnvBackend struct {
	prefix string
}

// NewEnvBackend creates an EnvBackend with the given variable prefix.
func NewEnvBackend(prefix string) *EnvBackend {
	return &EnvBackend{prefix: prefix}
}

// Name returns "env".
func (b *EnvBackend) Name() string { return "env" }

// Get retrieves credentials from environment variables.
// Expected variables:
//
//	{PREFIX}_{PROFILE}_TOKEN
//	{PREFIX}_{PROFILE}_TOKEN_ID
//	{PREFIX}_{PROFILE}_TOKEN_SECRET
//	{PREFIX}_{PROFILE}_USERNAME
//	{PREFIX}_{PROFILE}_PASSWORD
func (b *EnvBackend) Get(_ context.Context, profile string) (*domain.Credentials, error) {
	if err := ValidateName(profile); err != nil {
		return nil, err
	}
	upper := strings.ToUpper(strings.ReplaceAll(profile, "-", "_"))
	prefix := fmt.Sprintf("%s_%s", b.upperPrefix(), upper)

	token := os.Getenv(prefix + "_TOKEN")
	tokenID := os.Getenv(prefix + "_TOKEN_ID")
	tokenSecret := os.Getenv(prefix + "_TOKEN_SECRET")
	username := os.Getenv(prefix + "_USERNAME")
	password := os.Getenv(prefix + "_PASSWORD")

	if token == "" && tokenID == "" && username == "" {
		return nil, fmt.Errorf("no credentials found for profile %q in environment", profile)
	}

	creds := &domain.Credentials{
		Type:        credentialType(token, tokenID, username),
		Token:       token,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		Username:    username,
		Password:    password,
	}
	if err := ValidateCredentials(profile, creds); err != nil {
		return nil, err
	}
	return creds, nil
}

// Store is not supported for env backend.
func (b *EnvBackend) Store(_ context.Context, _ string, _ *domain.Credentials) error {
	return fmt.Errorf("env backend does not support storing credentials")
}

// Delete is not supported for env backend.
func (b *EnvBackend) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("env backend does not support deleting credentials")
}

// List returns profile names found in environment variables.
func (b *EnvBackend) List(_ context.Context) ([]string, error) {
	prefix := b.upperPrefix() + "_"
	var profiles []string
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, prefix) {
			key := strings.TrimPrefix(env, prefix)
			parts := strings.SplitN(key, "_", 2)
			if len(parts) >= 2 {
				profile := strings.ToLower(parts[0])
				if !contains(profiles, profile) {
					profiles = append(profiles, profile)
				}
			}
		}
	}
	return profiles, nil
}

func (b *EnvBackend) upperPrefix() string {
	return strings.ToUpper(strings.ReplaceAll(b.prefix, "-", "_"))
}

func credentialType(token, tokenID, username string) string {
	if token != "" {
		return "token"
	}
	if tokenID != "" {
		return "token"
	}
	if username != "" {
		return "password"
	}
	return "unknown"
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
