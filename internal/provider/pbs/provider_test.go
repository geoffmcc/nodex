package pbs

import (
	"context"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/provider"
	"github.com/geoffmcc/nodex/internal/provider/pbs/client"
)

func TestProviderRegistered(t *testing.T) {
	if !provider.IsRegistered(ProviderName) {
		t.Fatal("pbs provider must self-register")
	}
	prov, err := provider.Get(ProviderName)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if prov.Name() != "pbs" {
		t.Errorf("Name() = %q, want pbs", prov.Name())
	}
}

func TestProviderCapabilities(t *testing.T) {
	p := &Provider{}
	caps := p.Capabilities()
	want := map[domain.Capability]bool{
		domain.CapabilityPBSSystem:     false,
		domain.CapabilityPBSDatastores: false,
		domain.CapabilityPBSSnapshots:  false,
		domain.CapabilityPBSTasks:      false,
		domain.CapabilityPBSJobs:       false,
		domain.CapabilityPBSGC:         false,
		domain.CapabilityPBSVerifyRun:  false,
		domain.CapabilityPBSSyncRun:    false,
		domain.CapabilityPBSPruneRun:   false,
		domain.CapabilityPBSGCRun:      false,
	}
	for _, c := range caps {
		if _, ok := want[c]; !ok {
			t.Errorf("unexpected capability %q", c)
			continue
		}
		want[c] = true
	}
	for c, seen := range want {
		if !seen {
			t.Errorf("missing capability %q", c)
		}
	}
	for _, c := range caps {
		if _, ok := domain.CapabilityMetadata()[c]; !ok {
			t.Errorf("capability %q missing from CapabilityMetadata", c)
		}
	}
}

// TestPBSMutationSafetyTiers pins the product decision gate outcomes: each
// guarded PBS mutation carries the safety tier the gate assigned.
func TestPBSMutationSafetyTiers(t *testing.T) {
	meta := domain.CapabilityMetadata()
	tests := []struct {
		cap  domain.Capability
		tier domain.SafetyTier
	}{
		{domain.CapabilityPBSVerifyRun, domain.TierReversible},
		{domain.CapabilityPBSSyncRun, domain.TierDisruptive},
		{domain.CapabilityPBSPruneRun, domain.TierDestructive},
		{domain.CapabilityPBSGCRun, domain.TierDisruptive},
	}
	for _, tt := range tests {
		m, ok := meta[tt.cap]
		if !ok {
			t.Errorf("capability %q missing from metadata", tt.cap)
			continue
		}
		if m.Category != domain.CapMutation {
			t.Errorf("capability %q must be a mutation", tt.cap)
		}
		if m.Safety != tt.tier {
			t.Errorf("capability %q tier = %q, want %q", tt.cap, m.Safety, tt.tier)
		}
	}
}

// TestProviderImplementsDeclaredInterfaces asserts the structural contract:
// every capability the provider declares maps to interfaces the provider
// actually implements.
func TestProviderImplementsDeclaredInterfaces(t *testing.T) {
	var prov domain.Provider = &Provider{}
	checks := map[string]bool{}
	_, checks["PBSSystemInspector"] = prov.(domain.PBSSystemInspector)
	_, checks["PBSDatastoreInspector"] = prov.(domain.PBSDatastoreInspector)
	_, checks["PBSSnapshotInspector"] = prov.(domain.PBSSnapshotInspector)
	_, checks["PBSTaskInspector"] = prov.(domain.PBSTaskInspector)
	_, checks["PBSJobInspector"] = prov.(domain.PBSJobInspector)
	_, checks["PBSGCInspector"] = prov.(domain.PBSGCInspector)
	_, checks["PBSVerifyRunner"] = prov.(domain.PBSVerifyRunner)
	_, checks["PBSSyncRunner"] = prov.(domain.PBSSyncRunner)
	_, checks["PBSPruneRunner"] = prov.(domain.PBSPruneRunner)
	_, checks["PBSGCRunner"] = prov.(domain.PBSGCRunner)

	meta := domain.CapabilityMetadata()
	for _, c := range prov.Capabilities() {
		for _, iface := range meta[c].Interfaces {
			ok, known := checks[iface]
			if !known {
				t.Errorf("capability %q declares unknown interface %q", c, iface)
				continue
			}
			if !ok {
				t.Errorf("capability %q declared but interface %q not implemented", c, iface)
			}
		}
	}
}

// TestProviderDoesNotImplementPVEInterfaces guards the provider boundary: a
// PBS provider must not masquerade as a PVE inspector.
func TestProviderDoesNotImplementPVEInterfaces(t *testing.T) {
	var prov domain.Provider = &Provider{}
	if _, ok := prov.(domain.NodeInspector); ok {
		t.Error("pbs provider must not implement the PVE NodeInspector")
	}
	if _, ok := prov.(domain.VMInspector); ok {
		t.Error("pbs provider must not implement the PVE VMInspector")
	}
	if _, ok := prov.(domain.BackupProvider); ok {
		t.Error("pbs provider must not implement the PVE BackupProvider")
	}
}

func TestNotConnectedErrors(t *testing.T) {
	p := &Provider{}
	ctx := context.Background()
	if err := p.Health(ctx); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Errorf("Health without Connect: %v", err)
	}
	if _, err := p.PBSDatastores(ctx); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Errorf("PBSDatastores without Connect: %v", err)
	}
	if _, err := p.PBSTasks(ctx, domain.PBSTaskFilter{}); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Errorf("PBSTasks without Connect: %v", err)
	}
}

func TestConnectValidatesCredentials(t *testing.T) {
	p := &Provider{}
	err := p.Connect(context.Background(), "https://pbs.example.invalid:8007", &domain.Credentials{Type: "token"})
	if err == nil {
		t.Fatal("expected incomplete token credentials to be rejected")
	}
}

func TestConnectRejectsHTTPEndpoint(t *testing.T) {
	p := &Provider{}
	creds := &domain.Credentials{Type: "token", TokenID: "a@pbs!t", TokenSecret: "synthetic-secret-value"}
	if err := p.Connect(context.Background(), "http://pbs.example.invalid:8007", creds); err == nil {
		t.Fatal("expected http endpoint to be rejected")
	}
}

func TestMapSnapshots(t *testing.T) {
	items := []client.SnapshotItem{
		{
			BackupType: "vm", BackupID: "100", BackupTime: 1752000000,
			Size: 42, Owner: "automation@pbs!nodex", Protected: true,
			Files:        []client.SnapshotFile{{Filename: "drive.fidx"}},
			Verification: &client.VerificationItem{State: "ok", UPID: "UPID:x"},
		},
	}
	mapped := MapSnapshots("backups", "prod", items)
	if len(mapped) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(mapped))
	}
	s := mapped[0]
	if s.Store != "backups" || s.Namespace != "prod" {
		t.Errorf("store/namespace not injected: %+v", s)
	}
	if s.BackupType != "vm" || s.BackupID != "100" || !s.Protected {
		t.Errorf("field mapping wrong: %+v", s)
	}
	if len(s.Files) != 1 || s.Files[0] != "drive.fidx" {
		t.Errorf("files mapping wrong: %v", s.Files)
	}
	if s.Verification == nil || s.Verification.State != "ok" {
		t.Errorf("verification mapping wrong: %+v", s.Verification)
	}
}

func TestMapTaskStatusFieldNames(t *testing.T) {
	// The status endpoint's "type"/"id" map onto the domain's
	// WorkerType/WorkerID.
	s := MapTaskStatus(&client.TaskStatusData{
		UPID: "UPID:x", Node: "pbs", Type: "prune", ID: "backups",
		Status: "stopped", ExitStatus: "OK",
	})
	if s.WorkerType != "prune" || s.WorkerID != "backups" {
		t.Errorf("type/id mapping wrong: %+v", s)
	}
}

func TestMapGCStatuses(t *testing.T) {
	mapped := MapGCStatuses([]client.GCStatusData{{
		Store: "backups", LastRunState: "OK", RemovedBytes: 7, StillBad: 1,
	}})
	if len(mapped) != 1 || mapped[0].Store != "backups" || mapped[0].RemovedBytes != 7 || mapped[0].StillBad != 1 {
		t.Errorf("gc mapping wrong: %+v", mapped)
	}
}

func TestMapDatastoreUsagePartialFailureRow(t *testing.T) {
	mapped := MapDatastoreUsages([]client.DatastoreUsageItem{{
		Store: "removable", MountStatus: "notmounted", Error: "not mounted",
	}})
	if mapped[0].Error != "not mounted" {
		t.Errorf("error row must be preserved, got %+v", mapped[0])
	}
}
