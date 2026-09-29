//go:build windows

package agent

import "os"

// Windows ownership is governed by the ACL inherited from the per-user
// configuration directory; the standard library exposes no portable SID
// comparison for os.FileInfo.
func checkReceiptOwner(os.FileInfo) error { return nil }
