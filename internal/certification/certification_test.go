package certification

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
)

const testProfile = "nodex-test-profile"

func TestVerifyEndpointIdentityBoundsStalledTLSHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan struct{})
	defer close(serverDone)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		<-serverDone
	}()

	previousTimeout := certificationIdentityTimeout
	certificationIdentityTimeout = 100 * time.Millisecond
	t.Cleanup(func() { certificationIdentityTimeout = previousTimeout })
	started := time.Now()
	err = VerifyEndpointIdentity(
		context.Background(), "https://"+listener.Addr().String(), "",
		strings.Repeat("0", 64), strings.Repeat("1", 64),
	)
	if err == nil {
		t.Fatal("stalled TLS handshake was accepted")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("identity check took %s, expected its timeout to bound the TLS handshake", elapsed)
	}
}

func TestMutationLedgerPathUsesSingleDefaultLedger(t *testing.T) {
	root := t.TempDir()
	setCertificationConfigEnv(t, root)
	defaultDir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(defaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	defaultPath := filepath.Join(defaultDir, "certification-ledger.json")
	if err := validateMutationLedgerPath(defaultPath); err != nil {
		t.Fatalf("default ledger rejected: %v", err)
	}

	alternate := filepath.Join(root, "alternate-ledger.json")
	if err := validateMutationLedgerPath(alternate); err == nil || !strings.Contains(err.Error(), "must use the default") {
		t.Fatalf("alternate ledger error = %v, want shared-accounting rejection", err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("directory symlink creation is not available on every Windows runner")
	}
	aliasDir := filepath.Join(root, "config-alias")
	if err := os.Symlink(defaultDir, aliasDir); err != nil {
		t.Skipf("create directory symlink: %v", err)
	}
	if err := validateMutationLedgerPath(filepath.Join(aliasDir, "certification-ledger.json")); err == nil || !strings.Contains(err.Error(), "must use the default") {
		t.Fatalf("symlink alias error = %v, want rejection to preserve the shared lock identity", err)
	}
}

type ledgerGuardProvider struct{}

func (ledgerGuardProvider) Name() string                                               { return "proxmox" }
func (ledgerGuardProvider) Version() string                                            { return "test" }
func (ledgerGuardProvider) Connect(context.Context, string, *domain.Credentials) error { return nil }
func (ledgerGuardProvider) Close() error                                               { return nil }
func (ledgerGuardProvider) Health(context.Context) error                               { return nil }
func (ledgerGuardProvider) Capabilities() []domain.Capability                          { return nil }

func TestRunRejectsAlternateMutationLedgerBeforeNetwork(t *testing.T) {
	setCertificationConfigEnv(t, t.TempDir())
	expires := time.Now().Add(time.Hour).Unix()
	auth := &Authorization{
		Environment: "lab", Profile: testProfile, Endpoint: "https://127.0.0.1:1",
		Provider: "proxmox", ExpectedFingerprint: strings.Repeat("a", 64),
		TrustedCAIdentity: strings.Repeat("b", 64), Nodes: []string{"pve-test"},
		Storage: []string{"local"}, VMIDMin: 9000, VMIDMax: 9010,
		Suites: []string{"disposable-mutations"}, AllowMutations: true,
		MaxResources: 1, ExpiresAt: expires,
	}
	req := Request{
		Profile: testProfile, Node: "pve-test", Name: "nodex-cert-accounting",
		Storage: "local", VMID: 9001, ConfirmTarget: "nodex-cert-accounting",
		OptIn: true, Environment: "lab", Suite: "disposable-mutations",
		Endpoint: auth.Endpoint, Authorization: auth,
	}
	_, err := Run(context.Background(), ledgerGuardProvider{}, req, filepath.Join(t.TempDir(), "alternate.json"), time.Now())
	if err == nil || !strings.Contains(err.Error(), "must use the default per-configuration ledger") {
		t.Fatalf("Run error = %v, want alternate-ledger rejection before network", err)
	}
}

func setCertificationConfigEnv(t *testing.T, root string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("AppData", filepath.Join(root, "appdata"))
}

func TestValidateRequestIsFailClosed(t *testing.T) {
	base := Request{Profile: testProfile, Node: "pve-test", VMID: 9000, Name: "nodex-cert-smoke", Storage: "local", ConfirmTarget: "nodex-cert-smoke", OptIn: true}
	tests := []struct {
		name   string
		change func(*Request)
		want   string
	}{
		{"requires opt in", func(r *Request) { r.OptIn = false }, "opt-in"},
		{"requires explicit profile", func(r *Request) { r.Profile = "" }, "explicit profile"},
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
	r := Request{Profile: testProfile, Environment: "lab", Suite: "readonly", Endpoint: "https://pve.example.test", Node: "pve1", VMID: 9001, Storage: "local", Authorization: &Authorization{Environment: "lab", Profile: testProfile, Endpoint: "https://pve.example.test", Provider: "proxmox", ExpectedFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TrustedCAIdentity: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Nodes: []string{"pve1"}, Storage: []string{"local"}, VMIDMin: 9000, VMIDMax: 9099, Suites: []string{"readonly"}, MaxResources: 1, ExpiresAt: time.Now().Add(time.Hour).Unix()}}
	if err := ValidateAuthorization(r); err != nil {
		t.Fatalf("valid authorization rejected: %v", err)
	}
	r.Profile = "nodex-test-other"
	if err := ValidateAuthorization(r); err == nil {
		t.Fatal("profile that differs from the authorization was accepted")
	}
	r.Profile = testProfile
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
		Profile:             testProfile,
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
		NewEntry(testProfile, "pve-test", 9002, "nodex-cert-b", "local", time.Unix(2, 0)),
		NewEntry(testProfile, "pve-test", 9001, "nodex-cert-a", "local", time.Unix(1, 0)),
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

func TestLoadAcceptsNeverWrittenEmptyLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"entries":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := Load(path)
	if err != nil {
		t.Fatalf("Load empty first-run ledger: %v", err)
	}
	if len(l.Entries) != 0 || l.Digest == "" {
		t.Fatalf("empty ledger = %+v, want no entries and computed digest", l)
	}
}

func TestLoadRejectsMissingDigestOnPopulatedLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	empty := `{"schema":2,"generation":1,"entries":[]}`
	if err := os.WriteFile(path, []byte(empty), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "no digest") {
		t.Fatalf("Load populated-generation ledger error = %v, want missing-digest error", err)
	}
}

func TestLegacyLedgerIsReadableButCannotAuthorizeMutations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	legacy := `{"schema":1,"entries":[{"id":"legacy-entry","profile":"nodex-test-profile","node":"pve-test","vmid":9005,"name":"nodex-cert-legacy","storage":"local","state":"created","cleanup":"required","created_at":5,"updated_at":5}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load legacy ledger: %v", err)
	}
	if loaded.IntegrityStatus() != "legacy_unverified" || len(loaded.Entries) != 1 {
		t.Fatalf("legacy ledger status/entries = %q/%d, want legacy_unverified/1", loaded.IntegrityStatus(), len(loaded.Entries))
	}
	if got := loaded.Report().IntegrityStatus; got != "legacy_unverified" {
		t.Fatalf("report integrity status = %q, want legacy_unverified", got)
	}
	loaded.Entries[0].Environment = "lab"
	loaded.Entries[0].EndpointIdentity = "https://pve.example.test"
	loaded.Entries[0].ProviderIdentity = "proxmox"
	loaded.Entries[0].Fingerprint = strings.Repeat("a", 64)
	loaded.Entries[0].CAIdentity = strings.Repeat("b", 64)

	before, err := os.ReadFile(path) // #nosec G304 -- path is created under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	entry := NewEntry(testProfile, "pve-test", 9006, "nodex-cert-new", "local", time.Unix(6, 0))
	if _, err := Reserve(path, entry, 1); err == nil || !strings.Contains(err.Error(), "unauthenticated claims") {
		t.Fatalf("Reserve legacy ledger error = %v, want unverified-ledger block", err)
	}
	if _, err := ClaimCleanup(path, "legacy-entry", cleanupAuthorization(loaded.Entries[0])); err == nil || !strings.Contains(err.Error(), "unauthenticated claims") {
		t.Fatalf("ClaimCleanup legacy ledger error = %v, want unverified-ledger block", err)
	}
	if err := UpdateEntry(path, "legacy-entry", func(e *Entry) { e.State = "cleaned" }); err == nil || !strings.Contains(err.Error(), "unauthenticated claims") {
		t.Fatalf("UpdateEntry legacy ledger error = %v, want unverified-ledger block", err)
	}
	if err := verifyLease(path, "legacy-entry", "lease", 1); err == nil || !strings.Contains(err.Error(), "unauthenticated claims") {
		t.Fatalf("verifyLease legacy ledger error = %v, want unverified-ledger block", err)
	}
	if err := Save(path, loaded); err == nil || !strings.Contains(err.Error(), "unauthenticated claims") {
		t.Fatalf("Save legacy ledger error = %v, want unverified-ledger block", err)
	}

	after, err := os.ReadFile(path) // #nosec G304 -- path is created under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("legacy ledger changed after blocked mutation attempts")
	}
}

func TestMigrateCompletedLegacyPreservesExactBackup(t *testing.T) {
	root := t.TempDir()
	setCertificationConfigEnv(t, root)
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "certification-ledger.json")
	legacy := `{"schema":1,"entries":[{"id":"legacy-done","profile":"nodex-test-profile","node":"pve-test","vmid":9005,"name":"nodex-cert-done","storage":"local","state":"cleaned","cleanup":"complete","created_at":5,"updated_at":6}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	backup, count, err := MigrateCompletedLegacy(path)
	if err != nil {
		t.Fatalf("MigrateCompletedLegacy: %v", err)
	}
	if backup != path+".legacy-v1.bak" || count != 1 {
		t.Fatalf("migration result = (%q, %d), want backup path and one entry", backup, count)
	}
	backupBytes, err := os.ReadFile(backup)
	if err != nil || string(backupBytes) != legacy {
		t.Fatalf("backup bytes = %q, error = %v; want exact original", backupBytes, err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load migrated ledger: %v", err)
	}
	if loaded.Schema != SchemaVersion || loaded.IntegrityStatus() != "verified" || len(loaded.Entries) != 1 || loaded.Entries[0].ID != "legacy-done" {
		t.Fatalf("migrated ledger did not preserve and verify history: %+v", loaded)
	}
}

func TestMigrateCompletedLegacyRejectsPendingWithoutChanges(t *testing.T) {
	root := t.TempDir()
	setCertificationConfigEnv(t, root)
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "certification-ledger.json")
	legacy := `{"schema":1,"entries":[{"id":"legacy-pending","profile":"nodex-test-profile","node":"pve-test","vmid":9005,"name":"nodex-cert-pending","storage":"local","state":"created","cleanup":"required","created_at":5,"updated_at":5}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MigrateCompletedLegacy(path); err == nil || !strings.Contains(err.Error(), "pending or leased") {
		t.Fatalf("migration error = %v, want pending-entry refusal", err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != legacy {
		t.Fatalf("pending ledger changed: bytes=%q err=%v", actual, err)
	}
	if _, err := os.Stat(path + ".legacy-v1.bak"); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup for rejected ledger: %v", err)
	}
}

func TestMigrateCompletedLegacyRejectsConflictingBackup(t *testing.T) {
	root := t.TempDir()
	setCertificationConfigEnv(t, root)
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "certification-ledger.json")
	legacy := `{"schema":1,"entries":[{"id":"legacy-done","profile":"nodex-test-profile","node":"pve-test","vmid":9005,"name":"nodex-cert-done","storage":"local","state":"cleaned","cleanup":"complete","created_at":5,"updated_at":6}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	backup := path + ".legacy-v1.bak"
	if err := os.WriteFile(backup, []byte("unrelated backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MigrateCompletedLegacy(path); err == nil || !strings.Contains(err.Error(), "different contents") {
		t.Fatalf("migration error = %v, want conflicting-backup refusal", err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != legacy {
		t.Fatalf("legacy ledger changed: bytes=%q err=%v", actual, err)
	}
}

func TestReserveRequiresCleanupIntent(t *testing.T) {
	entry := NewEntry(testProfile, "pve-test", 9003, "nodex-cert-intent", "local", time.Unix(3, 0))
	entry.State = "creating"
	if _, err := Reserve(filepath.Join(t.TempDir(), "ledger.json"), entry, 1); err == nil {
		t.Fatal("entry without initial reservation state was accepted")
	}
}

func TestClaimCleanupResumesCheckpointedDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	entry := NewEntry(testProfile, "pve-test", 9004, "nodex-cert-resume", "local", time.Unix(4, 0))
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
	entry := NewEntry(testProfile, "pve-test", 9005, "nodex-cert-creating", "local", time.Now())
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
	entry := NewEntry(testProfile, "pve-test", 9007, "nodex-cert-expired-create", "local", time.Now())
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
	entry := NewEntry(testProfile, "pve-test", 9008, "nodex-cert-expired-lease", "local", time.Now())
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
	entry := NewEntry(testProfile, "pve-test", 9006, "nodex-cert-fenced", "local", time.Now())
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
