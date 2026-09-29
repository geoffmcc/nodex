//go:build !windows

package atomicwrite

import "os"

func platformRename(tmpPath, dest string, _ bool) error {
	return os.Rename(tmpPath, dest)
}
