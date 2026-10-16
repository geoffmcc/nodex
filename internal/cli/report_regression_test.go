package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/agent"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func TestReportNodeUpdatesConfirmation(t *testing.T) {
	isolateConfigAndHome(t)
	var out, diag bytes.Buffer
	err := runNodeUpdates(t.Context(), &Context{Writer: &out, ErrW: &diag, Opts: Options{NonInteractive: true}}, []string{"fixture"})
	if err == nil || !strings.Contains(err.Error(), "confirmation required") {
		t.Fatalf("gate must precede configuration/connection: %v", err)
	}
	setupE2EConfig(t)
	t.Cleanup(func() { e2eNodeUpdatesCalls = 0 })
	for _, agentMode := range []bool{false, true} {
		for _, yes := range []bool{false, true} {
			e2eNodeUpdatesCalls = 0
			args := []string{"--profile", "e2e", "--non-interactive", "node", "updates", "fixture"}
			if agentMode {
				args = append(args, "--agent")
			}
			if yes {
				args = append(args, "--yes")
			}
			var out, diag bytes.Buffer
			err := Run(t.Context(), args, &out, &diag)
			if yes {
				if err != nil || e2eNodeUpdatesCalls != 1 {
					t.Fatalf("confirmed: calls=%d err=%v output=%s", e2eNodeUpdatesCalls, err, out.String())
				}
			} else {
				if err == nil || app.ExitCodeFromError(err) != app.ExitUsage || e2eNodeUpdatesCalls != 0 {
					t.Fatalf("unconfirmed: calls=%d err=%v", e2eNodeUpdatesCalls, err)
				}
				if agentMode {
					r := decodeAgentResult(t, out.Bytes())
					if r.Submission != agent.SubmissionNotAttempted || r.Execution != agent.ExecutionNotStarted {
						t.Fatal(r)
					}
				}
			}
		}
	}
}

func TestReportInventoryTableContextAndEmptyResults(t *testing.T) {
	var out bytes.Buffer
	ctx := &Context{Writer: &out, Opts: Options{Output: output.FormatTable}}
	if err := writeVMs(ctx, nil); err != nil || out.String() != "No VMs found.\n" {
		t.Fatal(err, out.String())
	}
	out.Reset()
	if err := writeContainers(ctx, []domain.Container{{ID: "node/9000", Memory: 1024, Disk: 2048}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CPU-LIMIT", "MEMORY-LIMIT", "DISK-LIMIT", "unknown"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s: %s", want, out.String())
		}
	}
	out.Reset()
	ctx.Opts.Output = output.FormatJSON
	if err := writeVMs(ctx, nil); err != nil || strings.TrimSpace(out.String()) != "[]" {
		t.Fatal(err, out.String())
	}
	out.Reset()
	ctx.Opts.Output = output.FormatTable
	stores := []domain.Storage{
		{ID: "storage/node/one", Name: "one", Type: "pbs", Total: 100, CapacityIdentity: &domain.StorageCapacityIdentity{Key: "same", Scope: "pbs_datastore", Evidence: "config"}},
		{ID: "storage/node/two", Name: "two", Type: "pbs", Total: 100, CapacityIdentity: &domain.StorageCapacityIdentity{Key: "same", Scope: "pbs_datastore", Evidence: "config", Namespace: "nightly"}},
		{ID: "storage/node/other", Name: "other", Type: "pbs", Total: 100, CapacityIdentity: &domain.StorageCapacityIdentity{Key: "distinct", Scope: "pbs_datastore", Evidence: "config"}},
		{ID: "storage/node/unknown", Name: "unknown", Type: "dir", Total: 100},
	}
	if err := writeStorages(ctx, stores); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NAMESPACE", "CAPACITY-GROUP", "(root)", "nightly", "unresolved", "do not sum"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s: %s", want, out.String())
		}
	}
	if strings.Count(out.String(), "pbs_datastore:1") != 2 || strings.Count(out.String(), "pbs_datastore:2") != 1 {
		t.Fatal(out.String())
	}
}

func TestReportCapabilitiesAreImplementationMetadata(t *testing.T) {
	for _, format := range []string{"json", "yaml", "table"} {
		var out, diag bytes.Buffer
		if err := Run(t.Context(), []string{"--output", format, "provider", "capabilities", "proxmox"}, &out, &diag); err != nil {
			t.Fatal(err)
		}
		if format == "table" {
			if !strings.Contains(out.String(), "readiness and authorization are not assessed") {
				t.Fatal(out.String())
			}
		} else if !strings.Contains(out.String(), "implementation") || !strings.Contains(out.String(), "not_assessed") {
			t.Fatal(out.String())
		}
	}
}

func TestReportEveryDiscoveredIDCanBeDescribed(t *testing.T) {
	var out, diag bytes.Buffer
	if err := Run(t.Context(), []string{"operation", "list"}, &out, &diag); err != nil {
		t.Fatal(err)
	}
	var ops []OperationContract
	if err := json.Unmarshal(out.Bytes(), &ops); err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		out.Reset()
		if err := Run(t.Context(), []string{"operation", "describe", op.ID}, &out, &diag); err != nil {
			t.Errorf("%s: %v", op.ID, err)
			continue
		}
		var got OperationContract
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Path != op.Path {
			t.Fatalf("%s: %v %+v", op.ID, err, got)
		}
	}
}

func TestReportInvalidAgentInputDoesNotSuggestAutomaticRetry(t *testing.T) {
	isolateConfigAndHome(t)
	for _, args := range [][]string{
		{"--agent", "status", "--bad-flag"}, {"--agent", "--timeout", "invalid", "status"},
		{"--agent", "--output", "table", "status"}, {"--agent", "--limit", "-1", "container", "list"},
		{"--agent", "--request-id", "bad!", "node", "list"},
	} {
		var out, diag bytes.Buffer
		if err := Run(t.Context(), args, &out, &diag); err == nil {
			t.Fatal(args)
		}
		r := decodeAgentResult(t, out.Bytes())
		if r.Retry != agent.RetryDoNotAutomatic || r.Submission != agent.SubmissionNotAttempted {
			t.Fatalf("%v: %+v", args, r)
		}
	}
}

func TestReportMissingReceiptMessage(t *testing.T) {
	isolateConfigAndHome(t)
	var out, diag bytes.Buffer
	err := Run(t.Context(), []string{"--agent", "agent", "receipt", "show", "fixture-missing"}, &out, &diag)
	if err == nil || app.ExitCodeFromError(err) != app.ExitNotFound {
		t.Fatal(err)
	}
	r := decodeAgentResult(t, out.Bytes())
	if r.Error == nil || r.Error.Message != `receipt "fixture-missing": not found` || strings.Contains(out.String(), "GetFileAttributes") {
		t.Fatal(out.String())
	}
}

func TestReportReceiptRecoveryRequiresAdvertisedConfirmation(t *testing.T) {
	isolateConfigAndHome(t)
	for _, op := range []string{"refresh", "reconcile"} {
		for _, agentMode := range []bool{false, true} {
			var out, diag bytes.Buffer
			args := []string{"--non-interactive", "agent", "receipt", op, "fixture-missing"}
			if agentMode {
				args = append(args, "--agent")
			}
			err := Run(t.Context(), args, &out, &diag)
			if err == nil || app.ExitCodeFromError(err) != app.ExitUsage || !strings.Contains(err.Error(), "confirmation required") {
				t.Fatalf("%s: %v", op, err)
			}
			if agentMode {
				r := decodeAgentResult(t, out.Bytes())
				if r.Submission != agent.SubmissionNotAttempted || r.Retry != agent.RetryDoNotAutomatic {
					t.Fatal(r)
				}
			}
		}
	}
}
