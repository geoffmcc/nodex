//go:build !windows

package config

import (
	"os"
	"syscall"
)

// Lock acquires an exclusive file lock on the given path.
// Creates the file if it does not exist.
func Lock(path string) (*os.File, error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
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
	f.Close()
	return err
}
