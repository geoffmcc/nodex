package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/geoffmcc/nodex/internal/domain"
)

// FileBackend stores credentials in JSON files.
type FileBackend struct {
	dir string
}

// NewFileBackend creates a FileBackend using the given directory.
func NewFileBackend(dir string) *FileBackend {
	return &FileBackend{dir: dir}
}

// Name returns "file".
func (b *FileBackend) Name() string { return "file" }

// Get retrieves credentials for the given profile.
func (b *FileBackend) Get(_ context.Context, profile string) (*domain.Credentials, error) {
	path := b.path(profile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read credential file: %w", err)
	}
	var creds domain.Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse credential file: %w", err)
	}
	return &creds, nil
}

// Store saves credentials for the given profile.
func (b *FileBackend) Store(_ context.Context, profile string, creds *domain.Credentials) error {
	if err := os.MkdirAll(b.dir, 0o700); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	path := b.path(profile)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write credential file: %w", err)
	}
	return nil
}

// Delete removes credentials for the given profile.
func (b *FileBackend) Delete(_ context.Context, profile string) error {
	path := b.path(profile)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete credential file: %w", err)
	}
	return nil
}

// List returns all profile names with stored credentials.
func (b *FileBackend) List(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return nil, fmt.Errorf("read credential directory: %w", err)
	}
	var profiles []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			profiles = append(profiles, e.Name()[:len(e.Name())-5])
		}
	}
	return profiles, nil
}

func (b *FileBackend) path(profile string) string {
	return filepath.Join(b.dir, profile+".json")
}
