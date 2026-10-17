package cli

import (
	"bytes"
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
)

func TestValidateCTDeleteState(t *testing.T) {
	tests := []struct {
		name       string
		containers []domain.Container
		wantExit   int
		wantText   string
	}{
		{
			name:       "running container gives shutdown remedy",
			containers: []domain.Container{{ID: "pve1/200", Status: "running"}},
			wantExit:   app.ExitConflict,
			wantText:   "nodex container shutdown pve1/200",
		},
		{
			name:       "stopped container can proceed",
			containers: []domain.Container{{ID: "pve1/200", Status: "stopped"}},
		},
		{
			name:     "absent container is not found",
			wantExit: app.ExitNotFound,
			wantText: "not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCTDeleteState(tt.containers, "pve1/200")
			if tt.wantExit == 0 {
				if err != nil {
					t.Fatalf("validateCTDeleteState: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantText)
			}
			var exitErr *app.ExitCoder
			if !stderrors.As(err, &exitErr) || exitErr.ExitCode != tt.wantExit {
				t.Fatalf("error = %v, want exit %d", err, tt.wantExit)
			}
		})
	}
}

func TestRunCTDeleteHelpDocumentsStoppedPrecondition(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"container", "delete", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("container delete --help: %v (stderr %q)", err, stderr.String())
	}
	for _, want := range []string{"stopped container", "shut it down first", "--yes", "--force", "--confirm-target"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("help missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestAgentCTDeletePreflightConflictIsNotSubmitted(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	e2eContainerStatusOverride = "running"
	t.Cleanup(func() { e2eContainerStatusOverride = "" })

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"--agent", "--profile", "e2e", "--request-id", "agent-ct-delete-running-20261017",
		"--yes", "--force", "--confirm-target", "e2e-node/200", "--output", "json",
		"container", "delete", "e2e-node/200",
	}, &stdout, &stderr)
	if err == nil || app.ExitCodeFromError(err) != app.ExitConflict {
		t.Fatalf("container delete error = %v, want running-container conflict", err)
	}
	out := stdout.String()
	for _, want := range []string{`"submission": "not_attempted"`, `"execution": "not_started"`, `"retry": "do_not_retry_automatically"`, "nodex container shutdown e2e-node/200"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in agent result: %s", want, out)
		}
	}
	for _, unexpected := range []string{"next_actions", "task_id", "OUTCOME_UNKNOWN"} {
		if strings.Contains(out, unexpected) {
			t.Errorf("agent result contains %q for a preflight rejection: %s", unexpected, out)
		}
	}
}

func TestAgentCTDeletePreflightNotFoundIsNotSubmitted(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	e2eGuestStatusError = 404
	t.Cleanup(func() { e2eGuestStatusError = 0 })

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"--agent", "--profile", "e2e", "--request-id", "agent-ct-delete-missing-20261017",
		"--yes", "--force", "--confirm-target", "e2e-node/998", "--output", "json",
		"container", "delete", "e2e-node/998",
	}, &stdout, &stderr)
	if err == nil || app.ExitCodeFromError(err) != app.ExitNotFound {
		t.Fatalf("container delete error = %v, want not-found", err)
	}
	out := stdout.String()
	for _, want := range []string{`"submission": "not_attempted"`, `"execution": "not_started"`, `"retry": "do_not_retry_automatically"`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in agent result: %s", want, out)
		}
	}
	for _, unexpected := range []string{"next_actions", "task_id", "OUTCOME_UNKNOWN"} {
		if strings.Contains(out, unexpected) {
			t.Errorf("agent result contains %q for a preflight rejection: %s", unexpected, out)
		}
	}
}
