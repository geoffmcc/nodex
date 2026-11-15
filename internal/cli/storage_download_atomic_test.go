package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFinalizeDownloadReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "volume.iso")
	if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	tmp, err := os.CreateTemp(dir, ".nodex-tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString("new"); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}

	if err := finalizeDownload(tmpPath, dest, true); err != nil {
		t.Fatalf("finalizeDownload: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("destination content = %q, want %q", got, "new")
	}
}

// On Unix, a failed replace must never delete the previous destination.
// Removing it before the rename is exactly the data-loss bug this guards.
func TestFinalizeDownloadPreservesDestinationOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permissions are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}

	dir := t.TempDir()
	dest := filepath.Join(dir, "volume.iso")
	if err := os.WriteFile(dest, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	tmp, err := os.CreateTemp(dir, ".nodex-tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString("new"); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}

	// Make the rename fail by removing write permission on the directory.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := finalizeDownload(tmpPath, dest, true); err == nil {
		t.Fatal("finalizeDownload succeeded, want a rename failure")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("previous destination was lost: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("destination content = %q, want the previous contents %q", got, "old")
	}
}
