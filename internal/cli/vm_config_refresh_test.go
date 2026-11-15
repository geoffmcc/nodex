package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
)

type vmConfigRefreshProbe struct {
	*e2eMockProvider
	config map[string]interface{}
	err    error
}

func (p *vmConfigRefreshProbe) VMConfig(context.Context, string, int) (map[string]interface{}, error) {
	return p.config, p.err
}

func TestVMShowRefreshesConfiguredLimits(t *testing.T) {
	cases := []struct {
		name         string
		config       map[string]interface{}
		err          error
		cpu          int
		memory, disk int64
	}{
		{name: "recent update", config: map[string]interface{}{"cores": 2, "sockets": "2", "memory": 320, "scsi0": "local-lvm:vm-1-disk-0,size=2G", "ide2": "local:iso/a.iso,media=cdrom,size=9G"}, cpu: 4, memory: 320 << 20, disk: 2 << 30},
		{name: "multiple disks retain aggregate", config: map[string]interface{}{"scsi0": "local:a,size=2G", "scsi1": "local:b,size=4G"}, cpu: 1, memory: 256 << 20, disk: 1 << 30},
		{name: "invalid sockets retain CPU", config: map[string]interface{}{"cores": 2, "sockets": "invalid"}, cpu: 1, memory: 256 << 20, disk: 1 << 30},
		{name: "config unavailable", err: errors.New("permission denied"), cpu: 1, memory: 256 << 20, disk: 1 << 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vm := domain.VM{ID: "pve/998174", CPU: 1, Memory: 256 << 20, Disk: 1 << 30}
			p := &vmConfigRefreshProbe{config: tc.config, err: tc.err}
			enrichVMConfig(context.Background(), p, &vm)
			if vm.CPU != tc.cpu || vm.Memory != tc.memory || vm.Disk != tc.disk {
				t.Fatalf("got CPU %d memory %d disk %d", vm.CPU, vm.Memory, vm.Disk)
			}
		})
	}
}
