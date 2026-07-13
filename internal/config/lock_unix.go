//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"syscall"
)

// Lock acquires an exclusive file lock on the given path.
// Creates the file if it does not exist.
func Lock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- lock path is derived from the validated config path.
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}

	return f, nil
}

// Unlock releases a file lock and closes the file.
func Unlock(f *os.File) error {
	if f == nil {
		return nil
	}
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
	return err
}
