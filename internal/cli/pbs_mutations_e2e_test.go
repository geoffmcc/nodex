package cli

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/safety"
)

// pbsE2EConflictMode makes the mock report a running garbage-collection
// task on datastore "backups".
var pbsE2EConflictMode bool

// pbsE2ERunCalls records mutation invocations so tests can assert that
// refused operations never executed.
var pbsE2ERunCalls []string

const pbsE2EMutationUPID = "UPID:pbs-e2e:0000CCCC:0000DDDD:00000003:65f00002:verificationjob:backups:automation@pbs!nodex:"

func (p *pbsE2EMockProvider) PBSRunVerifyJob(_ context.Context, id string) (string, error) {
	pbsE2ERunCalls = append(pbsE2ERunCalls, "verify-job:"+id)
	return pbsE2EMutationUPID, nil
}

func (p *pbsE2EMockProvider) PBSVerifyDatastore(_ context.Context, store string) (string, error) {
	pbsE2ERunCalls = append(pbsE2ERunCalls, "verify-datastore:"+store)
	return pbsE2EMutationUPID, nil
}

func (p *pbsE2EMockProvider) PBSRunSyncJob(_ context.Context, id string) (string, error) {
	pbsE2ERunCalls = append(pbsE2ERunCalls, "sync-job:"+id)
	return pbsE2EMutationUPID, nil
}

func (p *pbsE2EMockProvider) PBSRunPruneJob(_ context.Context, id string) (string, error) {
	pbsE2ERunCalls = append(pbsE2ERunCalls, "prune-job:"+id)
	return pbsE2EMutationUPID, nil
}

func (p *pbsE2EMockProvider) PBSRunGarbageCollection(_ context.Context, store string) (string, error) {
	pbsE2ERunCalls = append(pbsE2ERunCalls, "gc:"+store)
	if store == "failstore" {
		return "UPID:pbs-e2e:0000EEEE:0000FFFF:00000004:65f00003:garbage_collection:failstore:automation@pbs!nodex:", nil
	}
	return pbsE2EMutationUPID, nil
}

func seedPBSMutationTest(t *testing.T) {
	t.Helper()
	seedPBSE2EConfig(t)
	pbsE2ERunCalls = nil
	t.Cleanup(func() {
		pbsE2ERunCalls = nil
		pbsE2EConflictMode = false
	})
}

// withStdin replaces the CLI stdin source for one test.
func withStdin(t *testing.T, input string) {
	t.Helper()
	prev := osIn
	osIn = func() io.Reader { return strings.NewReader(input) }
	t.Cleanup(func() { osIn = prev })
}

func TestPBSMutation_VerifyRunJobRequiresYes(t *testing.T) {
	seedPBSMutationTest(t)
	_, _, err := runPBSCommand(t, "pbs", "verify", "run", "v-daily")
	if err == nil {
		t.Fatal("expected authorization-required error without --yes")
	}
	if !stderrors.Is(err, safety.ErrAuthorizationRequired) {
		t.Errorf("error = %v, want ErrAuthorizationRequired", err)
	}
	if len(pbsE2ERunCalls) != 0 {
		t.Errorf("refused operation must not execute, got calls: %v", pbsE2ERunCalls)
	}
}

func TestPBSMutation_VerifyRunJobWithYes(t *testing.T) {
	seedPBSMutationTest(t)
	stdout, _, err := runPBSCommand(t, "--output", "json", "--yes", "pbs", "verify", "run", "v-daily")
	if err != nil {
		t.Fatalf("verify run: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("invalid OperationResult JSON: %v\n%s", jsonErr, stdout)
	}
	if result["operation"] != "pbs verify run" || result["submitted"] != true || result["success"] != true {
		t.Errorf("unexpected result: %v", result)
	}
	if result["upid"] != pbsE2EMutationUPID {
		t.Errorf("upid = %v, want %v", result["upid"], pbsE2EMutationUPID)
	}
	if got := fmt.Sprintf("%v", pbsE2ERunCalls); got != "[verify-job:v-daily]" {
		t.Errorf("calls = %s", got)
	}
}

func TestPBSMutation_VerifyRunDatastore(t *testing.T) {
	seedPBSMutationTest(t)
	_, _, err := runPBSCommand(t, "--yes", "pbs", "verify", "run", "--datastore", "backups")
	if err != nil {
		t.Fatalf("verify run --datastore: %v", err)
	}
	if got := fmt.Sprintf("%v", pbsE2ERunCalls); got != "[verify-datastore:backups]" {
		t.Errorf("calls = %s", got)
	}
}

func TestPBSMutation_VerifyRunUsage(t *testing.T) {
	seedPBSMutationTest(t)
	for _, args := range [][]string{
		{"pbs", "verify", "run"}, // neither
		{"pbs", "verify", "run", "v-daily", "--datastore", "backups"}, // both
		{"pbs", "verify", "run", "--datastore"},                       // missing value
		{"pbs", "verify", "run", "a", "b"},                            // extra positional
		{"--yes", "pbs", "verify", "run", "--bogus", "x"},             // unknown flag
	} {
		_, _, err := runPBSCommand(t, args...)
		if err == nil {
			t.Errorf("Run(%v) succeeded, want usage error", args)
			continue
		}
		var exitCode *app.ExitCoder
		if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
			t.Errorf("Run(%v) error = %v, want ExitUsage", args, err)
		}
	}
}

func TestPBSMutation_VerifyRunUnknownJob(t *testing.T) {
	seedPBSMutationTest(t)
	_, _, err := runPBSCommand(t, "--yes", "pbs", "verify", "run", "no-such-job")
	if err == nil {
		t.Fatal("expected not-found error")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitNotFound {
		t.Errorf("error = %v, want ExitNotFound", err)
	}
	if len(pbsE2ERunCalls) != 0 {
		t.Errorf("unknown job must not execute, got calls: %v", pbsE2ERunCalls)
	}
}

func TestPBSMutation_SyncRunNeedsYesAndForce(t *testing.T) {
	seedPBSMutationTest(t)
	_, _, err := runPBSCommand(t, "--yes", "pbs", "sync", "run", "s-offsite")
	if err == nil {
		t.Fatal("expected disruptive gate to require --force")
	}
	if !stderrors.Is(err, safety.ErrAuthorizationRequired) {
		t.Errorf("error = %v, want ErrAuthorizationRequired", err)
	}
	if len(pbsE2ERunCalls) != 0 {
		t.Errorf("refused operation must not execute, got calls: %v", pbsE2ERunCalls)
	}

	_, _, err = runPBSCommand(t, "--yes", "--force", "pbs", "sync", "run", "s-offsite")
	if err != nil {
		t.Fatalf("sync run with --yes --force: %v", err)
	}
	if got := fmt.Sprintf("%v", pbsE2ERunCalls); got != "[sync-job:s-offsite]" {
		t.Errorf("calls = %s", got)
	}
}

func TestPBSMutation_PruneRunTypeConfirm(t *testing.T) {
	seedPBSMutationTest(t)

	// Correct typed confirmation.
	withStdin(t, "p-daily\n")
	stdout, _, err := runPBSCommand(t, "--output", "json", "--yes", "--force", "pbs", "prune", "run", "p-daily")
	if err != nil {
		t.Fatalf("prune run with type confirm: %v", err)
	}
	if !strings.Contains(stdout, "\"destructive\"") {
		t.Errorf("result should carry destructive safety tier:\n%s", stdout)
	}
	if got := fmt.Sprintf("%v", pbsE2ERunCalls); got != "[prune-job:p-daily]" {
		t.Errorf("calls = %s", got)
	}
}

func TestPBSMutation_PruneRunTypeConfirmMismatch(t *testing.T) {
	seedPBSMutationTest(t)
	withStdin(t, "wrong-name\n")
	_, _, err := runPBSCommand(t, "--yes", "--force", "pbs", "prune", "run", "p-daily")
	if err == nil {
		t.Fatal("expected type-confirm mismatch to refuse")
	}
	if !stderrors.Is(err, safety.ErrTypeConfirmMismatch) {
		t.Errorf("error = %v, want ErrTypeConfirmMismatch", err)
	}
	if len(pbsE2ERunCalls) != 0 {
		t.Errorf("mismatch must not execute, got calls: %v", pbsE2ERunCalls)
	}
}

func TestPBSMutation_NonInteractiveFailsClosed(t *testing.T) {
	seedPBSMutationTest(t)
	commands := [][]string{
		{"--non-interactive", "pbs", "verify", "run", "v-daily"},
		{"--non-interactive", "--yes", "pbs", "sync", "run", "s-offsite"},
		{"--non-interactive", "--yes", "--force", "pbs", "prune", "run", "p-daily"},
		{"--non-interactive", "pbs", "garbage-collection", "run", "backups"},
	}
	for _, args := range commands {
		_, _, err := runPBSCommand(t, args...)
		if err == nil {
			t.Errorf("Run(%v) succeeded, want fail-closed error", args)
			continue
		}
		var exitCode *app.ExitCoder
		if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
			t.Errorf("Run(%v) error = %v, want ExitUsage (fail closed)", args, err)
		}
	}
	if len(pbsE2ERunCalls) != 0 {
		t.Errorf("non-interactive refusals must not execute, got calls: %v", pbsE2ERunCalls)
	}
}

func TestPBSMutation_GCRun(t *testing.T) {
	seedPBSMutationTest(t)
	stdout, _, err := runPBSCommand(t, "--output", "json", "--yes", "--force", "pbs", "garbage-collection", "run", "backups")
	if err != nil {
		t.Fatalf("gc run: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("invalid OperationResult JSON: %v", jsonErr)
	}
	if result["safety"] != "disruptive" || result["submitted"] != true {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestPBSMutation_ConflictBlocksExecution(t *testing.T) {
	seedPBSMutationTest(t)
	pbsE2EConflictMode = true

	commands := [][]string{
		{"--yes", "--force", "pbs", "garbage-collection", "run", "backups"},
		{"--yes", "pbs", "verify", "run", "v-daily"},
		{"--yes", "--force", "pbs", "sync", "run", "s-offsite"},
	}
	for _, args := range commands {
		_, _, err := runPBSCommand(t, args...)
		if err == nil {
			t.Errorf("Run(%v) succeeded, want conflict refusal", args)
			continue
		}
		var exitCode *app.ExitCoder
		if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitConflict {
			t.Errorf("Run(%v) error = %v, want ExitConflict", args, err)
		}
	}
	if len(pbsE2ERunCalls) != 0 {
		t.Errorf("conflicting operations must not execute, got calls: %v", pbsE2ERunCalls)
	}
}

func TestPBSMutation_ConflictDifferentStoreDoesNotBlock(t *testing.T) {
	seedPBSMutationTest(t)
	pbsE2EConflictMode = true
	// The conflicting task runs on "backups"; GC on another store proceeds.
	_, _, err := runPBSCommand(t, "--yes", "--force", "pbs", "garbage-collection", "run", "otherstore")
	if err != nil {
		t.Fatalf("gc run on unrelated store: %v", err)
	}
	if got := fmt.Sprintf("%v", pbsE2ERunCalls); got != "[gc:otherstore]" {
		t.Errorf("calls = %s", got)
	}
}

func TestPBSMutation_WaitPollsTask(t *testing.T) {
	seedPBSMutationTest(t)
	stdout, _, err := runPBSCommand(t, "--output", "json", "--yes", "--force", "--wait", "pbs", "garbage-collection", "run", "backups")
	if err != nil {
		t.Fatalf("gc run --wait: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("invalid OperationResult JSON: %v", jsonErr)
	}
	if result["waited"] != true || result["success"] != true || result["status"] != "OK" {
		t.Errorf("unexpected wait result: %v", result)
	}
}

func TestPBSMutation_UnsupportedOnPVEProfile(t *testing.T) {
	seedPBSMutationTest(t)
	_, _, err := runPBSCommand(t, "--profile", "pve", "--yes", "pbs", "verify", "run", "v-daily")
	if err == nil {
		t.Fatal("expected unsupported-capability error")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUnsupportedCap {
		t.Errorf("error = %v, want ExitUnsupportedCap", err)
	}
}

// TestPBSMutation_RegistryDeclarations pins the operation-registry contract
// for the guarded mutations.
func TestPBSMutation_RegistryDeclarations(t *testing.T) {
	tests := []struct {
		path        string
		tier        safety.Tier
		typeConfirm bool
	}{
		{"pbs verify run", safety.TierReversible, false},
		{"pbs sync run", safety.TierDisruptive, false},
		{"pbs prune run", safety.TierDestructive, true},
		{"pbs garbage-collection run", safety.TierDisruptive, false},
	}
	for _, tt := range tests {
		op := LookupOperation(tt.path)
		if op == nil {
			t.Errorf("missing registry entry %q", tt.path)
			continue
		}
		if op.Inspection {
			t.Errorf("%q must be a mutation", tt.path)
		}
		if op.SafetyTier != tt.tier {
			t.Errorf("%q tier = %v, want %v", tt.path, op.SafetyTier, tt.tier)
		}
		if op.RequiresTypeConfirm != tt.typeConfirm {
			t.Errorf("%q RequiresTypeConfirm = %v, want %v", tt.path, op.RequiresTypeConfirm, tt.typeConfirm)
		}
		if !op.Waitable || !op.ProducesUPID || !op.UsesOperationResult {
			t.Errorf("%q must be waitable, produce a UPID, and use OperationResult", tt.path)
		}
	}
}
