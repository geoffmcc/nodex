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
// On Windows, os.Rename fails if dest already exists and is open. This
// function handles the overwrite by removing an existing regular-file dest
// before the rename when overwrite is true. When overwrite is false and dest
// exists, os.ErrExist is returned.
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

// replaceFile handles the rename and overwrite logic.
func replaceFile(tmpPath, dest string, overwrite bool) error {
	if overwrite {
		// On Windows, os.Rename fails if the target exists (even as a
		// regular file), so remove it first when overwriting.
		info, err := os.Lstat(dest)
		if err == nil {
			if info.Mode().IsRegular() {
				if err := os.Remove(dest); err != nil {
					return fmt.Errorf("atomic write: remove existing file %s: %w", dest, err)
				}
			}
			// Non-regular files are left untouched; Rename will fail
			// with an appropriate error.
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("atomic write: stat destination: %w", err)
		}
	} else {
		// When overwriting is not allowed, check for existence.
		if info, err := os.Lstat(dest); err == nil {
			if info.Mode().IsRegular() {
				return fmt.Errorf("%w: %s already exists (use overwrite to replace)", os.ErrExist, dest)
			}
			return fmt.Errorf("%w: %s exists and is not a regular file", os.ErrExist, dest)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("atomic write: stat destination: %w", err)
		}
	}

	if err := os.Rename(tmpPath, dest); err != nil {
		return fmt.Errorf("atomic write: rename to destination: %w", err)
	}
	return nil
}
