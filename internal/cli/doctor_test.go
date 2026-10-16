package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/credentials"
	"github.com/geoffmcc/nodex/internal/domain"
)

func TestDoctorProfileSelectionHonorsExplicitProfile(t *testing.T) {
	cfg := &config.Config{Profiles: map[string]config.Profile{
		"nodex-test-lab": {Provider: config.ProviderProxmox},
		"other":          {Provider: config.ProviderProxmox},
	}}
	got := doctorProfileNames(cfg, "nodex-test-lab")
	if len(got) != 1 || got[0] != "nodex-test-lab" {
		t.Fatalf("doctor profiles = %v, want only nodex-test-lab", got)
	}
}

// storeSecureCredentialFixture creates a credential file through the real file
// backend so it carries the platform-appropriate protections (0600 on Unix, a
// protected owner-only DACL on Windows) that a raw os.WriteFile would not.
func storeSecureCredentialFixture(t *testing.T, dir, name string) {
	t.Helper()
	creds := &domain.Credentials{Type: "token", TokenID: "fixture", TokenSecret: "fixture"}
	if err := credentials.NewFileBackend(dir).Store(context.Background(), name, creds); err != nil {
		t.Fatalf("store credential fixture: %v", err)
	}
}

func privateDoctorTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatalf("restrict fixture directory: %v", err)
		}
	}
	return dir
}

func TestCheckCredentialFile(t *testing.T) {
	dir := privateDoctorTempDir(t)
	storeSecureCredentialFixture(t, dir, "prod")
	path := filepath.Join(dir, "prod.json")

	profile := config.Profile{CredentialRef: "file:prod"}
	got, ok := checkCredentialFile("production", profile, dir)
	if !ok || got.Status != "pass" || got.Name != "credentials/production" {
		t.Fatalf("checkCredentialFile() = (%+v, %v), want pass", got, ok)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok = checkCredentialFile("production", profile, dir)
		if !ok || got.Status != "fail" || !strings.Contains(got.Message, "0600") {
			t.Fatalf("checkCredentialFile() insecure mode = (%+v, %v), want fail recommending 0600", got, ok)
		}
	}
}

func TestCheckCredentialFileMissingAndNonFileBackend(t *testing.T) {
	got, ok := checkCredentialFile("production", config.Profile{CredentialRef: "file:missing"}, privateDoctorTempDir(t))
	if !ok || got.Status != "fail" || !strings.Contains(got.Message, "missing") {
		t.Fatalf("missing credential = (%+v, %v), want fail", got, ok)
	}

	if got, ok := checkCredentialFile("production", config.Profile{CredentialRef: "keyring:production"}, t.TempDir()); ok {
		t.Fatalf("non-file backend should be skipped, got %+v", got)
	}
	if got, ok := checkCredentialFile("production", config.Profile{}, privateDoctorTempDir(t)); ok {
		t.Fatalf("implicit missing file should be skipped because env credentials may be configured, got %+v", got)
	}
}

func TestCheckCredentialFileDefaultReference(t *testing.T) {
	dir := privateDoctorTempDir(t)
	storeSecureCredentialFixture(t, dir, "production")
	got, ok := checkCredentialFile("production", config.Profile{}, dir)
	if !ok || got.Status != "pass" {
		t.Fatalf("implicit file fallback = (%+v, %v), want pass", got, ok)
	}
}

func TestCheckSSHKey(t *testing.T) {
	if got, ok := checkSSHKey("production", config.Profile{}); ok {
		t.Fatalf("unconfigured SSH key should be skipped, got %+v", got)
	}

	missing := config.Profile{SSHKeyFile: filepath.Join(privateDoctorTempDir(t), "missing")}
	got, ok := checkSSHKey("production", missing)
	if !ok || got.Status != "fail" || !strings.Contains(got.Message, "missing") {
		t.Fatalf("missing SSH key = (%+v, %v), want fail", got, ok)
	}

	dir := privateDoctorTempDir(t)
	storeSecureCredentialFixture(t, dir, "sshkey")
	path := filepath.Join(dir, "sshkey.json")
	profile := config.Profile{SSHKeyFile: path}
	got, ok = checkSSHKey("production", profile)
	if !ok || got.Status != "pass" || got.Name != "profile/production/ssh-key" {
		t.Fatalf("secure SSH key = (%+v, %v), want pass", got, ok)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok = checkSSHKey("production", profile)
		if !ok || got.Status != "fail" || !strings.Contains(got.Message, "0600") {
			t.Fatalf("insecure SSH key = (%+v, %v), want fail recommending 0600", got, ok)
		}
	}
}

func TestCheckKnownHosts(t *testing.T) {
	if got, ok := checkKnownHosts("production", config.Profile{}); ok {
		t.Fatalf("unconfigured SSH key should skip known_hosts check, got %+v", got)
	}

	home := t.TempDir()
	path := filepath.Join(home, ".ssh", "known_hosts")
	got, ok := checkKnownHostsFile("production", path)
	if !ok || got.Status != "fail" || !strings.Contains(got.Message, "missing") {
		t.Fatalf("missing known_hosts = (%+v, %v), want fail", got, ok)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("example.test ssh-ed25519 AAAA"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok = checkKnownHostsFile("production", path)
	if !ok || got.Status != "pass" || !strings.Contains(got.Message, "host key is checked") {
		t.Fatalf("readable known_hosts = (%+v, %v), want pass", got, ok)
	}
}
