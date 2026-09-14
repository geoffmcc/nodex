package certification

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateRequestIsFailClosed(t *testing.T) {
	base := Request{Profile: RequiredProfile, Node: "pve-test", VMID: 9000, Name: "nodex-cert-smoke", Storage: "local", ConfirmTarget: "nodex-cert-smoke", OptIn: true}
	tests := []struct {
		name   string
		change func(*Request)
		want   string
	}{
		{"requires opt in", func(r *Request) { r.OptIn = false }, "opt-in"},
		{"requires test profile", func(r *Request) { r.Profile = "production" }, "nodex-test-admin"},
		{"requires prefix", func(r *Request) { r.Name = "smoke" }, NamePrefix},
		{"requires exact confirmation", func(r *Request) { r.ConfirmTarget = "nodex-cert-other" }, "exactly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			tt.change(&r)
			if err := ValidateRequest(r); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateAuthorizationRequiresExactBinding(t *testing.T) {
	r := Request{Profile: RequiredProfile, Environment: "lab", Suite: "readonly", Endpoint: "https://pve.example.test", Node: "pve1", VMID: 9001, Storage: "local", Authorization: &Authorization{Environment: "lab", Profile: RequiredProfile, Endpoint: "https://pve.example.test", Provider: "proxmox", ExpectedFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TrustedCAIdentity: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Nodes: []string{"pve1"}, Storage: []string{"local"}, VMIDMin: 9000, VMIDMax: 9099, Suites: []string{"readonly"}, MaxResources: 1, ExpiresAt: time.Now().Add(time.Hour).Unix()}}
	if err := ValidateAuthorization(r); err != nil {
		t.Fatalf("valid authorization rejected: %v", err)
	}
	r.Node = "other-node"
	if err := ValidateAuthorization(r); err == nil {
		t.Fatal("unauthorized node accepted")
	}
	r.Node = "pve1"
	r.Authorization.ExpiresAt = time.Now().Add(-time.Hour).Unix()
	if err := ValidateAuthorization(r); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired authorization error = %v", err)
	}
}

func cleanupAuthorization(entry Entry) *Authorization {
	return &Authorization{
		Environment:         entry.Environment,
		Profile:             RequiredProfile,
		Endpoint:            entry.EndpointIdentity,
		Provider:            entry.ProviderIdentity,
		ExpectedFingerprint: entry.Fingerprint,
		TrustedCAIdentity:   entry.CAIdentity,
		Nodes:               []string{entry.Node},
		Storage:             []string{entry.Storage},
		VMIDMin:             entry.VMID,
		VMIDMax:             entry.VMID,
		Suites:              []string{"disposable-mutations"},
		AllowMutations:      true,
		ExpiresAt:           time.Now().Add(time.Hour).Unix(),
	}
}

func TestLedgerRoundTripIsSanitizedAndSorted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	l := New(path)
	l.Entries = append(l.Entries,
		NewEntry(RequiredProfile, "pve-test", 9002, "nodex-cert-b", "local", time.Unix(2, 0)),
		NewEntry(RequiredProfile, "pve-test", 9001, "nodex-cert-a", "local", time.Unix(1, 0)),
	)
	if err := Save(path, l); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 2 || loaded.Entries[0].Name != "nodex-cert-a" {
		t.Fatalf("unexpected ledger ordering: %+v", loaded.Entries)
	}
	b, err := os.ReadFile(path) // #nosec G304 -- path is created under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret") || strings.Contains(string(b), "credential") {
		t.Fatalf("ledger contains credential material: %s", b)
	}
}

func TestReserveRequiresCleanupIntent(t *testing.T) {
	entry := NewEntry(RequiredProfile, "pve-test", 9003, "nodex-cert-intent", "local", time.Unix(3, 0))
	entry.State = "creating"
	if _, err := Reserve(filepath.Join(t.TempDir(), "ledger.json"), entry, 1); err == nil {
		t.Fatal("entry without initial reservation state was accepted")
	}
}

func TestClaimCleanupResumesCheckpointedDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	entry := NewEntry(RequiredProfile, "pve-test", 9004, "nodex-cert-resume", "local", time.Unix(4, 0))
	entry.State = "created"
	entry.Environment = "lab"
	entry.EndpointIdentity = "https://pve.example.test"
	entry.ProviderIdentity = "proxmox"
	entry.Fingerprint = strings.Repeat("a", 64)
	entry.CAIdentity = strings.Repeat("b", 64)
	if err := Save(path, &Ledger{Schema: SchemaVersion, Entries: []Entry{entry}}); err != nil {
		t.Fatal(err)
	}
	auth := cleanupAuthorization(entry)
	claimed, err := ClaimCleanup(path, entry.ID, auth)
	if err != nil {
		t.Fatalf("claim cleanup: %v", err)
	}
	if claimed.State != "deleting" || claimed.Cleanup != "in_progress" {
		t.Fatalf("unexpected claimed entry: %+v", claimed)
	}
	if err := UpdateEntryWithLease(path, entry.ID, claimed.LeaseID, claimed.Revision, func(e *Entry) {
		e.DeleteUPID = "UPID:pve-test/9004/0"
		e.LeaseExpiresAt = time.Now().Add(-time.Second).Unix()
	}); err != nil {
		t.Fatalf("checkpoint delete task: %v", err)
	}
	resumed, err := ClaimCleanup(path, entry.ID, auth)
	if err != nil {
		t.Fatalf("resume cleanup: %v", err)
	}
	if resumed.DeleteUPID != "UPID:pve-test/9004/0" || resumed.Cleanup != "in_progress" {
		t.Fatalf("checkpointed cleanup was not resumed: %+v", resumed)
	}
	if resumed.LeaseID == claimed.LeaseID {
		t.Fatal("expired cleanup lease was not fenced with a new token")
	}
}

func TestClaimCleanupRejectsCreationInProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	entry := NewEntry(RequiredProfile, "pve-test", 9005, "nodex-cert-creating", "local", time.Now())
	entry.State = "creating"
	entry.Environment = "lab"
	entry.EndpointIdentity = "https://pve.example.test"
	entry.ProviderIdentity = "proxmox"
	entry.Fingerprint = strings.Repeat("a", 64)
	entry.CAIdentity = strings.Repeat("b", 64)
	if err := Save(path, &Ledger{Schema: SchemaVersion, Entries: []Entry{entry}}); err != nil {
		t.Fatal(err)
	}
	auth := cleanupAuthorization(entry)
	if _, err := ClaimCleanup(path, entry.ID, auth); err == nil || !strings.Contains(err.Error(), "creation is still in progress") {
		t.Fatalf("cleanup claim error = %v", err)
	}
}

func TestClaimCleanupRecoversExpiredCreationLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	entry := NewEntry(RequiredProfile, "pve-test", 9007, "nodex-cert-expired-create", "local", time.Now())
	entry.State = "creating"
	entry.Environment = "lab"
	entry.EndpointIdentity = "https://pve.example.test"
	entry.ProviderIdentity = "proxmox"
	entry.Fingerprint = strings.Repeat("a", 64)
	entry.CAIdentity = strings.Repeat("b", 64)
	entry.LeaseID = "expired-create-lease"
	entry.LeaseExpiresAt = time.Now().Add(-time.Second).Unix()
	if err := Save(path, &Ledger{Schema: SchemaVersion, Entries: []Entry{entry}}); err != nil {
		t.Fatal(err)
	}
	claimed, err := ClaimCleanup(path, entry.ID, cleanupAuthorization(entry))
	if err != nil {
		t.Fatalf("expired creation cleanup claim: %v", err)
	}
	if claimed.State != "deleting" || claimed.Cleanup != "in_progress" || claimed.LeaseID == entry.LeaseID {
		t.Fatalf("expired creation lease was not recovered: %+v", claimed)
	}
}

func TestVerifyLeaseRejectsExpiredLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	entry := NewEntry(RequiredProfile, "pve-test", 9008, "nodex-cert-expired-lease", "local", time.Now())
	entry.State = "deleting"
	entry.Cleanup = "in_progress"
	entry.Environment = "lab"
	entry.EndpointIdentity = "https://pve.example.test"
	entry.ProviderIdentity = "proxmox"
	entry.Fingerprint = strings.Repeat("a", 64)
	entry.CAIdentity = strings.Repeat("b", 64)
	entry.LeaseID = "expired-lease"
	entry.LeaseExpiresAt = time.Now().Add(-time.Second).Unix()
	if err := Save(path, &Ledger{Schema: SchemaVersion, Entries: []Entry{entry}}); err != nil {
		t.Fatal(err)
	}
	if err := verifyLease(path, entry.ID, entry.LeaseID, entry.Revision); err == nil || !strings.Contains(err.Error(), "lease has expired") {
		t.Fatalf("expired lease verification error = %v", err)
	}
}

func TestLeaseFencesStaleWorkerUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	entry := NewEntry(RequiredProfile, "pve-test", 9006, "nodex-cert-fenced", "local", time.Now())
	entry.Environment = "lab"
	entry.EndpointIdentity = "https://pve.example.test"
	entry.ProviderIdentity = "proxmox"
	entry.Fingerprint = strings.Repeat("a", 64)
	entry.CAIdentity = strings.Repeat("b", 64)
	reservedLedger, err := Reserve(path, entry, 1)
	if err != nil {
		t.Fatal(err)
	}
	reserved := reservedLedger.Entries[0]
	if err := UpdateEntryWithLease(path, entry.ID, reserved.LeaseID, reserved.Revision, func(e *Entry) { e.State = "created" }); err != nil {
		t.Fatal(err)
	}
	auth := cleanupAuthorization(entry)
	claimed, err := ClaimCleanup(path, entry.ID, auth)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateEntryWithLease(path, entry.ID, claimed.LeaseID, claimed.Revision-1, func(e *Entry) { e.Error = "stale revision" }); err == nil {
		t.Fatal("stale revision update was accepted")
	}
	if err := UpdateEntryWithLease(path, entry.ID, claimed.LeaseID, claimed.Revision, func(e *Entry) {
		e.LeaseExpiresAt = time.Now().Add(-time.Second).Unix()
	}); err != nil {
		t.Fatal(err)
	}
	resumed, err := ClaimCleanup(path, entry.ID, auth)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.LeaseID == claimed.LeaseID {
		t.Fatal("cleanup recovery did not issue a new fencing token")
	}
	if err := UpdateEntryWithLease(path, entry.ID, claimed.LeaseID, claimed.Revision, func(e *Entry) { e.Error = "stale worker" }); err == nil {
		t.Fatal("stale worker update was accepted")
	}
}
