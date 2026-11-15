package execsafe

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeExecutable(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatal(err)
	}
	// WriteFile is subject to umask; force the mode under test.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAcceptsSafeExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	writeExecutable(t, path, 0o755)
	if err := Check(path); err != nil {
		t.Fatalf("Check(%q) = %v, want nil", path, err)
	}
}

func TestCheckRejectsWorldWritableExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	writeExecutable(t, path, 0o777)
	if err := Check(path); err == nil {
		t.Fatalf("Check(%q) = nil, want world-writable rejection", path)
	}
}

func TestCheckRejectsWorldWritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	dir := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	// Mkdir is subject to umask; force the mode the check is about.
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tool")
	writeExecutable(t, path, 0o755)
	if err := Check(path); err == nil {
		t.Fatalf("Check(%q) = nil, want world-writable-directory rejection", path)
	}
}

func TestCheckAllowsStickyWorldWritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}
	dir := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, os.ModeSticky|0o777); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tool")
	writeExecutable(t, path, 0o755)
	if err := Check(path); err != nil {
		t.Fatalf("Check(%q) = %v, want nil for sticky directory", path, err)
	}
}

func TestLookPathRejectsMissingExecutable(t *testing.T) {
	if _, err := LookPath("nodex-definitely-missing-executable"); err == nil {
		t.Fatal("LookPath(missing) = nil error, want failure")
	}
}
