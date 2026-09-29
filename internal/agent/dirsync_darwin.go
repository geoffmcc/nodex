//go:build darwin

package agent

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Darwin File.Sync uses F_FULLFSYNC, which is appropriate for regular files
// but is not a portable directory fsync operation. Invoke fsync on the
// validated directory descriptor to persist the rename entry.
func syncReceiptDirectory(dir string) error {
	f, err := os.Open(dir) // #nosec G304 -- dir is a validated private receipt directory.
	if err != nil {
		return fmt.Errorf("open receipt directory for sync: %w", err)
	}
	if err := unix.Fsync(int(f.Fd())); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync receipt directory: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close receipt directory after sync: %w", err)
	}
	return nil
}
