//go:build !windows

package cli

import (
	"context"
	"errors"
	"io"
	"math"
	"os"

	"golang.org/x/sys/unix"
)

func prepareWatchConsole(_, _ *os.File) (func() error, error) { return nil, nil }

func readWatchKey(ctx context.Context, in *os.File) (byte, error) {
	fd := int(in.Fd())
	if fd < 0 || fd > math.MaxInt32 {
		return 0, errors.New("terminal file descriptor is outside poll range")
	}
	for ctx.Err() == nil {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 50)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			return 0, io.EOF
		}
		if fds[0].Revents&unix.POLLIN != 0 {
			var key [1]byte
			n, err := unix.Read(fd, key[:])
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if err != nil {
				return 0, err
			}
			if n == 0 {
				return 0, io.EOF
			}
			return key[0], nil
		}
	}
	return 0, ctx.Err()
}
