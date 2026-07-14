package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider"
)

const e2eMockProviderName = "nodex-e2e-mock"

func init() {
	provider.Register(e2eMockProviderName, func() domain.Provider { return &e2eMockProvider{} })
}

type e2eMockProvider struct {
	connected bool
}

func (p *e2eMockProvider) Name() string                   { return e2eMockProviderName }
func (p *e2eMockProvider) Version() string                { return "e2e" }
func (p *e2eMockProvider) Close() error                   { return nil }
func (p *e2eMockProvider) Health(_ context.Context) error { return nil }
func (p *e2eMockProvider) Capabilities() []domain.Capability {
	return []domain.Capability{
		domain.CapabilityNodes, domain.CapabilityVMs, domain.CapabilityContainers,
		domain.CapabilityStorage, domain.CapabilityCluster,
		domain.CapabilityNodeDetail, domain.CapabilityFirewallAdvanced,
		domain.CapabilityHAStatus, domain.CapabilityBackupContent,
		domain.CapabilitySDN, domain.CapabilitySnapshotDetail,
		domain.CapabilityPools, domain.CapabilityClusterLog,
		domain.CapabilityLifecycle,
		domain.CapabilityConfig, domain.CapabilitySnapshotMutation,
		domain.CapabilityDelete, domain.CapabilityTemplate,
		domain.CapabilityCloudInit,
	}
}
func (p *e2eMockProvider) Connect(_ context.Context, endpoint string, creds *domain.Credentials) error {
	if endpoint == "https://e2e.example.invalid" && creds != nil && creds.Token == "e2e-token" {
		p.connected = true
	}
	return nil
}
func (p *e2eMockProvider) Nodes(_ context.Context) ([]domain.Node, error) {
	if !p.connected {
		return nil, nil
	}
	return []domain.Node{{ID: "node/e2e-node", Name: "e2e-node", Status: "online", Role: "node", Platform: "mock"}}, nil
}
func (p *e2eMockProvider) VMs(_ context.Context) ([]domain.VM, error) {
	return []domain.VM{{ID: "e2e-node/100", Name: "e2e-vm", Status: "running", Node: "e2e-node", CPU: 2, Memory: 1024, Disk: 2048}}, nil
}
func (p *e2eMockProvider) Containers(_ context.Context) ([]domain.Container, error) {
	return []domain.Container{{ID: "e2e-node/200", Name: "e2e-ct", Status: "running", Node: "e2e-node", OS: "debian", Memory: 512, Disk: 1024}}, nil
}
func (p *e2eMockProvider) Storage(_ context.Context) ([]domain.Storage, error) {
	return []domain.Storage{{ID: "storage/e2e-node/local", Name: "local", Type: "dir", Status: "available", Node: "e2e-node", Total: 4096, Used: 1024, Avail: 3072}}, nil
}
func (p *e2eMockProvider) Cluster(_ context.Context) (*domain.Cluster, error) {
	return &domain.Cluster{Name: "e2e", Version: "test", Nodes: 1}, nil
}
func (p *e2eMockProvider) VMConfig(_ context.Context, node string, vmid int) (map[string]interface{}, error) {
	return map[string]interface{}{
		"vmid":   vmid,
		"name":   "e2e-vm",
		"cores":  2,
		"memory": 1024,
	}, nil
}
func (p *e2eMockProvider) ContainerConfig(_ context.Context, node string, vmid int) (map[string]interface{}, error) {
	return map[string]interface{}{
		"vmid":     vmid,
		"hostname": "e2e-ct",
		"cores":    1,
		"memory":   512,
		"swap":     256,
	}, nil
}
func (p *e2eMockProvider) StorageContent(_ context.Context, node, storage string) ([]domain.StorageContentItem, error) {
	return []domain.StorageContentItem{
		{Content: "iso", Volid: "local:iso/debian-12.iso", Size: 5368709120, Format: "iso"},
		{Content: "images", Volid: "local-lvm:vm-100-disk-0", Size: 34359738368, Format: "raw", VMID: 100},
	}, nil
}
func (p *e2eMockProvider) Tasks(_ context.Context, node string) ([]domain.Task, error) {
	return []domain.Task{
		{UPID: "UPID:e2e-node/00012345/0", Type: "vzdump", State: "stopped", Status: "OK", Node: node, StartTime: 1700000000, EndTime: 1700000010},
		{UPID: "UPID:e2e-node/00012346/0", Type: "qmstart", State: "running", Node: node, StartTime: 1700000005},
	}, nil
}
func (p *e2eMockProvider) Task(_ context.Context, node, upid string) (*domain.Task, error) {
	return &domain.Task{
		UPID:      upid,
		Type:      "vzdump",
		State:     "stopped",
		Status:    "OK",
		Node:      node,
		StartTime: 1700000000,
		EndTime:   1700000010,
	}, nil
}
func (p *e2eMockProvider) VMSnapshots(_ context.Context, node string, vmid int) ([]domain.Snapshot, error) {
	return []domain.Snapshot{
		{Name: "before-upgrade", VMID: vmid, Ctime: 1700000000, Parent: "current", Node: node, Target: fmt.Sprintf("%s/%d", node, vmid)},
		{Name: "current", VMID: vmid, Ctime: 1700000010, Node: node, Target: fmt.Sprintf("%s/%d", node, vmid)},
	}, nil
}
func (p *e2eMockProvider) ContainerSnapshots(_ context.Context, node string, vmid int) ([]domain.Snapshot, error) {
	return []domain.Snapshot{
		{Name: "clean", VMID: vmid, Ctime: 1700000000, Node: node, Target: fmt.Sprintf("%s/%d", node, vmid)},
	}, nil
}
func (p *e2eMockProvider) Events(_ context.Context) ([]domain.Event, error) {
	return []domain.Event{
		{Type: "node", Time: 1700000000, Node: "e2e-node", ID: "node/e2e-node", Message: "node online"},
		{Type: "vm", Time: 1700000001, Node: "e2e-node", ID: "vm/100", Message: "VM started"},
	}, nil
}
func (p *e2eMockProvider) Syslog(_ context.Context, node string) ([]domain.SyslogEntry, error) {
	return []domain.SyslogEntry{
		{Time: 1700000000, Node: node, Level: "info", Message: "system startup"},
		{Time: 1700000001, Node: node, Level: "err", Message: "disk failure"},
	}, nil
}
func (p *e2eMockProvider) Backups(_ context.Context, node string) ([]domain.Backup, error) {
	return []domain.Backup{
		{UPID: "UPID:e2e-node/00012345/0", Type: "vzdump", State: "stopped", Status: "OK", Node: node, StartTime: 1700000000, EndTime: 1700000010},
	}, nil
}
func (p *e2eMockProvider) FirewallRules(_ context.Context) ([]domain.FirewallRule, error) {
	return []domain.FirewallRule{
		{Type: "in", Action: "ACCEPT", Enable: 1, Pos: 1, Proto: "tcp", Dport: "22", Comment: "SSH"},
	}, nil
}
func (p *e2eMockProvider) HAResources(_ context.Context) ([]domain.HAResource, error) {
	return []domain.HAResource{
		{ID: "ha:vm/100", Type: "vm", State: "started", Node: "e2e-node", Group: "default"},
	}, nil
}
func (p *e2eMockProvider) HAGroups(_ context.Context) ([]domain.HAGroup, error) {
	return []domain.HAGroup{
		{ID: "default", Type: "group", Nodes: "e2e-node", Comment: "Default HA group"},
	}, nil
}

// Optional interfaces

func (p *e2eMockProvider) NodeStatus(_ context.Context, name string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"node":       name,
		"status":     "online",
		"cpu":        0.12,
		"maxcpu":     8,
		"mem":        2147483648,
		"maxmem":     8589934592,
		"disk":       10737418240,
		"maxdisk":    107374182400,
		"uptime":     12345,
		"level":      "",
		"kversion":   "6.8.0",
		"pveversion": "pve-manager/8.1.0",
		"loadavg":    []float64{0.10, 0.15, 0.20},
	}, nil
}
func (p *e2eMockProvider) NodeServices(_ context.Context, node string) ([]domain.NodeService, error) {
	return nil, nil
}
func (p *e2eMockProvider) NodeNetwork(_ context.Context, node string) ([]domain.NodeNetwork, error) {
	return nil, nil
}
func (p *e2eMockProvider) NodeDNS(_ context.Context, node string) (*domain.NodeDNS, error) {
	return &domain.NodeDNS{DNS1: "8.8.8.8"}, nil
}
func (p *e2eMockProvider) NodeTime(_ context.Context, node string) (*domain.NodeTime, error) {
	return &domain.NodeTime{TimeZone: "UTC", Epoch: 1700000000}, nil
}
func (p *e2eMockProvider) NodeDisks(_ context.Context, node string) ([]domain.NodeDisk, error) {
	return nil, nil
}
func (p *e2eMockProvider) NodeCertificates(_ context.Context, node string) ([]domain.NodeCertificate, error) {
	return nil, nil
}
func (p *e2eMockProvider) NodeSubscription(_ context.Context, node string) (*domain.NodeSubscription, error) {
	return &domain.NodeSubscription{Status: "valid"}, nil
}
func (p *e2eMockProvider) NodeUpdates(_ context.Context, node string) ([]domain.NodeUpdate, error) {
	return nil, nil
}
func (p *e2eMockProvider) FirewallAliases(_ context.Context) ([]domain.FirewallAlias, error) {
	return nil, nil
}
func (p *e2eMockProvider) FirewallIPSet(_ context.Context, name string) ([]domain.FirewallIPSetEntry, error) {
	return nil, nil
}
func (p *e2eMockProvider) FirewallIPSets(_ context.Context) ([]domain.FirewallIPSet, error) {
	return nil, nil
}
func (p *e2eMockProvider) FirewallSecurityGroups(_ context.Context) ([]domain.FirewallSecurityGroup, error) {
	return nil, nil
}
func (p *e2eMockProvider) FirewallOptions(_ context.Context) (*domain.FirewallOptions, error) {
	return &domain.FirewallOptions{Enable: 1, Log: 0}, nil
}
func (p *e2eMockProvider) NodeFirewallRules(_ context.Context, node string) ([]domain.FirewallRule, error) {
	return nil, nil
}
func (p *e2eMockProvider) VMFirewallRules(_ context.Context, node string, vmid int) ([]domain.FirewallRule, error) {
	return nil, nil
}
func (p *e2eMockProvider) HAStatus(_ context.Context) (*domain.HAStatus, error) {
	return &domain.HAStatus{Quorum: 1, Status: "online"}, nil
}
func (p *e2eMockProvider) HACurrent(_ context.Context) ([]domain.HACurrent, error) {
	return []domain.HACurrent{{ID: "vm/100", Type: "vm", State: "started", Node: "e2e-node"}}, nil
}
func (p *e2eMockProvider) BackupContent(_ context.Context, node, storage string) ([]domain.BackupContentItem, error) {
	return []domain.BackupContentItem{
		{Content: "backup", Volid: "backup:vzdump-qemu-100-2024_01_01-12_00_00.vma.zst", Size: 1073741824, Format: "vma.zst"},
	}, nil
}
func (p *e2eMockProvider) SDNZones(_ context.Context) ([]domain.SDNZone, error) {
	return nil, nil
}
func (p *e2eMockProvider) SDNVNets(_ context.Context) ([]domain.SDNVNet, error) {
	return nil, nil
}
func (p *e2eMockProvider) VMSnapshotConfig(_ context.Context, node string, vmid int, name string) (map[string]interface{}, error) {
	return map[string]interface{}{"name": name, "vmid": vmid, "parent": "current"}, nil
}
func (p *e2eMockProvider) ContainerSnapshotConfig(_ context.Context, node string, vmid int, name string) (map[string]interface{}, error) {
	return map[string]interface{}{"name": name, "vmid": vmid}, nil
}

func (p *e2eMockProvider) Pools(_ context.Context) ([]domain.Pool, error) {
	return []domain.Pool{
		{PoolID: "admins", Comment: "Admin resources", Members: []string{"qemu/100", "qemu/101"}},
		{PoolID: "devs", Comment: "Dev resources", Members: []string{"qemu/200"}},
	}, nil
}

func (p *e2eMockProvider) ClusterLog(_ context.Context) ([]domain.ClusterLogEntry, error) {
	return []domain.ClusterLogEntry{
		{N: 1, Message: "starting cluster services"},
		{N: 2, Message: "node e2e-node joined quorum"},
	}, nil
}

// LifecycleProvider methods
func (p *e2eMockProvider) VMStart(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12345, 1700000000), nil
}
func (p *e2eMockProvider) VMStop(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12346, 1700000000), nil
}
func (p *e2eMockProvider) VMShutdown(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12347, 1700000000), nil
}
func (p *e2eMockProvider) VMReset(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12348, 1700000000), nil
}
func (p *e2eMockProvider) VMReboot(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12349, 1700000000), nil
}
func (p *e2eMockProvider) VMSuspend(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12350, 1700000000), nil
}
func (p *e2eMockProvider) VMResume(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12351, 1700000000), nil
}
func (p *e2eMockProvider) VMPause(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12352, 1700000000), nil
}
func (p *e2eMockProvider) VMUnpause(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12353, 1700000000), nil
}
func (p *e2eMockProvider) CTStart(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12354, 1700000000), nil
}
func (p *e2eMockProvider) CTStop(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12355, 1700000000), nil
}
func (p *e2eMockProvider) CTShutdown(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12356, 1700000000), nil
}
func (p *e2eMockProvider) CTReboot(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12357, 1700000000), nil
}
func (p *e2eMockProvider) CTSuspend(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12358, 1700000000), nil
}
func (p *e2eMockProvider) CTResume(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12359, 1700000000), nil
}

// Phase 3: ConfigProvider, SnapshotMutationProvider, DeleteProvider, TemplateProvider, CloudInitProvider

func (p *e2eMockProvider) VMConfigUpdate(_ context.Context, node string, vmid int, params map[string]string) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12360, 1700000000), nil
}

func (p *e2eMockProvider) CTConfigUpdate(_ context.Context, node string, vmid int, params map[string]string) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12361, 1700000000), nil
}

func (p *e2eMockProvider) VMSnapshotCreate(_ context.Context, node string, vmid int, name, description string) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12362, 1700000000), nil
}

func (p *e2eMockProvider) VMSnapshotDelete(_ context.Context, node string, vmid int, name string) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12363, 1700000000), nil
}

func (p *e2eMockProvider) VMSnapshotRollback(_ context.Context, node string, vmid int, name string) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12364, 1700000000), nil
}

func (p *e2eMockProvider) CTSnapshotCreate(_ context.Context, node string, vmid int, name, description string) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12365, 1700000000), nil
}

func (p *e2eMockProvider) CTSnapshotDelete(_ context.Context, node string, vmid int, name string) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12366, 1700000000), nil
}

func (p *e2eMockProvider) CTSnapshotRollback(_ context.Context, node string, vmid int, name string) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12367, 1700000000), nil
}

func (p *e2eMockProvider) VMDelete(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12368, 1700000000), nil
}

func (p *e2eMockProvider) CTDelete(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12369, 1700000000), nil
}

func (p *e2eMockProvider) VMTemplate(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12370, 1700000000), nil
}

func (p *e2eMockProvider) CTTemplate(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12371, 1700000000), nil
}

func (p *e2eMockProvider) VMCloudInit(_ context.Context, node string, vmid int) (string, error) {
	return fmt.Sprintf("UPID:%s/%08X/%08X", node, 12372, 1700000000), nil
}

func (p *e2eMockProvider) ClusterStatuses(_ context.Context) ([]domain.ClusterStatusDetail, error) {
	return []domain.ClusterStatusDetail{
		{Type: "cluster", ID: "cluster/e2e", Name: "e2e", Status: "online", Quorate: 3, Version: 1},
		{Type: "node", ID: "node/e2e-node", Name: "e2e-node", Status: "online", IP: "10.0.0.1"},
	}, nil
}

func TestRunE2EWithMockProvider(t *testing.T) {
	isolateConfigAndHome(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	cfg := config.DefaultConfig()
	cfg.CurrentProfile = "e2e"
	cfg.Profiles["e2e"] = config.Profile{
		Provider:      e2eMockProviderName,
		Endpoint:      "https://e2e.example.invalid",
		CredentialRef: "env:e2e",
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "node list", args: []string{"--output", "json", "node", "list"}, want: []string{`"name": "e2e-node"`, `"platform": "mock"`}},
		{name: "node status", args: []string{"--output", "json", "node", "status", "e2e-node"}, want: []string{`"node": "e2e-node"`, `"status": "online"`}},
		{name: "vm show", args: []string{"--output", "json", "vm", "show", "e2e-node/100"}, want: []string{`"id": "e2e-node/100"`, `"name": "e2e-vm"`}},
		{name: "container list", args: []string{"--output", "json", "container", "list"}, want: []string{`"id": "e2e-node/200"`, `"os": "debian"`}},
		{name: "storage show", args: []string{"--output", "json", "storage", "show", "local"}, want: []string{`"id": "storage/e2e-node/local"`, `"avail": 3072`}},
		{name: "cluster status", args: []string{"--output", "json", "cluster", "status"}, want: []string{`"name": "e2e"`, `"version": "test"`, `"nodes": 1`}},
		{name: "vm config", args: []string{"--output", "json", "vm", "config", "e2e-node/100"}, want: []string{`"vmid": 100`, `"name": "e2e-vm"`, `"cores": 2`}},
		{name: "container config", args: []string{"--output", "json", "container", "config", "e2e-node/200"}, want: []string{`"vmid": 200`, `"hostname": "e2e-ct"`, `"cores": 1`}},
		{name: "storage content", args: []string{"--output", "json", "storage", "content", "e2e-node", "local"}, want: []string{`"content": "iso"`, `"volid": "local:iso/debian-12.iso"`, `"size": 5368709120`}},
		{name: "task list", args: []string{"--output", "json", "task", "list", "e2e-node"}, want: []string{`"upid": "UPID:e2e-node/00012345/0"`, `"type": "vzdump"`, `"state": "stopped"`}},
		{name: "task show", args: []string{"--output", "json", "task", "show", "e2e-node", "UPID:e2e-node/00012345/0"}, want: []string{`"upid": "UPID:e2e-node/00012345/0"`, `"status": "OK"`, `"node": "e2e-node"`}},
		{name: "vm snapshots", args: []string{"--output", "json", "vm", "snapshots", "e2e-node/100"}, want: []string{`"name": "before-upgrade"`, `"parent": "current"`, `"vmid": 100`}},
		{name: "container snapshots", args: []string{"--output", "json", "container", "snapshots", "e2e-node/200"}, want: []string{`"name": "clean"`, `"vmid": 200`}},
		{name: "status", args: []string{"--output", "json", "status"}, want: []string{`"cluster": "e2e"`, `"nodes": 1`, `"vms_running": 1`}},
		{name: "event list", args: []string{"--output", "json", "event", "list"}, want: []string{`"type": "node"`, `"message": "node online"`, `"id": "node/e2e-node"`}},
		{name: "log", args: []string{"--output", "json", "log", "e2e-node"}, want: []string{`"level": "info"`, `"message": "system startup"`, `"node": "e2e-node"`}},
		{name: "backup list", args: []string{"--output", "json", "backup", "list", "e2e-node"}, want: []string{`"type": "vzdump"`, `"state": "stopped"`, `"node": "e2e-node"`}},
		{name: "firewall list", args: []string{"--output", "json", "firewall", "list"}, want: []string{`"action": "ACCEPT"`, `"dport": "22"`, `"comment": "SSH"`}},
		{name: "ha list", args: []string{"--output", "json", "ha", "list"}, want: []string{`"type": "vm"`, `"state": "started"`, `"group": "default"`}},
		{name: "ha groups", args: []string{"--output", "json", "ha", "groups"}, want: []string{`"id": "default"`, `"nodes": "e2e-node"`, `"comment": "Default HA group"`}},
		{name: "pools list", args: []string{"--output", "json", "pools", "list"}, want: []string{`"poolid": "admins"`, `"comment": "Admin resources"`, `"qemu/100"`}},
		{name: "cluster log", args: []string{"--output", "json", "cluster", "log"}, want: []string{`"n": 1`, `"t": "starting cluster services"`, `"n": 2`}},
		{name: "status with ha", args: []string{"--output", "json", "status"}, want: []string{`"quorum": 3`, `"ha":`, `"status": "online"`}},
		// Lifecycle commands (Tier 1, need --yes)
		{name: "vm start", args: []string{"--yes", "vm", "start", "e2e-node/100"}, want: []string{"UPID:e2e-node"}},
		{name: "vm stop", args: []string{"--yes", "vm", "stop", "e2e-node/100"}, want: []string{"UPID:e2e-node"}},
		{name: "vm shutdown", args: []string{"--yes", "vm", "shutdown", "e2e-node/100"}, want: []string{"UPID:e2e-node"}},
		{name: "vm resume", args: []string{"--yes", "vm", "resume", "e2e-node/100"}, want: []string{"UPID:e2e-node"}},
		{name: "vm pause", args: []string{"--yes", "vm", "pause", "e2e-node/100"}, want: []string{"UPID:e2e-node"}},
		{name: "vm unpause", args: []string{"--yes", "vm", "unpause", "e2e-node/100"}, want: []string{"UPID:e2e-node"}},
		// Lifecycle commands (Tier 2, need --yes --force)
		{name: "vm reset", args: []string{"--yes", "--force", "vm", "reset", "e2e-node/100"}, want: []string{"UPID:e2e-node"}},
		{name: "vm reboot", args: []string{"--yes", "--force", "vm", "reboot", "e2e-node/100"}, want: []string{"UPID:e2e-node"}},
		// Container lifecycle
		{name: "container start", args: []string{"--yes", "container", "start", "e2e-node/200"}, want: []string{"UPID:e2e-node"}},
		{name: "container stop", args: []string{"--yes", "container", "stop", "e2e-node/200"}, want: []string{"UPID:e2e-node"}},
		{name: "container shutdown", args: []string{"--yes", "container", "shutdown", "e2e-node/200"}, want: []string{"UPID:e2e-node"}},
		{name: "container reboot", args: []string{"--yes", "--force", "container", "reboot", "e2e-node/200"}, want: []string{"UPID:e2e-node"}},
		{name: "container suspend", args: []string{"--yes", "container", "suspend", "e2e-node/200"}, want: []string{"UPID:e2e-node"}},
		{name: "container resume", args: []string{"--yes", "container", "resume", "e2e-node/200"}, want: []string{"UPID:e2e-node"}},
		// Phase 3: Config updates
		{name: "vm update", args: []string{"--yes", "vm", "update", "e2e-node/100", "memory=4096", "cores=4"}, want: []string{"UPID:e2e-node"}},
		{name: "container update", args: []string{"--yes", "container", "update", "e2e-node/200", "memory=2048", "cores=2"}, want: []string{"UPID:e2e-node"}},
		// Phase 3: Snapshot mutations
		{name: "vm snapshot create", args: []string{"--yes", "vm", "snapshot", "create", "e2e-node/100", "snap1"}, want: []string{"UPID:e2e-node"}},
		{name: "vm snapshot create with desc", args: []string{"--yes", "vm", "snapshot", "create", "e2e-node/100", "snap2", "before-change"}, want: []string{"UPID:e2e-node"}},
		{name: "ct snapshot create", args: []string{"--yes", "container", "snapshot", "create", "e2e-node/200", "snap1"}, want: []string{"UPID:e2e-node"}},
		// Phase 3: Template and cloud-init
		{name: "vm cloud-init", args: []string{"--yes", "vm", "cloud-init", "e2e-node/100"}, want: []string{"UPID:e2e-node"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := Run(context.Background(), tt.args, &stdout, &stderr); err != nil {
				t.Fatalf("Run(%v): %v stderr=%q", tt.args, err, stderr.String())
			}
			out := stdout.String()
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("Run(%v) output missing %q:\n%s", tt.args, want, out)
				}
			}
		})
	}
}

// Phase 7: Multi-Cluster

func TestRunProfileExport(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "export", "e2e"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("profile export: %v stderr=%q", err, stderr.String())
	}
	out := stdout.String()
	// Sanitized export should not include credential_ref.
	if strings.Contains(out, "credential_ref") || strings.Contains(out, "env:e2e") {
		t.Fatalf("profile export leaked credential_ref: %s", out)
	}
	if !strings.Contains(out, `"name": "e2e"`) {
		t.Fatalf("profile export missing name: %s", out)
	}
	if !strings.Contains(out, `"provider": "nodex-e2e-mock"`) {
		t.Fatalf("profile export missing provider: %s", out)
	}
}

func TestRunProfileImport(t *testing.T) {
	isolateConfigAndHome(t)

	// Seed config with one existing profile.
	seed := config.DefaultConfig()
	seed.Profiles["existing"] = config.Profile{Provider: "proxmox"}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(seed, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	// Simulate stdin for import.
	importJSON := `{"provider": "proxmox", "endpoint": "https://pve2.example.invalid:8006", "ca_file": "/etc/ssl/ca.pem"}`
	oldStdin := stdinReader
	stdinReader = strings.NewReader(importJSON)
	t.Cleanup(func() { stdinReader = oldStdin })

	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), []string{"profile", "import", "lab2"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("profile import: %v stderr=%q", err, stderr.String())
	}

	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	p, ok := cfg.Profiles["lab2"]
	if !ok {
		t.Fatal("imported profile not found")
	}
	if p.Provider != "proxmox" {
		t.Fatalf("provider = %q, want proxmox", p.Provider)
	}
	if p.Endpoint != "https://pve2.example.invalid:8006" {
		t.Fatalf("endpoint = %q", p.Endpoint)
	}
	if p.CAFile != "/etc/ssl/ca.pem" {
		t.Fatalf("ca_file = %q", p.CAFile)
	}
}

func TestRunProfileImportAlreadyExists(t *testing.T) {
	isolateConfigAndHome(t)

	seed := config.DefaultConfig()
	seed.Profiles["lab2"] = config.Profile{Provider: "proxmox"}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(seed, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	importJSON := `{"provider": "proxmox", "endpoint": "https://pve.example.invalid:8006"}`
	oldStdin := stdinReader
	stdinReader = strings.NewReader(importJSON)
	t.Cleanup(func() { stdinReader = oldStdin })

	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), []string{"profile", "import", "lab2"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for duplicate profile import")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists' error, got: %v", err)
	}
}

func TestRunProfileExportNotFound(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "export", "nonexistent"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestRunProfileExportNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "export"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestRunProfileImportNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "import"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestRunStatusAll(t *testing.T) {
	isolateConfigAndHome(t)
	setupMultiProfileConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "json", "--all", "status"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("status --all: %v stderr=%q", err, stderr.String())
	}
	out := stdout.String()
	// Envelope should have schema, results with profile entries, and summary.
	if !strings.Contains(out, `"schema"`) {
		t.Fatalf("status --all missing schema: %s", out)
	}
	if !strings.Contains(out, `"profile": "e2e"`) {
		t.Fatalf("status --all missing e2e profile: %s", out)
	}
	if !strings.Contains(out, `"profile": "lab"`) {
		t.Fatalf("status --all missing lab profile: %s", out)
	}
	if !strings.Contains(out, `"summary"`) {
		t.Fatalf("status --all missing summary: %s", out)
	}
}

func TestRunNodesAll(t *testing.T) {
	isolateConfigAndHome(t)
	setupMultiProfileConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "json", "--all", "node", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("nodes --all: %v stderr=%q", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"schema"`) {
		t.Fatalf("nodes --all missing schema envelope: %s", out)
	}
	if !strings.Contains(out, `"profile": "e2e"`) {
		t.Fatalf("nodes --all missing e2e profile: %s", out)
	}
	if !strings.Contains(out, `"profile": "lab"`) {
		t.Fatalf("nodes --all missing lab profile: %s", out)
	}
	if !strings.Contains(out, `"name": "e2e-node"`) {
		t.Fatalf("nodes --all missing node: %s", out)
	}
	if !strings.Contains(out, `"summary"`) {
		t.Fatalf("nodes --all missing summary: %s", out)
	}
}

func TestRunVMsAll(t *testing.T) {
	isolateConfigAndHome(t)
	setupMultiProfileConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "json", "--all", "vm", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("vms --all: %v stderr=%q", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"schema"`) {
		t.Fatalf("vms --all missing schema envelope: %s", out)
	}
	if !strings.Contains(out, `"profile": "e2e"`) {
		t.Fatalf("vms --all missing e2e profile: %s", out)
	}
	if !strings.Contains(out, `"profile": "lab"`) {
		t.Fatalf("vms --all missing lab profile: %s", out)
	}
	if !strings.Contains(out, `"id": "e2e-node/100"`) {
		t.Fatalf("vms --all missing VM: %s", out)
	}
	if !strings.Contains(out, `"summary"`) {
		t.Fatalf("vms --all missing summary: %s", out)
	}
}

func TestRunContainersAll(t *testing.T) {
	isolateConfigAndHome(t)
	setupMultiProfileConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "json", "--all", "container", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("containers --all: %v stderr=%q", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"schema"`) {
		t.Fatalf("containers --all missing schema envelope: %s", out)
	}
	if !strings.Contains(out, `"profile": "e2e"`) {
		t.Fatalf("containers --all missing e2e profile: %s", out)
	}
	if !strings.Contains(out, `"profile": "lab"`) {
		t.Fatalf("containers --all missing lab profile: %s", out)
	}
	if !strings.Contains(out, `"id": "e2e-node/200"`) {
		t.Fatalf("containers --all missing container: %s", out)
	}
	if !strings.Contains(out, `"summary"`) {
		t.Fatalf("containers --all missing summary: %s", out)
	}
}

func TestRunAllTableOutput(t *testing.T) {
	isolateConfigAndHome(t)
	setupMultiProfileConfig(t)

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "status_all_table", args: []string{"--output", "table", "--all", "status"}, want: []string{"PROFILE", "ENDPOINT", "VERSION", "NODES", "VMS"}},
		{name: "nodes_all_table", args: []string{"--output", "table", "--all", "node", "list"}, want: []string{"PROFILE", "NAME", "STATUS"}},
		{name: "vms_all_table", args: []string{"--output", "table", "--all", "vm", "list"}, want: []string{"PROFILE", "ID", "NAME", "STATUS"}},
		{name: "containers_all_table", args: []string{"--output", "table", "--all", "container", "list"}, want: []string{"PROFILE", "ID", "NAME", "STATUS"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("Run(%v): %v stderr=%q", tt.args, err, stderr.String())
			}
			out := stdout.String()
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Fatalf("output missing %q:\n%s", w, out)
				}
			}
		})
	}
}

func TestRunAllOnMutationRejected(t *testing.T) {
	isolateConfigAndHome(t)
	setupMultiProfileConfig(t)

	tests := []struct {
		name string
		args []string
	}{
		{name: "vm_start", args: []string{"--all", "--yes", "vm", "start", "e2e-node/100"}},
		{name: "vm_stop", args: []string{"--all", "--yes", "vm", "stop", "e2e-node/100"}},
		{name: "vm_delete", args: []string{"--all", "--yes", "vm", "delete", "e2e-node/100"}},
		{name: "container_start", args: []string{"--all", "--yes", "container", "start", "e2e-node/200"}},
		{name: "backup_create", args: []string{"--all", "backup", "create", "e2e-node/100"}},
		{name: "storage_delete", args: []string{"--all", "storage", "delete", "local:iso/test.iso"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatalf("expected error for --all with mutation %q, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), "not supported") {
				t.Fatalf("error should mention 'not supported', got: %v", err)
			}
		})
	}
}

func TestDeterministicMultiProfileOrdering(t *testing.T) {
	isolateConfigAndHome(t)
	setupMultiProfileConfig(t)

	// Run node list --all twice and verify that profile order is deterministic.
	// (Exact JSON equality is not guaranteed because the "duration" field
	// depends on wall-clock timing.)
	var run1Stdout, run1Stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "json", "--all", "node", "list"}, &run1Stdout, &run1Stderr)
	if err != nil {
		t.Fatalf("run1: %v stderr=%q", err, run1Stderr.String())
	}

	var run2Stdout, run2Stderr bytes.Buffer
	err = Run(context.Background(), []string{"--output", "json", "--all", "node", "list"}, &run2Stdout, &run2Stderr)
	if err != nil {
		t.Fatalf("run2: %v stderr=%q", err, run2Stderr.String())
	}

	// Parse and extract profile order from both runs.
	extractOrder := func(in string) []string {
		var order []string
		// The results array contains "profile" keys in order.
		// Simple string scan is sufficient for deterministic output.
		start := 0
		for {
			idx := strings.Index(in[start:], `"profile"`)
			if idx == -1 {
				break
			}
			start += idx
			colon := strings.Index(in[start:], ":")
			if colon == -1 {
				break
			}
			start += colon + 1
			end := strings.Index(in[start:], ",")
			if end == -1 {
				end = strings.Index(in[start:], "\n")
			}
			if end == -1 {
				break
			}
			name := strings.TrimSpace(strings.Trim(in[start:start+end], `" `))
			if name != "" {
				order = append(order, name)
			}
			start += end
		}
		return order
	}

	order1 := extractOrder(run1Stdout.String())
	order2 := extractOrder(run2Stdout.String())

	if len(order1) != len(order2) {
		t.Fatalf("profile count differs: %d vs %d", len(order1), len(order2))
	}
	for i := range order1 {
		if order1[i] != order2[i] {
			t.Fatalf("profile order mismatch at position %d: %q vs %q", i, order1[i], order2[i])
		}
	}
}

func TestMultiProfileOutputEnvelopeStructure(t *testing.T) {
	isolateConfigAndHome(t)
	setupMultiProfileConfig(t)

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "json", "--all", "status"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("status --all: %v", err)
	}
	out := stdout.String()

	// Verify top-level envelope fields.
	for _, want := range []string{
		`"schema":`,
		`"results":`,
		`"summary":`,
		`"total":`,
		`"success":`,
		`"failed":`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing envelope field %q in:\n%s", want, out)
		}
	}

	// Verify per-profile fields in results.
	for _, want := range []string{
		`"profile":`,
		`"success":`,
		`"data":`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing result field %q in:\n%s", want, out)
		}
	}
}

func TestMultiProfileListsFailedProfilesInOutput(t *testing.T) {
	isolateConfigAndHome(t)
	setupMultiProfileConfig(t)

	// Run with --all and --output table to verify that even empty or
	// error profiles appear in output. With the current mock, both
	// profiles succeed, so we verify the table includes both.
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "table", "--all", "vm", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("vm list --all table: %v", err)
	}
	out := stdout.String()

	// Both profiles should appear in table.
	if !strings.Contains(out, "e2e") {
		t.Fatalf("table output missing e2e profile:\n%s", out)
	}
	if !strings.Contains(out, "lab") {
		t.Fatalf("table output missing lab profile:\n%s", out)
	}
	// Table headers should include PROFILE column.
	if !strings.Contains(out, "PROFILE") {
		t.Fatalf("table output missing PROFILE header:\n%s", out)
	}
}

func setupMultiProfileConfig(t *testing.T) {
	t.Helper()
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	cfg := config.DefaultConfig()
	cfg.CurrentProfile = "e2e"
	cfg.Profiles["e2e"] = config.Profile{
		Provider:      e2eMockProviderName,
		Endpoint:      "https://e2e.example.invalid",
		CredentialRef: "env:e2e",
	}
	cfg.Profiles["lab"] = config.Profile{
		Provider:      e2eMockProviderName,
		Endpoint:      "https://e2e.example.invalid",
		CredentialRef: "env:e2e",
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("seed multi-profile config: %v", err)
	}
}
