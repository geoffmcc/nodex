package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
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
	// Unverifiable is true when the postcondition could not be checked at all,
	// for example because the provider exposes no suitable read or the read was
	// denied. It is deliberately distinct from a checked-but-contradicted
	// outcome: being unable to look is not evidence that the operation failed.
	Unverifiable bool
	// Changed is non-nil only when evidence supports it, which for a deletion
	// requires the resource to have been observed beforehand.
	Changed *bool
	// Detail explains the observation and is surfaced in structured output.
	Detail string
}

// postconditionVerifier checks one operation's postcondition.
type postconditionVerifier func(ctx context.Context) postconditionOutcome

// applyPostcondition records a verification outcome on a result.
//
// The three outcomes are kept distinct because they mean different things:
// a confirmed end state, a checked-and-contradicted end state, and a check
// that could not be performed. Collapsing the last two would report an
// inability to look as an operation failure.
func applyPostcondition(result *output.OperationResult, outcome postconditionOutcome) {
	switch {
	case outcome.Verified:
		result.Verification = "verified"
	case outcome.Unverifiable:
		result.Verification = "unsupported"
	default:
		result.Verification = "failed"
	}
	if outcome.Changed != nil {
		result.Changed = outcome.Changed
	}
	// A confirmed postcondition is reported by Verification itself, so only
	// report the detail when it explains an unresolved discrepancy.
	if outcome.Detail != "" && !outcome.Verified {
		result.Warnings = append(result.Warnings, outcome.Detail)
	}
}

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
//
// A probe that could not complete returns presenceUnknown together with the
// error that prevented the read. Carrying the cause matters because the
// postcondition report is the only place the operator learns why absence is
// unproven, and "absence is unproven" on its own is not actionable.
type resourceProbe func(ctx context.Context) (presenceState, error)

// errProbeUnsupported explains a probe that never reached the provider because
// the provider does not implement the authoritative single-resource read.
var errProbeUnsupported = errors.New("provider exposes no single-resource read for this resource type")

// classifyProbeRead turns one authoritative read into a presence state.
//
// Absence is recognised through app.IsNotFoundError rather than a bare 404
// status check because the two ways a provider can report "gone" look
// different on the wire. A real Proxmox cluster answers the config endpoint of
// a removed guest with HTTP 500 and a "does not exist" message, which the
// client normalises into app.ErrNotFound; a plain 404 would only ever come
// from a lenient mock. Requiring the 404 status meant a genuinely deleted
// guest was classified as unreadable, so deletion reported "absence is
// unproven" precisely when absence was proven.
func classifyProbeRead(configErr error) (presenceState, error) {
	if configErr == nil {
		return presencePresent, nil
	}
	if app.IsNotFoundError(configErr) {
		return presenceAbsent, nil
	}
	return presenceUnknown, configErr
}

// probeContainer reports whether a container exists, using the single-resource
// config endpoint rather than the cluster-wide listing. A listing is not used
// because inventory can lag the authoritative state and a stale entry would
// wrongly suggest the container survived.
func probeContainer(prov domain.Provider, node string, vmid int) resourceProbe {
	return func(ctx context.Context) (presenceState, error) {
		insp, ok := prov.(domain.ContainerInspector)
		if !ok {
			return presenceUnknown, errProbeUnsupported
		}
		_, err := insp.ContainerConfig(ctx, node, vmid)
		return classifyProbeRead(err)
	}
}

// probeVM reports whether a VM exists, using the single-resource config
// endpoint rather than the cluster-wide listing, for the same reason as
// probeContainer: a lagging inventory entry would wrongly suggest the VM
// survived its own deletion.
func probeVM(prov domain.Provider, node string, vmid int) resourceProbe {
	return func(ctx context.Context) (presenceState, error) {
		insp, ok := prov.(domain.VMInspector)
		if !ok {
			return presenceUnknown, errProbeUnsupported
		}
		_, err := insp.VMConfig(ctx, node, vmid)
		return classifyProbeRead(err)
	}
}

// probeVMSnapshot reports whether a named snapshot of a still-existing VM
// exists.
func probeVMSnapshot(prov domain.Provider, node string, vmid int, name string) resourceProbe {
	return func(ctx context.Context) (presenceState, error) {
		insp, ok := prov.(domain.SnapshotInspector)
		if !ok {
			return presenceUnknown, errProbeUnsupported
		}
		snaps, err := insp.VMSnapshots(ctx, node, vmid)
		if err != nil {
			return presenceUnknown, err
		}
		for _, s := range snaps {
			if s.Name == name {
				return presencePresent, nil
			}
		}
		return presenceAbsent, nil
	}
}

// probeContainerSnapshot reports whether a named snapshot of a still-existing
// container exists.
func probeContainerSnapshot(prov domain.Provider, node string, vmid int, name string) resourceProbe {
	return func(ctx context.Context) (presenceState, error) {
		insp, ok := prov.(domain.SnapshotInspector)
		if !ok {
			return presenceUnknown, errProbeUnsupported
		}
		snaps, err := insp.ContainerSnapshots(ctx, node, vmid)
		if err != nil {
			return presenceUnknown, err
		}
		for _, s := range snaps {
			if s.Name == name {
				return presencePresent, nil
			}
		}
		return presenceAbsent, nil
	}
}

// observePresence performs a single read. A read that proves nothing is
// surfaced as an error so the caller can report the postcondition as
// unverifiable instead of silently treating it as absence. The underlying
// cause is preserved so the report can name it.
func observePresence(ctx context.Context, probe resourceProbe) (presenceState, error) {
	state, err := probe(ctx)
	switch {
	case err != nil:
		return presenceUnknown, err
	case state == presencePresent, state == presenceAbsent:
		return state, nil
	default:
		return presenceUnknown, errors.New("resource state could not be read; absence is unproven")
	}
}

// --- Lifecycle state verification ---
//
// A completed start/stop task is not evidence that the guest reached the
// requested state. The cluster-wide listing that most reads use aggregates
// guest status and can keep reporting the previous state for several seconds
// after the task finishes, so lifecycle postconditions are confirmed with the
// per-guest status endpoint instead.

// guestStateVerifier builds a verifier that confirms a guest reached a
// requested lifecycle state.
//
// priorState is the observation taken before the mutation. It separates "the
// guest was already in that state, so nothing changed" from "the operation
// moved it there".
func guestStateVerifier(prov domain.Provider, resourceType, node string, vmid int, desired string, priorState presenceState, priorStatus string) postconditionVerifier {
	label := fmt.Sprintf("%s %s/%d", resourceType, node, vmid)

	return func(ctx context.Context) postconditionOutcome {
		insp, supported := prov.(domain.GuestStatusInspector)
		if !supported {
			return postconditionOutcome{Unverifiable: true}
		}
		deadline := time.Now().Add(postconditionSettleWindow)
		for {
			observed, err := insp.GuestStatus(ctx, node, resourceType, vmid)
			switch {
			case err != nil:
				return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s status could not be read after the operation: %v", label, err)}
			case strings.EqualFold(observed, desired):
				out := postconditionOutcome{Verified: true, Detail: fmt.Sprintf("%s is %s", label, observed)}
				// Causation needs a prior observation: a guest seen in
				// another state was moved by this operation, a guest already
				// in this state was not, and a guest whose prior state was
				// never read leaves changed unknown.
				if priorState == presencePresent {
					changed := !strings.EqualFold(priorStatus, desired)
					out.Changed = &changed
				}
				return out
			}

			if !time.Now().Before(deadline) {
				return postconditionOutcome{
					Detail: fmt.Sprintf("%s was %q after the operation completed and %s of settling, expected %q",
						label, observed, postconditionSettleWindow, desired),
				}
			}
			select {
			case <-ctx.Done():
				return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s could not be confirmed before the deadline: %v", label, ctx.Err())}
			case <-time.After(postconditionPollInterval):
			}
		}
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
				// The read proved nothing, so absence cannot be claimed in
				// either direction.
				return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s could not be read after the operation: %v", target, err)}
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
				return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s could not be confirmed before the deadline: %v", target, ctx.Err())}
			case <-time.After(postconditionPollInterval):
			}
		}
	}
}
