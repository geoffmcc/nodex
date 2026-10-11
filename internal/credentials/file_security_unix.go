//go:build !windows

package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func checkCredentialDirectory(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve credential directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect credential directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("credential directory %s must be a real directory; remove symlinks and select a private local directory", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("credential directory %s must be owned by the current user", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("credential directory %s has permissions %04o; restrict it to the current user (recommended 0700) or use the OS keyring backend", path, info.Mode().Perm())
	}
	// A writable ancestor could let another user replace the private directory
	// between validation and use. A root-owned sticky directory (for example
	// /tmp) is the only accepted writable ancestor.
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		parentInfo, statErr := os.Lstat(parent)
		if statErr != nil {
			return fmt.Errorf("credential directory ancestor %s is missing, not a directory, or a symlink", parent)
		}
		if parentInfo.Mode()&os.ModeSymlink != 0 {
			parentStat, ok := parentInfo.Sys().(*syscall.Stat_t)
			if !ok || parentStat.Uid != 0 {
				return fmt.Errorf("credential directory ancestor %s is a non-root-owned symlink", parent)
			}
			resolved, resolveErr := filepath.EvalSymlinks(parent)
			if resolveErr != nil {
				return fmt.Errorf("resolve credential directory ancestor %s: %w", parent, resolveErr)
			}
			parent = resolved
			parentInfo, statErr = os.Lstat(parent)
			if statErr != nil || parentInfo.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("credential directory ancestor target %s is invalid", parent)
			}
		}
		if !parentInfo.IsDir() {
			return fmt.Errorf("credential directory ancestor %s is not a directory", parent)
		}
		if parentInfo.Mode().Perm()&0o022 != 0 {
			parentStat, ok := parentInfo.Sys().(*syscall.Stat_t)
			if !ok || parentStat.Uid != 0 || parentInfo.Mode()&os.ModeSticky == 0 {
				return fmt.Errorf("credential directory ancestor %s is group/world writable; choose a private directory", parent)
			}
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	return nil
}

func openCredentialFile(path string) (*os.File, error) {
	if err := checkCredentialDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if err := validateCredentialFileHandle(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func protectCredentialFile(f *os.File) error {
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	return validateCredentialFileHandle(f)
}

func validateCredentialFileHandle(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened credential file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("credential path is not a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("credential file must be owned by the current user")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("credential file has permissions %04o; restrict it to the current user (recommended 0600) or use the OS keyring backend", info.Mode().Perm())
	}
	return nil
}

func checkCredentialFile(path string) error {
	f, err := openCredentialFile(path)
	if err != nil {
		return err
	}
	return f.Close()
}
