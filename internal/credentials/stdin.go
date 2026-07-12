package credentials

import (
	"context"
	"fmt"

	"github.com/geoffmcc/nodex/internal/domain"
)

// StdinBackend reads credentials from stdin (interactive prompt).
type StdinBackend struct {
	reader func(prompt string) (string, error)
}

// NewStdinBackend creates a StdinBackend with the given reader function.
func NewStdinBackend(reader func(prompt string) (string, error)) *StdinBackend {
	return &StdinBackend{reader: reader}
}

// Name returns "stdin".
func (b *StdinBackend) Name() string { return "stdin" }

// Get prompts the user for credentials interactively.
func (b *StdinBackend) Get(_ context.Context, profile string) (*domain.Credentials, error) {
	tokenID, err := b.reader("Token ID: ")
	if err != nil {
		return nil, fmt.Errorf("read token ID: %w", err)
	}
	tokenSecret, err := b.reader("Token Secret: ")
	if err != nil {
		return nil, fmt.Errorf("read token secret: %w", err)
	}
	return &domain.Credentials{
		Type:        "token",
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
	}, nil
}

// Store is not supported for stdin backend.
func (b *StdinBackend) Store(_ context.Context, _ string, _ *domain.Credentials) error {
	return fmt.Errorf("stdin backend does not support storing credentials")
}

// Delete is not supported for stdin backend.
func (b *StdinBackend) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("stdin backend does not support deleting credentials")
}

// List is not supported for stdin backend.
func (b *StdinBackend) List(_ context.Context) ([]string, error) {
	return nil, nil
}
