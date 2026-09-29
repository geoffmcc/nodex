//go:build windows

package atomicwrite

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// platformRename uses same-volume MoveFileEx replacement so an existing
// destination is not deleted before its replacement is committed.
func platformRename(tmpPath, dest string, overwrite bool) error {
	if !overwrite {
		return os.Rename(tmpPath, dest)
	}
	from, err := windows.UTF16PtrFromString(tmpPath)
	if err != nil {
		return fmt.Errorf("atomic write: encode temporary path: %w", err)
	}
	to, err := windows.UTF16PtrFromString(dest)
	if err != nil {
		return fmt.Errorf("atomic write: encode destination path: %w", err)
	}
	flags := uint32(windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH)
	if err := windows.MoveFileEx(from, to, flags); err != nil {
		return fmt.Errorf("atomic write: atomically replace destination: %w", err)
	}
	return nil
}
