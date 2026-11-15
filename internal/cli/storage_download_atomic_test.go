package cli

import (
	"os"
	"path/filepath"
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

// A missing source makes replacement fail while the destination directory remains writable.
func TestFinalizeDownloadPreservesDestinationOnFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "volume.iso")
	if err := os.WriteFile(dest, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := finalizeDownload(filepath.Join(dir, "missing"), dest, true); err == nil {
		t.Fatal("expected failure")
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "old" {
		t.Fatalf("destination = %q, error %v", got, err)
	}
}
