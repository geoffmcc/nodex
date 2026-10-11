//go:build !windows

package credentials

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

func TestFileBackendFailsClosedOnBroadModeAndDoesNotRepairIt(t *testing.T) {
	dir := privateCredentialTestDir(t)
	backend := NewFileBackend(dir)
	path := filepath.Join(dir, "prod.json")
	data := []byte(`{"type":"token","token_id":"fixture","token_secret":"never-output"}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Get(context.Background(), "prod"); err == nil {
		t.Fatal("broadly readable credential file was used")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("unsafe file was unexpectedly changed: info=%v err=%v", info, err)
	}
}

func TestFileBackendRejectsSymlinkAndSpecialFile(t *testing.T) {
	dir := privateCredentialTestDir(t)
	backend := NewFileBackend(dir)
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{"type":"token","token_id":"fixture","token_secret":"fixture"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Get(context.Background(), "link"); err == nil {
		t.Fatal("symlink credential was followed")
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Get(context.Background(), "pipe"); err == nil {
		t.Fatal("FIFO credential path was accepted")
	}
}

func TestFileBackendRefusesUnsafeDirectoryWithoutChangingPermissions(t *testing.T) {
	dir := privateCredentialTestDir(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	backend := NewFileBackend(dir)
	err := backend.Store(context.Background(), "prod", &domain.Credentials{Type: "token", TokenID: "fixture", TokenSecret: "fixture"})
	if err == nil {
		t.Fatal("Store accepted a broadly accessible credential directory")
	}
	info, statErr := os.Stat(dir)
	if statErr != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("unsafe directory was unexpectedly repaired: info=%v err=%v", info, statErr)
	}
}

func TestFileBackendRefusesWritableParentDirectory(t *testing.T) {
	parent := privateCredentialTestDir(t)
	if err := os.Chmod(parent, 0o777); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "credentials")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := checkCredentialDirectory(dir); err == nil {
		t.Fatal("credential directory below a replaceable parent was accepted")
	}
}

// writeSecureCredentialFileForTest writes a credential fixture that satisfies
// the Unix file policy: mode 0600 owned by the current user.
func writeSecureCredentialFileForTest(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write credential fixture: %v", err)
	}
}
