package client

import (
	"encoding/json"
	"testing"
)

func TestNodeItemContract(t *testing.T) {
	raw := `{
		"status": "online",
		"maxmem": 8589934592,
		"cpu": 0.12,
		"maxcpu": 4,
		"uptime": 123456,
		"node": "pve1",
		"id": "node/pve1",
		"level": "",
		"mem": 4294967296,
		"disk": 10737418240,
		"maxdisk": 107374182400,
		"type": "node",
		"ip": "10.0.0.1"
	}`
	var item NodeItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal NodeItem: %v", err)
	}
	if item.Status != "online" {
		t.Errorf("Status = %q, want online", item.Status)
	}
	if item.Node != "pve1" {
		t.Errorf("Node = %q, want pve1", item.Node)
	}
	if item.ID != "node/pve1" {
		t.Errorf("ID = %q, want node/pve1", item.ID)
	}
	if item.Maxmem != 8589934592 {
		t.Errorf("Maxmem = %d, want 8589934592", item.Maxmem)
	}
	if item.CPU != 0.12 {
		t.Errorf("CPU = %f, want 0.12", item.CPU)
	}
	if item.Maxcpu != 4 {
		t.Errorf("Maxcpu = %d, want 4", item.Maxcpu)
	}
	if item.Uptime == nil || *item.Uptime != 123456 {
		t.Errorf("Uptime = %v, want 123456", item.Uptime)
	}
	if item.IP != "10.0.0.1" {
		t.Errorf("IP = %q, want 10.0.0.1", item.IP)
	}
}

func TestNodeItemContractOmitsUptime(t *testing.T) {
	raw := `{"status":"offline","node":"pve2","id":"node/pve2","type":"node"}`
	var item NodeItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal NodeItem: %v", err)
	}
	if item.Uptime != nil {
		t.Errorf("Uptime = %v, want nil for omitted field", *item.Uptime)
	}
}

func TestClusterResourceContract(t *testing.T) {
	raw := `{
		"id": "qemu/100",
		"type": "qemu",
		"status": "running",
		"name": "web-server",
		"node": "pve1",
		"cpu": 0.45,
		"maxcpu": 2,
		"mem": 2147483648,
		"maxmem": 4294967296,
		"disk": 34359738368,
		"maxdisk": 53687091200,
		"ip": "10.0.0.10",
		"vmid": 100,
		"tags": "web;production"
	}`
	var r ClusterResource
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal ClusterResource: %v", err)
	}
	if r.Type != "qemu" {
		t.Errorf("Type = %q, want qemu", r.Type)
	}
	if r.VMID != 100 {
		t.Errorf("VMID = %d, want 100", r.VMID)
	}
	if r.MaxCPU != 2 {
		t.Errorf("MaxCPU = %d, want 2", r.MaxCPU)
	}
	if r.MaxMem != 4294967296 {
		t.Errorf("MaxMem = %d, want 4294967296", r.MaxMem)
	}
}

func TestClusterResourceContractStorage(t *testing.T) {
	raw := `{
		"id": "storage/pve1/local-lvm",
		"type": "storage",
		"status": "available",
		"node": "pve1",
		"storage": "local-lvm",
		"content": "images,rootdir",
		"disk": 1073741824,
		"maxdisk": 4294967296,
		"shared": 1
	}`
	var r ClusterResource
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("unmarshal ClusterResource (storage): %v", err)
	}
	if r.Type != "storage" {
		t.Errorf("Type = %q, want storage", r.Type)
	}
	if r.Storage != "local-lvm" {
		t.Errorf("Storage = %q, want local-lvm", r.Storage)
	}
	if r.Content != "images,rootdir" {
		t.Errorf("Content = %q, want images,rootdir", r.Content)
	}
}

func TestVersionDataContract(t *testing.T) {
	raw := `{"release":"8.2.4","version":"8.2.4","repoid":"v8.2.4"}`
	var v VersionData
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal VersionData: %v", err)
	}
	if v.Release != "8.2.4" {
		t.Errorf("Release = %q, want 8.2.4", v.Release)
	}
	if v.Version != "8.2.4" {
		t.Errorf("Version = %q, want 8.2.4", v.Version)
	}
	if v.Repoid != "v8.2.4" {
		t.Errorf("Repoid = %q, want v8.2.4", v.Repoid)
	}
}

func TestNodeStatusDataContract(t *testing.T) {
	raw := `{
		"cpu": 0.25,
		"maxcpu": 8,
		"mem": 4294967296,
		"maxmem": 17179869184,
		"disk": 21474836480,
		"maxdisk": 214748364800,
		"uptime": 86400,
		"level": "",
		"id": "node/pve1",
		"node": "pve1",
		"type": "node",
		"status": "online",
		"kversion": "6.8.12-1-pve",
		"pveversion": "pve-manager/8.2.4",
		"loadavg": [0.12, 0.34, 0.56],
		"wait": 0.01,
		"ksm": 0,
		"numa": 1
	}`
	var d NodeStatusData
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal NodeStatusData: %v", err)
	}
	if d.CPU != 0.25 {
		t.Errorf("CPU = %f, want 0.25", d.CPU)
	}
	if d.MaxCPU != 8 {
		t.Errorf("MaxCPU = %d, want 8", d.MaxCPU)
	}
	if d.Uptime != 86400 {
		t.Errorf("Uptime = %d, want 86400", d.Uptime)
	}
	if d.KVersion != "6.8.12-1-pve" {
		t.Errorf("KVersion = %q, want 6.8.12-1-pve", d.KVersion)
	}
	if d.PVEVersion != "pve-manager/8.2.4" {
		t.Errorf("PVEVersion = %q, want pve-manager/8.2.4", d.PVEVersion)
	}
	if len(d.LoadAvg) != 3 {
		t.Errorf("LoadAvg length = %d, want 3", len(d.LoadAvg))
	}
	if d.LoadAvg[0] != 0.12 || d.LoadAvg[1] != 0.34 || d.LoadAvg[2] != 0.56 {
		t.Errorf("LoadAvg = %v, want [0.12 0.34 0.56]", d.LoadAvg)
	}
	if d.Numa != 1 {
		t.Errorf("Numa = %d, want 1", d.Numa)
	}
}

func TestClusterStatusItemContract(t *testing.T) {
	raw := `{
		"type": "cluster",
		"id": "cluster/0",
		"name": "mycluster",
		"status": "online",
		"quorate": 1,
		"version": 3,
		"commit": "v8.2.4"
	}`
	var item ClusterStatusItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal ClusterStatusItem: %v", err)
	}
	if item.Type != "cluster" {
		t.Errorf("Type = %q, want cluster", item.Type)
	}
	if item.Name != "mycluster" {
		t.Errorf("Name = %q, want mycluster", item.Name)
	}
	if item.Quorate != 1 {
		t.Errorf("Quorate = %d, want 1", item.Quorate)
	}
	if item.Version != 3 {
		t.Errorf("Version = %d, want 3", item.Version)
	}
}

func TestVMConfigDataContract(t *testing.T) {
	raw := `{
		"vmid": 100,
		"name": "web-server",
		"cores": 4,
		"memory": 8192,
		"net0": "virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr0",
		"scsi0": "local-lvm:vm-100-disk-0,size=50G",
		"boot": "order=scsi0;net0",
		"onboot": 1,
		"agent": 1,
		"ostype": "l26",
		"description": "Web Server",
		"protection": 0,
		"tags": "web;production",
		"scsihw": "virtio-scsi-pci",
		"bios": "seabios",
		"ide2": "none,media=cdrom",
		"vmgenid": "some-uuid"
	}`
	var d VMConfigData
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal VMConfigData: %v", err)
	}
	if d.VMID != 100 {
		t.Errorf("VMID = %d, want 100", d.VMID)
	}
	if d.Name != "web-server" {
		t.Errorf("Name = %q, want web-server", d.Name)
	}
	if d.CPU != 4 {
		t.Errorf("CPU = %d, want 4", d.CPU)
	}
	if d.Memory != 8192 {
		t.Errorf("Memory = %d, want 8192", d.Memory)
	}
	if d.Net0 != "virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr0" {
		t.Errorf("Net0 = %q, want expected value", d.Net0)
	}
	if d.OnBoot != 1 {
		t.Errorf("OnBoot = %d, want 1", d.OnBoot)
	}
	if d.Agent != 1 {
		t.Errorf("Agent = %d, want 1", d.Agent)
	}
	if d.Bios != "seabios" {
		t.Errorf("Bios = %q, want seabios", d.Bios)
	}
}

func TestContainerConfigDataContract(t *testing.T) {
	raw := `{
		"vmid": 200,
		"hostname": "db-server",
		"cores": 2,
		"memory": 4096,
		"swap": 1024,
		"rootfs": "local-lvm:vm-200-disk-0,size=30G",
		"mp0": "local-lvm:vm-200-disk-1,size=100G",
		"net0": "name=eth0,bridge=vmbr0,ip=10.0.0.20/24",
		"onboot": 1,
		"ostype": "debian",
		"description": "Database Server",
		"features": "nesting=1",
		"architecture": "amd64",
		"nameserver": "8.8.8.8",
		"searchdomain": "example.com"
	}`
	var d ContainerConfigData
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal ContainerConfigData: %v", err)
	}
	if d.VMID != 200 {
		t.Errorf("VMID = %d, want 200", d.VMID)
	}
	if d.Hostname != "db-server" {
		t.Errorf("Hostname = %q, want db-server", d.Hostname)
	}
	if d.CPU != 2 {
		t.Errorf("CPU = %d, want 2", d.CPU)
	}
	if d.Memory != 4096 {
		t.Errorf("Memory = %d, want 4096", d.Memory)
	}
	if d.Swap != 1024 {
		t.Errorf("Swap = %d, want 1024", d.Swap)
	}
	if d.Architecture != "amd64" {
		t.Errorf("Architecture = %q, want amd64", d.Architecture)
	}
	if d.Nameserver != "8.8.8.8" {
		t.Errorf("Nameserver = %q, want 8.8.8.8", d.Nameserver)
	}
}

func TestStorageContentItemContract(t *testing.T) {
	raw := `{
		"content": "iso",
		"ctime": 1700000000,
		"format": "iso",
		"volid": "local:iso/debian-12.0-amd64-netinst.iso",
		"size": 5368709120,
		"subtype": "",
		"vmid": 0
	}`
	var item StorageContentItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal StorageContentItem: %v", err)
	}
	if item.Content != "iso" {
		t.Errorf("Content = %q, want iso", item.Content)
	}
	if item.Volid != "local:iso/debian-12.0-amd64-netinst.iso" {
		t.Errorf("Volid = %q, want expected", item.Volid)
	}
	if item.Size != 5368709120 {
		t.Errorf("Size = %d, want 5368709120", item.Size)
	}
}

func TestTaskListItemContract(t *testing.T) {
	raw := `{
		"upid": "UPID:pve1/00012345/0",
		"type": "vzdump",
		"state": "stopped",
		"starttime": 1700000000,
		"endtime": 1700000010,
		"status": "OK",
		"pid": 1234
	}`
	var item TaskListItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal TaskListItem: %v", err)
	}
	if item.UPID != "UPID:pve1/00012345/0" {
		t.Errorf("UPID = %q, want expected", item.UPID)
	}
	if item.Type != "vzdump" {
		t.Errorf("Type = %q, want vzdump", item.Type)
	}
	if item.State != "stopped" {
		t.Errorf("State = %q, want stopped", item.State)
	}
	if item.Status != "OK" {
		t.Errorf("Status = %q, want OK", item.Status)
	}
	if item.ExitStatus != "" {
		t.Errorf("ExitStatus = %q, want empty for task list row", item.ExitStatus)
	}
	if item.StartTime != 1700000000 {
		t.Errorf("StartTime = %d, want 1700000000", item.StartTime)
	}
	if item.EndTime != 1700000010 {
		t.Errorf("EndTime = %d, want 1700000010", item.EndTime)
	}
}

func TestTaskStatusItemContract(t *testing.T) {
	raw := `{
		"upid": "UPID:pve-test:00002183:000434BF:6A56BE4B:qmstart:100:root@pam!token:",
		"type": "qmstart",
		"status": "stopped",
		"exitstatus": "OK",
		"starttime": 1784069707,
		"pid": 8579
	}`
	var item TaskListItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal TaskListItem status response: %v", err)
	}
	if item.Status != "stopped" {
		t.Errorf("Status = %q, want stopped", item.Status)
	}
	if item.ExitStatus != "OK" {
		t.Errorf("ExitStatus = %q, want OK", item.ExitStatus)
	}
}

func TestSnapshotListItemContract(t *testing.T) {
	raw := `{
		"name": "before-upgrade",
		"vmid": 100,
		"ctime": 1700000000,
		"parent": "current"
	}`
	var item SnapshotListItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal SnapshotListItem: %v", err)
	}
	if item.Name != "before-upgrade" {
		t.Errorf("Name = %q, want before-upgrade", item.Name)
	}
	if item.Parent != "current" {
		t.Errorf("Parent = %q, want current", item.Parent)
	}
}

func TestEventItemContract(t *testing.T) {
	raw := `{
		"type": "node",
		"time": 1700000000,
		"node": "pve1",
		"id": "node/pve1",
		"message": "node online"
	}`
	var item EventItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal EventItem: %v", err)
	}
	if item.Type != "node" {
		t.Errorf("Type = %q, want node", item.Type)
	}
	if item.Time != 1700000000 {
		t.Errorf("Time = %d, want 1700000000", item.Time)
	}
}

func TestSyslogItemContract(t *testing.T) {
	raw := `{
		"time": 1700000000,
		"node": "pve1",
		"sysloglevel": "info",
		"message": "system startup"
	}`
	var item SyslogItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal SyslogItem: %v", err)
	}
	if item.SyslogLevel != "info" {
		t.Errorf("SyslogLevel = %q, want info", item.SyslogLevel)
	}
	if item.Message != "system startup" {
		t.Errorf("Message = %q, want system startup", item.Message)
	}
}

func TestFirewallRuleItemContract(t *testing.T) {
	raw := `{
		"type": "in",
		"action": "ACCEPT",
		"enable": 1,
		"pos": 1,
		"proto": "tcp",
		"dest": "10.0.0.0/24",
		"dport": "22",
		"source": "10.0.0.0/8",
		"sport": "",
		"icmp_type": "",
		"log": "n",
		"comment": "SSH access"
	}`
	var item FirewallRuleItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal FirewallRuleItem: %v", err)
	}
	if item.Type != "in" {
		t.Errorf("Type = %q, want in", item.Type)
	}
	if item.Action != "ACCEPT" {
		t.Errorf("Action = %q, want ACCEPT", item.Action)
	}
	if item.Pos != 1 {
		t.Errorf("Pos = %d, want 1", item.Pos)
	}
	if item.Proto != "tcp" {
		t.Errorf("Proto = %q, want tcp", item.Proto)
	}
	if item.Dport != "22" {
		t.Errorf("Dport = %q, want 22", item.Dport)
	}
}

func TestHAResourceItemContract(t *testing.T) {
	raw := `{
		"id": "ha:vm/100",
		"type": "vm",
		"state": "started",
		"node": "pve1",
		"group": "default",
		"max_relocate": 3
	}`
	var item HAResourceItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal HAResourceItem: %v", err)
	}
	if item.ID != "ha:vm/100" {
		t.Errorf("ID = %q, want ha:vm/100", item.ID)
	}
	if item.Type != "vm" {
		t.Errorf("Type = %q, want vm", item.Type)
	}
	if item.State != "started" {
		t.Errorf("State = %q, want started", item.State)
	}
	if item.MaxRelay != 3 {
		t.Errorf("MaxRelay = %d, want 3", item.MaxRelay)
	}
}

func TestHAGroupItemContract(t *testing.T) {
	raw := `{
		"id": "default",
		"type": "group",
		"nodes": "pve1,pve2",
		"comment": "Default HA group",
		"nofailback": 0
	}`
	var item HAGroupItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal HAGroupItem: %v", err)
	}
	if item.ID != "default" {
		t.Errorf("ID = %q, want default", item.ID)
	}
	if item.Nodes != "pve1,pve2" {
		t.Errorf("Nodes = %q, want pve1,pve2", item.Nodes)
	}
	if item.Comment != "Default HA group" {
		t.Errorf("Comment = %q, want Default HA group", item.Comment)
	}
}

func TestBackupStatusItemContract(t *testing.T) {
	raw := `{
		"upid": "UPID:pve1/00012345/0",
		"type": "vzdump",
		"state": "stopped",
		"starttime": 1700000000,
		"endtime": 1700000010,
		"status": "OK",
		"node": "pve1",
		"storage": "local"
	}`
	var item BackupStatusItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal BackupStatusItem: %v", err)
	}
	if item.Type != "vzdump" {
		t.Errorf("Type = %q, want vzdump", item.Type)
	}
	if item.Storage != "local" {
		t.Errorf("Storage = %q, want local", item.Storage)
	}
}

func TestNodeListResponseContract(t *testing.T) {
	raw := `{"data":[{"status":"online","maxmem":8589934592,"cpu":0.12,"maxcpu":4,"uptime":123,"node":"pve1","id":"node/pve1","level":"","mem":4294967296,"disk":10737418240,"maxdisk":107374182400,"type":"node"}]}`
	var resp NodeListResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal NodeListResponse: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(Data) = %d, want 1", len(resp.Data))
	}
	if resp.Data[0].Node != "pve1" {
		t.Errorf("Data[0].Node = %q, want pve1", resp.Data[0].Node)
	}
}

func TestClusterResourcesResponseContract(t *testing.T) {
	raw := `{"data":[
		{"id":"qemu/100","type":"qemu","status":"running","name":"web","node":"pve1","vmid":100,"cpu":0.5,"maxcpu":2,"mem":1073741824,"maxmem":2147483648},
		{"id":"lxc/200","type":"lxc","status":"stopped","name":"db","node":"pve1","vmid":200,"mem":536870912,"maxmem":1073741824},
		{"id":"storage/pve1/local","type":"storage","status":"available","node":"pve1","storage":"local","content":"images,iso","disk":0,"maxdisk":107374182400}
	]}`
	var resp ClusterResourcesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal ClusterResourcesResponse: %v", err)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("len(Data) = %d, want 3", len(resp.Data))
	}
	if resp.Data[0].Type != "qemu" {
		t.Errorf("Data[0].Type = %q, want qemu", resp.Data[0].Type)
	}
	if resp.Data[1].Type != "lxc" {
		t.Errorf("Data[1].Type = %q, want lxc", resp.Data[1].Type)
	}
	if resp.Data[2].Type != "storage" {
		t.Errorf("Data[2].Type = %q, want storage", resp.Data[2].Type)
	}
}

func TestMinimalNodeItemContract(t *testing.T) {
	raw := `{"status":"online","node":"pve1","id":"node/pve1","type":"node"}`
	var item NodeItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		t.Fatalf("unmarshal minimal NodeItem: %v", err)
	}
	if item.Maxmem != 0 {
		t.Errorf("Maxmem = %d, want 0 for omitted field", item.Maxmem)
	}
	if item.CPU != 0 {
		t.Errorf("CPU = %f, want 0 for omitted field", item.CPU)
	}
}

func TestEmptyArrayResponsesContract(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty nodes", `{"data":[]}`},
		{"empty cluster resources", `{"data":[]}`},
		{"empty events", `{"data":[]}`},
		{"empty firewall", `{"data":[]}`},
		{"empty HA resources", `{"data":[]}`},
		{"empty HA groups", `{"data":[]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch tt.name {
			case "empty nodes":
				var resp NodeListResponse
				if err := json.Unmarshal([]byte(tt.raw), &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(resp.Data) != 0 {
					t.Errorf("len(Data) = %d, want 0", len(resp.Data))
				}
			case "empty cluster resources":
				var resp ClusterResourcesResponse
				if err := json.Unmarshal([]byte(tt.raw), &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
			case "empty events":
				var resp EventListResponse
				if err := json.Unmarshal([]byte(tt.raw), &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
			case "empty firewall":
				var resp FirewallRuleResponse
				if err := json.Unmarshal([]byte(tt.raw), &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
			case "empty HA resources":
				var resp HAResourceResponse
				if err := json.Unmarshal([]byte(tt.raw), &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
			case "empty HA groups":
				var resp HAGroupResponse
				if err := json.Unmarshal([]byte(tt.raw), &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
			}
		})
	}
}
