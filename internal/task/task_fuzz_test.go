package task

import (
	"testing"
)

// FuzzParseUPID tests UPID parsing at the trust boundary where untrusted
// task IDs arrive from Proxmox API responses or CLI input.
// It must never panic, must not crash on any input, and must return either
// a valid UPID or an error (never a nil UPID with nil error).
func FuzzParseUPID(f *testing.F) {
	// Seed corpus with valid Proxmox UPID formats and edge cases.
	seeds := []string{
		// Valid full format.
		"UPID:pve1:00000A1B:0023A45B:6789ABCD:vzdump:100:root@pam:",
		// Valid minimal colon format.
		"UPID:proxmox:00012345",
		// Valid slash format.
		"UPID:proxmox/00012345/0",
		"UPID:pve1/100/1700000000",
		// Edge cases.
		"",
		"UPID:",
		"UPID::",
		"not-a-upid",
		"UPID",
		"UPID:pve1:",
		"UPID:pve1:nothex",
		"UPID:pve1:FFFFFFFF",
		"UPID:pve1/",
		"UPID:pve1/abc",
		"UPID:pve1/99999999999999999999999",
		"UPID:pve1/1/99999999999999999999999",
		// Control characters and special inputs.
		"UPID:pve1:\x00",
		"UPID:pve1:\n",
		"UPID:pve1:\t:",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		// Guard against excessively large inputs.
		if len(raw) > 4096 {
			return
		}

		// ParseUPID must never panic.
		u, err := ParseUPID(raw)

		// If err is nil, u must be non-nil and have its Raw field set.
		if err == nil {
			if u == nil {
				t.Errorf("ParseUPID(%q) returned nil UPID without error", raw)
				return
			}
			if u.Raw != raw {
				t.Errorf("ParseUPID(%q) returned UPID with Raw=%q", raw, u.Raw)
			}
		}
	})
}
