package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/agent"
	"github.com/geoffmcc/nodex/internal/app"
)

func TestTemplateConvergence(t *testing.T) {
	for _, scenario := range []string{"delayed", "mismatch", "unreadable", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			p := &createdConfigReadbackProvider{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			read := templateConfigVerifier(p, "container", "node", 200)
			verify := func(ctx context.Context) postconditionOutcome {
				calls++
				switch scenario {
				case "delayed":
					if calls == 3 {
						p.ctConfig = map[string]interface{}{"template": "1"}
					}
				case "unreadable":
					p.err = errors.New("read failed")
				case "cancelled":
					cancel()
				}
				return read(ctx)
			}
			got := convergeTemplateReadback(verify, time.Millisecond, 3)(ctx)
			if got.Verified != (scenario == "delayed") || got.Unverifiable != (scenario == "unreadable" || scenario == "cancelled") {
				t.Fatalf("unexpected outcome: %+v", got)
			}
			wantCalls := 3
			if scenario == "unreadable" || scenario == "cancelled" {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("reads = %d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestTemplateReceiptRecoversDelayedVisibility(t *testing.T) {
	initialMutationCalls := e2eCTTemplateCalls
	seedPreservationReceipt(t, func(r *agent.Receipt) {
		r.Operation = "container template"
		r.Context.Node, r.Context.ResourceID, r.Context.ResourceType = "e2e-node", "200", "container"
		r.Execution, r.Verification = agent.ExecutionSucceeded, agent.VerificationFailed
		r.Retry = agent.RetryDoNotAutomatic
		r.Error = &agent.AgentError{Code: "POSTCONDITION_FAILED", Message: "original missing flag", Exit: app.ExitPartialFailure}
	})
	e2eContainerTemplateConfigOverride = false
	t.Cleanup(func() { e2eContainerTemplateConfigOverride = false })
	r, err := refreshPreservationReceipt(t)
	if app.ExitCodeFromError(err) != app.ExitPartialFailure || r.Verification != agent.VerificationFailed {
		t.Fatalf("mismatch must remain failed: %+v, %v", r, err)
	}
	e2eContainerTemplateConfigOverride = true
	for i := 0; i < 2; i++ {
		r, err = refreshPreservationReceipt(t)
		if err != nil || r.Verification != agent.VerificationPassed || r.Error != nil || r.Execution != agent.ExecutionSucceeded || r.Submission != agent.SubmissionAccepted || r.TaskID != "" || r.Changed != nil || r.Retry != agent.RetryDoNotAutomatic {
			t.Fatalf("re-observation failed: %+v, %v", r, err)
		}
		if !hasPreservationWarning(r.Warnings, "PREVIOUS_OBSERVATION_ERROR") || !hasPreservationWarning(r.Warnings, "CAUSATION_NOT_ESTABLISHED") {
			t.Fatalf("history/caveat missing: %+v", r.Warnings)
		}
	}
	if e2eCTTemplateCalls != initialMutationCalls {
		t.Fatal("receipt refresh resubmitted template conversion")
	}
}

func TestTemplateReceiptReadFailureAndTargetBinding(t *testing.T) {
	for _, resource := range []string{"container", "vm"} {
		p := &createdConfigReadbackProvider{err: errors.New("offline")}
		r := agent.Receipt{Result: agent.Result{Operation: "container template", Context: agent.ExecutionContext{Node: "node", ResourceID: "200", ResourceType: resource}, Submission: agent.SubmissionAccepted, Execution: agent.ExecutionSucceeded}}
		err := reobserveTemplateReceipt(context.Background(), p, &r)
		if resource == "vm" {
			if err == nil {
				t.Fatal("mismatched resource identity accepted")
			}
		} else if err != nil || r.Verification != agent.VerificationUnknown || r.Execution != agent.ExecutionSucceeded || r.Error == nil {
			t.Fatalf("read failure lost known execution: %+v, %v", r, err)
		}
	}
}
