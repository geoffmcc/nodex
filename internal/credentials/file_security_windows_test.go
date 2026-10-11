//go:build windows

package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"golang.org/x/sys/windows"
)

func TestWindowsFileBackendStoresProtectedOwnerACL(t *testing.T) {
	dir := t.TempDir()
	backend := NewFileBackend(dir)
	if err := backend.Store(context.Background(), "fixture", &domain.Credentials{Type: "token", TokenID: "fixture", TokenSecret: "fixture"}); err != nil {
		t.Fatalf("Store with Windows protected DACL: %v", err)
	}
	path := filepath.Join(dir, "fixture.json")
	file, err := openWindowsObject(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	sd, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("credential file DACL is inheritable")
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if dacl == nil || dacl.AceCount != 1 {
		count := uint16(0)
		if dacl != nil {
			count = dacl.AceCount
		}
		t.Fatalf("credential file ACL ACE count = %d, want one owner-only ACE", count)
	}
	if err := CheckCredentialFilePermissions(path); err != nil {
		t.Fatalf("stored owner-only ACL rejected on read: %v", err)
	}
}

func TestWindowsFileBackendRejectsInheritedCredentialFileACL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inherited.json")
	if err := os.WriteFile(path, []byte(`{"type":"token","token_id":"fixture","token_secret":"fixture"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckCredentialFilePermissions(path); err == nil {
		t.Fatal("file with inherited temporary-directory ACL was accepted")
	}
}

// writeSecureCredentialFileForTest writes a credential fixture that satisfies
// the Windows file policy: a protected DACL limited to the current user. A raw
// os.WriteFile would inherit the parent directory ACL and be rejected.
func writeSecureCredentialFileForTest(t *testing.T, path string, data []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("create credential fixture: %v", err)
	}
	if err := protectCredentialFile(f); err != nil {
		_ = f.Close()
		t.Fatalf("protect credential fixture: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		t.Fatalf("write credential fixture: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close credential fixture: %v", err)
	}
}
