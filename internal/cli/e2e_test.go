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

func (p *e2eMockProvider) Name() string    { return e2eMockProviderName }
func (p *e2eMockProvider) Version() string { return "e2e" }
func (p *e2eMockProvider) Close() error    { return nil }
func (p *e2eMockProvider) Capabilities() []domain.Capability {
	return []domain.Capability{domain.CapabilityNodes, domain.CapabilityVMs, domain.CapabilityContainers, domain.CapabilityStorage, domain.CapabilityCluster}
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
