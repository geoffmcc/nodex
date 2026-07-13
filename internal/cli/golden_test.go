package cli

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/config"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestGoldenJSON(t *testing.T) {
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
	}{
		{name: "node_list", args: []string{"--output", "json", "node", "list"}},
		{name: "node_show", args: []string{"--output", "json", "node", "show", "e2e-node"}},
		{name: "node_status", args: []string{"--output", "json", "node", "status", "e2e-node"}},
		{name: "vm_list", args: []string{"--output", "json", "vm", "list"}},
		{name: "vm_show", args: []string{"--output", "json", "vm", "show", "e2e-node/100"}},
		{name: "vm_config", args: []string{"--output", "json", "vm", "config", "e2e-node/100"}},
		{name: "vm_snapshots", args: []string{"--output", "json", "vm", "snapshots", "e2e-node/100"}},
		{name: "container_list", args: []string{"--output", "json", "container", "list"}},
		{name: "container_show", args: []string{"--output", "json", "container", "show", "e2e-node/200"}},
		{name: "container_config", args: []string{"--output", "json", "container", "config", "e2e-node/200"}},
		{name: "container_snapshots", args: []string{"--output", "json", "container", "snapshots", "e2e-node/200"}},
		{name: "storage_list", args: []string{"--output", "json", "storage", "list"}},
		{name: "storage_show", args: []string{"--output", "json", "storage", "show", "local"}},
		{name: "storage_content", args: []string{"--output", "json", "storage", "content", "e2e-node", "local"}},
		{name: "cluster_status", args: []string{"--output", "json", "cluster", "status"}},
		{name: "task_list", args: []string{"--output", "json", "task", "list", "e2e-node"}},
		{name: "task_show", args: []string{"--output", "json", "task", "show", "e2e-node", "UPID:e2e-node/00012345/0"}},
		{name: "event_list", args: []string{"--output", "json", "event", "list"}},
		{name: "log", args: []string{"--output", "json", "log", "e2e-node"}},
		{name: "backup_list", args: []string{"--output", "json", "backup", "list", "e2e-node"}},
		{name: "firewall_list", args: []string{"--output", "json", "firewall", "list"}},
		{name: "ha_list", args: []string{"--output", "json", "ha", "list"}},
		{name: "ha_groups", args: []string{"--output", "json", "ha", "groups"}},
		{name: "status", args: []string{"--output", "json", "status"}},
		{name: "node_services", args: []string{"--output", "json", "node", "services", "e2e-node"}},
		{name: "node_network", args: []string{"--output", "json", "node", "network", "e2e-node"}},
		{name: "node_dns", args: []string{"--output", "json", "node", "dns", "e2e-node"}},
		{name: "node_time", args: []string{"--output", "json", "node", "time", "e2e-node"}},
		{name: "node_disks", args: []string{"--output", "json", "node", "disks", "e2e-node"}},
		{name: "node_certificates", args: []string{"--output", "json", "node", "certificates", "e2e-node"}},
		{name: "node_subscription", args: []string{"--output", "json", "node", "subscription", "e2e-node"}},
		{name: "node_updates", args: []string{"--output", "json", "node", "updates", "e2e-node"}},
		{name: "firewall_aliases", args: []string{"--output", "json", "firewall", "aliases"}},
		{name: "firewall_ipsets", args: []string{"--output", "json", "firewall", "ipsets"}},
		{name: "firewall_ipset", args: []string{"--output", "json", "firewall", "ipset", "test-set"}},
		{name: "firewall_security_groups", args: []string{"--output", "json", "firewall", "security-groups"}},
		{name: "firewall_options", args: []string{"--output", "json", "firewall", "options"}},
		{name: "firewall_node_rules", args: []string{"--output", "json", "firewall", "node-rules", "e2e-node"}},
		{name: "firewall_vm_rules", args: []string{"--output", "json", "firewall", "vm-rules", "e2e-node/100"}},
	}

	goldenDir := filepath.Join("testdata", "golden")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatalf("create golden dir: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := Run(context.Background(), tt.args, &stdout, &stderr); err != nil {
				t.Fatalf("Run(%v): %v stderr=%q", tt.args, err, stderr.String())
			}
			got := stdout.String()
			goldenPath := filepath.Join(goldenDir, tt.name+".json")

			if *updateGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("updated golden: %s", goldenPath)
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Fatalf("golden file missing; run with -update to create: %s", goldenPath)
				}
				t.Fatalf("read golden: %v", err)
			}
			if got != string(want) {
				t.Errorf("output mismatch for %s\ngot:\n%s\nwant:\n%s\nRun with -update to refresh golden files", tt.name, got, string(want))
			}
		})
	}
}

func TestGoldenYAML(t *testing.T) {
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
	}{
		{name: "node_list", args: []string{"--output", "yaml", "node", "list"}},
		{name: "vm_list", args: []string{"--output", "yaml", "vm", "list"}},
		{name: "status", args: []string{"--output", "yaml", "status"}},
	}

	goldenDir := filepath.Join("testdata", "golden", "yaml")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatalf("create golden yaml dir: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := Run(context.Background(), tt.args, &stdout, &stderr); err != nil {
				t.Fatalf("Run(%v): %v stderr=%q", tt.args, err, stderr.String())
			}
			got := stdout.String()
			goldenPath := filepath.Join(goldenDir, tt.name+".yaml")

			if *updateGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("updated golden: %s", goldenPath)
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Fatalf("golden file missing; run with -update to create: %s", goldenPath)
				}
				t.Fatalf("read golden: %v", err)
			}
			if got != string(want) {
				t.Errorf("output mismatch for %s\ngot:\n%s\nwant:\n%s\nRun with -update to refresh golden files", tt.name, got, string(want))
			}
		})
	}
}

func TestGoldenTable(t *testing.T) {
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
	}{
		{name: "node_list", args: []string{"--output", "table", "node", "list"}},
		{name: "vm_list", args: []string{"--output", "table", "vm", "list"}},
		{name: "node_status", args: []string{"--output", "table", "node", "status", "e2e-node"}},
		{name: "storage_list", args: []string{"--output", "table", "storage", "list"}},
		{name: "task_list", args: []string{"--output", "table", "task", "list", "e2e-node"}},
		{name: "ha_list", args: []string{"--output", "table", "ha", "list"}},
		{name: "firewall_list", args: []string{"--output", "table", "firewall", "list"}},
		{name: "backup_list", args: []string{"--output", "table", "backup", "list", "e2e-node"}},
		{name: "cluster_status", args: []string{"--output", "table", "cluster", "status"}},
	}

	goldenDir := filepath.Join("testdata", "golden", "table")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatalf("create golden table dir: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := Run(context.Background(), tt.args, &stdout, &stderr); err != nil {
				t.Fatalf("Run(%v): %v stderr=%q", tt.args, err, stderr.String())
			}
			got := stdout.String()
			goldenPath := filepath.Join(goldenDir, tt.name+".txt")

			if *updateGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("updated golden: %s", goldenPath)
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Fatalf("golden file missing; run with -update to create: %s", goldenPath)
				}
				t.Fatalf("read golden: %v", err)
			}
			if got != string(want) {
				t.Errorf("output mismatch for %s\ngot:\n%s\nwant:\n%s\nRun with -update to refresh golden files", tt.name, got, string(want))
			}
		})
	}
}

func TestGoldenUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no_args", args: []string{}, want: "Nodex"},
		{name: "help", args: []string{"help"}, want: "Commands:"},
		{name: "version", args: []string{"version"}, want: "Nodex"},
		{name: "unknown_command", args: []string{"bogus"}, want: "unknown command"},
		{name: "vm_list_usage", args: []string{"vm"}, want: "Subcommands:"},
		{name: "node_list_usage", args: []string{"node"}, want: "Subcommands:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			out := stdout.String()
			combined := out
			if err != nil {
				combined += err.Error()
			}
			if !strings.Contains(combined, tt.want) {
				t.Errorf("output missing %q:\nstdout=%s\nerr=%v", tt.want, out, err)
			}
		})
	}
}

func TestGoldenFormatErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "bad_format", args: []string{"--output", "csv", "node", "list"}, want: "invalid output format"},
		{name: "bad_timeout", args: []string{"--timeout", "0s", "node", "list"}, want: "timeout must be greater than zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q missing %q", err.Error(), tt.want)
			}

		})
	}
}
