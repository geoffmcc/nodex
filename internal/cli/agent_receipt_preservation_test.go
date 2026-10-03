package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/agent"
)

// P1-C regression coverage.
//
// The original evidence is a `container update` receipt: the provider applies the
// change inline and returns no task, so refresh could never reconcile it. The old
// behavior downgraded `submission: accepted` to `unknown` and continued to
// recommend the same impossible reconciliation step, which both erased a known
// fact and pointed the caller at a dead end.
//
// Receipts here are seeded directly so the refresh path is exercised without
// depending on a provider mutation actually taking place.

const preservationRequestID = "req_0123456789abcdef0123456789abcdef"

// seedPreservationReceipt writes a receipt whose execution context matches the
// e2e profile, because refresh refuses to inspect a receipt from any other
// endpoint or trust configuration.
func seedPreservationReceipt(t *testing.T, mutate func(*agent.Receipt)) {
	t.Helper()
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")

	_, identity, err := resolveExplicitProfile("e2e")
	if err != nil {
		t.Fatalf("resolve e2e profile identity: %v", err)
	}

	now := "2026-01-01T00:00:00Z"
	receipt := agent.Receipt{
		Result: agent.Result{
			SchemaVersion: agent.ResultSchemaVersion,
			RequestID:     preservationRequestID,
			ReceiptID:     preservationRequestID,
			Operation:     "container update",
			Context:       identity,
			Submission:    agent.SubmissionAccepted,
			Execution:     agent.ExecutionUnknown,
			Verification:  agent.VerificationUnknown,
			Retry:         agent.RetryReconcileFirst,
			StartedAt:     now,
			ObservedAt:    now,
			NextActions: []agent.NextAction{
				{Operation: "agent receipt reconcile", Arguments: map[string]any{"request_id": preservationRequestID}},
			},
		},
		InputFingerprint: strings.Repeat("a", 64),
		UpdatedAt:        now,
	}
	if mutate != nil {
		mutate(&receipt)
	}

	store, err := agent.DefaultStore()
	if err != nil {
		t.Fatalf("open receipt store: %v", err)
	}
	lease, err := store.Lock(preservationRequestID)
	if err != nil {
		t.Fatalf("lock receipt: %v", err)
	}
	defer func() { _ = lease.Close() }()
	if err := lease.Save(receipt); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
}

func refreshPreservationReceipt(t *testing.T) (agent.Receipt, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--profile", "e2e", "agent", "receipt", "refresh", preservationRequestID}, &stdout, &stderr)
	var receipt agent.Receipt
	if uerr := json.Unmarshal(stdout.Bytes(), &receipt); uerr != nil {
		t.Fatalf("decode receipt: %v\nstdout: %s\nstderr: %s", uerr, stdout.String(), stderr.String())
	}
	return receipt, err
}

func TestReceiptRefreshPreservesKnownAcceptanceWhenCompletionUnobservable(t *testing.T) {
	seedPreservationReceipt(t, nil)

	receipt, err := refreshPreservationReceipt(t)
	if err == nil {
		t.Fatal("a taskless receipt cannot be reconciled; refresh must report that rather than claim success")
	}

	if receipt.Submission != agent.SubmissionAccepted {
		t.Errorf("submission = %q, want accepted; refresh erased a fact established at submission time", receipt.Submission)
	}
	if receipt.Execution == agent.ExecutionSucceeded || receipt.Execution == agent.ExecutionFailed {
		t.Errorf("execution = %q; completion was never observed and must not be invented", receipt.Execution)
	}
	if receipt.Error == nil || receipt.Error.Code != "RECONCILIATION_UNAVAILABLE" {
		t.Errorf("the later failure must still be recorded as an observation, got %+v", receipt.Error)
	}
	if !hasPreservationWarning(receipt.Warnings, "REFRESH_UNAVAILABLE") {
		t.Errorf("warnings = %+v, want a REFRESH_UNAVAILABLE explanation of the retained facts", receipt.Warnings)
	}
}

func TestReceiptRefreshDoesNotRecommendUnavailableRecovery(t *testing.T) {
	seedPreservationReceipt(t, nil)

	receipt, _ := refreshPreservationReceipt(t)

	for _, a := range receipt.NextActions {
		switch a.Operation {
		case "agent receipt reconcile":
			t.Errorf("next action %q is the action that just failed and cannot succeed for a taskless receipt", a.Operation)
		case "agent receipt refresh":
			t.Error("next action refresh would repeat task inspection that is impossible without a task")
		}
	}
	if len(receipt.NextActions) == 0 {
		t.Error("an unresolved receipt must still offer some achievable guidance")
	}
	if receipt.Retry != agent.RetryDoNotAutomatic {
		t.Errorf("retry = %q, want %q; the request may have taken effect and a blind resubmit could apply it twice",
			receipt.Retry, agent.RetryDoNotAutomatic)
	}
}

func TestReceiptRefreshIsIdempotentAcrossRepeats(t *testing.T) {
	seedPreservationReceipt(t, nil)

	first, _ := refreshPreservationReceipt(t)
	second, _ := refreshPreservationReceipt(t)

	if first.Submission != second.Submission {
		t.Errorf("submission eroded across repeated refresh: %q then %q", first.Submission, second.Submission)
	}
	if first.Retry != second.Retry {
		t.Errorf("retry guidance eroded across repeated refresh: %q then %q", first.Retry, second.Retry)
	}
	if len(second.Warnings) != len(first.Warnings) {
		t.Errorf("warnings grew on repeat: %d then %d; refresh should be idempotent", len(first.Warnings), len(second.Warnings))
	}
	if len(second.NextActions) != len(first.NextActions) {
		t.Errorf("next actions grew on repeat: %d then %d", len(first.NextActions), len(second.NextActions))
	}
}

func TestReceiptRefreshPreservesTerminalOutcomeWhenObservationFails(t *testing.T) {
	seedPreservationReceipt(t, func(r *agent.Receipt) {
		r.Submission = agent.SubmissionAccepted
		r.Execution = agent.ExecutionSucceeded
		r.Verification = agent.VerificationUnknown // forces refresh to attempt reconciliation
		r.Retry = agent.RetryDoNotAutomatic
	})

	receipt, err := refreshPreservationReceipt(t)
	if err == nil {
		t.Fatal("refresh should surface the observation failure")
	}
	if receipt.Submission != agent.SubmissionAccepted || receipt.Execution != agent.ExecutionSucceeded {
		t.Errorf("terminal outcome was altered by a failed observation: %+v", receipt.Result)
	}
}

func TestUnattemptedReceiptIsNotPointlesslyReconciled(t *testing.T) {
	// Not-attempted means the provider never took the request. There is nothing
	// to reconcile, so refresh must be a clean no-op rather than manufacture an
	// observation failure.
	seedPreservationReceipt(t, func(r *agent.Receipt) {
		r.Submission = agent.SubmissionNotAttempted
		r.Execution = agent.ExecutionNotStarted
		r.Retry = agent.RetrySafe
	})

	receipt, err := refreshPreservationReceipt(t)
	if err != nil {
		t.Fatalf("refresh of an unattempted receipt should succeed quietly: %v", err)
	}
	if receipt.Submission != agent.SubmissionNotAttempted || receipt.Execution != agent.ExecutionNotStarted {
		t.Errorf("unattempted receipt was altered: %+v", receipt.Result)
	}
	if receipt.Error != nil {
		t.Errorf("unattempted receipt gained an observation failure: %+v", receipt.Error)
	}
}

func TestUnknownSubmissionRecordsUnreconcilableObservation(t *testing.T) {
	// A lost response leaves submission unknown with no task. Refresh cannot
	// settle it, so it must record that fact rather than invent an outcome.
	seedPreservationReceipt(t, func(r *agent.Receipt) {
		r.Submission = agent.SubmissionUnknown
		r.Execution = agent.ExecutionUnknown
		r.Retry = agent.RetryReconcileFirst
	})

	receipt, err := refreshPreservationReceipt(t)
	if err == nil {
		t.Fatal("a taskless unknown receipt cannot be reconciled; refresh must report that")
	}
	if receipt.Submission != agent.SubmissionUnknown || receipt.Execution != agent.ExecutionUnknown {
		t.Errorf("unknown outcome was invented from no evidence: %+v", receipt.Result)
	}
	if receipt.Error == nil || receipt.Error.Code != "RECONCILIATION_UNAVAILABLE" {
		t.Errorf("the failed observation must still be recorded, got %+v", receipt.Error)
	}
	if receipt.Retry != agent.RetryReconcileFirst {
		t.Errorf("retry = %q, want reconcile_first; whether the request landed at all is still open", receipt.Retry)
	}
}

func TestVerifiedSynchronousReceiptSurvivesRefresh(t *testing.T) {
	// The P1-B container update path produces accepted/succeeded/passed with no
	// task. Refreshing it must be a no-op that preserves the verified facts.
	changed := false
	seedPreservationReceipt(t, func(r *agent.Receipt) {
		r.Submission = agent.SubmissionAccepted
		r.Execution = agent.ExecutionSucceeded
		r.Verification = agent.VerificationPassed
		r.Changed = &changed
		r.Retry = agent.RetryDoNotAutomatic
		r.NextActions = nil
	})

	receipt, err := refreshPreservationReceipt(t)
	if err != nil {
		t.Fatalf("a verified terminal receipt should not need reconciliation: %v", err)
	}
	if receipt.Submission != agent.SubmissionAccepted || receipt.Execution != agent.ExecutionSucceeded || receipt.Verification != agent.VerificationPassed {
		t.Errorf("verified receipt was altered by refresh: %+v", receipt.Result)
	}
	if receipt.Changed == nil || *receipt.Changed {
		t.Errorf("changed = %v, want a preserved false", receipt.Changed)
	}
	if receipt.Error != nil {
		t.Errorf("verified receipt gained an error: %+v", receipt.Error)
	}
}

func hasPreservationWarning(warnings []agent.Warning, code string) bool {
	for _, w := range warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}
