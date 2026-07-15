package proxmox

import (
	"testing"

	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
)

func TestMapNodeNetworkUsesPVE9Fields(t *testing.T) {
	got := mapNodeNetwork(client.NodeNetworkItem{
		Iface:  "vmbr0",
		Type:   "bridge",
		Active: 1,
		CIDR:   "10.47.60.200/24",
	})

	if got.Name != "vmbr0" {
		t.Fatalf("Name = %q, want vmbr0", got.Name)
	}
	if got.Status != "active" {
		t.Fatalf("Status = %q, want active", got.Status)
	}
	if got.IP != "10.47.60.200/24" {
		t.Fatalf("IP = %q, want CIDR", got.IP)
	}
}
