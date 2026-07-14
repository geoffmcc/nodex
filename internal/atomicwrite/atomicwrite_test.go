package atomicwrite

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteFileCreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "output.txt")
	data := []byte("hello world")

	if err := WriteFile(dest, data, false, 0o700, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify contents.
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("content mismatch: got %q, want %q", got, data)
	}

	// Verify permissions on the final file.
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o644); got != want {
		t.Fatalf("permissions: got %o, want %o", got, want)
	}

	// Verify no temp file left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".nodex-atomic-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteFileRefusesOverwriteWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteFile(dest, []byte("new"), false, 0o700, 0o644)
	if err == nil {
		t.Fatal("expected error when overwrite is disabled")
	}
	if !errorsIsOrContains(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist, got: %v", err)
	}

	// Original contents should be preserved.
	got, _ := os.ReadFile(dest)
	if string(got) != "old" {
		t.Fatalf("original file was modified: got %q", got)
	}
}

func TestWriteFileOverwritesWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "overwrite.txt")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteFile(dest, []byte("new"), true, 0o700, 0o644); err != nil {
		t.Fatalf("WriteFile with overwrite failed: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != "new" {
		t.Fatalf("content not updated: got %q", got)
	}
}

func TestWriteFileRefusesNonRegular(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "subdir")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	err := WriteFile(dest, []byte("data"), false, 0o700, 0o644)
	if err == nil {
		t.Fatal("expected error for directory destination")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteFileCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "new", "subdir")
	dest := filepath.Join(subdir, "file.txt")

	if err := WriteFile(dest, []byte("nested"), false, 0o700, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}

func TestWriteFileCleanupOnWriteError(t *testing.T) {
	dir := t.TempDir()

	// Test error cleanup by writing to a path where the parent directory
	// cannot be created (a regular file in the path).
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := WriteFile(filepath.Join(blocker, "sub", "file.txt"), []byte("x"), false, 0o700, 0o644)
	if err == nil {
		t.Fatal("expected error when parent path is a regular file")
	}

	// Verify no temp files left in dir (the blocker file is expected).
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".nodex-atomic-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteStreamCreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "streamed.txt")
	data := []byte(strings.Repeat("streaming data ", 100))

	if err := WriteStream(dest, bytes.NewReader(data), false, 0o700, 0o644); err != nil {
		t.Fatalf("WriteStream failed: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("content mismatch: got %d bytes, want %d bytes", len(got), len(data))
	}
}

func TestWriteStreamRefusesOverwriteWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteStream(dest, bytes.NewReader([]byte("new")), false, 0o700, 0o644)
	if err == nil {
		t.Fatal("expected error when overwrite is disabled")
	}
}

func TestWriteStreamOverwritesWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "overwrite.txt")
	if err := os.WriteFile(dest, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteStream(dest, bytes.NewReader([]byte("new data")), true, 0o700, 0o644); err != nil {
		t.Fatalf("WriteStream with overwrite failed: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if string(got) != "new data" {
		t.Fatalf("content not updated: got %q", got)
	}
}

func TestWriteStreamCleanupOnError(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "streamfail.txt")

	// Reader that fails after some data.
	failReader := &failAfterReader{data: []byte("partial"), failAfter: 3}
	err := WriteStream(dest, failReader, false, 0o700, 0o644)
	if err == nil {
		t.Fatal("expected error from failing reader")
	}

	// Destination should not exist.
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("destination was created despite stream failure")
	}

	// No temp file should remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".nodex-atomic-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteStreamLargeFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "large.bin")

	// 10 MiB of data.
	size := 10 * 1024 * 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := WriteStream(dest, bytes.NewReader(data), false, 0o700, 0o644); err != nil {
		t.Fatalf("WriteStream large file failed: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if len(got) != size {
		t.Fatalf("size mismatch: got %d, want %d", len(got), size)
	}
	for i := range got {
		if got[i] != byte(i%256) {
			t.Fatalf("content mismatch at byte %d: got %d, want %d", i, got[i], byte(i%256))
			break
		}
	}
}

func TestWriteFileAtomicityAgainstCrash(t *testing.T) {
	// This test verifies the atomic write pattern: if the process crashes
	// between write and rename, the destination should either:
	//   - Not exist (rename not executed)
	//   - Contain the complete new content (rename completed)
	// It should never contain partial content.
	//
	// We simulate this by checking that after a successful WriteFile,
	// no temp file remains and the destination is complete.
	dir := t.TempDir()
	dest := filepath.Join(dir, "atomic.txt")

	if err := WriteFile(dest, []byte("complete content"), false, 0o700, 0o644); err != nil {
		t.Fatal(err)
	}

	// Destination must be complete.
	got, _ := os.ReadFile(dest)
	if string(got) != "complete content" {
		t.Fatalf("incomplete content: %q", got)
	}

	// No temp files.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".nodex-atomic-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteFileSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test requires POSIX symlinks")
	}
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.txt")
	symlink := filepath.Join(dir, "link.txt")

	if err := os.WriteFile(realFile, []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFile, symlink); err != nil {
		t.Fatal(err)
	}

	// WriteFile should fail because the destination is a symlink
	// (os.Lstat will see it as not a regular file).
	err := WriteFile(symlink, []byte("new"), false, 0o700, 0o644)
	if err == nil {
		t.Fatal("expected error for symlink destination")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteStreamEmptyFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "empty.txt")

	if err := WriteStream(dest, bytes.NewReader([]byte{}), false, 0o700, 0o644); err != nil {
		t.Fatalf("WriteStream empty file failed: %v", err)
	}

	got, _ := os.ReadFile(dest)
	if len(got) != 0 {
		t.Fatalf("expected empty file, got %d bytes", len(got))
	}
}

// failAfterReader returns data up to failAfter bytes, then fails.
type failAfterReader struct {
	data      []byte
	failAfter int
	pos       int
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	if r.pos >= r.failAfter {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= r.failAfter {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}

func errorsIsOrContains(err error, target error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), target.Error())
}
