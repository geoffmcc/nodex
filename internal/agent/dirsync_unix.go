//go:build !windows

package agent

import (
	"fmt"
	"os"
)

func syncReceiptDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open receipt directory for sync: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync receipt directory: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close receipt directory after sync: %w", err)
	}
	return nil
}
