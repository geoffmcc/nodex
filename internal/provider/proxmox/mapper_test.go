package proxmox

import (
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/provider/proxmox/client"
)

func TestMapNodeUsesProxmoxNodeAndIDFields(t *testing.T) {
	uptimeSeconds := 123
	node := MapNode(client.NodeItem{
		ID:     "node/proxmox",
		Node:   "proxmox",
		Name:   "legacy-name",
		Status: "online",
		Type:   "node",
		Uptime: &uptimeSeconds,
	})

	if node.ID != "node/proxmox" {
		t.Fatalf("ID = %q, want node/proxmox", node.ID)
	}
	if node.Name != "proxmox" {
		t.Fatalf("Name = %q, want proxmox", node.Name)
	}
	if node.Uptime == nil || *node.Uptime != 123*time.Second {
		t.Fatalf("Uptime = %v, want 123s", node.Uptime)
	}
}

func TestMapNodeDoesNotInventMissingFields(t *testing.T) {
	node := MapNode(client.NodeItem{
		ID:     "node/proxmox",
		Node:   "proxmox",
		Status: "online",
		Type:   "node",
	})

	if node.Name != "proxmox" || node.ID != "node/proxmox" {
		t.Fatalf("mapped node = %+v", node)
	}
	if node.IP != "" {
		t.Fatalf("IP = %q, want unavailable empty value", node.IP)
	}
	if node.Uptime != nil {
		t.Fatalf("Uptime = %v, want nil for omitted API field", *node.Uptime)
	}
}

func TestMapNodesHandlesMultipleAndPartialEntries(t *testing.T) {
	nodes := MapNodes([]client.NodeItem{
		{ID: "node/a", Node: "a", Status: "online", Type: "node"},
		{Name: "legacy", Status: "unknown"},
		{ID: "node/partial"},
	})

	if len(nodes) != 3 {
		t.Fatalf("len(nodes) = %d, want 3", len(nodes))
	}
	if nodes[0].ID != "node/a" || nodes[0].Name != "a" {
		t.Fatalf("first node = %+v", nodes[0])
	}
	if nodes[1].ID != "legacy" || nodes[1].Name != "legacy" {
		t.Fatalf("legacy fallback node = %+v", nodes[1])
	}
	if nodes[2].ID != "node/partial" || nodes[2].Name != "" {
		t.Fatalf("partial node = %+v", nodes[2])
	}
}

func TestMapGuestResources(t *testing.T) {
	vm := MapVM(client.ClusterResource{
		Type:    "qemu",
		VMID:    100,
		Name:    "vm-one",
		Node:    "proxmox",
		Status:  "running",
		MaxCPU:  2,
		MaxMem:  2147483648,
		MaxDisk: 34359738368,
	})
	if vm.ID != "proxmox/100" || vm.Name != "vm-one" || vm.CPU != 2 || vm.Memory != 2147483648 {
		t.Fatalf("mapped VM = %+v", vm)
	}

	container := MapContainer(client.ClusterResource{
		Type:    "lxc",
		VMID:    200,
		Name:    "ct-one",
		Node:    "proxmox",
		Status:  "stopped",
		MaxMem:  1073741824,
		MaxDisk: 8589934592,
	})
	if container.ID != "proxmox/200" || container.Name != "ct-one" || container.Memory != 1073741824 {
		t.Fatalf("mapped container = %+v", container)
	}
}

func TestMapStorageUsesStorageNameFallback(t *testing.T) {
	storage := MapStorage(client.ClusterResource{
		ID:      "storage/proxmox/local-lvm",
		Type:    "storage",
		Storage: "local-lvm",
		Node:    "proxmox",
		Status:  "available",
		Disk:    1024,
		MaxDisk: 4096,
		Content: "images,rootdir",
	})

	if storage.Name != "local-lvm" || storage.ID != "storage/proxmox/local-lvm" {
		t.Fatalf("mapped storage identity = %+v", storage)
	}
	if storage.Total != 4096 || storage.Used != 1024 || storage.Avail != 3072 {
		t.Fatalf("mapped storage capacity = %+v", storage)
	}
	if len(storage.Content) != 2 || storage.Content[0] != "images" || storage.Content[1] != "rootdir" {
		t.Fatalf("mapped storage content = %+v", storage.Content)
	}
}
