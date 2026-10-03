package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// --- Synchronous config-update verification ---
//
// Some Proxmox config endpoints apply the change inline and return no task
// identifier. Demanding a task there produced a receipt that reported the
// outcome as unknown and recommended a reconcile step that could never
// succeed, even though the change had already been applied.
//
// For those endpoints the postcondition is read back and compared field by
// field. A changed digest alone is insufficient: the requested values are
// compared individually so a partial application is reported as a failure
// rather than as success.

// configFieldMismatch records a requested key whose readback value differs.
type configFieldMismatch struct {
	Key      string
	Request  string
	Observed string
}

func (m configFieldMismatch) String() string {
	return fmt.Sprintf("%s: requested %q, observed %q", m.Key, m.Request, m.Observed)
}

// canonicalConfigValue renders a provider config value as a comparable string.
// Readback is typed (ints, bools, strings) while requests always arrive as
// strings from the command line, so both sides are normalized here. Numbers
// are compared numerically so "2" and 2 are the same setting.
func canonicalConfigValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case bool:
		if t {
			return "1"
		}
		return "0"
	case int:
		return strconv.Itoa(t)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case float32:
		return trimFloat(float64(t))
	case float64:
		return trimFloat(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// verifyConfigReadback compares each requested key against the provider's
// current config. It returns whether every requested field matched.
//
// A key absent from the readback is reported as a mismatch rather than
// skipped: the provider omitting a field is not evidence the field was set.
func verifyConfigReadback(observed map[string]interface{}, requested map[string]string) (bool, []configFieldMismatch) {
	mismatches := make([]configFieldMismatch, 0, len(requested))
	for key, want := range requested {
		raw, present := observed[key]
		got := ""
		if present {
			got = canonicalConfigValue(raw)
		}
		wantCanonical := strings.TrimSpace(want)
		// Compare numerically when both sides are numeric so that a request
		// of "2" matches a provider value of 2, and "08" matches 8.
		if n1, err1 := strconv.ParseFloat(wantCanonical, 64); err1 == nil {
			if n2, err2 := strconv.ParseFloat(got, 64); err2 == nil && n1 == n2 {
				continue
			}
		}
		if !present || got != wantCanonical {
			if !present {
				got = ""
			}
			mismatches = append(mismatches, configFieldMismatch{Key: key, Request: wantCanonical, Observed: got})
		}
	}
	return len(mismatches) == 0, mismatches
}

// readbackContainerConfig reads the current container config for comparison.
func readbackContainerConfig(ctx context.Context, prov domain.Provider, node string, vmid int) (map[string]interface{}, error) {
	ci, ok := prov.(domain.ContainerInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: container configuration readback not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return ci.ContainerConfig(ctx, node, vmid)
}

// runSynchronousConfigUpdate models a config endpoint that applies inline and
// returns no task identifier.
//
// The change is confirmed by reading the requested fields back rather than by
// polling a task. This reports the endpoint contract accurately: the request
// completed, and the postcondition was observed. It does not claim the change
// is already effective in a running guest, which is a separate question the
// plan treats as distinct from the saved setting.
func runSynchronousConfigUpdate(
	ctx context.Context,
	cmdCtx *Context,
	prov domain.Provider,
	operation, target, node string,
	vmid int,
	requested map[string]string,
	before map[string]interface{},
	beforeErr error,
) error {
	profileName, _ := resolveProfileName(cmdCtx)
	result := output.NewOperationResult(operation, prov.Name(), profileName)
	result.Target = target
	result.Safety = "reversible"
	result.Submitted = true
	result.Success = true
	// No task was returned because none exists for this endpoint. Recording
	// that explicitly is what lets a receipt report a confirmed completion
	// instead of an unobserved asynchronous outcome.
	result.Synchronous = true

	observed, err := readbackContainerConfig(ctx, prov, node, vmid)
	if err != nil {
		// The change was applied; only the postcondition is unobservable.
		// Report that honestly rather than claiming either success or failure.
		result.Warnings = append(result.Warnings,
			"configuration was applied by a synchronous provider endpoint, but readback verification is unavailable: "+err.Error())
		if writeErr := output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result); writeErr != nil {
			return writeErr
		}
		return app.MarkEmitted(app.NewExitError(
			fmt.Errorf("applied %s but could not verify: %w", target, err),
			app.ExitAmbiguousOutcome,
		))
	}

	matched, mismatches := verifyConfigReadback(observed, requested)

	// "Changed" is only reportable when the prior state was actually observed.
	// Guessing either way would misrepresent an already-correct value as an
	// applied change, or an applied change as a no-op.
	if beforeErr == nil && len(requested) > 0 {
		priorMatched, _ := verifyConfigReadback(before, requested)
		changed := !priorMatched
		result.Changed = &changed
		if priorMatched {
			result.Warnings = append(result.Warnings,
				"the requested values were already set; no change was necessary")
		}
	}

	if !matched {
		result.Status = "verification-failed"
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%d of %d requested fields did not read back as requested", len(mismatches), len(requested)))
		for _, m := range mismatches {
			result.Warnings = append(result.Warnings, "  "+m.String())
		}
		if writeErr := output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result); writeErr != nil {
			return writeErr
		}
		return app.MarkEmitted(app.NewExitError(
			fmt.Errorf("%s applied but readback did not match the request: %s", target, joinMismatches(mismatches)),
			app.ExitPartialFailure,
		))
	}

	result.Status = "verified"
	return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result)
}

func joinMismatches(mismatches []configFieldMismatch) string {
	parts := make([]string, 0, len(mismatches))
	for _, m := range mismatches {
		parts = append(parts, m.String())
	}
	return strings.Join(parts, "; ")
}
