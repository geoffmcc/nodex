package cli

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/agent"
	"github.com/geoffmcc/nodex/internal/app"
)

// TestAgentInspectionFailurePreservesProviderError pins the two properties a
// failed read-only inspection must keep.
//
// The evaluation showed both failing at once. The inspection path paired a
// failed execution with a blanket safe retry; Result.Validate rejects that
// pairing, so writeAgentResult refused to emit the envelope and surfaced
// "invalid agent result: failed execution must not be retried automatically"
// instead. That internal complaint both hid the provider's real NOT_FOUND and
// discarded its exit code, so the process exited 0.
func TestAgentInspectionFailurePreservesProviderError(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	const target = "e2e-node/999999"

	// The non-agent request is the reference behaviour agent mode must mirror
	// while adding a structured envelope.
	var plainOut, plainErr bytes.Buffer
	plainRunErr := Run(context.Background(), []string{"--output", "json", "--profile", "e2e", "vm", "show", target}, &plainOut, &plainErr)
	if got := app.ExitCodeFromError(plainRunErr); got != app.ExitNotFound {
		t.Fatalf("reference request exit = %d, want %d (%v)", got, app.ExitNotFound, plainRunErr)
	}

	var stdout, stderr bytes.Buffer
	runErr := Run(context.Background(), []string{"--agent", "--profile", "e2e", "vm", "show", target}, &stdout, &stderr)

	if runErr == nil {
		t.Fatal("a failed inspection returned no error, so the process would exit 0")
	}
	if got := app.ExitCodeFromError(runErr); got != app.ExitNotFound {
		t.Fatalf("agent exit = %d, want %d (%v)", got, app.ExitNotFound, runErr)
	}
	if !app.IsEmitted(runErr) {
		t.Fatal("the error was not marked emitted, so stderr would duplicate the envelope")
	}
	if stderr.Len() != 0 {
		t.Fatalf("agent mode wrote to stderr alongside the envelope: %s", stderr.String())
	}

	// decodeAgentResult calls Result.Validate, so getting here at all proves the
	// envelope is internally consistent rather than a rejected internal fault.
	if strings.Contains(stdout.String(), "invalid agent result") {
		t.Fatalf("envelope was replaced by an internal validation complaint: %s", stdout.String())
	}
	result := decodeAgentResult(t, stdout.Bytes())
	if result.Error == nil {
		t.Fatal("envelope carried no error")
	}
	if result.Error.Exit != app.ExitNotFound {
		t.Fatalf("envelope error exit = %d, want %d (%+v)", result.Error.Exit, app.ExitNotFound, result.Error)
	}
	if result.Execution != agent.ExecutionFailed {
		t.Fatalf("execution = %q, want %q", result.Execution, agent.ExecutionFailed)
	}
	if result.Retry != agent.RetryDoNotAutomatic {
		t.Fatalf("retry = %q, want %q", result.Retry, agent.RetryDoNotAutomatic)
	}
}

// TestAgentInspectionUsageErrorExitsWithUsageCode covers the second reported
// symptom directly: a malformed target produced a valid envelope carrying
// USAGE_ERROR with exit_code 2 while the process still exited 0, because the
// inspection path returned the (nil) result of a successful write.
func TestAgentInspectionUsageErrorExitsWithUsageCode(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	runErr := Run(context.Background(), []string{"--agent", "--profile", "e2e", "--output", "json", "vm", "show", "999999"}, &stdout, &stderr)

	if runErr == nil {
		t.Fatal("a rejected target returned no error, so the process would exit 0")
	}
	if got := app.ExitCodeFromError(runErr); got != app.ExitUsage {
		t.Fatalf("agent exit = %d, want %d (%v)", got, app.ExitUsage, runErr)
	}

	result := decodeAgentResult(t, stdout.Bytes())
	if result.Error == nil || result.Error.Exit != app.ExitUsage {
		t.Fatalf("envelope did not carry the usage error: %+v", result)
	}
}

// TestTransientReadFailureSeparatesRetryableFromDefinitive pins which read
// failures are worth repeating. A timeout or a 5xx leaves the question open, so
// the envelope says so; a 404 or a validation rejection produced a definitive
// answer and repeating the read would only reproduce it.
func TestTransientReadFailureSeparatesRetryableFromDefinitive(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"not found", app.NotFoundError("VM \"proxmox/90101\": not found"), false},
		{"provider 404", app.NewProviderError(http.StatusNotFound, "no such guest", nil), false},
		{"validation rejection", app.NewProviderError(http.StatusUnprocessableEntity, "invalid parameter", nil), false},
		{"request timeout", app.NewProviderError(http.StatusRequestTimeout, "timeout", nil), false},
		{"server error", app.NewProviderError(http.StatusInternalServerError, "internal error", nil), true},
		{"bad gateway", app.NewProviderError(http.StatusBadGateway, "upstream unavailable", nil), true},
		{"transport failure", stderrors.New("connection reset by peer"), true},
		{"deadline exceeded", app.NewExitError(stderrors.New("context deadline exceeded"), app.ExitTimeout), true},
		{"plain usage error", app.NewExitError(stderrors.New("bad flag"), app.ExitUsage), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := transientReadFailure(tc.err); got != tc.want {
				t.Fatalf("transientReadFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestInspectionRetryAlwaysSatisfiesTheResultContract guards the invariant that
// caused the original defect. Retry guidance and execution state are chosen
// together; if a future change ever pairs a failed execution with a retry the
// validator rejects, the envelope cannot be emitted and the provider error is
// lost again.
func TestInspectionRetryAlwaysSatisfiesTheResultContract(t *testing.T) {
	for _, execution := range []agent.Execution{agent.ExecutionNotStarted, agent.ExecutionFailed, agent.ExecutionSucceeded} {
		t.Run(string(execution), func(t *testing.T) {
			now := "2026-01-02T03:04:05Z"
			result := agent.Result{
				SchemaVersion: agent.ResultSchemaVersion,
				RequestID:     "req_inspection_retry",
				Operation:     "vm show",
				Submission:    agent.SubmissionNotAttempted,
				Execution:     execution,
				Verification:  agent.VerificationNotRequested,
				Retry:         inspectionRetry(),
				StartedAt:     now,
				ObservedAt:    now,
			}
			if err := result.Validate(); err != nil {
				t.Fatalf("inspection retry guidance is not emittable for %q: %v", execution, err)
			}
		})
	}
}

// TestAgentInspectionErrorKeepsProviderDetail guards against the error message
// being replaced by a generic string. The operator has to be able to tell a
// missing guest from an unreadable one.
func TestAgentInspectionErrorKeepsProviderDetail(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--agent", "--profile", "e2e", "vm", "show", "e2e-node/999999"}, &stdout, &stderr); err == nil {
		t.Fatal("expected the missing guest to fail")
	}
	result := decodeAgentResult(t, stdout.Bytes())
	if !strings.Contains(result.Error.Message, "999999") {
		t.Fatalf("error message lost the target it refers to: %q", result.Error.Message)
	}
	if result.Error.Code == "" || result.Error.Code == fmt.Sprintf("%v", result.Error.Exit) {
		t.Fatalf("error code is not a stable machine-readable code: %+v", result.Error)
	}
}
