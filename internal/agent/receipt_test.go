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

func canonicalTestDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temporary test directory: %v", err)
	}
	return dir
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
	store := NewStore(filepath.Join(canonicalTestDir(t), "ledger"))
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
	store := NewStore(filepath.Join(canonicalTestDir(t), "ledger"))
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
	store := NewStore(filepath.Join(canonicalTestDir(t), "ledger"))
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

func TestListRecentPrefixFiltersBeforeApplyingLimit(t *testing.T) {
	store := NewStore(filepath.Join(canonicalTestDir(t), "ledger"))
	for _, id := range []string{"eval_run_a", "eval_run_b", "other_recent"} {
		lease, err := store.Lock(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := lease.Save(validTestReceipt(id)); err != nil {
			t.Fatal(err)
		}
		_ = lease.Close()
	}

	// Make the unrelated receipt newer than both matching receipts. A prefix
	// filter must select from the full store before applying the one-row limit.
	unrelatedPath := filepath.Join(store.dir, "other_recent.json")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(unrelatedPath, future, future); err != nil {
		t.Fatal(err)
	}
	items, total, err := store.ListRecentPrefix("eval_run_", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || total != 2 || !strings.HasPrefix(items[0].RequestID, "eval_run_") {
		t.Fatalf("ListRecentPrefix = %v, total %d; want one matching row and total 2", items, total)
	}
}

func TestListRecentPrefixRejectsInvalidPrefix(t *testing.T) {
	store := NewStore(filepath.Join(canonicalTestDir(t), "ledger"))
	if _, _, err := store.ListRecentPrefix("../unsafe", 10); err == nil {
		t.Fatal("invalid request ID prefix was accepted")
	}
}

func TestStoreRejectsSymlinkPathsAndOversizedRecords(t *testing.T) {
	base := canonicalTestDir(t)
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
	base := canonicalTestDir(t)
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
	store := NewStore(filepath.Join(canonicalTestDir(t), "ledger"))
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

// P1-C: a receipt persisted before the relaxation must still validate and load.
// The change permits do_not_retry_automatically for an accepted-but-unresolved
// operation; it must not invalidate anything that was already legal.
func TestPreviouslyPersistedReceiptsRemainReadable(t *testing.T) {
	cases := []struct {
		name         string
		submission   Submission
		execution    Execution
		retry        Retry
		verification Verification
	}{
		{"accepted-unresolved-reconcile-first", SubmissionAccepted, ExecutionUnknown, RetryReconcileFirst, VerificationUnknown},
		{"accepted-running-reconcile-first", SubmissionAccepted, ExecutionRunning, RetryReconcileFirst, VerificationUnknown},
		{"accepted-unresolved-not-automatic", SubmissionAccepted, ExecutionUnknown, RetryDoNotAutomatic, VerificationUnknown},
		{"accepted-running-not-automatic", SubmissionAccepted, ExecutionRunning, RetryDoNotAutomatic, VerificationUnknown},
		{"unknown-unresolved-reconcile-first", SubmissionUnknown, ExecutionUnknown, RetryReconcileFirst, VerificationUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validTestReceipt("req_" + tc.name)
			r.Submission, r.Execution, r.Retry, r.Verification = tc.submission, tc.execution, tc.retry, tc.verification
			if err := r.Validate(); err != nil {
				t.Fatalf("previously valid combination rejected: %v", err)
			}
			b, err := json.Marshal(r)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back Receipt
			if err := json.Unmarshal(b, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if err := back.Validate(); err != nil {
				t.Fatalf("round-tripped receipt invalid: %v", err)
			}
			if back.Submission != tc.submission || back.Execution != tc.execution || back.Retry != tc.retry {
				t.Fatalf("round-trip changed outcome: %+v", back.Result)
			}
		})
	}
}

// An accepted-but-unresolved receipt may still not claim a safe automatic retry.
func TestAcceptedUnresolvedStillRejectsUnsafeRetry(t *testing.T) {
	r := validTestReceipt("req_unsafe_unresolved")
	r.Submission, r.Execution, r.Verification, r.Retry = SubmissionAccepted, ExecutionUnknown, VerificationUnknown, RetrySafe
	if err := r.Validate(); err == nil {
		t.Fatal("an accepted request of unknown completion must not be marked safe to retry automatically")
	}
}

// An unknown submission must still be reconciled first: whether the request
// landed at all is genuinely open, so the relaxation must not reach it.
func TestUnknownSubmissionStillRequiresReconcileFirst(t *testing.T) {
	r := validTestReceipt("req_unknown_requires_reconcile")
	r.Submission, r.Execution, r.Verification, r.Retry = SubmissionUnknown, ExecutionUnknown, VerificationUnknown, RetryDoNotAutomatic
	if err := r.Validate(); err == nil {
		t.Fatal("an unknown submission must not be marked as not automatically retryable")
	}
}
