package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	if err := checkCredentialDirectory(b.dir); err != nil {
		return nil, err
	}
	f, err := openCredentialFile(path)
	if err != nil {
		return nil, fmt.Errorf("open credential file: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(f, 1<<20)) // #nosec G304 -- profile names are validated and resolved under the credential directory.
	closeErr := f.Close()
	if readErr != nil {
		clear(data)
		return nil, fmt.Errorf("read credential file: %w", readErr)
	}
	if closeErr != nil {
		clear(data)
		return nil, fmt.Errorf("close credential file: %w", closeErr)
	}
	if len(data) == 1<<20 {
		clear(data)
		return nil, fmt.Errorf("credential file exceeds the 1 MiB limit")
	}
	// Clear the temporary serialized copy after decoding. The returned
	// Credentials use immutable Go strings and cannot be reliably zeroized.
	defer clear(data)
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
		return fmt.Errorf("create credential directory: %w (secure the directory or use credential_ref: keyring:<profile>)", err)
	}
	if err := checkCredentialDirectory(b.dir); err != nil {
		return err
	}
	data, err := json.Marshal(creds) // #nosec G117 -- this backend intentionally stores credential material in a 0600 file.
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	// Avoid retaining the serialized secret buffer after the atomic write.
	defer clear(data)
	path := b.path(profile)
	if err := checkCredentialFile(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("refuse unsafe existing credential file: %w", err)
	}
	tmp, err := os.CreateTemp(b.dir, "."+profile+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create credential temp file: %w (secure the directory or use credential_ref: keyring:<profile>)", err)
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := protectCredentialFile(tmp); err != nil {
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
	if err := checkCredentialFile(path); err != nil {
		return fmt.Errorf("verify stored credential file: %w", err)
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
	if err := checkCredentialFile(path); err != nil {
		return fmt.Errorf("refuse unsafe credential file: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete credential file: %w", err)
	}
	return nil
}

// List returns all profile names with stored credentials.
func (b *FileBackend) List(_ context.Context) ([]string, error) {
	if err := checkCredentialDirectory(b.dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return nil, fmt.Errorf("read credential directory: %w", err)
	}
	var profiles []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			if err := checkCredentialFile(filepath.Join(b.dir, e.Name())); err != nil {
				return nil, fmt.Errorf("credential profile %q is unsafe: %w", strings.TrimSuffix(e.Name(), ".json"), err)
			}
			profiles = append(profiles, e.Name()[:len(e.Name())-5])
		}
	}
	return profiles, nil
}

func (b *FileBackend) path(profile string) string {
	return filepath.Join(b.dir, profile+".json")
}

// CheckCredentialFilePermissions validates the credential object, parent
// directory, ownership, type, and platform-specific access controls.
func CheckCredentialFilePermissions(path string) error {
	return CheckSecretFilePermissions(path)
}

// CheckSecretFilePermissions rejects secret files unless the platform helper
// can validate the opened object and its access controls. Unsafe files are
// never read or used by the file backend.
func CheckSecretFilePermissions(path string) error {
	return checkCredentialFile(path)
}

// CredentialFilePath returns the on-disk path the file backend uses for name.
// An empty dir means the default credential directory.
func CredentialFilePath(dir, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	if dir == "" {
		dir = defaultCredDir()
	}
	return filepath.Join(dir, name+".json"), nil
}
