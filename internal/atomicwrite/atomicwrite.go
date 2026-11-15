// Package atomicwrite provides safe atomic file-writing helpers.
//
// All functions use the write-to-temp-then-rename pattern:
//  1. Create a temporary file in the same directory as the destination.
//  2. Write all data to the temporary file.
//  3. Sync to ensure durability.
//  4. Close the temporary file.
//  5. Atomically rename the temporary file to the destination.
//
// On failure the temporary file is removed. The destination is never left
// with partial or corrupt content.
package atomicwrite

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// WriteFile atomically writes data to dest using a temp-file + rename.
//
// The destination directory is created with dirPerm if it does not exist.
// The temporary file is created with filePerm (applied via Chmod after
// creation), synced, closed, and then atomically renamed to dest.
//
// On Windows, overwrite uses same-volume MoveFileEx replacement with
// MOVEFILE_WRITE_THROUGH instead of removing the old file before rename.
// When overwrite is false and dest exists, os.ErrExist is returned.
func WriteFile(dest string, data []byte, overwrite bool, dirPerm, filePerm os.FileMode) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("atomic write: create directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".nodex-atomic-*")
	if err != nil {
		return fmt.Errorf("atomic write: create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if runtime.GOOS != "windows" {
		if err := tmp.Chmod(filePerm); err != nil {
			return fmt.Errorf("atomic write: chmod temp file: %w", err)
		}
	}

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("atomic write: write temp file: %w", err)
	}

	// On Windows, Sync may fail on some filesystems (e.g., temp directories).
	// Treat sync errors as non-fatal; Close still ensures data integrity.
	if runtime.GOOS != "windows" {
		if err := tmp.Sync(); err != nil {
			return fmt.Errorf("atomic write: sync temp file: %w", err)
		}
	} else {
		_ = tmp.Sync()
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomic write: close temp file: %w", err)
	}

	if err := replaceFile(tmpPath, dest, overwrite); err != nil {
		return err
	}

	success = true
	return nil
}

// WriteStream atomically writes the contents of src to dest using a
// temp-file + rename. The data is streamed (not buffered entirely in
// memory), making it suitable for large files.
//
// The temporary file is created in the same directory as dest. After the
// stream completes successfully the file is synced, closed, and renamed.
// On failure the temp file is cleaned up.
//
// The caller is responsible for closing src.
func WriteStream(dest string, src io.Reader, overwrite bool, dirPerm, filePerm os.FileMode) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("atomic write: create directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".nodex-atomic-*")
	if err != nil {
		return fmt.Errorf("atomic write: create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if runtime.GOOS != "windows" {
		if err := tmp.Chmod(filePerm); err != nil {
			return fmt.Errorf("atomic write: chmod temp file: %w", err)
		}
	}

	// Stream data through a LimitReader when size limits are needed.
	// The caller should wrap src with LimitReader before passing it in.
	if _, err := io.Copy(tmp, src); err != nil {
		return fmt.Errorf("atomic write: stream to temp file: %w", err)
	}

	// On Windows, Sync may fail on some filesystems (e.g., temp directories).
	if runtime.GOOS != "windows" {
		if err := tmp.Sync(); err != nil {
			return fmt.Errorf("atomic write: sync temp file: %w", err)
		}
	} else {
		_ = tmp.Sync()
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomic write: close temp file: %w", err)
	}

	if err := replaceFile(tmpPath, dest, overwrite); err != nil {
		return err
	}

	success = true
	return nil
}

// Commit publishes an already-written temporary file at dest.
//
// It is the commit step of the temp-file + rename pattern for callers that
// stream or otherwise produce the temporary file themselves (for example a
// download written directly to disk). tmpPath must be on the same filesystem
// as dest, normally in the same directory.
//
// With overwrite false the destination is never replaced. Creating the
// destination link is atomic, so a destination created after
// the caller's own pre-flight check is refused rather than clobbered. This
// matters for commands whose --force decision is made before a potentially
// long transfer.
//
// On failure dest is left untouched and tmpPath is left in place for the
// caller to clean up. After a successful no-replace commit, removal of the
// temporary link is best effort; callers may clean it up again.
func Commit(tmpPath, dest string, overwrite bool) error {
	if tmpPath == "" || dest == "" {
		return fmt.Errorf("atomic write: commit requires both source and destination paths")
	}
	if filepath.Dir(filepath.Clean(tmpPath)) != filepath.Dir(filepath.Clean(dest)) {
		return fmt.Errorf("atomic write: temporary file %s and destination %s must share a directory", tmpPath, dest)
	}
	if err := checkDestination(dest, overwrite); err != nil {
		return err
	}
	if err := commitFile(tmpPath, dest, overwrite); err != nil {
		return destError(dest, fmt.Errorf("atomic write: commit destination: %w", err))
	}
	return nil
}

// replaceFile handles the rename and overwrite logic for WriteFile and
// WriteStream.
func replaceFile(tmpPath, dest string, overwrite bool) error {
	if err := checkDestination(dest, overwrite); err != nil {
		return err
	}
	if err := platformRename(tmpPath, dest, overwrite); err != nil {
		return fmt.Errorf("atomic write: rename to destination: %w", err)
	}
	return nil
}

// checkDestination rejects destinations that cannot be replaced safely. The
// check provides an early error; Commit also enforces no-replace atomically.
func checkDestination(dest string, overwrite bool) error {
	info, err := os.Lstat(dest)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("atomic write: stat destination: %w", err)
	}
	if !info.Mode().IsRegular() {
		if overwrite {
			return fmt.Errorf("atomic write: destination %s exists and is not a regular file", dest)
		}
		return fmt.Errorf("%w: %s exists and is not a regular file", os.ErrExist, dest)
	}
	if !overwrite {
		return fmt.Errorf("%w: %s already exists (use overwrite to replace)", os.ErrExist, dest)
	}
	return nil
}

// destError adds an actionable message when the atomic no-replace commit lost
// a race with another writer. The wrapped error still matches os.ErrExist.
func destError(dest string, err error) error {
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%w: %s already exists (use overwrite to replace)", os.ErrExist, dest)
	}
	return err
}

// commitFile uses a hard link as an atomic create-if-absent operation. The
// temporary file and destination share a directory, hence a filesystem.
func commitFile(tmpPath, dest string, overwrite bool) error {
	if overwrite {
		return platformRename(tmpPath, dest, true)
	}
	if err := os.Link(tmpPath, dest); err != nil {
		return err
	}
	// The destination is committed. Cleanup failure must not report a failed commit.
	_ = os.Remove(tmpPath)
	return nil
}
