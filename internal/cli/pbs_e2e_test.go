package cli

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider"
)

const pbsE2EMockProviderName = "nodex-pbs-e2e-mock"

func init() {
	provider.Register(pbsE2EMockProviderName, func() domain.Provider { return &pbsE2EMockProvider{} })
}

// pbsE2EEmptyMode makes every mock listing return empty results. The
// provider registry creates a fresh instance per connection, so the toggle
// is package-level.
var pbsE2EEmptyMode bool

// pbsE2EMockProvider implements domain.Provider plus every PBS inspector
// interface with canned fictional data.
type pbsE2EMockProvider struct{}

func (p *pbsE2EMockProvider) isEmpty() bool { return pbsE2EEmptyMode }

func (p *pbsE2EMockProvider) Name() string                   { return pbsE2EMockProviderName }
func (p *pbsE2EMockProvider) Version() string                { return "e2e" }
func (p *pbsE2EMockProvider) Close() error                   { return nil }
func (p *pbsE2EMockProvider) Health(_ context.Context) error { return nil }
func (p *pbsE2EMockProvider) Connect(_ context.Context, _ string, _ *domain.Credentials) error {
	return nil
}

func (p *pbsE2EMockProvider) Capabilities() []domain.Capability {
	return []domain.Capability{
		domain.CapabilityPBSSystem, domain.CapabilityPBSDatastores,
		domain.CapabilityPBSSnapshots, domain.CapabilityPBSTasks,
		domain.CapabilityPBSJobs, domain.CapabilityPBSGC,
	}
}

const pbsE2EUPID = "UPID:pbs-e2e:00001234:00005678:00000001:65f00000:garbage_collection:backups:automation@pbs!nodex:"

func (p *pbsE2EMockProvider) PBSVersionInfo(_ context.Context) (*domain.PBSVersionInfo, error) {
	return &domain.PBSVersionInfo{Version: "4.0.1", Release: "1", RepoID: "fictionalrepo"}, nil
}

func (p *pbsE2EMockProvider) PBSNodeStatus(_ context.Context) (*domain.PBSNodeStatus, error) {
	return &domain.PBSNodeStatus{
		CPU: 0.05, Wait: 0.01, Uptime: 86400,
		LoadAvg:       []float64{0.5, 0.4, 0.3},
		KernelVersion: "Linux 6.8.12-e2e",
		CPUModel:      "Fictional CPU", CPUs: 8,
		MemoryTotal: 16384, MemoryUsed: 4096, MemoryFree: 12288,
		SwapTotal: 8192, SwapUsed: 0,
		RootTotal: 100000, RootUsed: 20000, RootAvail: 80000,
		BootMode: "efi",
	}, nil
}

func (p *pbsE2EMockProvider) PBSSubscription(_ context.Context) (*domain.PBSSubscription, error) {
	return &domain.PBSSubscription{Status: "notfound", Message: "There is no subscription key"}, nil
}

func (p *pbsE2EMockProvider) PBSCertificates(_ context.Context) ([]domain.PBSCertificate, error) {
	if p.isEmpty() {
		return nil, nil
	}
	return []domain.PBSCertificate{{
		Filename: "proxy.pem", Subject: "CN=pbs-e2e.example.invalid",
		Issuer: "CN=Fictional CA", Fingerprint: "aa:bb:cc",
		NotAfter: 1783072000, PublicKeyType: "id-ecPublicKey", PublicKeyBits: 384,
		SAN: []string{"pbs-e2e.example.invalid"},
	}}, nil
}

func (p *pbsE2EMockProvider) PBSDatastores(_ context.Context) ([]domain.PBSDatastore, error) {
	if p.isEmpty() {
		return nil, nil
	}
	return []domain.PBSDatastore{{
		Name: "backups", Path: "/mnt/datastore/backups", Comment: "e2e datastore",
		GCSchedule: "daily", PruneSchedule: "daily", VerifyNew: true,
	}}, nil
}

func (p *pbsE2EMockProvider) PBSDatastore(_ context.Context, name string) (*domain.PBSDatastore, error) {
	return &domain.PBSDatastore{
		Name: name, Path: "/mnt/datastore/" + name, Comment: "e2e datastore",
		GCSchedule: "daily", VerifyNew: true,
	}, nil
}

func (p *pbsE2EMockProvider) PBSDatastoreStatus(_ context.Context, store string) (*domain.PBSDatastoreStatus, error) {
	return &domain.PBSDatastoreStatus{Store: store, Total: 1000, Used: 400, Avail: 600}, nil
}

func (p *pbsE2EMockProvider) PBSDatastoreUsages(_ context.Context) ([]domain.PBSDatastoreUsage, error) {
	return []domain.PBSDatastoreUsage{{Store: "backups", Total: 1000, Used: 400, Avail: 600, MountStatus: "ok"}}, nil
}

func (p *pbsE2EMockProvider) PBSSnapshots(_ context.Context, store string, filter domain.PBSSnapshotFilter) ([]domain.PBSSnapshot, error) {
	if p.isEmpty() {
		return nil, nil
	}
	snaps := []domain.PBSSnapshot{
		{
			Store: store, Namespace: filter.Namespace,
			BackupType: "vm", BackupID: "100", BackupTime: 1752000000,
			Size: 1024, Owner: "automation@pbs!nodex", Protected: true,
			Files:        []string{"drive-scsi0.img.fidx"},
			Verification: &domain.PBSVerificationState{State: "ok", UPID: pbsE2EUPID},
		},
		{
			Store: store, Namespace: filter.Namespace,
			BackupType: "host", BackupID: "dns-primary", BackupTime: 1752003600,
			Size: 512, Owner: "automation@pbs!nodex",
		},
	}
	if filter.BackupType != "" {
		var out []domain.PBSSnapshot
		for _, s := range snaps {
			if s.BackupType == filter.BackupType {
				out = append(out, s)
			}
		}
		return out, nil
	}
	return snaps, nil
}

func (p *pbsE2EMockProvider) PBSTasks(_ context.Context, filter domain.PBSTaskFilter) ([]domain.PBSTask, error) {
	if p.isEmpty() {
		return nil, nil
	}
	tasks := []domain.PBSTask{
		{
			UPID: pbsE2EUPID, Node: "pbs-e2e", WorkerType: "garbage_collection",
			WorkerID: "backups", User: "automation@pbs!nodex",
			StartTime: 1752000000, EndTime: 1752000300, Status: "OK",
		},
		{
			UPID: "UPID:pbs-e2e:0000AAAA:0000BBBB:00000002:65f00001:verificationjob:backups:automation@pbs!nodex:",
			Node: "pbs-e2e", WorkerType: "verificationjob", WorkerID: "backups",
			User: "automation@pbs!nodex", StartTime: 1752001000, Status: "running",
		},
	}
	if filter.Running {
		var out []domain.PBSTask
		for _, t := range tasks {
			if t.Status == "running" {
				out = append(out, t)
			}
		}
		return out, nil
	}
	return tasks, nil
}

func (p *pbsE2EMockProvider) PBSTaskStatus(_ context.Context, upid string) (*domain.PBSTaskStatus, error) {
	return &domain.PBSTaskStatus{
		UPID: upid, Node: "pbs-e2e", PID: 4660,
		WorkerType: "garbage_collection", WorkerID: "backups",
		User: "automation@pbs!nodex", StartTime: 1752000000, EndTime: 1752000300,
		Status: "stopped", ExitStatus: "OK",
	}, nil
}

func (p *pbsE2EMockProvider) PBSTaskLog(_ context.Context, upid string) ([]domain.PBSTaskLogLine, error) {
	return []domain.PBSTaskLogLine{
		{LineNumber: 1, Text: "starting garbage collection"},
		{LineNumber: 2, Text: "TASK OK"},
	}, nil
}

func (p *pbsE2EMockProvider) PBSVerifyJobs(_ context.Context) ([]domain.PBSVerifyJob, error) {
	if p.isEmpty() {
		return nil, nil
	}
	return []domain.PBSVerifyJob{{
		ID: "v-daily", Store: "backups", Schedule: "daily", OutdatedAfter: 30, Comment: "verify all",
	}}, nil
}

func (p *pbsE2EMockProvider) PBSPruneJobs(_ context.Context) ([]domain.PBSPruneJob, error) {
	if p.isEmpty() {
		return nil, nil
	}
	return []domain.PBSPruneJob{{
		ID: "p-daily", Store: "backups", Schedule: "daily",
		KeepDaily: 7, KeepWeekly: 4,
	}}, nil
}

func (p *pbsE2EMockProvider) PBSSyncJobs(_ context.Context) ([]domain.PBSSyncJob, error) {
	if p.isEmpty() {
		return nil, nil
	}
	return []domain.PBSSyncJob{{
		ID: "s-offsite", Store: "backups", Remote: "offsite", RemoteStore: "replica",
		SyncDirection: "pull", Schedule: "hourly",
	}}, nil
}

func (p *pbsE2EMockProvider) PBSGCStatuses(_ context.Context) ([]domain.PBSGCStatus, error) {
	if p.isEmpty() {
		return nil, nil
	}
	return []domain.PBSGCStatus{{
		Store: "backups", Schedule: "daily", LastRunState: "OK",
		LastRunEndtime: 1752000300, NextRun: 1752086400,
		RemovedBytes: 1024, PendingBytes: 512,
	}}, nil
}

func (p *pbsE2EMockProvider) PBSGCStatus(_ context.Context, store string) (*domain.PBSGCStatus, error) {
	return &domain.PBSGCStatus{
		Store: store, Schedule: "daily", LastRunState: "OK",
		LastRunEndtime: 1752000300, RemovedBytes: 1024,
	}, nil
}

// seedPBSE2EConfig isolates the environment and writes a config whose
// current profile uses the PBS e2e mock provider. A second profile ("pve")
// uses the PVE e2e mock so capability mismatches can be exercised.
func seedPBSE2EConfig(t *testing.T) {
	t.Helper()
	isolateConfigAndHome(t)
	t.Setenv("NODEX_PBS_E2E_TOKEN", "e2e-token")
	t.Setenv("NODEX_PVE_TOKEN", "e2e-token")
	cfg := config.DefaultConfig()
	cfg.CurrentProfile = "pbs-e2e"
	cfg.Profiles["pbs-e2e"] = config.Profile{
		Provider:      pbsE2EMockProviderName,
		Endpoint:      "https://pbs-e2e.example.invalid",
		CredentialRef: "env:pbs-e2e", // #nosec G101 -- backend reference, not a secret
	}
	cfg.Profiles["pve"] = config.Profile{
		Provider:      e2eMockProviderName,
		Endpoint:      "https://e2e.example.invalid",
		CredentialRef: "env:pve",
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func runPBSCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestPBSE2E_TableOutputs(t *testing.T) {
	seedPBSE2EConfig(t)
	tests := []struct {
		name     string
		args     []string
		contains []string
	}{
		{"status", []string{"pbs", "status"}, []string{"CPU:", "Memory:", "Kernel:", "Linux 6.8.12-e2e"}},
		{"version", []string{"pbs", "version"}, []string{"Proxmox Backup Server 4.0.1", "release 1"}},
		{"subscription", []string{"pbs", "subscription"}, []string{"notfound"}},
		{"certificates", []string{"pbs", "certificates"}, []string{"proxy.pem", "CN=Fictional CA"}},
		{"datastore list", []string{"pbs", "datastore", "list"}, []string{"backups", "/mnt/datastore/backups", "daily"}},
		{"datastore show", []string{"pbs", "datastore", "show", "backups"}, []string{"Name:", "backups", "Usage:"}},
		{"snapshot list", []string{"pbs", "snapshot", "list", "--datastore", "backups"}, []string{"vm", "100", "dns-primary", "ok"}},
		{"snapshot list filtered", []string{"pbs", "snapshot", "list", "--datastore", "backups", "--backup-type", "host"}, []string{"dns-primary"}},
		{"task list", []string{"pbs", "task", "list"}, []string{"garbage_collection", "verificationjob", "OK"}},
		{"task list running", []string{"pbs", "task", "list", "--running"}, []string{"verificationjob", "running"}},
		{"task show", []string{"pbs", "task", "show", pbsE2EUPID}, []string{"UPID:", "garbage_collection", "Exit status: OK"}},
		{"task log", []string{"pbs", "task", "log", pbsE2EUPID}, []string{"starting garbage collection", "TASK OK"}},
		{"verify list", []string{"pbs", "verify", "list"}, []string{"v-daily", "backups", "30d"}},
		{"prune list", []string{"pbs", "prune", "list"}, []string{"p-daily", "daily=7", "weekly=4"}},
		{"sync list", []string{"pbs", "sync", "list"}, []string{"s-offsite", "offsite", "replica", "pull"}},
		{"gc status", []string{"pbs", "garbage-collection", "status"}, []string{"backups", "OK"}},
		{"gc status one store", []string{"pbs", "garbage-collection", "status", "--datastore", "backups"}, []string{"backups", "OK"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := runPBSCommand(t, append([]string{"--output", "table"}, tt.args...)...)
			if err != nil {
				t.Fatalf("Run(%v): %v (stderr: %s)", tt.args, err, stderr)
			}
			for _, want := range tt.contains {
				if !strings.Contains(stdout, want) {
					t.Errorf("output of %v missing %q:\n%s", tt.args, want, stdout)
				}
			}
		})
	}
}

func TestPBSE2E_JSONAndYAMLParity(t *testing.T) {
	seedPBSE2EConfig(t)
	commands := [][]string{
		{"pbs", "status"},
		{"pbs", "version"},
		{"pbs", "subscription"},
		{"pbs", "certificates"},
		{"pbs", "datastore", "list"},
		{"pbs", "datastore", "show", "backups"},
		{"pbs", "snapshot", "list", "--datastore", "backups"},
		{"pbs", "task", "list"},
		{"pbs", "task", "show", pbsE2EUPID},
		{"pbs", "task", "log", pbsE2EUPID},
		{"pbs", "verify", "list"},
		{"pbs", "prune", "list"},
		{"pbs", "sync", "list"},
		{"pbs", "garbage-collection", "status"},
	}
	for _, cmd := range commands {
		t.Run(strings.Join(cmd, "_"), func(t *testing.T) {
			jsonOut, _, err := runPBSCommand(t, append([]string{"--output", "json"}, cmd...)...)
			if err != nil {
				t.Fatalf("json run: %v", err)
			}
			if !json.Valid([]byte(jsonOut)) {
				t.Errorf("invalid JSON output:\n%s", jsonOut)
			}
			yamlOut, _, err := runPBSCommand(t, append([]string{"--output", "yaml"}, cmd...)...)
			if err != nil {
				t.Fatalf("yaml run: %v", err)
			}
			if strings.TrimSpace(yamlOut) == "" {
				t.Error("empty YAML output")
			}
			tableOut, _, err := runPBSCommand(t, append([]string{"--output", "table"}, cmd...)...)
			if err != nil {
				t.Fatalf("table run: %v", err)
			}
			if strings.TrimSpace(tableOut) == "" {
				t.Error("empty table output")
			}
		})
	}
}

// TestPBSE2E_EmptyListsStable verifies list commands emit stable empty
// results ([] in JSON) rather than null or errors.
func TestPBSE2E_EmptyListsStable(t *testing.T) {
	seedPBSE2EConfig(t)
	pbsE2EEmptyMode = true
	t.Cleanup(func() { pbsE2EEmptyMode = false })

	lists := [][]string{
		{"pbs", "certificates"},
		{"pbs", "datastore", "list"},
		{"pbs", "snapshot", "list", "--datastore", "backups"},
		{"pbs", "task", "list"},
		{"pbs", "verify", "list"},
		{"pbs", "prune", "list"},
		{"pbs", "sync", "list"},
		{"pbs", "garbage-collection", "status"},
	}
	for _, cmd := range lists {
		t.Run(strings.Join(cmd, "_"), func(t *testing.T) {
			out, _, err := runPBSCommand(t, append([]string{"--output", "json"}, cmd...)...)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			trimmed := strings.TrimSpace(out)
			if trimmed != "[]" {
				t.Errorf("expected stable empty list [], got: %q", trimmed)
			}
		})
	}
}

// TestPBSE2E_UnsupportedOnPVEProfile verifies pbs commands against a
// non-PBS provider fail with the unsupported-capability exit code.
func TestPBSE2E_UnsupportedOnPVEProfile(t *testing.T) {
	seedPBSE2EConfig(t)
	commands := [][]string{
		{"--profile", "pve", "pbs", "status"},
		{"--profile", "pve", "pbs", "datastore", "list"},
		{"--profile", "pve", "pbs", "snapshot", "list", "--datastore", "x"},
		{"--profile", "pve", "pbs", "task", "list"},
		{"--profile", "pve", "pbs", "verify", "list"},
		{"--profile", "pve", "pbs", "garbage-collection", "status"},
	}
	for _, cmd := range commands {
		t.Run(strings.Join(cmd, "_"), func(t *testing.T) {
			_, _, err := runPBSCommand(t, cmd...)
			if err == nil {
				t.Fatal("expected unsupported-capability error")
			}
			var exitCode *app.ExitCoder
			if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUnsupportedCap {
				t.Errorf("error = %v, want ExitUnsupportedCap", err)
			}
		})
	}
}

// TestPVEE2E_PBSCommandsDoNotBreakPVE verifies PVE commands fail with
// unsupported-capability against the PBS provider rather than panicking.
func TestPBSE2E_PVECommandsUnsupportedOnPBS(t *testing.T) {
	seedPBSE2EConfig(t)
	_, _, err := runPBSCommand(t, "node", "list")
	if err == nil {
		t.Fatal("expected node list to fail against pbs provider")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUnsupportedCap {
		t.Errorf("error = %v, want ExitUnsupportedCap", err)
	}
}

func TestPBSE2E_UsageErrors(t *testing.T) {
	seedPBSE2EConfig(t)
	commands := [][]string{
		{"pbs", "status", "extra"},
		{"pbs", "datastore"},
		{"pbs", "datastore", "bogus"},
		{"pbs", "datastore", "show"},
		{"pbs", "snapshot", "list"}, // missing --datastore
		{"pbs", "snapshot", "list", "--datastore", "backups", "--backup-type", "bogus"},
		{"pbs", "task"},
		{"pbs", "task", "show"},
		{"pbs", "task", "list", "--bogus"},
		{"pbs", "verify"},
		{"pbs", "prune", "bogus"},
		{"pbs", "sync"},
		{"pbs", "garbage-collection"},
		{"pbs", "garbage-collection", "status", "--datastore"},
	}
	for _, cmd := range commands {
		t.Run(strings.Join(cmd, "_"), func(t *testing.T) {
			_, _, err := runPBSCommand(t, cmd...)
			if err == nil {
				t.Fatal("expected usage error")
			}
			var exitCode *app.ExitCoder
			if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
				t.Errorf("error = %v, want ExitUsage", err)
			}
		})
	}
}

func TestPBSE2E_ProviderListShowsPBS(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"provider", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("provider list: %v", err)
	}
	if !strings.Contains(stdout.String(), "pbs") {
		t.Error("provider list must include pbs")
	}
}

func TestPBSE2E_ProviderCapabilities(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"provider", "capabilities", "pbs"}, &stdout, &stderr); err != nil {
		t.Fatalf("provider capabilities pbs: %v", err)
	}
	out := stdout.String()
	for _, cap := range []string{"pbs_system", "pbs_datastores", "pbs_snapshots", "pbs_tasks", "pbs_jobs", "pbs_gc"} {
		if !strings.Contains(out, cap) {
			t.Errorf("capabilities output missing %q:\n%s", cap, out)
		}
	}
}
