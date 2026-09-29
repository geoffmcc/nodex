//go:build !windows

package agent

import (
	"errors"
	"os"
	"syscall"
)

func checkReceiptOwner(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return errors.New("receipt is not owned by the current user")
	}
	return nil
}
