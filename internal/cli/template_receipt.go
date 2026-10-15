package cli

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/agent"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
)

func templateReceiptReobservable(r *agent.Receipt) bool {
	return (r.Operation == "vm template" || r.Operation == "container template") &&
		r.Submission == agent.SubmissionAccepted && r.Execution == agent.ExecutionSucceeded
}

// The caller validates the stored profile/trust identity before connecting.
// This observes current state, without claiming causation or repeating a write.
func reobserveTemplateReceipt(ctx context.Context, prov domain.Provider, r *agent.Receipt) error {
	resource := strings.Fields(r.Operation)[0]
	vmid, err := strconv.Atoi(r.Context.ResourceID)
	if err != nil || vmid <= 0 || r.Context.Node == "" || r.Context.ResourceType != resource {
		return errors.New("template receipt lacks a matching node/resource identity")
	}
	observed := templateConfigVerifier(prov, resource, r.Context.Node, vmid)(ctx)
	if r.Error != nil {
		addAgentWarning(&r.Result, agent.Warning{Code: "PREVIOUS_OBSERVATION_ERROR", Message: safeMessage(r.Error.Code + ": " + r.Error.Message)})
	}
	r.Error = nil
	r.Changed = nil
	r.Retry = agent.RetryDoNotAutomatic
	r.NextActions = nil
	r.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	switch {
	case observed.Verified:
		r.Verification = agent.VerificationPassed
		addAgentWarning(&r.Result, agent.Warning{Code: "CAUSATION_NOT_ESTABLISHED", Message: "template flag observed now; this does not prove the original request caused the state change"})
	case observed.Unverifiable:
		r.Verification = agent.VerificationUnknown
		r.Error = &agent.AgentError{Code: "POSTCONDITION_UNKNOWN", Message: safeMessage(observed.Detail), Exit: app.ExitPartialFailure}
	default:
		r.Verification = agent.VerificationFailed
		r.Error = &agent.AgentError{Code: "POSTCONDITION_FAILED", Message: safeMessage(observed.Detail), Exit: app.ExitPartialFailure}
	}
	return nil
}
