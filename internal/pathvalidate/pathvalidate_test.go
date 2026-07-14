package pathvalidate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateSafePathNormalFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "output.txt")
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSafePath(dest); err != nil {
		t.Fatalf("unexpected error for normal file: %v", err)
	}
}

func TestValidateSafePathNonExistent(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "newfile.txt")
	if err := ValidateSafePath(dest); err != nil {
		t.Fatalf("unexpected error for non-existent file: %v", err)
	}
}

func TestValidateSafePathRejectsTraversal(t *testing.T) {
	// A relative path starting with ".." — after cleaning, the ".."
	// components remain because there is no preceding component to cancel
	// them.
	dest := filepath.Clean("../../etc/passwd")
	err := ValidateSafePath(dest)
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
	if !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSafePathRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "mysubdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := ValidateSafePath(subdir)
	if err == nil {
		t.Fatal("expected error for directory")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSafePathRejectsEmpty(t *testing.T) {
	err := ValidateSafePath("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSafePathRejectsDotOnly(t *testing.T) {
	err := ValidateSafePath(".")
	if err == nil {
		t.Fatal("expected error for dot path")
	}
}

func TestValidateSafePathInDirNormal(t *testing.T) {
	dir := t.TempDir()
	result, err := ValidateSafePathInDir(dir, "subdir/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Clean(filepath.Join(dir, "subdir", "file.txt"))
	if result != expected {
		t.Fatalf("got %s, want %s", result, expected)
	}
}

func TestValidateSafePathInDirRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	_, err := ValidateSafePathInDir(dir, "../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for traversal")
	}
}

func TestValidateSafePathInDirRejectsAbsolute(t *testing.T) {
	dir := t.TempDir()
	absPath := "/etc/passwd"
	if runtime.GOOS == "windows" {
		absPath = "C:\\Windows\\System32\\drivers\\etc\\hosts"
	}
	_, err := ValidateSafePathInDir(dir, absPath)
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSafePathInDirRejectsDoubleDots(t *testing.T) {
	dir := t.TempDir()
	_, err := ValidateSafePathInDir(dir, "foo/../bar/../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for traversal with .. components")
	}
}

func TestValidateSafePathInDirCreatesNestedDir(t *testing.T) {
	dir := t.TempDir()
	result, err := ValidateSafePathInDir(dir, "a/b/c/d/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Clean(filepath.Join(dir, "a", "b", "c", "d", "file.txt"))
	if result != expected {
		t.Fatalf("got %s, want %s", result, expected)
	}
}

func TestValidateSafePathInDirEmptyUserPath(t *testing.T) {
	dir := t.TempDir()
	_, err := ValidateSafePathInDir(dir, "")
	if err == nil {
		t.Fatal("expected error for empty user path")
	}
}

func TestRejectNonRegularNormalFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "regular.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RejectNonRegular(f); err != nil {
		t.Fatalf("unexpected error for regular file: %v", err)
	}
}

func TestRejectNonRegularDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := RejectNonRegular(dir); err == nil {
		t.Fatal("expected error for directory")
	}
}

func TestRejectNonRegularNonExistent(t *testing.T) {
	dir := t.TempDir()
	err := RejectNonRegular(filepath.Join(dir, "nonexistent"))
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestValidateSafePathRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test requires POSIX symlinks")
	}
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.txt")
	symlink := filepath.Join(dir, "link.txt")

	if err := os.WriteFile(realFile, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFile, symlink); err != nil {
		t.Fatal(err)
	}

	err := ValidateSafePath(symlink)
	if err == nil {
		t.Fatal("expected error for symlink destination")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSafePathInDirWithSymlinkParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test requires POSIX symlinks")
	}
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	symDir := filepath.Join(dir, "link")

	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, symDir); err != nil {
		t.Fatal(err)
	}

	// Using symDir as a base should still work because ValidateSafePathInDir
	// resolves paths lexically, not through symlinks. The symlink check
	// is in ValidateSafePath (for the destination itself).
	result, err := ValidateSafePathInDir(symDir, "file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("empty result")
	}
}

func TestValidateSafePathWindowsAbsolute(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}
	dir := t.TempDir()
	_, err := ValidateSafePathInDir(dir, "C:\\Windows\\System32\\drivers\\etc\\hosts")
	if err == nil {
		t.Fatal("expected error for absolute Windows path")
	}
}

func TestValidateSafePathWindowsTraversal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific test")
	}
	dir := t.TempDir()
	_, err := ValidateSafePathInDir(dir, "..\\..\\..\\Windows\\System32")
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
}
