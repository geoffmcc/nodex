// Package pathvalidate provides filesystem-path safety checks.
//
// It defends against:
//   - Path-traversal attacks (e.g. "../../etc/passwd").
//   - Symlink attacks that redirect writes outside an expected directory.
//   - Non-regular destination files (devices, FIFOs, directories).
//
// These checks are appropriate for CLI tools that accept user-supplied
// output paths and for internal file operations where a compromised path
// could lead to data loss or privilege escalation.
package pathvalidate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrUnsafePath is returned when a path fails safety validation.
var ErrUnsafePath = errors.New("unsafe path")

// ValidateSafePath checks that dest is safe for file creation.
//
// Safety rules:
//   - dest must be a clean path (no ".." traversal that escapes).
//   - dest must not be a symlink on platforms that support symlinks.
//   - dest must not be a directory, device, or other non-regular file
//     (unless it does not exist, which is the normal case for new files).
//
// When dest does not exist, the parent directory is also checked for
// symlinks (on platforms that support them).
//
// This function uses os.Lstat (does not follow symlinks) to check for
// existing non-regular files.
func ValidateSafePath(dest string) error {
	cleaned := filepath.Clean(dest)

	// Reject empty and "." paths after cleaning.
	if cleaned == "." || cleaned == "" {
		return fmt.Errorf("%w: empty or relative-only path", ErrUnsafePath)
	}

	// Check for path traversal: after cleaning, the path should not
	// contain ".." components that escape the intended tree.  The
	// cleanest check is to verify that filepath.Clean did not reintroduce
	// ".." and that the result is absolute or rooted within a safe base.
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("%w: path contains traversal components: %s", ErrUnsafePath, dest)
	}

	// Check the destination itself for unsafe file types.
	if info, err := os.Lstat(cleaned); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s is not a regular file (type: %s)", ErrUnsafePath, dest, modeType(info.Mode()))
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%w: cannot stat destination: %w", ErrUnsafePath, err)
	}
	// If destination does not exist, that's acceptable — the parent
	// directory will be checked below.

	// On platforms with symlinks, check that the resolved parent directory
	// is not itself a symlink pointing outside the expected location.
	if runtime.GOOS != "windows" {
		parent := filepath.Dir(cleaned)
		if err := checkNoSymlink(parent); err != nil {
			return err
		}
	}

	return nil
}

// ValidateSafePathInDir checks that a user-supplied path is safely contained
// within baseDir. It resolves the full path by joining baseDir with
// userPath, cleans it, and verifies that the result stays within baseDir.
//
// This is the primary defense against path-traversal attacks when a user
// provides a relative path intended for a specific directory.
func ValidateSafePathInDir(baseDir, userPath string) (string, error) {
	cleanedBase := filepath.Clean(baseDir)

	// Reject empty user paths.
	if userPath == "" {
		return "", fmt.Errorf("%w: empty path not allowed", ErrUnsafePath)
	}

	// Reject userPath that is absolute or starts with a platform volume.
	if filepath.IsAbs(userPath) {
		return "", fmt.Errorf("%w: absolute path not allowed: %s", ErrUnsafePath, userPath)
	}

	// Build the full path.
	full := filepath.Join(cleanedBase, userPath)
	cleaned := filepath.Clean(full)

	// After cleaning, the result must still start with cleanedBase.
	// This catches ".." traversal.
	rel, err := filepath.Rel(cleanedBase, cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: cannot compute relative path: %w", ErrUnsafePath, err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%w: path escapes base directory: %s", ErrUnsafePath, userPath)
	}

	// Check for any remaining ".." components.
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("%w: path contains traversal components: %s", ErrUnsafePath, userPath)
	}

	return cleaned, nil
}

// RejectNonRegular checks that path is a regular file suitable for reading.
// It uses os.Lstat to avoid following symlinks.
func RejectNonRegular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file (type: %s)", path, modeType(info.Mode()))
	}
	return nil
}

// checkNoSymlink verifies that path does not resolve through a symlink.
// Only meaningful on platforms with symlink support (non-Windows).
func checkNoSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Parent doesn't exist yet — that's fine.
		}
		return fmt.Errorf("%w: cannot stat path: %w", ErrUnsafePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: path is a symlink: %s", ErrUnsafePath, path)
	}
	return nil
}

// modeType returns a human-readable description of an os.FileMode type.
func modeType(m os.FileMode) string {
	switch {
	case m.IsDir():
		return "directory"
	case m.IsRegular():
		return "regular file"
	case m&os.ModeSymlink != 0:
		return "symlink"
	case m&os.ModeDevice != 0:
		return "device"
	case m&os.ModeNamedPipe != 0:
		return "named pipe"
	case m&os.ModeSocket != 0:
		return "socket"
	case m&os.ModeCharDevice != 0:
		return "character device"
	default:
		return fmt.Sprintf("mode %o", m)
	}
}
