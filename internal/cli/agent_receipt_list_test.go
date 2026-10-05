package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/agent"
)

// writeReceipt stores a receipt directly so the list projection can be compared
// against what `receipt show` returns for the same record.
func writeReceipt(t *testing.T, id string, r agent.Receipt) {
	t.Helper()
	store, err := agent.DefaultStore()
	if err != nil {
		t.Fatalf("resolve receipt store: %v", err)
	}
	lease, err := store.Lock(id)
	if err != nil {
		t.Fatalf("lock %s: %v", id, err)
	}
	defer func() { _ = lease.Close() }()
	if err := lease.Save(r); err != nil {
		t.Fatalf("save %s: %v", id, err)
	}
}

func boolPtr(b bool) *bool { return &b }

// testReceipt builds a receipt the store will accept: an accepted request that
// succeeded and verified must not offer an automatic retry, and the stored
// fingerprint must be a 64-character hex digest.
func testReceipt(id string, changed *bool) agent.Receipt {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return agent.Receipt{
		Result: agent.Result{
			SchemaVersion: agent.ResultSchemaVersion,
			RequestID:     id,
			ReceiptID:     id,
			Operation:     "vm start",
			Submission:    agent.SubmissionAccepted,
			Execution:     agent.ExecutionSucceeded,
			Verification:  agent.VerificationPassed,
			Changed:       changed,
			Retry:         agent.RetryDoNotAutomatic,
			StartedAt:     now,
			ObservedAt:    now,
		},
		InputFingerprint: strings.Repeat("a", 64),
		UpdatedAt:        now,
	}
}

// The list view must not be a lossy projection of the stored receipt. An agent
// auditing itself through `receipt list` has to reach the same determination
// `receipt show` reports, or it concludes no operation ever confirmed a change.
func TestAgentReceiptListPreservesChangedDetermination(t *testing.T) {
	isolateConfigAndHome(t)

	ids := []string{
		"req_list_changed_true",
		"req_list_changed_false",
		"req_list_changed_absent",
	}
	writeReceipt(t, ids[0], testReceipt(ids[0], boolPtr(true)))
	writeReceipt(t, ids[1], testReceipt(ids[1], boolPtr(false)))
	writeReceipt(t, ids[2], testReceipt(ids[2], nil))

	var out, errOut bytes.Buffer
	if err := Run(context.Background(), []string{"agent", "receipt", "list"}, &out, &errOut); err != nil {
		t.Fatalf("receipt list: %v (stderr: %s)", err, errOut.String())
	}

	var list struct {
		Items []struct {
			RequestID    string `json:"request_id"`
			Execution    string `json:"execution"`
			Verification string `json:"verification"`
			Changed      *bool  `json:"changed"`
			StartedAt    string `json:"started_at"`
			ObservedAt   string `json:"observed_at"`
			Error        *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			Warnings []struct {
				Code string `json:"code"`
			} `json:"warnings"`
			SchemaVersion int `json:"schema_version"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out.Bytes(), &list); err != nil {
		t.Fatalf("decode receipt list: %v", err)
	}

	listed := make(map[string]struct {
		changed      *bool
		startedAt    string
		observedAt   string
		errorCode    string
		warningCodes []string
		schema       int
	}, len(list.Items))
	for _, item := range list.Items {
		e := listed[item.RequestID]
		e.changed = item.Changed
		e.startedAt = item.StartedAt
		e.observedAt = item.ObservedAt
		e.schema = item.SchemaVersion
		if item.Error != nil {
			e.errorCode = item.Error.Code
		}
		for _, w := range item.Warnings {
			e.warningCodes = append(e.warningCodes, w.Code)
		}
		listed[item.RequestID] = e
	}

	for _, id := range ids {
		var shown agent.Receipt
		var showOut, showErr bytes.Buffer
		if err := Run(context.Background(), []string{"agent", "receipt", "show", id}, &showOut, &showErr); err != nil {
			t.Fatalf("receipt show %s: %v (stderr: %s)", id, err, showErr.String())
		}
		if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
			t.Fatalf("decode receipt show %s: %v", id, err)
		}

		got, ok := listed[id]
		if !ok {
			t.Fatalf("%s missing from receipt list", id)
		}

		// `changed` is a *bool, so an unset determination must stay distinguishable
		// from a negative one. Comparing the decoded value keeps false != null.
		switch {
		case shown.Changed == nil && got.changed != nil:
			t.Errorf("%s: list changed = %v, show changed = null; an unset determination must not read as a negative one", id, *got.changed)
		case shown.Changed != nil && got.changed == nil:
			t.Errorf("%s: list changed = null, show changed = %v; the list view dropped the determination", id, *shown.Changed)
		case shown.Changed != nil && *shown.Changed != *got.changed:
			t.Errorf("%s: list changed = %v, show changed = %v", id, *got.changed, *shown.Changed)
		}

		if got.startedAt != shown.StartedAt {
			t.Errorf("%s: list started_at = %q, show started_at = %q", id, got.startedAt, shown.StartedAt)
		}
		if got.observedAt != shown.ObservedAt {
			t.Errorf("%s: list observed_at = %q, show observed_at = %q", id, got.observedAt, shown.ObservedAt)
		}
		if got.schema != shown.SchemaVersion {
			t.Errorf("%s: list schema_version = %d, show schema_version = %d", id, got.schema, shown.SchemaVersion)
		}
	}
}

// A failed operation is a determination too. An agent auditing itself through the
// list view has to see why an operation failed, not only that it did not report a
// change.
func TestAgentReceiptListPreservesErrorAndWarnings(t *testing.T) {
	isolateConfigAndHome(t)

	id := "req_list_error"
	r := testReceipt(id, nil)
	r.Execution = agent.ExecutionFailed
	r.Verification = agent.VerificationNotRequested
	r.Error = &agent.AgentError{Code: "VM_START_FAILED", Message: "the provider rejected the request", Exit: 3}
	r.Warnings = []agent.Warning{{Code: "REFRESH_UNAVAILABLE", Message: "the latest read-only refresh failed"}}
	writeReceipt(t, id, r)

	var out, errOut bytes.Buffer
	if err := Run(context.Background(), []string{"agent", "receipt", "list"}, &out, &errOut); err != nil {
		t.Fatalf("receipt list: %v (stderr: %s)", err, errOut.String())
	}

	var list struct {
		Items []struct {
			RequestID string `json:"request_id"`
			Error     *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Exit    int    `json:"exit_code"`
			} `json:"error"`
			Warnings []struct {
				Code string `json:"code"`
			} `json:"warnings"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out.Bytes(), &list); err != nil {
		t.Fatalf("decode receipt list: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(list.Items))
	}
	item := list.Items[0]
	if item.Error == nil {
		t.Fatalf("list dropped the recorded error for a failed operation; an agent auditing itself sees no cause")
	}
	if item.Error.Code != "VM_START_FAILED" || item.Error.Exit != 3 {
		t.Errorf("list error = %+v, want code VM_START_FAILED exit 3", item.Error)
	}
	if len(item.Warnings) != 1 || item.Warnings[0].Code != "REFRESH_UNAVAILABLE" {
		t.Errorf("list warnings = %+v, want one REFRESH_UNAVAILABLE", item.Warnings)
	}
}

func TestAgentReceiptListFiltersByRequestIDPrefix(t *testing.T) {
	isolateConfigAndHome(t)
	writeReceipt(t, "eval_run_a", testReceipt("eval_run_a", nil))
	writeReceipt(t, "eval_run_b", testReceipt("eval_run_b", nil))
	writeReceipt(t, "other_run_a", testReceipt("other_run_a", nil))

	var out, errOut bytes.Buffer
	args := []string{"agent", "receipt", "list", "--request-prefix", "eval_run_", "--limit", "1"}
	if err := Run(context.Background(), args, &out, &errOut); err != nil {
		t.Fatalf("receipt list with prefix: %v (stderr: %s)", err, errOut.String())
	}
	var result struct {
		RequestPrefix string `json:"request_prefix"`
		Items         []struct {
			RequestID string `json:"request_id"`
		} `json:"items"`
		Limit     int  `json:"limit"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode filtered receipt list: %v\n%s", err, out.String())
	}
	if result.RequestPrefix != "eval_run_" || result.Limit != 1 || !result.Truncated || len(result.Items) != 1 {
		t.Fatalf("filtered list summary = %+v, want prefix, one row, and truncated=true", result)
	}
	if !strings.HasPrefix(result.Items[0].RequestID, result.RequestPrefix) {
		t.Fatalf("returned request ID %q does not match prefix %q", result.Items[0].RequestID, result.RequestPrefix)
	}
}
