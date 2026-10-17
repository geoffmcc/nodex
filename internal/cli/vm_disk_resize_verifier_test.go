package cli

import (
	"context"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

type vmDiskResizeReadbackProvider struct {
	bareProvider
	config map[string]interface{}
	err    error
}

func (p *vmDiskResizeReadbackProvider) VMConfig(context.Context, string, int) (map[string]interface{}, error) {
	return p.config, p.err
}

func (p *vmDiskResizeReadbackProvider) VMs(context.Context) ([]domain.VM, error) {
	return nil, nil
}

func TestVMDiskResizeVerifier(t *testing.T) {
	const before = "local-lvm:vm-100-disk-0,size=1G"
	tests := []struct {
		name         string
		request      string
		observed     string
		wantVerified bool
		wantUnverif  bool
		wantChanged  *bool
	}{
		{name: "relative resize reached", request: "+1G", observed: "local-lvm:vm-100-disk-0,size=2G", wantVerified: true, wantChanged: boolPointer(true)},
		{name: "relative resize short", request: "+1G", observed: "local-lvm:vm-100-disk-0,size=1G"},
		{name: "absolute size reached", request: "2G", observed: "local-lvm:vm-100-disk-0,size=2G", wantVerified: true, wantChanged: boolPointer(true)},
		{name: "missing disk size", request: "+1G", observed: "local-lvm:vm-100-disk-0", wantUnverif: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &vmDiskResizeReadbackProvider{config: map[string]interface{}{"scsi0": tt.observed}}
			out := vmDiskResizeVerifier(provider, "pve1", 100, "scsi0", tt.request, map[string]interface{}{"scsi0": before}, nil)(context.Background())
			if out.Verified != tt.wantVerified || out.Unverifiable != tt.wantUnverif {
				t.Fatalf("outcome = %#v", out)
			}
			if tt.wantChanged == nil {
				if out.Changed != nil {
					t.Fatalf("Changed = %v, want nil", *out.Changed)
				}
			} else if out.Changed == nil || *out.Changed != *tt.wantChanged {
				t.Fatalf("Changed = %v, want %v", out.Changed, *tt.wantChanged)
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }
