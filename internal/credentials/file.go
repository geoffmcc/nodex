package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

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
	if err := ValidateName(profile); err != nil {
		return nil, err
	}
	path := b.path(profile)
	data, err := os.ReadFile(path) // #nosec G304 -- profile names are validated and resolved under the credential directory.
	if err != nil {
		return nil, fmt.Errorf("read credential file: %w", err)
	}
	if err := CheckCredentialFilePermissions(path); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	var creds domain.Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse credential file: %w", err)
	}
	if err := ValidateCredentials(profile, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// Store saves credentials for the given profile.
func (b *FileBackend) Store(_ context.Context, profile string, creds *domain.Credentials) error {
	if err := ValidateName(profile); err != nil {
		return err
	}
	if err := ValidateCredentials(profile, creds); err != nil {
		return err
	}
	if err := os.MkdirAll(b.dir, 0o700); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}
	data, err := json.Marshal(creds) // #nosec G117 -- this backend intentionally stores credential material in a 0600 file.
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	path := b.path(profile)
	tmp, err := os.CreateTemp(b.dir, "."+profile+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create credential temp file: %w", err)
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure credential temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write credential temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync credential temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close credential temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("write credential file: %w", err)
	}
	success = true
	return nil
}

// Delete removes credentials for the given profile.
func (b *FileBackend) Delete(_ context.Context, profile string) error {
	if err := ValidateName(profile); err != nil {
		return err
	}
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

// CheckCredentialFilePermissions checks if a credential file has overly broad permissions.
func CheckCredentialFilePermissions(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat credential file: %w", err)
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		return fmt.Errorf("credential file %s has permissions %o; recommended: 0600", path, mode)
	}
	return nil
}
