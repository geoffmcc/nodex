package cli

import (
	"strings"
	"testing"
)

// P2-B: help is one of the three surfaces that must agree with execution on
// the confirmation value. A destructive leaf that hides --confirm-target, or
// omits a worked example, leaves a caller guessing and then getting refused.
func TestHelp_TypeConfirmOperationsDocumentConfirmationFlags(t *testing.T) {
	// These are the operations proven non-interactive in the Nits-4 evidence,
	// where the missing flag syntax was the reported defect.
	tests := []struct{ path, wantTarget string }{
		{"vm delete", "--confirm-target <node>/<vmid>"},
		{"vm snapshot delete", "--confirm-target <name>"},
		{"container delete", "--confirm-target <node>/<vmid>"},
		{"container snapshot delete", "--confirm-target <name>"},
	}
	for _, tt := range tests {
		entry, ok := leafHelp[tt.path]
		if !ok {
			t.Errorf("%q has no help entry", tt.path)
			continue
		}
		for _, flag := range []string{"--yes", "--force", tt.wantTarget} {
			if !strings.Contains(entry.usage, flag) {
				t.Errorf("%q usage %q omits %q", tt.path, entry.usage, flag)
			}
		}
		if len(entry.examples) == 0 {
			t.Errorf("%q publishes no worked example", tt.path)
			continue
		}
		if !strings.Contains(strings.Join(entry.examples, "\n"), "--confirm-target") {
			t.Errorf("%q example %v omits --confirm-target", tt.path, entry.examples)
		}
	}
}

// The published examples must name a value of the documented shape, otherwise
// they teach the caller the wrong string.
func TestHelp_ConfirmationExampleMatchesPublishedFormat(t *testing.T) {
	for _, tt := range []struct{ path, marker string }{
		{"vm delete", "proxmox/100"},
		{"vm snapshot delete", "pre-upgrade"},
		{"container delete", "proxmox/9610"},
		{"container snapshot delete", "nx4fu-snap1"},
	} {
		entry := leafHelp[tt.path]
		if !strings.Contains(strings.Join(entry.examples, "\n"), tt.marker) {
			t.Errorf("%q examples %v do not demonstrate %q", tt.path, entry.examples, tt.marker)
		}
	}
}
