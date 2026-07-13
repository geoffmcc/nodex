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

func TestMapNodeStatusConvertsFieldsCorrectly(t *testing.T) {
	status := MapNodeStatus(&client.NodeStatusData{
		ID:         "node/proxmox",
		Node:       "proxmox",
		Status:     "online",
		Type:       "node",
		Uptime:     86400,
		PVEVersion: "pve-manager/8.2.4",
		CPU:        0.25,
		MaxCPU:     4,
		Mem:        2147483648,
		MaxMem:     8589934592,
		Disk:       10737418240,
		MaxDisk:    107374182400,
		LoadAvg:    []float64{0.12, 0.34, 0.56},
		KVersion:   "6.8.12-1-pve",
	})

	if status.ID != "node/proxmox" {
		t.Fatalf("ID = %q, want node/proxmox", status.ID)
	}
	if status.Name != "proxmox" {
		t.Fatalf("Name = %q, want proxmox", status.Name)
	}
	if status.Status != "online" {
		t.Fatalf("Status = %q, want online", status.Status)
	}
	if status.Role != "node" {
		t.Fatalf("Role = %q, want node", status.Role)
	}
	if status.Platform != "proxmox" {
		t.Fatalf("Platform = %q, want proxmox", status.Platform)
	}
	if status.Version != "pve-manager/8.2.4" {
		t.Fatalf("Version = %q, want pve-manager/8.2.4", status.Version)
	}
	if status.Uptime == nil || *status.Uptime != 86400*time.Second {
		t.Fatalf("Uptime = %v, want 86400s", status.Uptime)
	}
}

func TestMapNodeStatusHandlesZeroUptime(t *testing.T) {
	status := MapNodeStatus(&client.NodeStatusData{
		ID:     "node/proxmox",
		Node:   "proxmox",
		Status: "offline",
		Type:   "node",
		Uptime: 0,
	})

	if status.Uptime != nil {
		t.Fatalf("Uptime = %v, want nil for zero uptime", *status.Uptime)
	}
}

func TestMapNodeStatusFallsBackToIDForName(t *testing.T) {
	status := MapNodeStatus(&client.NodeStatusData{
		ID:     "node/backup",
		Node:   "",
		Status: "online",
		Type:   "node",
	})

	if status.Name != "node/backup" {
		t.Fatalf("Name = %q, want node/backup (fallback from ID)", status.Name)
	}
}
