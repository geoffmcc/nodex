// Package execsafe resolves and validates external executables before a
// caller hands one to os/exec.
//
// exec.LookPath alone is not a trust boundary: if PATH contains an
// attacker-writable directory, an unprivileged local user can drop a binary
// there and have it executed with the caller's privileges. Check rejects
// executables that are world-writable or that live in a world-writable
// directory without the sticky bit, matching the check Nodex already applies
// to ansible-playbook.
package execsafe

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// LookPath resolves name on PATH and validates the result. It returns the
// same "not found" error class as exec.LookPath when the executable is absent,
// and a validation error when the resolved path is unsafe.
func LookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("resolved executable path %q is not absolute", path)
	}
	if err := Check(path); err != nil {
		return "", err
	}
	return path, nil
}

// Check rejects executables that are world-writable or that live in a
// world-writable directory without the sticky bit. Windows permission bits are
// not meaningful, so it only checks that the path exists there.
func Check(path string) error {
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular executable %s", path)
	}
	if fi.Mode().Perm()&0o002 != 0 {
		return fmt.Errorf("refusing world-writable executable %s", path)
	}
	dir := filepath.Dir(path)
	di, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if di.Mode().Perm()&0o002 != 0 && di.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("refusing executable in world-writable directory %s", dir)
	}
	return nil
}
