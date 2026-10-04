package proxmox

import (
	"context"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
)

// GuestStatus bridges the domain-facing guest names to the transport path
// segments. A caller using the domain vocabulary must not have to know Proxmox's
// internal names, and an unknown guest type must be refused rather than guessed.
func TestGuestStatusRejectsUnknownGuestType(t *testing.T) {
	p := &Provider{}
	if _, err := p.GuestStatus(context.Background(), "pve1", "storage", 101); err == nil ||
		!strings.Contains(err.Error(), "unsupported guest type") {
		t.Fatalf("GuestStatus error = %v, want an unsupported guest type error", err)
	}
}

func TestGuestStatusRequiresConnection(t *testing.T) {
	p := &Provider{}
	if _, err := p.GuestStatus(context.Background(), "pve1", "vm", 101); err == nil {
		t.Fatal("expected an error when the provider is not connected")
	}
}

func TestGuestStatusSegmentMapsDomainNames(t *testing.T) {
	tests := map[string]string{
		"vm":        "qemu",
		"qemu":      "qemu",
		"VM":        "qemu",
		"container": "lxc",
		"ct":        "lxc",
		"lxc":       "lxc",
		"Container": "lxc",
	}
	for in, want := range tests {
		got, err := guestStatusSegment(in)
		if err != nil {
			t.Fatalf("guestStatusSegment(%q) error = %v", in, err)
		}
		if got != want {
			t.Fatalf("guestStatusSegment(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := guestStatusSegment("node"); err == nil {
		t.Fatal("expected an error for a non-guest type")
	}
}

func TestReportedGuestStatusUsesQMPStateForRunningVMs(t *testing.T) {
	tests := []struct {
		name      string
		guestType string
		status    client.GuestStatusData
		want      string
	}{
		{
			name:      "paused QEMU process",
			guestType: "qemu",
			status:    client.GuestStatusData{Status: "running", QMPStatus: "paused"},
			want:      "paused",
		},
		{
			name:      "running QEMU process",
			guestType: "qemu",
			status:    client.GuestStatusData{Status: "running", QMPStatus: "running"},
			want:      "running",
		},
		{
			name:      "stopped QEMU process",
			guestType: "qemu",
			status:    client.GuestStatusData{Status: "stopped", QMPStatus: "paused"},
			want:      "stopped",
		},
		{
			name:      "container ignores QMP state",
			guestType: "lxc",
			status:    client.GuestStatusData{Status: "running", QMPStatus: "paused"},
			want:      "running",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reportedGuestStatus(tt.guestType, tt.status); got != tt.want {
				t.Fatalf("reportedGuestStatus(%q, %+v) = %q, want %q", tt.guestType, tt.status, got, tt.want)
			}
		})
	}
}

// The per-guest read is the authoritative source for one guest, so the provider
// must advertise the capability that callers use to detect it.
func TestProviderExposesGuestStatusCapability(t *testing.T) {
	var p any = &Provider{}
	if _, ok := p.(domain.GuestStatusInspector); !ok {
		t.Fatal("Proxmox provider must implement domain.GuestStatusInspector")
	}
}
