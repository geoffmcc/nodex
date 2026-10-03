package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
)

// --- Postcondition verification for completed mutations ---
//
// A provider task reporting success only proves the provider accepted and
// finished the job. It is not evidence that the requested end state was
// reached, so operations that have an observable postcondition verify it with
// a separate, fresh read after the task completes.
//
// Two rules shape the implementation:
//
//   - Absence must be observed, never inferred. Only an explicit "not found"
//     answer from the authoritative single-resource endpoint establishes that
//     a resource is gone. A denied or failed lookup is reported as an
//     unverifiable postcondition, never as absence, because reporting
//     absence without evidence would overstate what is known.
//   - A postcondition is only checked once the task has completed. Reading
//     earlier would race the operation and could record a stale state as the
//     outcome.

// postconditionPollInterval is how often the postcondition is re-read while
// waiting for the requested state to appear.
const postconditionPollInterval = 500 * time.Millisecond

// postconditionSettleWindow bounds how long a completed operation is given to
// make its postcondition observable before the check is abandoned as
// unverified. Provider state can briefly lag a finished task, so a single
// immediate read is not conclusive either way.
const postconditionSettleWindow = 3 * time.Second

// postconditionOutcome is the result of one postcondition check.
type postconditionOutcome struct {
	// Verified is true when the requested end state was observed.
	Verified bool
	// Changed is non-nil only when evidence supports it, which for a deletion
	// requires the resource to have been observed beforehand.
	Changed *bool
	// Detail explains the observation and is surfaced in structured output.
	Detail string
}

// postconditionVerifier checks one operation's postcondition.
type postconditionVerifier func(ctx context.Context) postconditionOutcome

// presenceState describes what a single authoritative read established.
type presenceState int

const (
	// presenceUnknown means the read failed in a way that proves nothing,
	// such as a permission denial or a transport error.
	presenceUnknown presenceState = iota
	// presencePresent means the resource exists.
	presencePresent
	// presenceAbsent means the provider reported the resource does not exist.
	presenceAbsent
)

// resourceProbe reads one resource and reports whether it exists.
type resourceProbe func(ctx context.Context) presenceState

// probeContainer reports whether a container exists, using the single-resource
// config endpoint rather than the cluster-wide listing. A listing is not used
// because inventory can lag the authoritative state and a stale entry would
// wrongly suggest the container survived.
func probeContainer(ctx context.Context, prov domain.Provider, node string, vmid int) resourceProbe {
	return func(ctx context.Context) presenceState {
		insp, ok := prov.(domain.ContainerInspector)
		if !ok {
			return presenceUnknown
		}
		if _, err := insp.ContainerConfig(ctx, node, vmid); err != nil {
			if app.HTTPStatusFromError(err) == http.StatusNotFound {
				return presenceAbsent
			}
			return presenceUnknown
		}
		return presencePresent
	}
}

// probeContainerSnapshot reports whether a named snapshot of a still-existing
// container exists.
func probeContainerSnapshot(ctx context.Context, prov domain.Provider, node string, vmid int, name string) resourceProbe {
	return func(ctx context.Context) presenceState {
		insp, ok := prov.(domain.SnapshotInspector)
		if !ok {
			return presenceUnknown
		}
		snaps, err := insp.ContainerSnapshots(ctx, node, vmid)
		if err != nil {
			return presenceUnknown
		}
		for _, s := range snaps {
			if s.Name == name {
				return presencePresent
			}
		}
		return presenceAbsent
	}
}

// observePresence performs a single read. A read that proves nothing is
// surfaced as an error so the caller can report the postcondition as
// unverifiable instead of silently treating it as absence.
func observePresence(ctx context.Context, probe resourceProbe) (presenceState, error) {
	switch state := probe(ctx); state {
	case presencePresent:
		return presencePresent, nil
	case presenceAbsent:
		return presenceAbsent, nil
	default:
		return presenceUnknown, errors.New("resource state could not be read; absence is unproven")
	}
}

// absenceVerifier builds a verifier for operations whose postcondition is that
// a resource is gone.
//
// priorState is the observation taken before the mutation and is used as-is:
// re-reading it here would sample the post-mutation state and could report that
// nothing changed. A deletion can only claim it changed something when the
// resource was actually seen beforehand; otherwise Changed stays unknown.
func absenceVerifier(probe resourceProbe, target string, priorState presenceState) postconditionVerifier {
	return func(ctx context.Context) postconditionOutcome {
		deadline := time.Now().Add(postconditionSettleWindow)
		for {
			state, err := observePresence(ctx, probe)
			switch {
			case err != nil:
				return postconditionOutcome{Detail: fmt.Sprintf("%s could not be read after the operation: %v", target, err)}
			case state == presenceAbsent:
				out := postconditionOutcome{Verified: true, Detail: fmt.Sprintf("%s no longer exists", target)}
				if priorState == presencePresent {
					changed := true
					out.Changed = &changed
				}
				return out
			}

			if !time.Now().Before(deadline) {
				return postconditionOutcome{
					Detail: fmt.Sprintf("%s still existed after the operation completed and %s of settling",
						target, postconditionSettleWindow),
				}
			}
			select {
			case <-ctx.Done():
				return postconditionOutcome{Detail: fmt.Sprintf("%s could not be confirmed before the deadline: %v", target, ctx.Err())}
			case <-time.After(postconditionPollInterval):
			}
		}
	}
}
