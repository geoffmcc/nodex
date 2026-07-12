//go:build windows

package config

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
)

// Lock acquires an exclusive file lock on the given path.
// Creates the file if it does not exist.
func Lock(path string) (*os.File, error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	if err := lockFile(f); err != nil {
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
	err := unlockFile(f)
	f.Close()
	return err
}

func lockFile(f *os.File) error {
	var overlap syscall.Overlapped
	ret, _, err := procLockFileEx.Call(
		f.Fd(),
		lockfileExclusiveLock,
		0,
		1, 0,
		uintptr(unsafe.Pointer(&overlap)),
	)
	if ret == 0 {
		return err
	}
	return nil
}

func unlockFile(f *os.File) error {
	var overlap syscall.Overlapped
	ret, _, err := procUnlockFileEx.Call(
		f.Fd(),
		0,
		1, 0,
		uintptr(unsafe.Pointer(&overlap)),
	)
	if ret == 0 {
		return err
	}
	return nil
}
