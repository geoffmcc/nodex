package cli

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/agent"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/output"
)

func TestOperationDiscoveryCoversRegistryAndAliases(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--output", "json", "operation", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("operation list: %v (%s)", err, stderr.String())
	}
	var listed []OperationContract
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("decode operation list: %v\n%s", err, stdout.String())
	}
	if len(listed) != len(Operations()) {
		t.Fatalf("operation list count = %d, registry count = %d", len(listed), len(Operations()))
	}
	seen := make(map[string]bool, len(listed))
	for _, item := range listed {
		if item.Path == "" || item.ID != strings.ReplaceAll(item.Path, " ", ".") {
			t.Errorf("invalid compact operation item: %+v", item)
		}
		if seen[item.Path] {
			t.Errorf("duplicate discovered operation path %q", item.Path)
		}
		seen[item.Path] = true
		if item.InputSchema != nil || len(item.Flags) != 0 || len(item.Arguments) != 0 {
			t.Errorf("compact list contains detailed schema for %q", item.Path)
		}
	}
	if errs := ValidateRegistry(); len(errs) > 0 {
		t.Fatalf("command/operation registry has errors: %v", errs)
	}
	for _, alias := range []string{"firewall list", "firewall rules", "firewall security-group", "firewall groups", "firewall security-group delete"} {
		if op := LookupOperation(alias); op == nil {
			t.Errorf("alias %q is not discoverable", alias)
		}
	}
	securityGroupDelete := LookupOperation("firewall security-group delete")
	if securityGroupDelete == nil || securityGroupDelete.Path != "firewall group delete" || !containsString(OperationAliases(*securityGroupDelete), "firewall security-group delete") {
		t.Fatalf("nested dispatch alias was not projected to its canonical mutation: %+v", securityGroupDelete)
	}
	if meta, _, _, _, ok := resolveOperationInvocation([]string{"firewall", "security-group", "delete", "group-a"}); !ok || meta.Path != "firewall group delete" || meta.SafetyTier.String() != "destructive" {
		t.Fatalf("nested alias did not resolve to the destructive leaf: meta=%+v ok=%v", meta, ok)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"--output", "json", "operation", "describe", "firewall security-group delete"}, &stdout, &stderr); err != nil {
		t.Fatalf("describe nested alias: %v", err)
	}
	var aliasDescription OperationContract
	if err := json.Unmarshal(stdout.Bytes(), &aliasDescription); err != nil || aliasDescription.Path != "firewall group delete" {
		t.Fatalf("nested alias describe output is not canonical: %+v err=%v", aliasDescription, err)
	}
}

func TestAgentOperationContractsStayInParityWithExistingCLIGrammar(t *testing.T) {
	for _, op := range Operations() {
		if _, dispatch := knownDispatchCommands[op.Path]; dispatch {
			continue
		}
		entry, ok := leafHelp[op.Path]
		if !ok {
			t.Errorf("operation %q has no CLI argument grammar to expose", op.Path)
			continue
		}
		contract := describeOperation(op, true)
		if contract.ArgumentSyntax != strings.TrimSpace(entry.usage) {
			t.Errorf("%q discovered argument syntax %q differs from CLI help %q", op.Path, contract.ArgumentSyntax, entry.usage)
		}
		flags := handlerFlags[op.Path]
		commandPath := strings.Fields(op.Path)
		region := []string(nil)
		if isDispatchOp(op.Path) {
			parent := ""
			for prefixLen := len(commandPath) - 1; prefixLen > 0; prefixLen-- {
				candidate := strings.Join(commandPath[:prefixLen], " ")
				canonical := candidate
				if meta := LookupOperation(candidate); meta != nil {
					canonical = meta.Path
				}
				if _, ok := knownDispatchCommands[canonical]; ok {
					parent = candidate
					break
				}
			}
			commandPath = strings.Fields(parent)
			region = strings.Fields(strings.TrimPrefix(op.Path, parent+" "))
		}
		owned := commandFlagSet(commandPath, region)
		for _, name := range flags.exact {
			if !owned.owns(strings.TrimPrefix(name, "--"), false) {
				t.Errorf("%q accepted flag %s is absent from the global scanner's handler definition", op.Path, name)
			}
			if !hasDiscoveredFlag(contract.Flags, name) {
				t.Errorf("%q accepted flag %s is absent from operation discovery", op.Path, name)
			}
		}
		for _, name := range flags.params {
			if !owned.owns(name, true) {
				t.Errorf("%q accepted parameter --%s=<value> is absent from the global scanner's handler definition", op.Path, name)
			}
			if !hasDiscoveredFlag(contract.Flags, "--"+name) {
				t.Errorf("%q accepted parameter --%s=<value> is absent from operation discovery", op.Path, name)
			}
		}
		resolved, _, _, _, ok := resolveOperationInvocation(strings.Fields(op.Path))
		if !ok || resolved.Path != op.Path {
			t.Errorf("agent operation resolver does not reach canonical leaf %q: resolved=%+v ok=%v", op.Path, resolved, ok)
		}
	}
	_, _, remaining, err := parseGlobal([]string{"firewall", "ipset", "entry", "add", "set-a", "192.0.2.0/24", "--comment=fixture"})
	if err != nil || !containsString(remaining, "--comment=fixture") {
		t.Fatalf("nested multi-token handler flag was not accepted by the existing parser: remaining=%v err=%v", remaining, err)
	}
}

func hasDiscoveredFlag(flags []AgentFlag, name string) bool {
	for _, flag := range flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}

func TestOperationDescribeExposesInputAndSafetySchema(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--output=json", "operation", "describe", "pbs verify run"}, &stdout, &stderr); err != nil {
		t.Fatalf("operation describe: %v (%s)", err, stderr.String())
	}
	var described OperationContract
	if err := json.Unmarshal(stdout.Bytes(), &described); err != nil {
		t.Fatalf("decode describe output: %v\n%s", err, stdout.String())
	}
	if described.Path != "pbs verify run" || described.ProviderInterface != "PBSVerifyRunner" || !described.AgentSupported {
		t.Fatalf("unexpected operation contract: %+v", described)
	}
	if described.InputSchema == nil || described.ArgumentSyntax == "" || len(described.Flags) == 0 {
		t.Fatalf("detailed operation schema is incomplete: %+v", described)
	}
	assertJSONSchemaRepresentative(t, filepath.Join("..", "..", "docs", "agent", "schemas", "operation-contract-v1.schema.json"), stdout.Bytes())
	for _, path := range []string{"vm console", "profile set-credentials", "access user create", "cluster join", "environment health", "maintenance inventory", "maintenance apply", "certification run"} {
		meta := LookupOperation(path)
		if meta == nil {
			t.Fatalf("missing operation %q", path)
		}
		contract := describeOperation(*meta, true)
		if contract.AgentSupported || contract.AgentUnsupportedWhy == "" {
			t.Errorf("excluded operation %q has no explicit reason: %+v", path, contract)
		}
	}
	vmUpdate := describeOperation(*LookupOperation("vm update"), true)
	if len(vmUpdate.Arguments) != 2 || vmUpdate.Arguments[0].Name != "target" || vmUpdate.Arguments[1].Name != "key=value" {
		t.Errorf("VM update positional schema does not match its help grammar: %+v", vmUpdate.Arguments)
	}
	pbsVerify := describeOperation(*LookupOperation("pbs verify run"), true)
	if len(pbsVerify.Arguments) != 1 || pbsVerify.Arguments[0].Name != "job-id" || !strings.Contains(strings.Join(pbsVerify.Constraints, ";"), "exactly one") {
		t.Errorf("PBS verify alternatives are not described accurately: args=%+v constraints=%v", pbsVerify.Arguments, pbsVerify.Constraints)
	}
	for _, flag := range pbsVerify.Flags {
		if flag.Name == "--datastore" && flag.Required {
			t.Error("PBS verify datastore must be conditional on the absent job ID")
		}
	}
	if alternatives, ok := pbsVerify.InputSchema["oneOf"].([]any); !ok || len(alternatives) != 2 {
		t.Errorf("PBS verify input schema lacks its job-id/datastore exclusive choices: %#v", pbsVerify.InputSchema["oneOf"])
	}
	inputSchemaDoc := schemaDocument{path: "", root: pbsVerify.InputSchema}
	for name, payload := range map[string]any{
		"job-id":    map[string]any{"arguments": []any{"verify-daily"}, "flags": map[string]any{"--agent": true, "--profile": "backup", "--yes": true}},
		"datastore": map[string]any{"arguments": []any{}, "flags": map[string]any{"--agent": true, "--profile": "backup", "--yes": true, "--datastore": "store1"}},
	} {
		if err := validateSchemaValue(payload, pbsVerify.InputSchema, inputSchemaDoc, "$", 0); err != nil {
			t.Errorf("valid PBS verify %s input rejected by discovered schema: %v", name, err)
		}
	}
	invalidPBSInput := map[string]any{"arguments": []any{"verify-daily"}, "flags": map[string]any{"--agent": true, "--profile": "backup", "--yes": true, "--datastore": "store1"}}
	if err := validateSchemaValue(invalidPBSInput, pbsVerify.InputSchema, inputSchemaDoc, "$", 0); err == nil {
		t.Error("PBS verify schema allowed both mutually exclusive target forms")
	}
	firewall := describeOperation(*LookupOperation("firewall rule create"), true)
	actionFlag, typeFlag := false, false
	for _, flag := range firewall.Flags {
		if flag.Name == "--action" {
			actionFlag = flag.Required && strings.Join(flag.Choices, ",") == "accept,deny,reject"
		}
		if flag.Name == "--type" {
			typeFlag = flag.Required && strings.Join(flag.Choices, ",") == "in,out,group"
		}
	}
	if !actionFlag || !typeFlag {
		t.Errorf("firewall flag schema misses required enum fields: %+v", firewall.Flags)
	}
	migrate := targetContext(*LookupOperation("vm migrate"), []string{"pve1/100", "pve2"}, nil, "lab", "proxmox", "https://pve.example:8006", config.Profile{})
	if migrate.Node != "pve1" || migrate.ResourceID != "100" || len(migrate.RelatedTargets) != 1 || migrate.RelatedTargets[0].ResourceType != "node" || migrate.RelatedTargets[0].ResourceID != "pve2" {
		t.Errorf("migration context omits one side of its exact target set: %+v", migrate)
	}
}

func TestRemoteProviderMutationsHaveAgentReceiptCoverageOrSpecificExclusion(t *testing.T) {
	excluded := map[string]bool{
		"access user create": true, // collects a password interactively
		"cluster join":       true, // performs preflight only and refuses execution without peer credentials
		"container console":  true, // requires an interactive terminal
		"storage download":   true, // writes an arbitrary local file
		"vm console":         true, // requires an interactive terminal
	}
	for _, op := range Operations() {
		if op.Inspection || op.CapabilityInterface == "" {
			continue
		}
		contract := describeOperation(op, true)
		if excluded[op.Path] {
			if contract.AgentSupported || contract.AgentUnsupportedWhy == "" {
				t.Errorf("%q must have a specific agent-mode exclusion", op.Path)
			}
			continue
		}
		if !contract.AgentSupported {
			t.Errorf("provider mutation %q lacks agent receipt coverage: %s", op.Path, contract.AgentUnsupportedWhy)
		}
	}
	for _, path := range []string{
		"vm start", "vm update", "vm snapshot create", "vm delete", "container create", "storage upload",
		"backup create", "backup job create", "firewall rule create", "sdn zone create", "access acl add",
		"ceph osd create", "replication create", "pbs verify run", "pbs sync run", "pbs prune run", "pbs garbage-collection run",
	} {
		meta := LookupOperation(path)
		if meta == nil || !describeOperation(*meta, true).AgentSupported {
			t.Errorf("expected operation %q to be supported with durable agent receipts", path)
		}
	}
}

func TestAgentContractIsOfflineStructuredOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"agent", "contract"}, &stdout, &stderr); err != nil {
		t.Fatalf("agent contract: %v", err)
	}
	var contract AgentContract
	if err := json.Unmarshal(stdout.Bytes(), &contract); err != nil {
		t.Fatalf("invalid contract JSON: %v", err)
	}
	if contract.ContractVersion == "" || contract.NodeXVersion == "" || len(contract.Features) == 0 || contract.OutcomeSemantics["submission"] == "" {
		t.Fatalf("contract is incomplete: %+v", contract)
	}
	if !strings.Contains(contract.TrustBoundary, "not an isolation boundary") {
		t.Fatalf("trust boundary missing: %q", contract.TrustBoundary)
	}
	assertJSONSchemaRepresentative(t, filepath.Join("..", "..", "docs", "agent", "schemas", "agent-contract-v1.schema.json"), stdout.Bytes())
}

func TestAgentMonitorCheckBindsOneSnapshotTarget(t *testing.T) {
	isolateConfigAndHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fixture ok"))
	}))
	defer server.Close()
	cfg := config.DefaultConfig()
	cfg.Version = config.CurrentSchemaVersion
	cfg.Monitoring = &config.Monitoring{Targets: map[string]config.MonitorTarget{
		"fixture": {Type: "http", Address: server.URL, ExpectedStatus: http.StatusOK},
	}}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--agent", "--request-id", "req_monitor", "monitor", "check", "--target", "fixture"}, &stdout, &stderr); err != nil {
		t.Fatalf("agent monitor check: %v (%s)\n%s", err, stderr.String(), stdout.String())
	}
	result := decodeAgentResult(t, stdout.Bytes())
	if result.Context.ResourceID != "fixture" || result.Context.ResourceType != "monitor:http" || result.Context.Endpoint != normalizeMonitorAddress(server.URL) || result.Execution != agent.ExecutionSucceeded {
		t.Fatalf("monitor result is not bound to the selected config target: %+v", result)
	}
	if result.Observation == nil || result.Observation.Completeness != "unknown" {
		t.Fatalf("monitor completeness was overclaimed: %+v", result.Observation)
	}
	stdout.Reset()
	err = Run(context.Background(), []string{"--agent", "monitor", "check"}, &stdout, &stderr)
	if err == nil || decodeAgentResult(t, stdout.Bytes()).Error.Code != "TARGET_REQUIRED" {
		t.Fatalf("unbounded monitor fan-out was not refused: err=%v output=%s", err, stdout.String())
	}
}

func TestAgentMapsOperationSpecificVerificationWithoutChangingLegacyResult(t *testing.T) {
	changed := true
	now := "2026-01-02T03:04:05Z"
	base := agent.Result{
		SchemaVersion: agent.ResultSchemaVersion, RequestID: "req_verified_update", ReceiptID: "req_verified_update",
		Operation: "container os-update", Context: agent.ExecutionContext{Profile: "pve", Provider: "proxmox", Endpoint: "https://pve.example:8006", Node: "pve1", ResourceID: "200"},
		Submission: agent.SubmissionUnknown, Execution: agent.ExecutionUnknown, Verification: agent.VerificationUnsupported,
		Retry: agent.RetryReconcileFirst, StartedAt: now, ObservedAt: now,
	}
	legacy := output.NewOperationResult("container os-update", "proxmox", "pve")
	legacy.Submitted, legacy.Waited, legacy.Success, legacy.Status, legacy.Changed = true, true, true, "verified", &changed
	raw, err := output.MarshalJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	result := mutationOutcome(base, OperationMeta{Path: "container os-update"}, raw, nil)
	if result.Submission != agent.SubmissionAccepted || result.Execution != agent.ExecutionSucceeded || result.Verification != agent.VerificationPassed || result.Changed == nil || !*result.Changed {
		t.Fatalf("operation-specific verified result not preserved in agent envelope: %+v", result)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("mapped agent result invalid: %v", err)
	}
	unchanged := false
	legacy = output.NewOperationResult("container os-update", "proxmox", "pve")
	legacy.Success, legacy.Status, legacy.Changed = true, "no-updates", &unchanged
	raw, err = output.MarshalJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	result = mutationOutcome(base, OperationMeta{Path: "container os-update"}, raw, nil)
	if result.Submission != agent.SubmissionNotAttempted || result.Execution != agent.ExecutionSucceeded || result.Verification != agent.VerificationPassed || result.Changed == nil || *result.Changed {
		t.Fatalf("no-update inspection was not reported as an unchanged verified result: %+v", result)
	}
}

func TestAgentDoesNotInventCompletionForUnstructuredMutationHandler(t *testing.T) {
	now := "2026-01-02T03:04:05Z"
	base := agent.Result{
		SchemaVersion: agent.ResultSchemaVersion, RequestID: "req_sync_unknown", ReceiptID: "req_sync_unknown",
		Operation: "firewall alias create", Context: agent.ExecutionContext{Profile: "lab", Provider: "proxmox", Endpoint: "https://pve.example:8006", EndpointIdentity: "proxmox@https://pve.example:8006", ResourceType: "firewall", ResourceID: "alias-a"},
		Submission: agent.SubmissionUnknown, Execution: agent.ExecutionUnknown, Verification: agent.VerificationUnsupported,
		Retry: agent.RetryReconcileFirst, StartedAt: now, ObservedAt: now,
	}
	result := mutationOutcome(base, OperationMeta{Path: "firewall alias create"}, nil, nil)
	if result.Submission != agent.SubmissionAccepted || result.Execution != agent.ExecutionUnknown || result.Verification != agent.VerificationUnsupported || result.Changed != nil || result.Retry != agent.RetryReconcileFirst {
		t.Fatalf("unstructured provider success was overstated: %+v", result)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("unknown outcome result invalid: %v", err)
	}
}

func TestAgentPollingTimeoutAndCancellationRemainUnknown(t *testing.T) {
	for _, exit := range []int{app.ExitTimeout, app.ExitCancellation, app.ExitInterrupted, app.ExitSigterm, app.ExitAmbiguousOutcome} {
		t.Run(fmt.Sprintf("exit_%d", exit), func(t *testing.T) {
			now := "2026-01-02T03:04:05Z"
			base := agent.Result{SchemaVersion: agent.ResultSchemaVersion, RequestID: "req_wait_unknown", ReceiptID: "req_wait_unknown", Operation: "vm start", Submission: agent.SubmissionUnknown, Execution: agent.ExecutionUnknown, Verification: agent.VerificationUnsupported, Retry: agent.RetryReconcileFirst, StartedAt: now, ObservedAt: now}
			legacy := output.NewOperationResult("vm start", "proxmox", "lab")
			legacy.Submitted, legacy.Waited, legacy.Success = true, true, false
			legacy.UPID = "UPID:pve1:123"
			legacy.Error = &output.ResultError{Class: "timeout", Exit: exit, Detail: "observation stopped"}
			raw, err := output.MarshalJSON(legacy)
			if err != nil {
				t.Fatal(err)
			}
			result := mutationOutcome(base, OperationMeta{Path: "vm start", ProducesUPID: true}, raw, nil)
			if result.Submission != agent.SubmissionAccepted || result.Execution != agent.ExecutionUnknown || result.Retry != agent.RetryReconcileFirst {
				t.Fatalf("poll interruption was treated as remote cancellation/failure: %+v", result)
			}
		})
	}
}

func TestAgentDistinguishesDefinitiveProviderRejectionFromServerError(t *testing.T) {
	for _, test := range []struct {
		status     int
		submission agent.Submission
		execution  agent.Execution
		retry      agent.Retry
	}{{400, agent.SubmissionRejected, agent.ExecutionNotStarted, agent.RetryDoNotAutomatic}, {500, agent.SubmissionUnknown, agent.ExecutionUnknown, agent.RetryReconcileFirst}} {
		t.Run(fmt.Sprintf("status_%d", test.status), func(t *testing.T) {
			now := "2026-01-02T03:04:05Z"
			base := agent.Result{SchemaVersion: agent.ResultSchemaVersion, RequestID: "req_provider_status", ReceiptID: "req_provider_status", Operation: "vm start", Submission: agent.SubmissionUnknown, Execution: agent.ExecutionUnknown, Verification: agent.VerificationUnsupported, Retry: agent.RetryReconcileFirst, StartedAt: now, ObservedAt: now}
			err := app.NewProviderError(test.status, "provider response", nil)
			result := mutationOutcome(base, OperationMeta{Path: "vm start", ProducesUPID: true}, nil, err)
			if result.Submission != test.submission || result.Execution != test.execution || result.Retry != test.retry {
				t.Fatalf("HTTP %d outcome classification incorrect: %+v", test.status, result)
			}
		})
	}
}

func TestAgentMutationReceiptDeduplicatesAndBindsTarget(t *testing.T) {
	seedPBSMutationTest(t)
	args := []string{"--agent", "--profile", "pbs-e2e", "--request-id", "req_verify_1", "--yes", "pbs", "verify", "run", "v-daily"}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("agent PBS verify: %v (%s)\n%s", err, stderr.String(), stdout.String())
	}
	result := decodeAgentResult(t, stdout.Bytes())
	if result.Operation != "pbs verify run" || result.Submission != agent.SubmissionAccepted || result.Execution != agent.ExecutionRunning || result.Verification != agent.VerificationUnsupported {
		t.Fatalf("unexpected mutation outcome: %+v", result)
	}
	if result.TaskID != pbsE2EMutationUPID || result.ReceiptID != "req_verify_1" || result.Context.Profile != "pbs-e2e" || result.Context.Endpoint != "https://pbs-e2e.example.invalid:443" || result.Context.ResourceID != "v-daily" {
		t.Fatalf("receipt target/task binding is incomplete: %+v", result)
	}
	assertJSONSchemaRepresentative(t, filepath.Join("..", "..", "docs", "agent", "schemas", "agent-result-v1.schema.json"), stdout.Bytes())
	if len(pbsE2ERunCalls) != 1 {
		t.Fatalf("first call count = %d, want 1", len(pbsE2ERunCalls))
	}

	stdout.Reset()
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("duplicate agent request: %v", err)
	}
	duplicate := decodeAgentResult(t, stdout.Bytes())
	if len(pbsE2ERunCalls) != 1 || len(duplicate.Warnings) == 0 || duplicate.Warnings[0].Code != "DUPLICATE_REQUEST_ID" {
		t.Fatalf("duplicate request was not safely deduplicated: calls=%v result=%+v", pbsE2ERunCalls, duplicate)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"agent", "receipt", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("receipt list: %v", err)
	}
	var listed receiptListResult
	if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
		t.Fatalf("decode receipt list: %v\n%s", err, stdout.String())
	}
	if len(listed.Items) != 1 || listed.Truncated || listed.Items[0].RequestID != "req_verify_1" {
		t.Fatalf("receipt list summary incorrect: %+v", listed)
	}

	changedTarget := append([]string(nil), args...)
	changedTarget[len(changedTarget)-1] = "other-job"
	stdout.Reset()
	err := Run(context.Background(), changedTarget, &stdout, &stderr)
	if err == nil || app.ExitCodeFromError(err) != app.ExitConflict {
		t.Fatalf("reused request ID with a different target returned %v", err)
	}
	conflict := decodeAgentResult(t, stdout.Bytes())
	if conflict.Error == nil || conflict.Error.Code != "REQUEST_ID_CONFLICT" || len(pbsE2ERunCalls) != 1 {
		t.Fatalf("request conflict was not explicit and non-mutating: %+v calls=%v", conflict, pbsE2ERunCalls)
	}
}

func TestAgentMutationRequiresProfileAndExistingConfirmation(t *testing.T) {
	seedPBSMutationTest(t)
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--agent", "--request-id", "req_no_profile", "--yes", "pbs", "verify", "run", "v-daily"}, &stdout, &stderr)
	if err == nil || app.ExitCodeFromError(err) != app.ExitConfig {
		t.Fatalf("missing explicit profile error = %v", err)
	}
	if got := decodeAgentResult(t, stdout.Bytes()); got.Error == nil || got.Error.Code != "PROFILE_REQUIRED" {
		t.Fatalf("missing-profile response = %+v", got)
	}

	stdout.Reset()
	err = Run(context.Background(), []string{"--agent", "--profile", "pbs-e2e", "--request-id", "req_needs_yes", "pbs", "verify", "run", "v-daily"}, &stdout, &stderr)
	if err == nil || len(pbsE2ERunCalls) != 0 {
		t.Fatalf("confirmationless request should be refused without provider mutation; err=%v calls=%v", err, pbsE2ERunCalls)
	}
	refused := decodeAgentResult(t, stdout.Bytes())
	if refused.Submission != agent.SubmissionNotAttempted || refused.Execution != agent.ExecutionNotStarted || refused.Retry != agent.RetrySafe {
		t.Fatalf("confirmation refusal outcome is not explicit: %+v", refused)
	}
	store, _ := agent.DefaultStore()
	receipt, loadErr := store.Get("req_needs_yes")
	if loadErr != nil || receipt.Submission != agent.SubmissionNotAttempted {
		t.Fatalf("pre-submit failure receipt missing or inaccurate: receipt=%+v err=%v", receipt, loadErr)
	}
}

func TestAgentRejectsConflictingGlobalsAndRedactsRejectedSecretOptions(t *testing.T) {
	seedPBSMutationTest(t)
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--agent", "--output", "yaml", "operation", "list"}, &stdout, &stderr)
	if err == nil || decodeAgentResult(t, stdout.Bytes()).Error.Code != "CONFLICTING_OUTPUT" {
		t.Fatalf("conflicting agent output format was not rejected: err=%v output=%s", err, stdout.String())
	}
	stdout.Reset()
	err = Run(context.Background(), []string{"--agent", "--non-interactive=false", "version"}, &stdout, &stderr)
	if err == nil || decodeAgentResult(t, stdout.Bytes()).Error.Code != "INTERACTIVE_MODE_CONFLICT" {
		t.Fatalf("interactive agent request was not rejected: err=%v output=%s", err, stdout.String())
	}
	stdout.Reset()
	err = Run(context.Background(), []string{"--agent", "--profile", "pbs-e2e", "--profile", "pve", "pbs", "verify", "run", "v-daily"}, &stdout, &stderr)
	if err == nil || app.ExitCodeFromError(err) != app.ExitConflict {
		t.Fatalf("conflicting profile options returned %v", err)
	}
	if result := decodeAgentResult(t, stdout.Bytes()); result.Error == nil || result.Error.Code != "CONFLICTING_OPTIONS" || len(pbsE2ERunCalls) != 0 {
		t.Fatalf("conflicting execution context was not refused: %+v calls=%v", result, pbsE2ERunCalls)
	}

	stdout.Reset()
	const secret = "agent-secret-canary-789"
	args := []string{"--agent", "--request-id", "req_secret_flag", "--password=" + secret, "--profile", "pbs-e2e", "pbs", "verify", "run", "v-daily"}
	err = Run(context.Background(), args, &stdout, &stderr)
	if err == nil {
		t.Fatal("secret-bearing unsupported option should be rejected")
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
		t.Fatalf("rejected secret was exposed: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if len(pbsE2ERunCalls) != 0 {
		t.Fatalf("secret-bearing request reached provider: %v", pbsE2ERunCalls)
	}

	stdout.Reset()
	err = Run(context.Background(), []string{"--agent", "monitor", "check", "--target", "first", "--target", "second"}, &stdout, &stderr)
	if err == nil || app.ExitCodeFromError(err) != app.ExitConflict || decodeAgentResult(t, stdout.Bytes()).Error.Code != "CONFLICTING_OPTIONS" {
		t.Fatalf("conflicting operation targets were not rejected: err=%v output=%s", err, stdout.String())
	}
}

func TestAgentWriteAheadReceiptFailurePreventsSubmission(t *testing.T) {
	seedPBSMutationTest(t)
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(dir, "agent-receipts")
	if err := os.Symlink(t.TempDir(), receiptPath); err != nil {
		t.Skipf("symlink setup unavailable: %v", err)
	}
	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), []string{"--agent", "--profile", "pbs-e2e", "--request-id", "req_receipt_write_block", "--yes", "pbs", "verify", "run", "v-daily"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("unsafe receipt directory must block mutation")
	}
	result := decodeAgentResult(t, stdout.Bytes())
	if result.Error == nil || result.Error.Code != "FINGERPRINT_KEY_FAILED" || result.Submission != agent.SubmissionNotAttempted || len(pbsE2ERunCalls) != 0 {
		t.Fatalf("receipt write failure did not fail closed: result=%+v calls=%v", result, pbsE2ERunCalls)
	}
}

func TestAgentRejectsPasswordLikeKeyValueBeforeFingerprintOrReceipt(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	const secret = "low-entropy-secret-test"
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--agent", "--profile", "e2e", "--request-id", "req_vm_secret", "--yes", "vm", "update", "e2e-node/100", "cipassword=" + secret}, &stdout, &stderr)
	if err == nil {
		t.Fatal("password-like key=value must be refused in agent mode")
	}
	result := decodeAgentResult(t, stdout.Bytes())
	if result.Error == nil || result.Error.Code != "SECRET_INPUT_UNSUPPORTED" || strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
		t.Fatalf("sensitive value was not safely refused: result=%+v stdout=%q stderr=%q", result, stdout.String(), stderr.String())
	}
	store, _ := agent.DefaultStore()
	if _, err := store.Get("req_vm_secret"); !os.IsNotExist(err) {
		t.Fatalf("secret-bearing request created a durable receipt: %v", err)
	}
}

func TestAgentPVEVMMutationUsesBoundProviderAndWritesReceipt(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--agent", "--profile", "e2e", "--request-id", "req_vm_stop", "--yes", "vm", "stop", "e2e-node/100"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("agent VM stop: %v (%s)\n%s", err, stderr.String(), stdout.String())
	}
	result := decodeAgentResult(t, stdout.Bytes())
	if result.Operation != "vm stop" || result.Submission != agent.SubmissionAccepted || result.Execution != agent.ExecutionRunning || result.Context.Endpoint != "https://e2e.example.invalid:443" || result.Context.Node != "e2e-node" || result.Context.ResourceID != "100" {
		t.Fatalf("unexpected endpoint-bound PVE result: %+v", result)
	}
	store, _ := agent.DefaultStore()
	receipt, err := store.Get("req_vm_stop")
	if err != nil || receipt.Context.Endpoint != result.Context.Endpoint || receipt.TaskID == "" {
		t.Fatalf("PVE receipt missing exact target/task context: %+v err=%v", receipt, err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"--profile", "e2e", "agent", "receipt", "refresh", "req_vm_stop"}, &stdout, &stderr); err != nil {
		t.Fatalf("refresh PVE task and postcondition: %v\n%s", err, stdout.String())
	}
	var refreshed agent.Receipt
	if err := json.Unmarshal(stdout.Bytes(), &refreshed); err != nil {
		t.Fatalf("decode refreshed PVE receipt: %v", err)
	}
	if refreshed.Execution != agent.ExecutionSucceeded || refreshed.Verification != agent.VerificationFailed || refreshed.Changed != nil {
		t.Fatalf("task completion and guest postcondition were not kept distinct: %+v", refreshed.Result)
	}
}

func TestAgentLifecyclePostconditionPassIsSeparateFromTaskSuccess(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	e2eVMStatusOverride = ""
	t.Cleanup(func() { e2eVMStatusOverride = "" })
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--agent", "--profile", "e2e", "--request-id", "req_vm_stop_verified", "--yes", "vm", "stop", "e2e-node/100"}, &stdout, &stderr); err != nil {
		t.Fatalf("agent VM stop: %v (%s)", err, stderr.String())
	}
	submitted := decodeAgentResult(t, stdout.Bytes())
	if submitted.Execution != agent.ExecutionRunning || submitted.Verification != agent.VerificationUnsupported {
		t.Fatalf("task submission claimed an unobserved postcondition: %+v", submitted)
	}
	e2eVMStatusOverride = "stopped"
	stdout.Reset()
	if err := Run(context.Background(), []string{"--profile", "e2e", "agent", "receipt", "refresh", "req_vm_stop_verified"}, &stdout, &stderr); err != nil {
		t.Fatalf("refresh VM task: %v\n%s", err, stdout.String())
	}
	var refreshed agent.Receipt
	if err := json.Unmarshal(stdout.Bytes(), &refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.Execution != agent.ExecutionSucceeded || refreshed.Verification != agent.VerificationPassed || refreshed.Changed != nil {
		t.Fatalf("observed lifecycle postcondition was not separated from causation: %+v", refreshed.Result)
	}
}

func TestAgentLifecycleAlreadyDesiredStateIsVerifiedWithoutSubmission(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--agent", "--profile", "e2e", "--request-id", "req_vm_already_running", "--yes", "vm", "start", "e2e-node/100"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("idempotent VM start: %v (%s)", err, stderr.String())
	}
	result := decodeAgentResult(t, stdout.Bytes())
	if result.Submission != agent.SubmissionNotAttempted || result.Execution != agent.ExecutionSucceeded || result.Verification != agent.VerificationPassed || result.Changed == nil || *result.Changed {
		t.Fatalf("already-desired guest state was misreported: %+v", result)
	}
}

func TestAgentReceiptRefreshRejectsProviderTaskFromWrongNode(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	e2eTaskNodeOverride = "different-node"
	t.Cleanup(func() { e2eTaskNodeOverride = "" })
	var stdout, stderr bytes.Buffer
	args := []string{"--agent", "--profile", "e2e", "--request-id", "req_wrong_task_node", "--yes", "vm", "stop", "e2e-node/100"}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("submit VM stop: %v", err)
	}
	stdout.Reset()
	err := Run(context.Background(), []string{"--profile", "e2e", "agent", "receipt", "refresh", "req_wrong_task_node"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("provider task from a different node must not reconcile the receipt")
	}
	var receipt agent.Receipt
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil {
		t.Fatalf("decode unknown receipt: %v\n%s", err, stdout.String())
	}
	if receipt.Submission != agent.SubmissionUnknown || receipt.Execution != agent.ExecutionUnknown || receipt.Error == nil || receipt.Error.Code != "RECONCILIATION_UNAVAILABLE" {
		t.Fatalf("mismatched task identity was guessed: %+v", receipt.Result)
	}
}

func TestAgentLostMutationResponseRemainsUnknownAndIsNotRetried(t *testing.T) {
	seedPBSMutationTest(t)
	pbsE2ELoseVerifyResponse = true
	args := []string{"--agent", "--profile", "pbs-e2e", "--request-id", "req_lost_response", "--yes", "pbs", "verify", "run", "v-daily"}
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), args, &stdout, &stderr)
	if err == nil {
		t.Fatal("lost provider response must remain a non-success/unknown outcome")
	}
	result := decodeAgentResult(t, stdout.Bytes())
	if result.Submission != agent.SubmissionUnknown || result.Execution != agent.ExecutionUnknown || result.Retry != agent.RetryReconcileFirst || result.TaskID != "" {
		t.Fatalf("lost response was guessed as accepted/completed: %+v", result)
	}
	if len(pbsE2ERunCalls) != 1 {
		t.Fatalf("initial call count = %d, want 1", len(pbsE2ERunCalls))
	}
	stdout.Reset()
	if retryErr := Run(context.Background(), args, &stdout, &stderr); retryErr != nil {
		t.Fatalf("duplicate must return prior ambiguity without execution: %v", retryErr)
	}
	prior := decodeAgentResult(t, stdout.Bytes())
	if prior.Submission != agent.SubmissionUnknown || len(pbsE2ERunCalls) != 1 {
		t.Fatalf("ambiguous receipt was resubmitted: result=%+v calls=%v", prior, pbsE2ERunCalls)
	}
	stdout.Reset()
	reconcileErr := Run(context.Background(), []string{"--agent", "--profile", "pbs-e2e", "agent", "receipt", "reconcile", "req_lost_response"}, &stdout, &stderr)
	if reconcileErr == nil {
		t.Fatal("receipt without an authoritative task ID must remain explicitly unreconciled")
	}
	var reconciled agent.Receipt
	if err := json.Unmarshal(stdout.Bytes(), &reconciled); err != nil || reconciled.Submission != agent.SubmissionUnknown || reconciled.Execution != agent.ExecutionUnknown {
		t.Fatalf("agent-mode reconciliation lost its structured receipt: %+v err=%v", reconciled, err)
	}
	if len(pbsE2ERunCalls) != 1 {
		t.Fatalf("reconciliation submitted another mutation: %v", pbsE2ERunCalls)
	}
}

func TestAgentReceiptRefreshDoesNotFollowChangedProfile(t *testing.T) {
	seedPBSMutationTest(t)
	var stdout, stderr bytes.Buffer
	args := []string{"--agent", "--profile", "pbs-e2e", "--request-id", "req_bound_profile", "--yes", "pbs", "verify", "run", "v-daily"}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("submit: %v", err)
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.ReadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles["pbs-e2e"]
	p.Endpoint = "https://different.example.invalid:8007"
	cfg.Profiles["pbs-e2e"] = p
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	err = Run(context.Background(), []string{"--profile", "pbs-e2e", "agent", "receipt", "refresh", "req_bound_profile", "--output", "json"}, &stdout, &stderr)
	if err == nil || app.ExitCodeFromError(err) != app.ExitConflict {
		t.Fatalf("refresh after endpoint change = %v, want conflict", err)
	}
	if len(pbsE2ERunCalls) != 1 {
		t.Fatalf("refresh resubmitted the mutation: %v", pbsE2ERunCalls)
	}
}

func TestAgentReceiptRefreshReadsTaskButDoesNotClaimPostcondition(t *testing.T) {
	seedPBSMutationTest(t)
	var stdout, stderr bytes.Buffer
	args := []string{"--agent", "--profile", "pbs-e2e", "--request-id", "req_refresh_task", "--yes", "pbs", "verify", "run", "v-daily"}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("submit: %v", err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"--profile", "pbs-e2e", "agent", "receipt", "refresh", "req_refresh_task"}, &stdout, &stderr); err != nil {
		t.Fatalf("refresh: %v (%s)", err, stdout.String())
	}
	var receipt agent.Receipt
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil {
		t.Fatalf("decode refreshed receipt: %v\n%s", err, stdout.String())
	}
	assertJSONSchemaRepresentative(t, filepath.Join("..", "..", "docs", "agent", "schemas", "agent-receipt-v1.schema.json"), stdout.Bytes())
	if receipt.Execution != agent.ExecutionSucceeded || receipt.Verification != agent.VerificationUnsupported || receipt.Changed != nil {
		t.Fatalf("task completion was incorrectly treated as a verified resource change: %+v", receipt.Result)
	}
	if len(pbsE2ERunCalls) != 1 {
		t.Fatalf("read-only reconciliation submitted another operation: %v", pbsE2ERunCalls)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, stderrors.New("stdout unavailable") }

func TestAgentOutputFailureLeavesDeduplicationReceipt(t *testing.T) {
	seedPBSMutationTest(t)
	args := []string{"--agent", "--profile", "pbs-e2e", "--request-id", "req_output_failure", "--yes", "pbs", "verify", "run", "v-daily"}
	if err := Run(context.Background(), args, failedWriter{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected stdout failure")
	}
	store, _ := agent.DefaultStore()
	receipt, err := store.Get("req_output_failure")
	if err != nil || receipt.Submission != agent.SubmissionAccepted || receipt.TaskID != pbsE2EMutationUPID {
		t.Fatalf("mutation receipt not usable after output failure: %+v err=%v", receipt, err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), args, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("retry by request ID should return the existing result: %v", err)
	}
	if len(pbsE2ERunCalls) != 1 {
		t.Fatalf("stdout failure led to a second mutation: %v", pbsE2ERunCalls)
	}
}

func TestAgentReceiptUpdateFailureAfterSubmissionReportsKnownTaskAndNeverRepeats(t *testing.T) {
	seedPBSMutationTest(t)
	requestID := "req_receipt_update_failure"
	configDir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	receiptFile := filepath.Join(configDir, "agent-receipts", requestID+".json")
	pbsE2EAfterVerifyResponse = func() {
		if err := os.Remove(receiptFile); err != nil {
			t.Errorf("inject receipt replacement: remove intent: %v", err)
			return
		}
		if err := os.Mkdir(receiptFile, 0o700); err != nil {
			t.Errorf("inject receipt replacement: create non-file target: %v", err)
		}
	}
	var stdout, stderr bytes.Buffer
	args := []string{"--agent", "--profile", "pbs-e2e", "--request-id", requestID, "--yes", "pbs", "verify", "run", "v-daily"}
	err = Run(context.Background(), args, &stdout, &stderr)
	if err == nil || app.ExitCodeFromError(err) != app.ExitAmbiguousOutcome {
		t.Fatalf("receipt update failure must be explicit and ambiguous, got %v", err)
	}
	result := decodeAgentResult(t, stdout.Bytes())
	if result.Submission != agent.SubmissionAccepted || result.Execution != agent.ExecutionRunning || result.TaskID != pbsE2EMutationUPID || result.Error == nil || result.Error.Code != "RECEIPT_UPDATE_FAILED" || result.Error.Exit != app.ExitAmbiguousOutcome {
		t.Fatalf("post-submission persistence failure hid known provider facts: %+v", result)
	}
	if len(pbsE2ERunCalls) != 1 {
		t.Fatalf("mutation call count = %d, want 1", len(pbsE2ERunCalls))
	}
	stdout.Reset()
	if retryErr := Run(context.Background(), args, &stdout, &stderr); retryErr == nil || len(pbsE2ERunCalls) != 1 {
		t.Fatalf("damaged receipt state allowed an automatic duplicate: err=%v calls=%v", retryErr, pbsE2ERunCalls)
	}
}

func decodeAgentResult(t *testing.T, raw []byte) agent.Result {
	t.Helper()
	var result agent.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode agent result: %v\n%s", err, raw)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("agent result violates its state contract: %v\n%s", err, raw)
	}
	return result
}

func assertJSONSchemaRepresentative(t *testing.T, schemaPath string, raw []byte) {
	t.Helper()
	if err := validateAgainstJSONSchema(schemaPath, raw); err != nil {
		t.Errorf("representative output does not satisfy %s: %v", schemaPath, err)
	}
}
