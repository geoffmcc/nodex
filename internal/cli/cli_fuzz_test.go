package cli

import (
	"testing"
)

// FuzzParseNodeVMID tests the node/VMID parsing at the trust boundary
// where user-supplied target identifiers arrive from CLI arguments.
// The format is "<node>/<vmid>". It must never panic and must return
// an error for invalid inputs.
func FuzzParseNodeVMID(f *testing.F) {
	seeds := []string{
		// Valid forms.
		"pve1/100",
		"proxmox/1",
		"node-name/999999",
		// Edge cases.
		"",
		"/",
		"node/",
		"/100",
		"node/0",
		"node/-1",
		"node/abc",
		"node/100/extra",
		"node/9999999999999999999", // overflow
		"node/99999999999999999999",
		// Special characters.
		"node/100\x00",
		"node\x00/100",
		"node/\n100",
		"\t/100",
		// Extremely long inputs.
		"node-with-very-long-name-that-exceeds-reasonable-limits/100",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, arg string) {
		if len(arg) > 2048 {
			return
		}

		// Must never panic.
		node, vmid, err := parseNodeVMID(arg)

		// If no error, result must be consistent.
		if err == nil {
			if node == "" {
				t.Errorf("parseNodeVMID(%q) returned empty node without error", arg)
			}
			if vmid <= 0 {
				t.Errorf("parseNodeVMID(%q) returned vmid=%d (non-positive) without error", arg, vmid)
			}
		}
	})
}

// FuzzParseKeyValueArgs tests key=value argument parsing at the trust
// boundary where CLI arguments for VM/container updates arrive.
func FuzzParseKeyValueArgs(f *testing.F) {
	// We can't fuzz with a slice directly using Go native fuzzing,
	// but we can convert a single string representation to args.
	// For simplicity, test with single-element slices.

	f.Fuzz(func(t *testing.T, arg string) {
		if len(arg) > 4096 {
			return
		}

		// Test single argument parsing (most common case).
		args := []string{arg}
		// Must never panic.
		_, _ = parseKeyValueArgs(args)

		// Test with multiple fuzzed args combined.
		if len(arg) > 0 && len(arg) < 512 {
			multiArgs := []string{arg, "second=value", arg}
			_, _ = parseKeyValueArgs(multiArgs)
		}
	})
}
