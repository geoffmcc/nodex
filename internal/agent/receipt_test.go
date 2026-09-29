package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func validTestReceipt(id string) Receipt {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return Receipt{
		Result: Result{
			SchemaVersion: ResultSchemaVersion, RequestID: id, ReceiptID: id, Operation: "vm.start",
			Context:    ExecutionContext{Profile: "lab", Provider: "proxmox", Endpoint: "https://pve.example:8006", ResourceType: "vm", ResourceID: "100", Node: "pve1"},
			Submission: SubmissionUnknown, Execution: ExecutionUnknown, Verification: VerificationUnsupported,
			Retry: RetryReconcileFirst, StartedAt: now, ObservedAt: now,
		},
		InputFingerprint: strings.Repeat("a", 64), UpdatedAt: now,
	}
}

func TestResultValidateRejectsFalseSuccessAndUnsafeRetry(t *testing.T) {
	r := validTestReceipt("req-outcome")
	r.Execution = ExecutionSucceeded
	if err := r.Validate(); err == nil {
		t.Fatal("unknown submission with succeeded execution must be rejected")
	}
	r = validTestReceipt("req-retry")
	r.Retry = RetrySafe
	if err := r.Validate(); err == nil {
		t.Fatal("unknown operation with automatic-safe retry must be rejected")
	}
}

func TestResultValidateAllowsDocumentedOutcomeCombinations(t *testing.T) {
	r := validTestReceipt("req_completed")
	r.Submission, r.Execution, r.Verification, r.Retry = SubmissionAccepted, ExecutionSucceeded, VerificationPassed, RetryDoNotAutomatic
	if err := r.Validate(); err != nil {
		t.Fatalf("accepted, completed, verified result rejected: %v", err)
	}
	r = validTestReceipt("req_rejected")
	r.Submission, r.Execution, r.Retry = SubmissionRejected, ExecutionNotStarted, RetrySafe
	if err := r.Validate(); err != nil {
		t.Fatalf("rejected-before-execution result rejected: %v", err)
	}
	r = validTestReceipt("req_observed_read")
	r.Submission, r.Execution, r.Retry = SubmissionNotAttempted, ExecutionSucceeded, RetrySafe
	if err := r.Validate(); err != nil {
		t.Fatalf("read-only result with no mutation submission rejected: %v", err)
	}
}

func TestFingerprintSeparatesIdenticalResourceIDsOnDifferentEndpoints(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "ledger"))
	first := ExecutionContext{Profile: "one", Provider: "proxmox", Endpoint: "https://pve-a.example:8006", ResourceType: "vm", ResourceID: "100", Node: "pve1"}
	second := first
	second.Profile = "two"
	second.Endpoint = "https://pve-b.example:8006"
	a, err := store.Fingerprint("vm.stop", []string{"pve1/100"}, first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Fingerprint("vm.stop", []string{"pve1/100"}, second)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("identical VMIDs on different provider endpoints shared an idempotency fingerprint")
	}
	const lowEntropyText = "comment=secret-looking-text"
	fingerprint, err := store.Fingerprint("firewall.alias.create", []string{lowEntropyText}, first)
	if err != nil {
		t.Fatal(err)
	}
	receipt := validTestReceipt("req_hmac")
	receipt.InputFingerprint = fingerprint
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), lowEntropyText) {
		t.Fatal("receipt persisted raw normalized arguments instead of a keyed fingerprint")
	}
	keyInfo, err := os.Stat(filepath.Join(store.dir, "fingerprint.key"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("fingerprint key permissions = %o, want 0600", keyInfo.Mode().Perm())
	}
}

func TestStoreRoundTripAndRequestIDBinding(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "ledger"))
	r := validTestReceipt("req_roundtrip")
	lease, err := store.Lock(r.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Save(r); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(r.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Operation != r.Operation || got.InputFingerprint != r.InputFingerprint || got.Context.Endpoint != r.Context.Endpoint {
		t.Fatalf("receipt round trip differs: %+v", got)
	}
	if _, err := store.Get("../escape"); err == nil {
		t.Fatal("path traversal request ID was accepted")
	}
	if _, err := store.Get("req-other"); !os.IsNotExist(err) {
		t.Fatalf("missing receipt error = %v, want not-exist", err)
	}
}

func TestListRecentIsBoundedAndReportsTotal(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "ledger"))
	for _, id := range []string{"req_recent_a", "req_recent_b"} {
		lease, err := store.Lock(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := lease.Save(validTestReceipt(id)); err != nil {
			t.Fatal(err)
		}
		_ = lease.Close()
	}
	items, total, err := store.ListRecent(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || total != 2 {
		t.Fatalf("ListRecent(1) returned %d items with total %d", len(items), total)
	}
}

func TestStoreRejectsSymlinkPathsAndOversizedRecords(t *testing.T) {
	base := t.TempDir()
	actual := filepath.Join(base, "real")
	if err := os.Mkdir(actual, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "linked")
	if err := os.Symlink(actual, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := NewStore(filepath.Join(link, "agent-receipts")).Lock("req_symlink"); err == nil {
		t.Fatal("symlinked receipt directory was accepted")
	}

	store := NewStore(filepath.Join(base, "oversized"))
	lease, err := store.Lock("req_large")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lease.path, []byte(strings.Repeat("x", maxReceiptBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := lease.Load(); err == nil || !strings.Contains(err.Error(), "bounded") {
		t.Fatalf("oversized receipt error = %v", err)
	}
	_ = lease.Close()
}

func TestStoreRejectsSymlinkReceiptAndBroadPermissions(t *testing.T) {
	base := t.TempDir()
	store := NewStore(filepath.Join(base, "ledger"))
	lease, err := store.Lock("req_file_link")
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Save(validTestReceipt("req_file_link")); err != nil {
		t.Fatal(err)
	}
	_ = lease.Close()
	outside := filepath.Join(base, "outside.json")
	raw, err := json.Marshal(validTestReceipt("req_file_link"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(lease.path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, lease.path); err != nil {
		t.Skipf("symlink setup unavailable: %v", err)
	}
	if _, err := store.Get("req_file_link"); err == nil {
		t.Fatal("symlink receipt file was accepted")
	}
	if runtime.GOOS != "windows" {
		_ = os.Remove(lease.path)
		if err := os.WriteFile(lease.path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Get("req_file_link"); err == nil || !strings.Contains(err.Error(), "permissions") {
			t.Fatalf("overly broad receipt permissions error = %v", err)
		}
	}
}

func TestStoreSerializesConcurrentRequestIDs(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "ledger"))
	first, err := store.Lock("req_shared")
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan error, 1)
	entered := make(chan struct{})
	go func() {
		close(entered)
		second, err := store.Lock("req_shared")
		if err == nil {
			_ = second.Close()
		}
		acquired <- err
	}()
	<-entered
	select {
	case err := <-acquired:
		_ = first.Close()
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("second lease acquired while first held the request lock")
	case <-time.After(40 * time.Millisecond):
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second lease did not proceed after release")
	}
}
