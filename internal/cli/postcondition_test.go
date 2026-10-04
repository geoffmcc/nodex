package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/agent"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// --- Postcondition verification tests ---
//
// The load-bearing property is that absence is observed rather than inferred.
// A lookup that fails proves nothing, so reporting it as absence would tell the
// operator a resource is gone when it may still be there.

// scriptedCTProvider answers container reads from a fixed script of
// observations, so each read consumes the next entry.
type scriptedCTProvider struct {
	bareProvider
	steps []presenceState
	calls int
}

func (p *scriptedCTProvider) Containers(_ context.Context) ([]domain.Container, error) {
	return nil, nil
}

func (p *scriptedCTProvider) ContainerConfig(_ context.Context, _ string, _ int) (map[string]interface{}, error) {
	if p.calls >= len(p.steps) {
		p.calls++
		return nil, fmt.Errorf("read %d: script exhausted", p.calls)
	}
	state := p.steps[p.calls]
	p.calls++
	switch state {
	case presencePresent:
		return map[string]interface{}{"vmid": 9611}, nil
	case presenceAbsent:
		return nil, &app.ProviderError{StatusCode: http.StatusNotFound, Detail: "no such container"}
	default:
		return nil, errors.New("connection reset")
	}
}

func TestProbeContainerReportsPresenceAndAbsence(t *testing.T) {
	prov := &scriptedCTProvider{steps: []presenceState{presencePresent, presenceAbsent}}

	if got := probeContainer(prov, "proxmox", 9611)(context.Background()); got != presencePresent {
		t.Fatalf("expected present, got %v", got)
	}
	if got := probeContainer(prov, "proxmox", 9611)(context.Background()); got != presenceAbsent {
		t.Fatalf("expected absent, got %v", got)
	}
}

func TestProbeContainerTreatsDeniedAndFailedReadsAsUnknown(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"permission denied", &app.ProviderError{StatusCode: http.StatusForbidden, Detail: "permission denied"}},
		{"server error", &app.ProviderError{StatusCode: http.StatusInternalServerError, Detail: "internal error"}},
		{"transport failure", errors.New("connection reset by peer")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov := &readErrCTProvider{err: tc.err}
			probe := probeContainer(prov, "proxmox", 9611)
			if got := probe(context.Background()); got != presenceUnknown {
				t.Fatalf("expected unknown, got %v", got)
			}
			// The critical guarantee: a failed read must never be usable as
			// evidence that the resource is gone.
			state, err := observePresence(context.Background(), probe)
			if err == nil {
				t.Fatal("expected an unproven-absence error")
			}
			if state == presenceAbsent {
				t.Fatal("a failed read must not be reported as absence")
			}
		})
	}
}

type readErrCTProvider struct {
	bareProvider
	err error
}

func (p *readErrCTProvider) Containers(_ context.Context) ([]domain.Container, error) {
	return nil, nil
}

func (p *readErrCTProvider) ContainerConfig(_ context.Context, _ string, _ int) (map[string]interface{}, error) {
	return nil, p.err
}

func TestAbsenceVerifierConfirmsRemovalAndReportsChange(t *testing.T) {
	// Prior read observes the container; the postcondition read sees it gone.
	prov := &scriptedCTProvider{steps: []presenceState{presencePresent, presenceAbsent}}
	probe := probeContainer(prov, "proxmox", 9611)

	prior, err := observePresence(context.Background(), probe)
	if err != nil || prior != presencePresent {
		t.Fatalf("expected present precondition, got %v err=%v", prior, err)
	}

	outcome := absenceVerifier(probe, "container proxmox/9611", prior)(context.Background())
	if !outcome.Verified {
		t.Fatalf("expected verified, got %+v", outcome)
	}
	if outcome.Changed == nil || !*outcome.Changed {
		t.Fatalf("expected changed=true when the resource was seen beforehand, got %v", outcome.Changed)
	}
}

func TestAbsenceVerifierLeavesChangeUnknownWhenPriorReadFailed(t *testing.T) {
	// The prior read proves nothing, so absence alone cannot establish change.
	prov := &readErrCTProvider{err: errors.New("permission denied")}
	probe := probeContainer(prov, "proxmox", 9611)

	prior, err := observePresence(context.Background(), probe)
	if err == nil {
		t.Fatal("expected the prior read to be unproven")
	}

	outcome := absenceVerifier(probe, "container proxmox/9611", prior)(context.Background())
	if outcome.Verified {
		t.Fatalf("a denied read must not verify absence, got %+v", outcome)
	}
	if outcome.Changed != nil {
		t.Fatalf("changed must stay unknown, got %v", *outcome.Changed)
	}
	if outcome.Detail == "" {
		t.Fatal("expected an explanation of why the postcondition was unproven")
	}
}

func TestAbsenceVerifierRejectsStalePresentReadsUntilSettled(t *testing.T) {
	// Provider state lags the finished task: the resource is still reported
	// present for the first two reads and only then absent.
	prov := &scriptedCTProvider{steps: []presenceState{
		presencePresent, // precondition read
		presencePresent, // first postcondition read, still stale
		presencePresent, // second postcondition read, still stale
		presenceAbsent,  // finally converged
	}}
	probe := probeContainer(prov, "proxmox", 9611)

	prior, err := observePresence(context.Background(), probe)
	if err != nil || prior != presencePresent {
		t.Fatalf("expected present precondition, got %v err=%v", prior, err)
	}

	outcome := absenceVerifier(probe, "container proxmox/9611", prior)(context.Background())
	if !outcome.Verified {
		t.Fatalf("expected verification to tolerate a lagging read, got %+v", outcome)
	}
}

func TestAbsenceVerifierFailsWhenResourceStillPresent(t *testing.T) {
	// The task succeeded but the resource is still there, so the postcondition
	// is contradicted. The outcome is not verified and no change is claimed.
	prov := &scriptedCTProvider{steps: []presenceState{presencePresent, presencePresent}}
	probe := probeContainer(prov, "proxmox", 9611)

	prior, _ := observePresence(context.Background(), probe)
	outcome := absenceVerifier(probe, "container proxmox/9611", prior)(context.Background())
	if outcome.Verified {
		t.Fatalf("a surviving resource must not verify, got %+v", outcome)
	}
	if outcome.Changed != nil {
		t.Fatalf("changed must stay unknown when the postcondition is contradicted, got %v", *outcome.Changed)
	}
}

func TestAbsenceVerifierStopsAtDeadlineWithoutClaimingAbsence(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	alwaysPresent := func(context.Context) presenceState { return presencePresent }
	outcome := absenceVerifier(alwaysPresent, "container proxmox/9611", presencePresent)(ctx)
	if outcome.Verified {
		t.Fatalf("a cancelled context must not verify absence, got %+v", outcome)
	}
}

// scriptedSnapshotProvider answers snapshot listings from a fixed script.
type scriptedSnapshotProvider struct {
	bareProvider
	steps []presenceState
	calls int
}

func (p *scriptedSnapshotProvider) VMSnapshots(_ context.Context, _ string, _ int) ([]domain.Snapshot, error) {
	return nil, nil
}

func (p *scriptedSnapshotProvider) ContainerSnapshots(_ context.Context, _ string, _ int) ([]domain.Snapshot, error) {
	if p.calls >= len(p.steps) {
		p.calls++
		return nil, errors.New("listing failed")
	}
	state := p.steps[p.calls]
	p.calls++
	if state == presencePresent {
		return []domain.Snapshot{{Name: "nx4fu-snap1"}}, nil
	}
	return nil, nil
}

func TestProbeContainerSnapshotDetectsNamedSnapshotRemoval(t *testing.T) {
	prov := &scriptedSnapshotProvider{steps: []presenceState{presencePresent, presenceAbsent}}
	probe := probeContainerSnapshot(prov, "proxmox", 9610, "nx4fu-snap1")

	if got := probe(context.Background()); got != presencePresent {
		t.Fatalf("expected snapshot present, got %v", got)
	}
	if got := probe(context.Background()); got != presenceAbsent {
		t.Fatalf("expected snapshot absent, got %v", got)
	}
}

func TestProbeContainerSnapshotIgnoresOtherSnapshots(t *testing.T) {
	prov := &otherSnapshotsProvider{}
	probe := probeContainerSnapshot(prov, "proxmox", 9610, "nx4fu-snap1")
	if got := probe(context.Background()); got != presenceAbsent {
		t.Fatalf("a different snapshot name must not count as the target, got %v", got)
	}
}

type otherSnapshotsProvider struct{ bareProvider }

func (p *otherSnapshotsProvider) VMSnapshots(_ context.Context, _ string, _ int) ([]domain.Snapshot, error) {
	return nil, nil
}

// TestPostconditionVerificationOverridesProviderTaskStatus ensures a completed
// task is not reported as a verified outcome on its own. The provider status
// and the observed postcondition stay separate facts, and only the explicit
// postcondition verdict decides verification.
func TestPostconditionVerificationOverridesProviderTaskStatus(t *testing.T) {
	now := "2026-01-02T03:04:05Z"
	cases := []struct {
		name         string
		verification string
		want         agent.Verification
	}{
		{"absence confirmed", "verified", agent.VerificationPassed},
		{"postcondition contradicted", "failed", agent.VerificationFailed},
		// An unreadable postcondition is not a failure: the receipt must not
		// claim the state was disproven when it simply could not be read.
		{"postcondition unreadable", "unsupported", agent.VerificationUnsupported},
		{"no check attempted", "", agent.VerificationUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := agent.Result{
				SchemaVersion: agent.ResultSchemaVersion, RequestID: "req_del", ReceiptID: "req_del",
				Operation: "container delete", Context: agent.ExecutionContext{Profile: "pve", Provider: "proxmox", Node: "proxmox", ResourceID: "9611"},
				Submission: agent.SubmissionUnknown, Execution: agent.ExecutionUnknown, Verification: agent.VerificationUnsupported,
				Retry: agent.RetryReconcileFirst, StartedAt: now, ObservedAt: now,
			}
			legacy := output.NewOperationResult("container delete", "proxmox", "pve")
			legacy.Submitted, legacy.Waited, legacy.Success, legacy.Status = true, true, true, "OK"
			legacy.Verification = tc.verification
			raw, err := output.MarshalJSON(legacy)
			if err != nil {
				t.Fatal(err)
			}
			result := mutationOutcome(base, OperationMeta{Path: "container delete"}, raw, nil)
			if result.Execution != agent.ExecutionSucceeded {
				t.Fatalf("task completion must remain succeeded regardless of postcondition, got %q", result.Execution)
			}
			if result.Verification != tc.want {
				t.Fatalf("expected verification %q, got %q", tc.want, result.Verification)
			}
		})
	}
}

func (p *otherSnapshotsProvider) ContainerSnapshots(_ context.Context, _ string, _ int) ([]domain.Snapshot, error) {
	return []domain.Snapshot{{Name: "unrelated"}}, nil
}

// --- Lifecycle state verification tests ---
//
// A finished lifecycle task proves the task ran, not that the guest reached the
// requested state. These tests pin that the state is observed from the
// authoritative per-guest read, and that an inability to read it is reported as
// unverifiable rather than as failure.

// scriptedGuestProvider replays a fixed sequence of per-guest statuses.
type scriptedGuestProvider struct {
	bareProvider
	statuses []string
	errs     []error
	calls    int
}

func (p *scriptedGuestProvider) GuestStatus(_ context.Context, _ string, _ string, _ int) (string, error) {
	i := p.calls
	p.calls++
	if i < len(p.errs) && p.errs[i] != nil {
		return "", p.errs[i]
	}
	if i >= len(p.statuses) {
		return "", fmt.Errorf("read %d: script exhausted", i+1)
	}
	return p.statuses[i], nil
}

func TestGuestStateVerifierConfirmsReachedState(t *testing.T) {
	prov := &scriptedGuestProvider{statuses: []string{"running"}}

	out := guestStateVerifier(prov, "vm", "proxmox", 101, "running", presencePresent, "stopped")(context.Background())
	if !out.Verified {
		t.Fatalf("expected verified, got %+v", out)
	}
	if out.Unverifiable {
		t.Fatalf("a confirmed read must not be reported as unverifiable: %+v", out)
	}
	if out.Changed == nil || !*out.Changed {
		t.Fatalf("moving a guest from stopped to running must report changed, got %+v", out.Changed)
	}
}

func TestGuestStateVerifierReportsNoChangeWhenAlreadyDesired(t *testing.T) {
	prov := &scriptedGuestProvider{statuses: []string{"running"}}

	out := guestStateVerifier(prov, "vm", "proxmox", 100, "running", presencePresent, "running")(context.Background())
	if !out.Verified {
		t.Fatalf("expected verified, got %+v", out)
	}
	if out.Changed == nil || *out.Changed {
		t.Fatalf("a guest already in the requested state was not changed, got %+v", out.Changed)
	}
}

func TestGuestStateVerifierKeepsChangeUnknownWhenPriorReadFailed(t *testing.T) {
	prov := &scriptedGuestProvider{statuses: []string{"running"}}

	out := guestStateVerifier(prov, "vm", "proxmox", 101, "running", presenceUnknown, "")(context.Background())
	if !out.Verified {
		t.Fatalf("expected verified, got %+v", out)
	}
	if out.Changed != nil {
		t.Fatalf("without a prior observation change must stay unknown, got %v", *out.Changed)
	}
}

func TestGuestStateVerifierRejectsStaleReadsUntilDesired(t *testing.T) {
	prov := &scriptedGuestProvider{statuses: []string{"stopped", "stopped", "running"}}

	out := guestStateVerifier(prov, "vm", "proxmox", 101, "running", presencePresent, "stopped")(context.Background())
	if !out.Verified {
		t.Fatalf("a stale read must be retried, not accepted: %+v", out)
	}
	if prov.calls != 3 {
		t.Fatalf("expected the verifier to poll through stale reads, made %d reads", prov.calls)
	}
}

func TestGuestStateVerifierFailsWhenStateContradictsAfterSettling(t *testing.T) {
	prov := &scriptedGuestProvider{statuses: []string{"stopped", "stopped", "stopped", "stopped", "stopped", "stopped", "stopped", "stopped", "stopped", "stopped"}}

	out := guestStateVerifier(prov, "vm", "proxmox", 101, "running", presencePresent, "stopped")(context.Background())
	if out.Verified {
		t.Fatal("must not claim verified while the guest is still stopped")
	}
	if out.Unverifiable {
		t.Fatalf("a contradicted check is a failure, not an inability to check: %+v", out)
	}
	if out.Detail == "" {
		t.Fatal("a failed postcondition must explain the observed state")
	}
}

func TestGuestStateVerifierDenialIsUnverifiableNotFailure(t *testing.T) {
	prov := &scriptedGuestProvider{
		errs: []error{&app.ProviderError{StatusCode: http.StatusForbidden, Detail: "permission denied"}},
	}

	out := guestStateVerifier(prov, "vm", "proxmox", 101, "running", presencePresent, "stopped")(context.Background())
	if out.Verified {
		t.Fatal("a denied read proves nothing and must not verify")
	}
	if !out.Unverifiable {
		t.Fatalf("a denied read must be unverifiable, not a reported failure: %+v", out)
	}
}

func TestGuestStateVerifierUnsupportedProviderIsUnverifiable(t *testing.T) {
	out := guestStateVerifier(&bareProvider{}, "vm", "proxmox", 101, "running", presencePresent, "stopped")(context.Background())
	if out.Verified || !out.Unverifiable {
		t.Fatalf("a provider without per-guest status must be unverifiable: %+v", out)
	}
}

func TestGuestStateVerifierStopsWhenContextIsCancelled(t *testing.T) {
	prov := &scriptedGuestProvider{statuses: []string{"stopped"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := guestStateVerifier(prov, "vm", "proxmox", 101, "running", presencePresent, "stopped")(ctx)
	if out.Verified || !out.Unverifiable {
		t.Fatalf("a cancelled check must not claim verification: %+v", out)
	}
}

func TestApplyPostconditionMapsOutcomesToDistinctVerdicts(t *testing.T) {
	changed := true
	tests := []struct {
		name    string
		outcome postconditionOutcome
		want    string
	}{
		{"confirmed", postconditionOutcome{Verified: true}, "verified"},
		{"contradicted", postconditionOutcome{}, "failed"},
		{"unreadable", postconditionOutcome{Unverifiable: true, Detail: "denied"}, "unsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result output.OperationResult
			applyPostcondition(&result, tt.outcome)
			if result.Verification != tt.want {
				t.Fatalf("got %q, want %q", result.Verification, tt.want)
			}
		})
	}

	// An unverifiable read must still surface why it could not be checked.
	var warned output.OperationResult
	applyPostcondition(&warned, postconditionOutcome{Unverifiable: true, Detail: "permission denied"})
	if len(warned.Warnings) != 1 {
		t.Fatalf("expected the unresolved detail to be surfaced, got %v", warned.Warnings)
	}

	// A confirmed postcondition is reported by Verification, not as a warning.
	var quiet output.OperationResult
	applyPostcondition(&quiet, postconditionOutcome{Verified: true, Changed: &changed, Detail: "vm is running"})
	if len(quiet.Warnings) != 0 {
		t.Fatalf("a confirmed postcondition must not add warnings, got %v", quiet.Warnings)
	}
	if quiet.Changed == nil || !*quiet.Changed {
		t.Fatal("changed evidence must be preserved")
	}
}

// --- Single-guest read correctness ---
//
// A single-container read must not report a stale aggregate status, because that
// is exactly what made a completed lifecycle task look like it had not applied.

func TestContainerShowUsesAuthoritativeStatus(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	// The mock listing still reports the container as running while the
	// authoritative per-guest read reports it stopped, which is the lag the
	// listing-based read would have surfaced.
	e2eContainerStatusOverride = "stopped"
	defer func() { e2eContainerStatusOverride = "" }()

	var out, errOut strings.Builder
	if err := Run(context.Background(), []string{"--output", "json", "container", "show", "e2e-node/200"}, &out, &errOut); err != nil {
		t.Fatalf("container show: %v (stderr: %s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), `"status": "stopped"`) {
		t.Fatalf("expected the authoritative status, got: %s", out.String())
	}
}

// --- Deletion postconditions ---
//
// A completed delete task proves the task ran, not that the resource is gone.
// These cover both guest families and both resource kinds, because container
// delete was verified first and VM delete must not be left claiming no
// postcondition at all.

func TestDeletionPostconditionsAreVerified(t *testing.T) {
	tests := []struct {
		name string
		args []string
		// confirm is the exact value this command requires for type-in
		// confirmation, which differs per operation.
		confirm string
		// statusOverride changes the cluster listing's reported status,
		// which the VM delete precondition reads before allowing deletion.
		statusOverride string
	}{
		{
			name: "vm-delete",
			// Deleting a running VM is refused by a precondition that this
			// change does not alter, so the guest must read as stopped.
			args:           []string{"vm", "delete", "e2e-node/100"},
			confirm:        "e2e-node/100",
			statusOverride: "stopped",
		},
		{
			name:    "vm-snapshot-delete",
			args:    []string{"vm", "snapshot", "delete", "e2e-node/100", "before-upgrade"},
			confirm: "before-upgrade",
		},
		{
			name:    "container-delete",
			args:    []string{"container", "delete", "e2e-node/200"},
			confirm: "e2e-node/200",
		},
		{
			name:    "container-snapshot-delete",
			args:    []string{"container", "snapshot", "delete", "e2e-node/200", "clean"},
			confirm: "clean",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolateConfigAndHome(t)
			setupE2EConfig(t)

			if tc.statusOverride != "" {
				e2eVMStatusOverride = tc.statusOverride
				defer func() { e2eVMStatusOverride = "" }()
			}

			args := []string{"--output", "json", "--yes", "--force", "--wait", "--confirm-target", tc.confirm}
			args = append(args, tc.args...)
			var out, errOut strings.Builder
			if err := Run(context.Background(), args, &out, &errOut); err != nil {
				t.Fatalf("%s: %v (stderr: %s)", tc.name, err, errOut.String())
			}
			raw := out.String()
			for _, want := range []string{`"status": "OK"`, `"verification": "verified"`, `"changed": true`} {
				if !strings.Contains(raw, want) {
					t.Errorf("expected %q in output: %s", want, raw)
				}
			}
			// A verified absence is not a warning.
			if strings.Contains(raw, `"warnings"`) {
				t.Errorf("a verified postcondition must not add warnings: %s", raw)
			}
		})
	}
}

// An unreadable deletion check must not be reported as a surviving resource:
// absence that could not be read is unknown, not proof either way.
func TestDeletionPostconditionUnreadableIsNotFailure(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)

	var out, errOut strings.Builder
	args := []string{"--output", "json", "--yes", "--force", "--wait", "--confirm-target", "missing-snapshot",
		"vm", "snapshot", "delete", "e2e-node/100", "missing-snapshot"}
	if err := Run(context.Background(), args, &out, &errOut); err != nil {
		t.Fatalf("vm snapshot delete: %v (stderr: %s)", err, errOut.String())
	}
	raw := out.String()
	if strings.Contains(raw, `"verification": "failed"`) {
		t.Errorf("an unverifiable deletion must not be reported as failed: %s", raw)
	}
}
