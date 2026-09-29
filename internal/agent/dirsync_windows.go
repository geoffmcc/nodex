//go:build windows

package agent

// Directory handles cannot be synced portably via the standard library on
// Windows. Atomic replacement still uses atomicwrite's synced temporary file.
func syncReceiptDirectory(string) error { return nil }
