package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/ansible"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/monitor"
)

func TestCheckMonitorServiceUsesEnrolledReadOnlyCheck(t *testing.T) {
	previous := runMonitorServiceCheck
	t.Cleanup(func() { runMonitorServiceCheck = previous })
	calls := 0
	runMonitorServiceCheck = func(_ context.Context, host ansible.HostSpec, service string) (*ansible.RunResult, error) {
		calls++
		if host.Name != "fileserver" || host.Address != "192.0.2.4" || service != "smbd.service" {
			t.Fatalf("service check target = %+v, %q", host, service)
		}
		return &ansible.RunResult{
			Success: true, EvidenceComplete: true,
			TaskOutcomes: map[string][]ansible.TaskOutcome{
				"fileserver": {{EvidenceID: ansible.MonitorServiceEvidence, Message: `{"service":"smbd.service","found":true,"state":"running","status":"enabled"}`}},
			},
		}, nil
	}
	cfg := &config.Config{Inventory: &config.Inventory{Hosts: map[string]config.InventoryHost{
		"fileserver": {Address: "192.0.2.4", Role: config.RoleGeneric, SSHUser: "automation", Environment: "lab"},
	}}}
	result := checkMonitorService(context.Background(), cfg, "samba", config.MonitorTarget{
		Type: "service", Address: "fileserver", Service: "smbd.service", Environment: "lab",
	})
	if calls != 1 || result.State != monitor.Healthy {
		t.Fatalf("service result=%+v calls=%d", result, calls)
	}
}

func TestCheckMonitorServiceDoesNotInferHealthyWhenEvidenceMissing(t *testing.T) {
	previous := runMonitorServiceCheck
	t.Cleanup(func() { runMonitorServiceCheck = previous })
	runMonitorServiceCheck = func(context.Context, ansible.HostSpec, string) (*ansible.RunResult, error) {
		return &ansible.RunResult{Success: false, EvidenceComplete: false}, nil
	}
	cfg := &config.Config{Inventory: &config.Inventory{Hosts: map[string]config.InventoryHost{
		"fileserver": {Address: "192.0.2.4", Role: config.RoleGeneric, SSHUser: "automation"},
	}}}
	result := checkMonitorService(context.Background(), cfg, "samba", config.MonitorTarget{
		Type: "service", Address: "fileserver", Service: "smbd.service",
	})
	if result.State != monitor.Unknown {
		t.Fatalf("service result = %+v, want unknown", result)
	}
}

func TestMonitorCheckRunsAnsibleServiceTargetFromInventory(t *testing.T) {
	_, cfgDir := isolatedConfigDir(t)
	body := `version: 2
inventory:
  hosts:
    fileserver:
      address: 192.0.2.44
      role: generic
      ssh_user: automation
monitoring:
  targets:
    samba:
      type: service
      address: fileserver
      service: smbd.service
`
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := runMonitorServiceCheck
	t.Cleanup(func() { runMonitorServiceCheck = previous })
	runMonitorServiceCheck = func(_ context.Context, host ansible.HostSpec, service string) (*ansible.RunResult, error) {
		if host.Name != "fileserver" || service != "smbd.service" {
			t.Fatalf("service target host=%+v service=%q", host, service)
		}
		return &ansible.RunResult{
			Success: true, EvidenceComplete: true,
			Hosts: []ansible.HostResult{{Host: "fileserver", OK: 1}},
			TaskOutcomes: map[string][]ansible.TaskOutcome{
				"fileserver": {{EvidenceID: ansible.MonitorServiceEvidence, Message: `{"service":"smbd.service","found":true,"state":"running","status":"enabled"}`}},
			},
		}, nil
	}
	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"--output", "json", "monitor", "check", "--target", "samba"}, &stdout, &stderr); err != nil {
		t.Fatalf("monitor service check: %v; stderr=%s", err, stderr.String())
	}
	var report monitor.Report
	if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout.String())
	}
	if report.Overall != monitor.Healthy || len(report.Results) != 1 || report.Results[0].State != monitor.Healthy {
		t.Fatalf("service report = %+v", report)
	}
}
