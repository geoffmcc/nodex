package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

	if got := probeContainer(context.Background(), prov, "proxmox", 9611)(context.Background()); got != presencePresent {
		t.Fatalf("expected present, got %v", got)
	}
	if got := probeContainer(context.Background(), prov, "proxmox", 9611)(context.Background()); got != presenceAbsent {
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
			probe := probeContainer(context.Background(), prov, "proxmox", 9611)
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
	probe := probeContainer(context.Background(), prov, "proxmox", 9611)

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
	probe := probeContainer(context.Background(), prov, "proxmox", 9611)

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
	probe := probeContainer(context.Background(), prov, "proxmox", 9611)

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
	probe := probeContainer(context.Background(), prov, "proxmox", 9611)

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
	probe := probeContainerSnapshot(context.Background(), prov, "proxmox", 9610, "nx4fu-snap1")

	if got := probe(context.Background()); got != presencePresent {
		t.Fatalf("expected snapshot present, got %v", got)
	}
	if got := probe(context.Background()); got != presenceAbsent {
		t.Fatalf("expected snapshot absent, got %v", got)
	}
}

func TestProbeContainerSnapshotIgnoresOtherSnapshots(t *testing.T) {
	prov := &otherSnapshotsProvider{}
	probe := probeContainerSnapshot(context.Background(), prov, "proxmox", 9610, "nx4fu-snap1")
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
