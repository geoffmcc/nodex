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

func TestMapNodeNetworkBridgeAndVLAN(t *testing.T) {
	bridge := mapNodeNetwork(client.NodeNetworkItem{
		Iface:           "vmbr1",
		Type:            "bridge",
		Active:          1,
		BridgePorts:     "eno1 eno2",
		BridgeVLANAware: 1,
		BridgeVids:      "2-4094",
		MTU:             9000,
		Comments:        "management bridge",
	})
	if bridge.BridgePorts != "eno1 eno2" {
		t.Errorf("BridgePorts = %q, want eno1 eno2", bridge.BridgePorts)
	}
	if !bridge.BridgeVLANAware {
		t.Error("BridgeVLANAware = false, want true")
	}
	if bridge.BridgeVLANs != "2-4094" {
		t.Errorf("BridgeVLANs = %q, want 2-4094", bridge.BridgeVLANs)
	}
	if bridge.MTU != 9000 {
		t.Errorf("MTU = %d, want 9000", bridge.MTU)
	}
	if bridge.Comment != "management bridge" {
		t.Errorf("Comment = %q, want management bridge", bridge.Comment)
	}

	vlan := mapNodeNetwork(client.NodeNetworkItem{
		Iface:         "vlan10",
		Type:          "vlan",
		VLANID:        10,
		VLANRawDevice: "vmbr0",
	})
	if vlan.VLANID != 10 {
		t.Errorf("VLANID = %d, want 10", vlan.VLANID)
	}
	if vlan.VLANDevice != "vmbr0" {
		t.Errorf("VLANDevice = %q, want vmbr0", vlan.VLANDevice)
	}
}
