//go:build !windows

package client

import (
	"fmt"
	"os"
	"syscall"
)

// openRegularFileNoFollow opens the final path component without following a
// symlink. Inspecting the opened descriptor keeps the file-type and size
// checks bound to the object that will actually be uploaded.
func openRegularFileNoFollow(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open without following symlinks: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened file: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("%s is not a regular file (symlinks, directories, and special files are not supported)", path)
	}
	return file, nil
}
