package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/agent"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/provider"
)

// A synchronous config endpoint returns no task. The receipt must report the
// completion the endpoint actually guarantees, and must not demand a
// reconcile step that can never succeed.

func TestVerifyConfigReadback(t *testing.T) {
	tests := []struct {
		name      string
		observed  map[string]interface{}
		requested map[string]string
		wantOK    bool
		wantKeys  []string
	}{
		{
			name:      "applied values match",
			observed:  map[string]interface{}{"cores": 2, "memory": 2048, "unprivileged": "1"},
			requested: map[string]string{"cores": "2", "memory": "2048", "unprivileged": "1"},
			wantOK:    true,
		},
		{
			name:      "already correct values match",
			observed:  map[string]interface{}{"cores": 2},
			requested: map[string]string{"cores": "2"},
			wantOK:    true,
		},
		{
			name:      "numeric forms compare equal",
			observed:  map[string]interface{}{"cores": 2},
			requested: map[string]string{"cores": "02"},
			wantOK:    true,
		},
		{
			name:      "float request matches int readback",
			observed:  map[string]interface{}{"cores": 2},
			requested: map[string]string{"cores": "2.0"},
			wantOK:    true,
		},
		{
			name:      "bool readback matches string request",
			observed:  map[string]interface{}{"lock": true},
			requested: map[string]string{"lock": "1"},
			wantOK:    true,
		},
		{
			name:      "partial application is a mismatch",
			observed:  map[string]interface{}{"cores": 2, "memory": 2048},
			requested: map[string]string{"cores": "4", "memory": "2048"},
			wantOK:    false,
			wantKeys:  []string{"cores"},
		},
		{
			name:      "absent key is a mismatch not a skip",
			observed:  map[string]interface{}{"memory": 2048},
			requested: map[string]string{"cores": "2"},
			wantOK:    false,
			wantKeys:  []string{"cores"},
		},
		{
			name:      "empty string request matches empty readback",
			observed:  map[string]interface{}{"description": ""},
			requested: map[string]string{"description": ""},
			wantOK:    true,
		},
		{
			name:      "non numeric mismatch reported",
			observed:  map[string]interface{}{"name": "web"},
			requested: map[string]string{"name": "db"},
			wantOK:    false,
			wantKeys:  []string{"name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, mismatches := verifyConfigReadback(tt.observed, tt.requested)
			if ok != tt.wantOK {
				t.Fatalf("matched = %v, want %v (mismatches: %v)", ok, tt.wantOK, mismatches)
			}
			if len(mismatches) != len(tt.wantKeys) {
				t.Fatalf("mismatch count = %d, want %d: %v", len(mismatches), len(tt.wantKeys), mismatches)
			}
			for _, want := range tt.wantKeys {
				found := false
				for _, m := range mismatches {
					if m.Key == want {
						found = true
					}
				}
				if !found {
					t.Errorf("expected mismatch for key %q, got %v", want, mismatches)
				}
			}
		})
	}
}

func TestCanonicalConfigValue(t *testing.T) {
	cases := map[interface{}]string{
		nil:            "",
		"":             "",
		"  cores  ":    "cores",
		2:              "2",
		int64(2048):    "2048",
		float64(2):     "2",
		2.5:            "2.5",
		true:           "1",
		false:          "0",
		"unprivileged": "unprivileged",
	}
	for in, want := range cases {
		if got := canonicalConfigValue(in); got != want {
			t.Errorf("canonicalConfigValue(%#v) = %q, want %q", in, got, want)
		}
	}
}

// The reported regression: a synchronous container update that succeeded was
// reported as execution "unknown" and recommended a reconcile step that
// always failed.
func TestMutationOutcomeSynchronousConfigUpdateIsSucceeded(t *testing.T) {
	now := "2026-01-01T00:00:00Z"
	base := agent.Result{
		RequestID:     "req_0123456789abcdef0123456789abcdef",
		SchemaVersion: agent.ResultSchemaVersion,
		Operation:     "container update",
		Submission:    agent.SubmissionNotAttempted,
		Execution:     agent.ExecutionNotStarted,
		Verification:  agent.VerificationUnsupported,
		Retry:         agent.RetryDoNotAutomatic,
		StartedAt:     now,
		ObservedAt:    now,
	}

	changed := true
	legacy := output.NewOperationResult("container update", "proxmox", "pve")
	legacy.Target = "pve1/9601"
	legacy.Submitted = true
	legacy.Synchronous = true
	legacy.Success = true
	legacy.Status = "verified"
	legacy.Changed = &changed

	raw, err := output.MarshalJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}

	result := mutationOutcome(base, OperationMeta{Path: "container update", ProducesUPID: false}, raw, nil)

	if result.Submission != agent.SubmissionAccepted {
		t.Errorf("submission = %q, want %q", result.Submission, agent.SubmissionAccepted)
	}
	if result.Execution != agent.ExecutionSucceeded {
		t.Errorf("execution = %q, want %q", result.Execution, agent.ExecutionSucceeded)
	}
	if result.Verification != agent.VerificationPassed {
		t.Errorf("verification = %q, want %q", result.Verification, agent.VerificationPassed)
	}
	if result.TaskID != "" {
		t.Errorf("task_id = %q, want empty for a synchronous endpoint", result.TaskID)
	}

	// A confirmed terminal outcome must not be retried automatically, and
	// must not be sent back for reconciliation.
	if result.Retry == agent.RetryReconcileFirst {
		t.Errorf("retry = %q, must not demand reconciliation for a synchronous completion", result.Retry)
	}
	for _, w := range result.Warnings {
		if w.Code == "TASK_ID_REQUIRED" || w.Code == "EXECUTION_EVIDENCE_LIMITED" {
			t.Errorf("unexpected warning %s: %s", w.Code, w.Message)
		}
		if strings.Contains(strings.ToLower(w.Message), "reconcile") {
			t.Errorf("synchronous success must not suggest reconciliation: %s", w.Message)
		}
	}
}

func TestMutationOutcomeSynchronousVerificationFailed(t *testing.T) {
	now := "2026-01-01T00:00:00Z"
	base := agent.Result{
		RequestID:     "req_0123456789abcdef0123456789abcdef",
		SchemaVersion: agent.ResultSchemaVersion,
		Operation:     "container update",
		Submission:    agent.SubmissionNotAttempted,
		Execution:     agent.ExecutionNotStarted,
		Verification:  agent.VerificationUnsupported,
		Retry:         agent.RetryDoNotAutomatic,
		StartedAt:     now,
		ObservedAt:    now,
	}

	changed := true
	legacy := output.NewOperationResult("container update", "proxmox", "pve")
	legacy.Submitted = true
	legacy.Synchronous = true
	legacy.Success = true
	legacy.Status = "verification-failed"
	legacy.Changed = &changed
	legacy.Warnings = []string{"cores: requested \"4\", observed \"2\""}

	raw, err := output.MarshalJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}

	result := mutationOutcome(base, OperationMeta{Path: "container update", ProducesUPID: false}, raw, errVerificationFailed)

	if result.Verification != agent.VerificationFailed {
		t.Errorf("verification = %q, want %q", result.Verification, agent.VerificationFailed)
	}
	// The change was applied even though readback disagreed; the endpoint
	// completed, so execution is not an unknown async outcome.
	if result.Execution != agent.ExecutionSucceeded && result.Execution != agent.ExecutionFailed {
		t.Errorf("execution = %q, want a terminal execution state", result.Execution)
	}
}

var errVerificationFailed = errors.New("applied but readback did not match the request")

// Without the Synchronous marker the same payload must still degrade to an
// unobserved outcome. This guards against the fix silently changing the
// meaning of genuinely asynchronous results that return no usable task.
func TestMutationOutcomeWithoutSynchronousStaysUnknown(t *testing.T) {
	now := "2026-01-01T00:00:00Z"
	base := agent.Result{
		RequestID:     "req_0123456789abcdef0123456789abcdef",
		SchemaVersion: agent.ResultSchemaVersion,
		Operation:     "container update",
		Submission:    agent.SubmissionNotAttempted,
		Execution:     agent.ExecutionNotStarted,
		Verification:  agent.VerificationUnsupported,
		Retry:         agent.RetryDoNotAutomatic,
		StartedAt:     now,
		ObservedAt:    now,
	}

	legacy := output.NewOperationResult("container update", "proxmox", "pve")
	legacy.Submitted = true
	legacy.Success = true

	raw, err := output.MarshalJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}

	result := mutationOutcome(base, OperationMeta{Path: "container update", ProducesUPID: false}, raw, nil)
	if result.Execution != agent.ExecutionUnknown {
		t.Errorf("execution = %q, want %q for an unmarked async result", result.Execution, agent.ExecutionUnknown)
	}
}

func TestSynchronousFieldOmitsWhenUnset(t *testing.T) {
	legacy := output.NewOperationResult("vm start", "proxmox", "pve")
	raw, err := output.MarshalJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "synchronous") {
		t.Errorf("synchronous must be omitted when unset to preserve existing output: %s", raw)
	}
}

func TestSynchronousFieldSerializedWhenSet(t *testing.T) {
	legacy := output.NewOperationResult("container update", "proxmox", "pve")
	legacy.Synchronous = true
	raw, err := output.MarshalJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"synchronous": true`) {
		t.Errorf("synchronous must appear when set: %s", raw)
	}
}

// The provider readback is the only evidence for a synchronous endpoint, so an
// unreadable config must not be reported as verified.
func TestReadbackContainerConfigRequiresInspector(t *testing.T) {
	_, err := readbackContainerConfig(context.Background(), &bareProvider{}, "pve1", 9601)
	if err == nil {
		t.Fatal("expected an error when the provider cannot read container config")
	}
	var exitErr *app.ExitCoder
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected a classified exit error, got %T: %v", err, err)
	}
}

// syncCTProvider models the Proxmox LXC config endpoint: the change is applied
// inline and no task identifier is returned.
type syncCTProvider struct {
	bareProvider
	state *syncCTSettings
}

func (p *syncCTProvider) CTConfigUpdate(_ context.Context, _ string, _ int, _ map[string]string) (string, error) {
	p.state.calls++
	return "", nil
}

// VMConfigUpdate satisfies the rest of domain.ConfigProvider; the LXC config
// endpoint is what this provider models.
func (p *syncCTProvider) VMConfigUpdate(_ context.Context, _ string, _ int, _ map[string]string) (string, error) {
	p.state.calls++
	return "", nil
}

// Containers satisfies the list half of domain.ContainerInspector, which the
// readback assertion requires alongside ContainerConfig.
func (p *syncCTProvider) Containers(_ context.Context) ([]domain.Container, error) {
	return nil, nil
}

func (p *syncCTProvider) ContainerConfig(_ context.Context, _ string, vmid int) (map[string]interface{}, error) {
	if p.state.readErr != nil {
		return nil, p.state.readErr
	}
	// The first read happens before the mutation, the second after.
	src := p.state.observed
	if p.state.calls == 0 {
		if p.state.priorReadErr != nil {
			return nil, p.state.priorReadErr
		}
		src = p.state.prior
	}
	out := map[string]interface{}{"vmid": vmid}
	for k, v := range src {
		out[k] = v
	}
	return out, nil
}

const syncCTProviderName = "sync-ct-e2e"

// syncCTSettings is read by the provider factory, which only runs once the
// handler connects during the call under test.
type syncCTSettings struct {
	// prior is returned by the pre-mutation read, observed by the
	// post-mutation read.
	prior        map[string]interface{}
	observed     map[string]interface{}
	readErr      error
	priorReadErr error
	calls        int
}

var syncCTState syncCTSettings

func init() {
	provider.Register(syncCTProviderName, func() domain.Provider {
		return &syncCTProvider{state: &syncCTState}
	})
}

// seedSyncCTProfile registers a profile backed by the synchronous provider so
// the real handler path, including provider connection, is exercised.
func seedSyncCTProfile(t *testing.T) {
	t.Helper()
	isolateConfigAndHome(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	cfg := config.DefaultConfig()
	cfg.CurrentProfile = "syncct"
	cfg.Profiles["syncct"] = config.Profile{
		Provider:      syncCTProviderName,
		Endpoint:      "https://sync-ct.example.invalid",
		CredentialRef: "env:e2e",
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func syncCTCtx(t *testing.T) (*Context, *bytes.Buffer) {
	t.Helper()
	syncCTState = syncCTSettings{
		prior:    map[string]interface{}{},
		observed: map[string]interface{}{},
	}
	var out, errb bytes.Buffer
	return &Context{
		Opts:   Options{Yes: true, Output: "json", Profile: "syncct"},
		Writer: &out,
		ErrW:   &errb,
	}, &out
}

func TestRunCTUpdateSynchronousVerified(t *testing.T) {
	seedSyncCTProfile(t)
	cmdCtx, out := syncCTCtx(t)
	syncCTState.prior = map[string]interface{}{"cores": 1, "memory": 512}
	syncCTState.observed = map[string]interface{}{"cores": 4, "memory": 2048}

	if err := runCTUpdate(context.Background(), cmdCtx, []string{"e2e-node/200", "cores=4", "memory=2048"}); err != nil {
		t.Fatalf("runCTUpdate: %v", err)
	}
	if syncCTState.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", syncCTState.calls)
	}

	var got output.OperationResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode result: %v (output %q)", err, out.String())
	}
	if !got.Submitted || !got.Synchronous || !got.Success {
		t.Errorf("submitted=%v synchronous=%v success=%v, want all true", got.Submitted, got.Synchronous, got.Success)
	}
	if got.UPID != "" {
		t.Errorf("upid = %q, want empty for a synchronous endpoint", got.UPID)
	}
	if got.Status != "verified" {
		t.Errorf("status = %q, want %q", got.Status, "verified")
	}
	if got.Changed == nil || !*got.Changed {
		t.Errorf("changed = %v, want true when the request differs from the prior value", got.Changed)
	}
}

func TestRunCTUpdateSynchronousMismatchIsReported(t *testing.T) {
	seedSyncCTProfile(t)
	cmdCtx, out := syncCTCtx(t)
	syncCTState.prior = map[string]interface{}{"cores": 1}
	syncCTState.observed = map[string]interface{}{"cores": 1}

	err := runCTUpdate(context.Background(), cmdCtx, []string{"e2e-node/200", "cores=4"})
	if err == nil {
		t.Fatal("expected an error when readback does not match the request")
	}
	if !strings.Contains(err.Error(), "cores") {
		t.Errorf("error must name the mismatched field, got %v", err)
	}

	var got output.OperationResult
	if jsonErr := json.Unmarshal(out.Bytes(), &got); jsonErr != nil {
		t.Fatalf("decode result: %v (output %q)", jsonErr, out.String())
	}
	if got.Status != "verification-failed" {
		t.Errorf("status = %q, want %q", got.Status, "verification-failed")
	}
	if len(got.Warnings) == 0 {
		t.Error("expected warnings describing the mismatch")
	}
}

func TestRunCTUpdateSynchronousUnreadablePostcondition(t *testing.T) {
	seedSyncCTProfile(t)
	cmdCtx, out := syncCTCtx(t)
	syncCTState.readErr = errors.New("connection refused")

	err := runCTUpdate(context.Background(), cmdCtx, []string{"e2e-node/200", "cores=4"})
	if err == nil {
		t.Fatal("expected an error when the postcondition cannot be observed")
	}
	if app.ExitCodeFromError(err) != app.ExitAmbiguousOutcome {
		t.Errorf("exit = %d, want %d (ambiguous outcome)", app.ExitCodeFromError(err), app.ExitAmbiguousOutcome)
	}

	var got output.OperationResult
	if jsonErr := json.Unmarshal(out.Bytes(), &got); jsonErr != nil {
		t.Fatalf("decode result: %v (output %q)", jsonErr, out.String())
	}
	// The change was applied; only the evidence is missing. It must not be
	// reported as verified, nor as a known failure.
	if got.Status == "verified" {
		t.Error("must not report verified when readback is unavailable")
	}
	if !got.Submitted || !got.Synchronous {
		t.Errorf("submitted=%v synchronous=%v, want both true", got.Submitted, got.Synchronous)
	}
}

// A value that already matched must not be reported as an applied change.
// Distinguishing a no-op from a real change is only possible because the
// prior state is read before the mutation.
func TestRunCTUpdateSynchronousAlreadyCorrectReportsNoChange(t *testing.T) {
	seedSyncCTProfile(t)
	cmdCtx, out := syncCTCtx(t)
	syncCTState.prior = map[string]interface{}{"cores": 4}
	syncCTState.observed = map[string]interface{}{"cores": 4}

	if err := runCTUpdate(context.Background(), cmdCtx, []string{"e2e-node/200", "cores=4"}); err != nil {
		t.Fatalf("runCTUpdate: %v", err)
	}

	var got output.OperationResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode result: %v (output %q)", err, out.String())
	}
	if got.Status != "verified" {
		t.Errorf("status = %q, want verified", got.Status)
	}
	if got.Changed == nil {
		t.Fatal("changed must be reported when the prior state was observed")
	}
	if *got.Changed {
		t.Error("changed = true, want false when the value already matched")
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "already set") {
		t.Errorf("expected an already-set note, got warnings %v", got.Warnings)
	}
}

// When the prior state cannot be read, "changed" is genuinely unknowable and
// must stay nil rather than being guessed.
func TestRunCTUpdateSynchronousChangedUnknowableWhenPriorUnreadable(t *testing.T) {
	seedSyncCTProfile(t)
	cmdCtx, out := syncCTCtx(t)
	// The pre-mutation read fails, but the post-mutation read succeeds.
	syncCTState.priorReadErr = errors.New("temporary read failure")
	syncCTState.observed = map[string]interface{}{"cores": 4}

	if err := runCTUpdate(context.Background(), cmdCtx, []string{"e2e-node/200", "cores=4"}); err != nil {
		t.Fatalf("runCTUpdate: %v", err)
	}

	var got output.OperationResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode result: %v (output %q)", err, out.String())
	}
	if got.Status != "verified" {
		t.Errorf("status = %q, want verified; the postcondition was observed", got.Status)
	}
	if got.Changed != nil {
		t.Errorf("changed = %v, want nil when the prior state was unobservable", *got.Changed)
	}
}

// The applied-but-unverifiable case must not fall back to demanding a
// reconcile step. That was the original defect: the receipt recommended the
// one action guaranteed to fail.
func TestMutationOutcomeSynchronousUnverifiableDoesNotDemandReconcile(t *testing.T) {
	now := "2026-01-01T00:00:00Z"
	base := agent.Result{
		RequestID:     "req_0123456789abcdef0123456789abcdef",
		SchemaVersion: agent.ResultSchemaVersion,
		Operation:     "container update",
		Submission:    agent.SubmissionNotAttempted,
		Execution:     agent.ExecutionNotStarted,
		Verification:  agent.VerificationUnsupported,
		Retry:         agent.RetryReconcileFirst,
		StartedAt:     now,
		ObservedAt:    now,
	}

	legacy := output.NewOperationResult("container update", "proxmox", "pve")
	legacy.Submitted = true
	legacy.Synchronous = true
	legacy.Success = true
	legacy.Warnings = []string{"configuration was applied but readback verification is unavailable: connection refused"}

	raw, err := output.MarshalJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}

	result := mutationOutcome(base, OperationMeta{Path: "container update", ProducesUPID: false}, raw,
		errors.New("connection refused"))

	if result.Submission != agent.SubmissionAccepted {
		t.Errorf("submission = %q, want accepted; the change was applied", result.Submission)
	}
	if result.Execution != agent.ExecutionSucceeded {
		t.Errorf("execution = %q, want succeeded; the provider applied it inline", result.Execution)
	}
	if result.Verification != agent.VerificationUnsupported {
		t.Errorf("verification = %q, want unsupported; the postcondition was never observed", result.Verification)
	}
	if result.Retry == agent.RetryReconcileFirst {
		t.Error("retry must not demand reconciliation when no task exists")
	}
	for _, a := range result.NextActions {
		if a.Operation == "agent receipt reconcile" || a.Operation == "agent receipt refresh" {
			t.Errorf("unexpected next action %q for a completed synchronous operation", a.Operation)
		}
	}
}
