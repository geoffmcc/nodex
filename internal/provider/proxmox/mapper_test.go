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

func TestEnrichNodeIPsUsesClusterStatusAndPreservesKnownIP(t *testing.T) {
	nodes := MapNodes([]client.NodeItem{
		{ID: "node/pve1", Node: "pve1", Type: "node"},
		{ID: "node/pve2", Node: "pve2", Type: "node", IP: "192.0.2.22"},
	})
	enrichNodeIPs(nodes, []client.ClusterStatusItem{
		{Type: "cluster", Name: "cluster", IP: "192.0.2.99"},
		{Type: "node", Name: "pve1", IP: "192.0.2.21"},
		{Type: "node", ID: "node/pve3", IP: "192.0.2.23"},
	})
	if nodes[0].IP != "192.0.2.21" {
		t.Errorf("missing node IP = %q, want 192.0.2.21", nodes[0].IP)
	}
	if nodes[1].IP != "192.0.2.22" {
		t.Errorf("existing node IP overwritten: %q", nodes[1].IP)
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

// ptrInt returns a pointer to n for quorum fixtures, which must distinguish an
// explicit value from an absent field.
func ptrInt(n int) *int {
	return &n
}

func TestMapClusterStatusExtractsNameVersionAndNodeCount(t *testing.T) {
	items := []client.ClusterStatusItem{
		{Type: "cluster", ID: "cluster/0", Name: "mycluster", Status: "online", Quorate: ptrInt(1), Version: 3},
		{Type: "node", ID: "node/proxmox", Name: "proxmox", Status: "online"},
		{Type: "node", ID: "node/backup", Name: "backup", Status: "online"},
	}
	cluster := MapClusterStatus(items)
	if cluster.Name != "mycluster" {
		t.Fatalf("Name = %q, want mycluster", cluster.Name)
	}
	if cluster.Version != "3" {
		t.Fatalf("Version = %q, want 3", cluster.Version)
	}
	if cluster.Nodes != 2 {
		t.Fatalf("Nodes = %d, want 2", cluster.Nodes)
	}
}

// A standalone node returns only a node entry, which previously produced an
// empty cluster name and made `cluster status` useless for identifying the host.
func TestMapClusterStatusUsesSoleNodeNameWhenUnclustered(t *testing.T) {
	items := []client.ClusterStatusItem{
		{Type: "node", ID: "node/proxmox", Name: "proxmox", Status: "online", Version: 4},
	}
	cluster := MapClusterStatus(items)
	if cluster.Name != "proxmox" {
		t.Fatalf("Name = %q, want the sole node name", cluster.Name)
	}
	if !cluster.Standalone {
		t.Error("Standalone = false, want true when there is no cluster entry")
	}
	if cluster.Nodes != 1 {
		t.Fatalf("Nodes = %d, want 1", cluster.Nodes)
	}
	// The node's own version is the only version a standalone host has.
	if cluster.Version != "4" {
		t.Errorf("Version = %q, want 4", cluster.Version)
	}
	// A node with no cluster entry has no quorum concept, and reporting false
	// would be a fabrication.
	if cluster.Quorate != nil {
		t.Errorf("Quorate = %v, want nil when the endpoint reports no quorum", *cluster.Quorate)
	}
}

func TestMapClusterStatusReportsQuorumAndNodeDetail(t *testing.T) {
	items := []client.ClusterStatusItem{
		{Type: "cluster", ID: "cluster/0", Name: "mycluster", Status: "online", Quorate: ptrInt(1), Version: 3},
		{Type: "node", ID: "node/a", Name: "a", Status: "online", IP: "10.0.0.1", Version: 3},
		{Type: "node", ID: "node/b", Name: "b", Status: "online", IP: "10.0.0.2", Version: 3},
	}
	cluster := MapClusterStatus(items)
	if cluster.Standalone {
		t.Error("Standalone = true, want false when a cluster entry exists")
	}
	if cluster.Quorate == nil || !*cluster.Quorate {
		t.Errorf("Quorate = %v, want true", cluster.Quorate)
	}
	if len(cluster.NodeDetail) != 2 {
		t.Fatalf("NodeDetail has %d entries, want 2", len(cluster.NodeDetail))
	}
	first := cluster.NodeDetail[0]
	if first.Name != "a" || first.IP != "10.0.0.1" || first.Status != "online" {
		t.Errorf("NodeDetail[0] = %+v, want node a with address and status", first)
	}
}

func TestMapClusterStatusDoesNotInventQuorum(t *testing.T) {
	items := []client.ClusterStatusItem{
		{Type: "cluster", ID: "cluster/0", Name: "mycluster", Status: "online", Quorate: ptrInt(0), Version: 3},
		{Type: "node", ID: "node/a", Name: "a", Status: "online"},
	}
	cluster := MapClusterStatus(items)
	if cluster.Quorate == nil {
		t.Fatal("Quorate = nil, want an explicit not-quorate verdict")
	}
	if *cluster.Quorate {
		t.Error("Quorate = true, want false")
	}
}

// Several nodes with no cluster entry is an inconsistent response, not a
// standalone host, so no single name may be substituted for the missing one.
func TestMapClusterStatusLeavesNameEmptyWhenAmbiguous(t *testing.T) {
	items := []client.ClusterStatusItem{
		{Type: "node", ID: "node/a", Name: "a", Status: "online"},
		{Type: "node", ID: "node/b", Name: "b", Status: "online"},
	}
	cluster := MapClusterStatus(items)
	if cluster.Name != "" {
		t.Errorf("Name = %q, want empty when several nodes have no cluster entry", cluster.Name)
	}
	if cluster.Standalone {
		t.Error("Standalone = true, want false for a multi-node response with no cluster entry")
	}
	if cluster.Nodes != 2 {
		t.Errorf("Nodes = %d, want 2", cluster.Nodes)
	}
}

func TestMapClusterUsesProvidedName(t *testing.T) {
	cluster := MapCluster(&client.VersionData{Version: "9.2.11"}, 1, "mycluster")
	if cluster.Name != "mycluster" {
		t.Fatalf("Name = %q, want mycluster", cluster.Name)
	}
	if cluster.Version != "9.2.11" || cluster.Nodes != 1 {
		t.Fatalf("cluster = %+v, want version 9.2.11 and one node", cluster)
	}
}

func TestVMConfigToMapPreservesUnusedAndRawFields(t *testing.T) {
	got := vmConfigToMap(&client.VMConfigData{
		VMID:    100,
		Unused0: "local-lvm:vm-100-disk-1",
		Digest:  "abc123",
		Raw:     map[string]string{"serial0": "socket", "vga": "serial0"},
	})
	for key, want := range map[string]string{
		"unused0": "local-lvm:vm-100-disk-1",
		"digest":  "abc123",
		"serial0": "socket",
		"vga":     "serial0",
	} {
		if got[key] != want {
			t.Fatalf("config map[%q] = %#v, want %q", key, got[key], want)
		}
	}
}

func TestContainerConfigToMapPreservesRawFields(t *testing.T) {
	got := containerConfigToMap(&client.ContainerConfigData{
		VMID:   100,
		Digest: "def456",
		Raw:    map[string]string{"mp1": "local-lvm:ct-100-disk-1,mp=/data"},
	})
	for key, want := range map[string]string{"digest": "def456", "mp1": "local-lvm:ct-100-disk-1,mp=/data"} {
		if got[key] != want {
			t.Fatalf("config map[%q] = %#v, want %q", key, got[key], want)
		}
	}
}

func TestMapVMPreservesTemplateFlag(t *testing.T) {
	vm := MapVM(client.ClusterResource{ID: "node/100", Type: "qemu", Template: 1})
	if !vm.Template {
		t.Fatal("Template = false, want true")
	}
}

// TestSplitContentIsCanonical guards against the upstream-unstable ordering
// defect: PVE builds the content list from unordered hash iteration, so the
// same unchanged state returns a different order on each call. Without
// normalisation, nodex's own JSON output was unstable too.
func TestSplitContentIsCanonical(t *testing.T) {
	// The orderings observed from PVE for one unchanged storage.
	upstream := []string{
		"iso,import,backup,vztmpl",
		"vztmpl,backup,iso,import",
		"import,iso,vztmpl,backup",
		"vztmpl,import,backup,iso",
	}
	want := []string{"backup", "import", "iso", "vztmpl"}
	for _, in := range upstream {
		got := splitContent(in)
		if len(got) != len(want) {
			t.Fatalf("splitContent(%q) = %v, want %v", in, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("splitContent(%q) = %v, want %v", in, got, want)
			}
		}
	}
}

func TestSplitContentEmptyIsNil(t *testing.T) {
	if got := splitContent(""); got != nil {
		t.Errorf("splitContent(%q) = %v, want nil", "", got)
	}
}

func TestSplitContentSingle(t *testing.T) {
	got := splitContent("iso")
	if len(got) != 1 || got[0] != "iso" {
		t.Errorf("splitContent(%q) = %v, want [iso]", "iso", got)
	}
}
