package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/agent"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/provider"
	"github.com/geoffmcc/nodex/internal/redact"
	"github.com/geoffmcc/nodex/internal/safety"
	"github.com/geoffmcc/nodex/internal/task"
)

type agentConfigKey struct{}

const maxAgentCaptureBytes = 1 << 20

type boundedCapture struct {
	bytes.Buffer
	truncated bool
}

func addAgentWarning(result *agent.Result, warning agent.Warning) {
	for _, existing := range result.Warnings {
		if existing.Code == warning.Code && existing.Message == warning.Message {
			return
		}
	}
	const maxWarnings = 64
	if len(result.Warnings) < maxWarnings {
		result.Warnings = append(result.Warnings, warning)
		return
	}
	result.Warnings[maxWarnings-1] = agent.Warning{Code: "WARNINGS_TRUNCATED", Message: "additional warning details were omitted after the 64-warning limit"}
}

func (b *boundedCapture) Write(p []byte) (int, error) {
	remaining := maxAgentCaptureBytes - b.Len()
	if remaining > 0 {
		write := p
		if len(write) > remaining {
			write = write[:remaining]
		}
		if _, err := b.Buffer.Write(write); err != nil {
			return 0, err
		}
	}
	if len(p) > remaining {
		b.truncated = true
	}
	return len(p), nil
}

func agentConfigFromContext(ctx context.Context) *config.Config {
	cfg, _ := ctx.Value(agentConfigKey{}).(*config.Config)
	return cfg
}

// Run keeps the ordinary CLI path unchanged. --agent wraps that same dispatch
// path with strict JSON framing, explicit profile binding, and a write-ahead
// receipt for supported remote mutations.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	opts, helpPath, remaining, parseErr := parseGlobal(args)
	if !opts.Agent && (parseErr == nil || !containsAgentFlag(args)) {
		return runNormal(ctx, args, stdout, stderr)
	}
	if opts.Agent || containsAgentFlag(args) {
		opts.Agent = true
		return runAgent(ctx, args, opts, helpPath, remaining, parseErr, stdout, stderr)
	}
	// The flag was present but explicitly false. Preserve normal CLI behavior,
	// including its established parse errors.
	if parseErr != nil {
		return app.NewExitError(parseErr, app.ExitUsage)
	}
	return runNormal(ctx, args, stdout, stderr)
}

func containsAgentFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--agent" || arg == "-agent" || arg == "--agent=true" || arg == "-agent=true" {
			return true
		}
	}
	return false
}

func runAgent(ctx context.Context, original []string, opts Options, helpPath, remaining []string, parseErr error, stdout, stderr io.Writer) error {
	requestID := opts.RequestID
	if requestID == "" {
		var err error
		requestID, err = agent.NewRequestID()
		if err != nil {
			return emitAgentFailure(stdout, "", "unknown", agent.RetryDoNotAutomatic, app.ExitGeneral, "REQUEST_ID_FAILED", err)
		}
	}
	if !agent.ValidRequestID(requestID) {
		return emitAgentFailure(stdout, requestID, "unknown", agent.RetrySafe, app.ExitUsage, "INVALID_REQUEST_ID", errors.New("request ID must match [A-Za-z0-9][A-Za-z0-9_-]{0,63}"))
	}
	if parseErr != nil {
		return emitAgentFailure(stdout, requestID, operationFromRemaining(remaining), agent.RetrySafe, app.ExitUsage, "INVALID_ARGUMENTS", parseErr)
	}
	if agentArgsTooLarge(original) {
		return emitAgentFailure(stdout, requestID, operationFromRemaining(remaining), agent.RetrySafe, app.ExitUsage, "REQUEST_TOO_LARGE", errors.New("agent-mode arguments exceed 8192 bytes"))
	}
	if flag, conflict := conflictingGlobalOptions(original); conflict {
		return emitAgentFailure(stdout, requestID, operationFromRemaining(remaining), agent.RetrySafe, app.ExitConflict, "CONFLICTING_OPTIONS", fmt.Errorf("global option %s was supplied with conflicting values", flag))
	}
	if opts.outputSpecified && opts.Output != output.FormatJSON {
		return emitAgentFailure(stdout, requestID, operationFromRemaining(remaining), agent.RetrySafe, app.ExitUsage, "CONFLICTING_OUTPUT", errors.New("--agent requires --output json; table and YAML output are incompatible"))
	}
	if opts.nonInteractiveSpecified && !opts.NonInteractive {
		return emitAgentFailure(stdout, requestID, operationFromRemaining(remaining), agent.RetrySafe, app.ExitUsage, "INTERACTIVE_MODE_CONFLICT", errors.New("--agent cannot be combined with --non-interactive=false"))
	}
	if opts.All || opts.PasswordStdin {
		return emitAgentFailure(stdout, requestID, operationFromRemaining(remaining), agent.RetrySafe, app.ExitUsage, "INCOMPATIBLE_OPTION", errors.New("--agent does not support --all or --password-stdin"))
	}
	if len(remaining) == 0 && len(helpPath) == 0 {
		return emitAgentFailure(stdout, requestID, "unknown", agent.RetrySafe, app.ExitUsage, "COMMAND_REQUIRED", errors.New("a NodeX operation is required"))
	}
	if len(helpPath) > 0 {
		return runNormal(ctx, appendAgentOutputOptions(original, opts), stdout, stderr)
	}

	// Discovery and receipt-control commands are themselves machine-readable
	// operations. They do not enter the mutation execution wrapper.
	if isAgentManagementPath(remaining) {
		var captured boundedCapture
		err := runNormal(ctx, appendAgentOutputOptions(original, opts), &captured, stderr)
		if err == nil {
			if captured.truncated {
				return emitAgentFailure(stdout, requestID, operationFromRemaining(remaining), agent.RetrySafe, app.ExitOutputError, "OUTPUT_LIMIT", errors.New("structured management output exceeded the 1 MiB agent response limit"))
			}
			_, writeErr := stdout.Write(captured.Bytes())
			if writeErr != nil {
				return app.NewExitError(writeErr, app.ExitOutputError)
			}
			return nil
		}
		if raw := bytes.TrimSpace(captured.Bytes()); len(raw) > 0 && json.Valid(raw) {
			if _, writeErr := stdout.Write(captured.Bytes()); writeErr != nil {
				return app.NewExitError(writeErr, app.ExitOutputError)
			}
			return app.MarkEmitted(err)
		}
		return emitAgentFailure(stdout, requestID, operationFromRemaining(remaining), agent.RetrySafe, app.ExitCodeFromError(err), errorCode(err), err)
	}

	meta, handlerArgs, dispatchArgs, ok := resolveOperationInvocation(remaining)
	if !ok || meta == nil {
		return emitAgentFailure(stdout, requestID, operationFromRemaining(remaining), agent.RetrySafe, app.ExitUsage, "UNKNOWN_OPERATION", fmt.Errorf("unknown or incomplete operation path %q", strings.Join(remaining, " ")))
	}
	contract := describeOperation(*meta, true)
	if !contract.AgentSupported {
		return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitUnsupportedCap, "AGENT_MODE_UNSUPPORTED", errors.New(contract.AgentUnsupportedWhy))
	}
	if flag, conflict := conflictingOperationOptions(meta.Path, handlerArgs); conflict {
		return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitConflict, "CONFLICTING_OPTIONS", fmt.Errorf("operation option %s was supplied with conflicting values", flag))
	}
	if meta.Path == "monitor check" && flagValue(dispatchArgs, "--target") == "" {
		return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitUsage, "TARGET_REQUIRED", errors.New("agent-mode monitor check requires exactly one --target"))
	}
	if meta.Path == "profile diagnose-permissions" && (len(handlerArgs) != 1 || handlerArgs[0] != opts.Profile) ||
		meta.Path == "profile test" && len(handlerArgs) == 1 && handlerArgs[0] != opts.Profile {
		return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitConflict, "PROFILE_TARGET_MISMATCH", errors.New("the positional profile must exactly match the explicit --profile execution context"))
	}
	if containsSensitiveArgument(handlerArgs) || meta.SecuritySensitivity == SecCredentials {
		return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitUnsupportedCap, "SECRET_INPUT_UNSUPPORTED", errors.New("secret-bearing arguments are not accepted in agent mode"))
	}
	if opts.Profile == "" && requiresRemoteProfile(*meta) {
		return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitConfig, "PROFILE_REQUIRED", errors.New("remote agent-mode operations require an explicit --profile; current-profile defaults are not used"))
	}

	var cfg *config.Config
	var executionContext agent.ExecutionContext
	var extraFingerprintArgs []string
	if requiresRemoteProfile(*meta) || meta.Path == "monitor check" || meta.Path == "monitor targets" {
		var err error
		cfg, err = config.Read()
		if err != nil {
			return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitCodeFromError(err), errorCode(err), err)
		}
		if !requiresRemoteProfile(*meta) {
			executionContext = targetContext(*meta, handlerArgs, dispatchArgs, "", "", "", config.Profile{})
			if meta.Path == "monitor check" {
				targetName := executionContext.ResourceID
				if cfg.Monitoring == nil {
					return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitConfig, "MONITOR_TARGET_MISSING", errors.New("monitoring is not configured"))
				}
				target, ok := cfg.Monitoring.Targets[targetName]
				if !ok {
					return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitNotFound, "MONITOR_TARGET_NOT_FOUND", errors.New("selected monitor target was not found"))
				}
				executionContext.ResourceType = "monitor:" + target.Type
				executionContext.Provider = "monitor:" + target.Type
				executionContext.Endpoint = normalizeMonitorAddress(target.Address)
				executionContext.EndpointIdentity = executionContext.Provider + "@" + executionContext.Endpoint
				executionContext.Namespace = target.Resolver
				executionContext.TLSCAFile = target.CAFile
				targetConfig, _ := json.Marshal(target)
				extraFingerprintArgs = append(extraFingerprintArgs, "monitor_target_config="+string(targetConfig))
			}
		} else {
			p, exists := cfg.Profiles[opts.Profile]
			if !exists {
				return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitConfig, "PROFILE_NOT_FOUND", fmt.Errorf("profile %q was not found", opts.Profile))
			}
			endpoint, err := normalizeEndpoint(p.Endpoint)
			if err != nil {
				return emitAgentFailure(stdout, requestID, meta.Path, agent.RetrySafe, app.ExitConfig, "INVALID_PROFILE_ENDPOINT", err)
			}
			executionContext = targetContext(*meta, handlerArgs, dispatchArgs, opts.Profile, config.NormalizeProvider(p.Provider), endpoint, p)
			if meta.Path == "container os-update" {
				hostName, inventoryHost, hostErr := findPVEInventoryHost(cfg, opts.Profile, executionContext.Node)
				if hostErr != nil {
					return emitAgentFailureWithContext(stdout, requestID, meta.Path, executionContext, agent.RetrySafe, app.ExitConfig, "SSH_TARGET_UNAVAILABLE", hostErr)
				}
				executionContext.SSHHost = inventoryHost.Address
				executionContext.SSHUser = inventoryHost.SSHUser
				executionContext.SSHKeyFile = inventoryHost.SSHKeyFile
				executionContext.SSHKnownHosts = inventoryHost.KnownHostsFile
				executionContext.SSHInventoryHost = hostName
				executionContext.SSHPort = inventoryHost.SSHPort
			}
			if !provider.IsRegistered(config.NormalizeProvider(p.Provider)) {
				return emitAgentFailureWithContext(stdout, requestID, meta.Path, executionContext, agent.RetrySafe, app.ExitUnsupportedCap, "PROVIDER_UNAVAILABLE", fmt.Errorf("provider implementation %q is not registered in this build", p.Provider))
			}
		}
	} else {
		executionContext = targetContext(*meta, handlerArgs, dispatchArgs, "", "", "", config.Profile{})
	}
	if !meta.Inspection && requiresRemoteProfile(*meta) && executionContext.ResourceID == "" && executionContext.Node == "" && executionContext.Namespace == "" {
		return emitAgentFailureWithContext(stdout, requestID, meta.Path, executionContext, agent.RetrySafe, app.ExitUsage, "TARGET_REQUIRED", errors.New("mutation requires an explicit target from the operation arguments"))
	}

	var store *agent.Store
	var fingerprint string
	var err error
	if !meta.Inspection {
		store, err = agent.DefaultStore()
		if err != nil {
			return emitAgentFailureWithContext(stdout, requestID, meta.Path, executionContext, agent.RetrySafe, app.ExitConfig, "RECEIPT_STORE_UNAVAILABLE", err)
		}
		inputArgs := fingerprintArguments(meta.Path, handlerArgs, opts)
		inputArgs = append(inputArgs, extraFingerprintArgs...)
		fingerprint, err = store.Fingerprint(meta.Path, inputArgs, executionContext)
		if err != nil {
			return emitAgentFailureWithContext(stdout, requestID, meta.Path, executionContext, agent.RetrySafe, app.ExitConfig, "FINGERPRINT_KEY_FAILED", err)
		}
	}
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	result := agent.Result{
		SchemaVersion: agent.ResultSchemaVersion, RequestID: requestID, Operation: meta.Path, Context: executionContext,
		Submission: agent.SubmissionNotAttempted, Execution: agent.ExecutionNotStarted,
		Verification: agent.VerificationNotRequested, Retry: agent.RetrySafe,
		StartedAt: startedAt, ObservedAt: startedAt,
	}
	var lease *agent.Lease
	var receipt agent.Receipt
	if !meta.Inspection {
		lease, err = store.Lock(requestID)
		if err != nil {
			return emitAgentFailureWithContext(stdout, requestID, meta.Path, executionContext, agent.RetrySafe, app.ExitConfig, "RECEIPT_LOCK_FAILED", err)
		}
		defer func() { _ = lease.Close() }()
		existing, loadErr := lease.Load()
		if loadErr == nil {
			if existing.InputFingerprint != fingerprint {
				return emitAgentFailureWithContext(stdout, requestID, meta.Path, executionContext, agent.RetrySafe, app.ExitConflict, "REQUEST_ID_CONFLICT", errors.New("request ID already belongs to a different operation, input, or target"))
			}
			addAgentWarning(&existing.Result, agent.Warning{Code: "DUPLICATE_REQUEST_ID", Message: "existing result returned; provider operation was not resubmitted"})
			return writeAgentResult(stdout, existing.Result)
		}
		if !errors.Is(loadErr, os.ErrNotExist) {
			return emitAgentFailureWithContext(stdout, requestID, meta.Path, executionContext, agent.RetrySafe, app.ExitConfig, "RECEIPT_READ_FAILED", loadErr)
		}
		// Write-ahead state is deliberately unknown before entering the existing
		// handler: a process crash at this boundary cannot prove whether the
		// provider call began.
		result.ReceiptID = requestID
		result.Submission = agent.SubmissionUnknown
		result.Execution = agent.ExecutionUnknown
		result.Verification = agent.VerificationUnsupported
		result.Retry = agent.RetryReconcileFirst
		receipt = agent.Receipt{Result: result, InputFingerprint: fingerprint, UpdatedAt: startedAt}
		if err := lease.Save(receipt); err != nil {
			return emitAgentFailureWithContext(stdout, requestID, meta.Path, executionContext, agent.RetrySafe, app.ExitConfig, "RECEIPT_WRITE_FAILED", fmt.Errorf("mutation was not submitted because its write-ahead receipt could not be persisted: %w", err))
		}
	}

	effective := appendAgentExecutionOptions(original, opts)
	boundCtx := context.WithValue(ctx, agentConfigKey{}, cfg)
	var captured boundedCapture
	runErr := runNormal(boundCtx, effective, &captured, stderr)
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if meta.Inspection {
		result.ObservedAt = observedAt
		result.Observation = &agent.Observation{At: observedAt, Completeness: "unknown", Unsupported: []string{"the selected handler does not expose a separate completeness contract", "pagination metadata is unavailable"}}
		result.Data, result.Observation.Truncated = capturedData(captured.Bytes(), captured.truncated)
		if runErr != nil {
			result.Error = structuredError(runErr)
			result.Execution = readExecutionState(runErr)
			result.Retry = agent.RetrySafe
			result.Observation.Partial = len(result.Data) > 0
			return writeAgentResult(stdout, result)
		}
		result.Execution = agent.ExecutionSucceeded
		return writeAgentResult(stdout, result)
	}

	result.ObservedAt = observedAt
	result = mutationOutcome(result, *meta, captured.Bytes(), runErr)
	receipt.Result = result
	receipt.UpdatedAt = result.ObservedAt
	var postSubmitSaveErr error
	if saveErr := lease.Save(receipt); saveErr != nil {
		postSubmitSaveErr = saveErr
		addAgentWarning(&result, agent.Warning{Code: "RECEIPT_UPDATE_FAILED", Message: "receipt update failed after handler execution; persisted state may be older than this response"})
		if runErr != nil {
			addAgentWarning(&result, agent.Warning{Code: errorCode(runErr), Message: "the existing handler also returned: " + safeError(runErr)})
		}
		if result.Submission == agent.SubmissionNotAttempted || result.Submission == agent.SubmissionRejected {
			result.Submission = agent.SubmissionUnknown
			result.Execution = agent.ExecutionUnknown
		}
		if result.Submission == agent.SubmissionUnknown || result.Execution == agent.ExecutionUnknown || result.Execution == agent.ExecutionRunning {
			result.Retry = agent.RetryReconcileFirst
		}
		receiptExit := app.ExitGeneral
		if result.Submission == agent.SubmissionUnknown || result.Execution == agent.ExecutionUnknown || result.Execution == agent.ExecutionRunning {
			receiptExit = app.ExitAmbiguousOutcome
		}
		result.Error = &agent.AgentError{Code: "RECEIPT_UPDATE_FAILED", Message: safeError(saveErr), Exit: receiptExit}
	}
	if err := writeAgentResult(stdout, result); err != nil {
		// The ledger was persisted before attempting stdout. Re-running with the
		// same request ID returns the durable result instead of repeating the
		// mutation.
		if postSubmitSaveErr != nil {
			return app.NewExitError(fmt.Errorf("write agent result and receipt update failed for request %s; do not retry automatically (stdout: %s; receipt: %s)", requestID, safeError(err), safeError(postSubmitSaveErr)), app.ExitOutputError)
		}
		return app.NewExitError(fmt.Errorf("write agent result (receipt %s remains available): %w", requestID, err), app.ExitOutputError)
	}
	if postSubmitSaveErr != nil {
		message := fmt.Errorf("persist the observed receipt outcome: %w", postSubmitSaveErr)
		if runErr != nil {
			message = fmt.Errorf("%w; the existing handler also reported: %s", message, safeError(runErr))
		}
		return app.MarkEmitted(app.NewExitError(message, result.Error.Exit))
	}
	if runErr != nil {
		return app.MarkEmitted(app.NewExitError(runErr, app.ExitCodeFromError(runErr)))
	}
	return nil
}

func agentArgsTooLarge(args []string) bool {
	total := 0
	for _, arg := range args {
		total += len(arg) + 1
		if total > 8192 {
			return true
		}
	}
	return false
}

func conflictingGlobalOptions(args []string) (string, bool) {
	seen := make(map[string]string)
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "--" {
			break
		}
		if !strings.HasPrefix(token, "-") || token == "-" {
			continue
		}
		name, value, inline := strings.Cut(strings.TrimLeft(token, "-"), "=")
		key := "--" + name
		var normalized string
		switch {
		case isGlobalBool(name):
			b := true
			if inline {
				var err error
				b, err = strconv.ParseBool(value)
				if err != nil {
					continue // parseGlobal reports the malformed value with precedence.
				}
			}
			normalized = strconv.FormatBool(b)
		case isGlobalValue(name):
			if !inline {
				if i+1 >= len(args) {
					continue
				}
				i++
				value = args[i]
			}
			normalized = value
		default:
			continue
		}
		if previous, ok := seen[key]; ok && previous != normalized {
			return key, true
		}
		seen[key] = normalized
	}
	return "", false
}

func conflictingOperationOptions(operation string, args []string) (string, bool) {
	flags := handlerFlags[operation]
	seen := make(map[string]string)
	register := func(name, value string) (string, bool) {
		if previous, ok := seen[name]; ok && previous != value {
			return name, true
		}
		seen[name] = value
		return "", false
	}
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "--" {
			break
		}
		if strings.HasPrefix(token, "--") {
			name, value, inline := strings.Cut(token, "=")
			owned := false
			for _, exact := range flags.exact {
				if name == exact {
					owned = true
					break
				}
			}
			if owned {
				if !inline {
					if optionIsBoolean(name) {
						value = "true"
					} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
						i++
						value = args[i]
					}
				}
				if conflict, ok := register(name, value); ok {
					return conflict, true
				}
				continue
			}
			for _, param := range flags.params {
				if name == "--"+param && inline {
					if conflict, ok := register(name, value); ok {
						return conflict, true
					}
					break
				}
			}
			continue
		}
		if key, value, ok := strings.Cut(token, "="); ok {
			if conflict, duplicate := register(key, value); duplicate {
				return conflict, true
			}
		}
	}
	return "", false
}

func optionIsBoolean(name string) bool {
	switch name {
	case "--check", "--running", "--errors", "--follow", "--remove-credential", "--propagate":
		return true
	default:
		return false
	}
}

func appendAgentOutputOptions(args []string, opts Options) []string {
	var out []string
	if !opts.outputSpecified {
		out = append(out, "--output=json")
	}
	if !opts.nonInteractiveSpecified || !opts.NonInteractive {
		out = append(out, "--non-interactive")
	}
	return append(out, args...)
}

func appendAgentExecutionOptions(args []string, opts Options) []string {
	return appendAgentOutputOptions(args, opts)
}

func isAgentManagementPath(remaining []string) bool {
	return len(remaining) > 0 && (remaining[0] == "agent" || remaining[0] == "operation")
}

func resolveOperationInvocation(remaining []string) (*OperationMeta, []string, []string, bool) {
	if len(remaining) == 0 {
		return nil, nil, nil, false
	}
	c := commands[remaining[0]]
	if c == nil {
		return nil, nil, nil, false
	}
	path := []string{remaining[0]}
	idx := 1
	for idx < len(remaining) && c.sub != nil {
		next := c.sub[remaining[idx]]
		if next == nil {
			break
		}
		c = next
		path = append(path, remaining[idx])
		idx++
	}
	full := strings.Join(path, " ")
	dispatchPath := full
	if _, isDispatch := knownDispatchCommands[dispatchPath]; !isDispatch {
		if aliasMeta := LookupOperation(full); aliasMeta != nil {
			if _, isDispatch := knownDispatchCommands[aliasMeta.Path]; isDispatch {
				dispatchPath = aliasMeta.Path
			}
		}
	}
	if _, isDispatch := knownDispatchCommands[dispatchPath]; isDispatch {
		bestPath, bestTokens := "", 0
		for _, candidate := range knownDispatchCommands[dispatchPath] {
			suffix := strings.Fields(strings.TrimPrefix(candidate, dispatchPath+" "))
			if len(suffix) == 0 || idx+len(suffix) > len(remaining) || len(suffix) <= bestTokens {
				continue
			}
			matches := true
			for i := range suffix {
				if suffix[i] != remaining[idx+i] {
					matches = false
					break
				}
			}
			if matches {
				bestPath, bestTokens = candidate, len(suffix)
			}
		}
		if bestPath == "" {
			return nil, nil, nil, false
		}
		meta := LookupOperation(bestPath)
		return meta, remaining[idx+bestTokens:], remaining[idx:], meta != nil
	}
	meta := LookupOperation(full)
	if meta == nil || c.run == nil {
		return nil, nil, nil, false
	}
	return meta, remaining[idx:], remaining[idx:], true
}

func operationFromRemaining(remaining []string) string {
	if meta, _, _, ok := resolveOperationInvocation(remaining); ok {
		return meta.Path
	}
	if len(remaining) == 0 {
		return "unknown"
	}
	return strings.Join(remaining, " ")
}

func requiresRemoteProfile(op OperationMeta) bool {
	if op.Path == "certification report" {
		return false
	}
	if op.CapabilityInterface != "" {
		return true
	}
	if op.Path == "agent receipt refresh" || op.Path == "agent receipt reconcile" {
		return true
	}
	if op.Scope == ScopeSystem || op.Scope == ScopeProfile {
		return op.Path == "profile test" || op.Path == "profile diagnose-permissions" || strings.HasPrefix(op.Path, "environment health") || strings.HasPrefix(op.Path, "environment backup-health")
	}
	return true
}

func normalizeEndpoint(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("profile endpoint cannot be normalized safely")
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		port = "443"
	}
	u.Host = net.JoinHostPort(host, port)
	u.Path, u.RawPath = "", ""
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimSuffix(u.String(), "/"), nil
}

func normalizeMonitorAddress(value string) string {
	value = strings.TrimSpace(value)
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" {
		return strings.ToLower(value)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" && u.Scheme == "http" {
		port = "80"
	} else if port == "" && u.Scheme == "https" {
		port = "443"
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else {
		u.Host = host
	}
	return u.String()
}

func targetContext(op OperationMeta, args, dispatchArgs []string, profile, providerName, endpoint string, p config.Profile) agent.ExecutionContext {
	c := agent.ExecutionContext{Profile: profile, Provider: providerName, Endpoint: endpoint, TLSCAFile: p.CAFile, SSHHost: p.SSHHost, SSHUser: p.SSHUser, SSHKeyFile: p.SSHKeyFile, SSHPort: p.SSHPort}
	if providerName != "" && endpoint != "" {
		c.EndpointIdentity = providerName + "@" + endpoint
	}
	first := firstPositional(args)
	second := ""
	if len(args) > 1 {
		second = firstPositional(args[1:])
	}
	words := strings.Fields(op.Path)
	if len(words) > 0 {
		c.ResourceType = words[0]
	}
	if strings.HasPrefix(first, "--") {
		first = ""
	}
	switch c.ResourceType {
	case "vm", "container":
		if strings.HasPrefix(op.Path, "vm create") || strings.HasPrefix(op.Path, "container create") || strings.HasPrefix(op.Path, "container restore") {
			c.Node = first
			c.ResourceID = second
		} else if strings.HasSuffix(op.Path, " clone") {
			c.Node, c.ResourceID = splitTarget(first)
			newID := second
			if newID != "" {
				target := agent.ResourceTarget{ResourceType: c.ResourceType, ResourceID: newID, Node: c.Node}
				if len(args) > 3 {
					target.Namespace = firstPositional(args[3:]) // optional clone storage
				}
				c.RelatedTargets = append(c.RelatedTargets, target)
			}
		} else if strings.HasSuffix(op.Path, " migrate") {
			c.Node, c.ResourceID = splitTarget(first)
			if second != "" {
				c.RelatedTargets = append(c.RelatedTargets, agent.ResourceTarget{ResourceType: "node", ResourceID: second, Node: second})
			}
		} else {
			c.Node, c.ResourceID = splitTarget(first)
		}
		if c.ResourceType == "vm" && strings.HasPrefix(op.Path, "vm disk ") {
			disk := second
			if disk != "" {
				related := agent.ResourceTarget{ResourceType: "disk", ResourceID: disk, Node: c.Node}
				if strings.HasSuffix(op.Path, " move") && len(args) > 2 {
					related.Namespace = firstPositional(args[2:]) // destination storage
				}
				c.RelatedTargets = append(c.RelatedTargets, related)
			}
		}
	case "node", "network":
		c.Node = first
		c.ResourceID = first
	case "task":
		c.Node = first
		c.ResourceID = second
	case "ceph":
		c.Node = first
		c.ResourceID = second
	case "firewall":
		if strings.HasPrefix(op.Path, "firewall rule ") {
			switch first {
			case "cluster":
				c.Namespace = "cluster"
				if strings.HasSuffix(op.Path, " create") {
					c.ResourceID = "cluster-rules"
				} else {
					c.ResourceID = second
				}
			case "node":
				c.Node = second
				if strings.HasSuffix(op.Path, " create") {
					c.ResourceID = "node-firewall-rules"
				} else if len(args) > 2 {
					c.ResourceID = firstPositional(args[2:])
				}
			case "vm":
				c.Node, c.ResourceID = splitTarget(second)
				c.Namespace = "vm-firewall"
				if strings.HasSuffix(op.Path, " update") || strings.HasSuffix(op.Path, " delete") {
					position := firstPositional(args[2:])
					if position != "" {
						c.RelatedTargets = append(c.RelatedTargets, agent.ResourceTarget{ResourceType: "firewall_rule", ResourceID: position, Node: c.Node, Namespace: c.Namespace})
					}
				}
			default:
				c.ResourceID = first
			}
		} else if op.Path == "firewall options update" {
			c.Namespace, c.ResourceID = "cluster", "cluster-options"
		} else if op.Path == "firewall node-rules" {
			c.Node, c.ResourceID = first, first
		} else if op.Path == "firewall vm-rules" {
			c.Node, c.ResourceID = splitTarget(first)
			c.Namespace = "vm-firewall"
		} else {
			c.ResourceID = first
		}
		if strings.HasPrefix(op.Path, "firewall ipset entry ") && second != "" {
			c.RelatedTargets = append(c.RelatedTargets, agent.ResourceTarget{ResourceType: "firewall_ipset_entry", ResourceID: second, Namespace: first})
		}
	case "access":
		c.ResourceID = first
	case "sdn":
		if strings.HasPrefix(op.Path, "sdn subnet ") {
			c.Namespace, c.ResourceID = first, second
		} else {
			c.ResourceID = first
		}
	case "replication":
		if strings.HasSuffix(op.Path, " schedule") {
			c.Node, c.ResourceID = first, second
		} else {
			c.ResourceID = first
		}
	case "cluster":
		c.ResourceID = first
	case "storage":
		if op.Path == "storage show" {
			c.Namespace, c.ResourceID = "storage", first
		} else if op.Path != "storage list" {
			c.Node = first
			c.Namespace = second
			switch op.Path {
			case "storage upload":
				c.ResourceID = "content-upload"
			case "storage delete":
				if len(args) > 2 {
					c.ResourceID = firstPositional(args[2:])
				}
			default:
				if len(args) > 2 {
					c.ResourceID = firstPositional(args[2:])
				}
			}
		}
	case "pbs":
		c.Namespace = flagValue(dispatchArgs, "--datastore")
		if strings.HasSuffix(op.Path, "garbage-collection run") {
			c.Namespace, c.ResourceID = first, first
		} else {
			c.ResourceID = first
		}
	case "backup":
		if strings.HasPrefix(op.Path, "backup job ") {
			if op.Path == "backup job create" {
				c.ResourceID = "cluster-backup-schedules"
			} else if !strings.Contains(first, "=") {
				c.ResourceID = first
			}
		} else {
			c.Node, c.ResourceID = splitTarget(first)
			if c.Node == "" {
				c.Node = first
				c.ResourceID = second
			}
			switch op.Path {
			case "backup create":
				c.Namespace = second
				c.RelatedTargets = append(c.RelatedTargets, agent.ResourceTarget{ResourceType: "storage", ResourceID: second})
			case "backup content":
				c.Namespace = second
			case "backup restore":
				if len(args) > 3 {
					c.Namespace = firstPositional(args[3:])
				}
				if len(args) > 2 {
					c.RelatedTargets = append(c.RelatedTargets, agent.ResourceTarget{ResourceType: "backup_archive", ResourceID: firstPositional(args[2:]), Namespace: c.Namespace})
				}
			}
		}
	case "profile":
		c.ResourceID = first
	case "environment":
		c.ResourceID = first
	case "monitor":
		c.ResourceID = flagValue(dispatchArgs, "--target")
	default:
		c.ResourceID = first
	}
	return c
}

func firstPositional(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
		if strings.HasPrefix(arg, "--") {
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		}
		return arg
	}
	return ""
}

func splitTarget(value string) (string, string) {
	if i := strings.LastIndex(value, "/"); i > 0 && i < len(value)-1 {
		return value[:i], value[i+1:]
	}
	return "", value
}

func flagValue(args []string, name string) string {
	for i, arg := range args {
		if strings.HasPrefix(arg, name+"=") {
			return strings.TrimPrefix(arg, name+"=")
		}
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func fingerprintArguments(operation string, handlerArgs []string, opts Options) []string {
	flags := handlerFlags[operation]
	boolFlags := map[string]bool{"--check": true, "--running": true, "--errors": true, "--follow": true, "--remove-credential": true, "--propagate": true}
	out := make([]string, 0, len(handlerArgs)+5)
	for i := 0; i < len(handlerArgs); i++ {
		arg := handlerArgs[i]
		if arg == "--" {
			out = append(out, handlerArgs[i:]...)
			break
		}
		if strings.HasPrefix(arg, "--") {
			name, inline, hasInline := strings.Cut(arg, "=")
			if hasInline {
				out = append(out, name+"="+inline)
				continue
			}
			owned := false
			for _, exact := range flags.exact {
				if exact == name {
					owned = true
					break
				}
			}
			if owned && boolFlags[name] {
				out = append(out, name+"=true")
				continue
			}
			if owned && i+1 < len(handlerArgs) && !strings.HasPrefix(handlerArgs[i+1], "-") {
				out = append(out, name+"="+handlerArgs[i+1])
				i++
				continue
			}
		}
		out = append(out, arg)
	}
	for _, flag := range []struct {
		name  string
		value bool
	}{{"--yes", opts.Yes}, {"--force", opts.Force}, {"--expert", opts.Expert}, {"--wait", opts.Wait}} {
		out = append(out, fmt.Sprintf("%s=%t", flag.name, flag.value))
	}
	out = append(out, "--timeout="+opts.Timeout.String(), fmt.Sprintf("--limit=%d", opts.Limit))
	if opts.ConfirmTarget != "" {
		out = append(out, "--confirm-target="+opts.ConfirmTarget)
	}
	return out
}

func containsSensitiveArgument(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		flagName, _, _ := strings.Cut(lower, "=")
		if strings.HasPrefix(flagName, "--") {
			flagName = strings.ReplaceAll(strings.TrimLeft(flagName, "-"), "_", "-")
			for _, marker := range []string{"password", "passwd", "passphrase", "secret", "token", "credential", "api-key", "private-key", "keyfile", "sshkeys", "ssh-key", "access-key"} {
				if strings.Contains(flagName, marker) {
					return true
				}
			}
		}
		key, _, hasValue := strings.Cut(lower, "=")
		if strings.HasPrefix(key, "--") {
			key = strings.TrimLeft(key, "-")
		}
		key = strings.ReplaceAll(key, "_", "-")
		if hasValue {
			for _, marker := range []string{"password", "passwd", "passphrase", "cipassword", "rootpw", "secret", "token", "credential", "api-key", "private-key", "keyfile", "sshkeys", "ssh-key", "access-key"} {
				if strings.Contains(key, marker) {
					return true
				}
			}
		}
	}
	return false
}

func capturedData(data []byte, sourceTruncated bool) (json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, sourceTruncated
	}
	if !sourceTruncated && len(trimmed) <= maxAgentCaptureBytes && json.Valid(trimmed) {
		return append(json.RawMessage(nil), trimmed...), false
	}
	// Text/table data remains inert data and is bounded. It is never interpreted
	// as a command or as recovery guidance.
	const maxCapturedText = 64 << 10
	truncated := sourceTruncated
	if len(trimmed) > maxCapturedText {
		trimmed = trimmed[:maxCapturedText]
		truncated = true
	}
	text := redact.String(output.SanitizeTerminal(string(trimmed)))
	b, _ := json.Marshal(text)
	return b, truncated
}

func mutationOutcome(result agent.Result, operation OperationMeta, captured []byte, runErr error) agent.Result {
	result.Data = nil
	result.Retry = agent.RetryReconcileFirst
	result.Changed = nil
	result.Verification = agent.VerificationUnsupported
	var legacy output.OperationResult
	hasLegacyResult := json.Unmarshal(bytes.TrimSpace(captured), &legacy) == nil && legacy.Schema == output.SchemaVersionResult
	if hasLegacyResult {
		for _, warning := range legacy.Warnings {
			addAgentWarning(&result, agent.Warning{Code: "OPERATION_WARNING", Message: safeMessage(warning)})
		}
		if legacy.Changed != nil {
			changed := *legacy.Changed
			result.Changed = &changed
		}
		taskID, taskIDValid := safeTaskID(legacy.UPID)
		if taskIDValid {
			result.TaskID = taskID
		}
		if legacy.Submitted {
			result.Submission = agent.SubmissionAccepted
			if legacy.Waited {
				if legacy.Success {
					result.Execution = agent.ExecutionSucceeded
				} else if legacy.Error != nil && legacy.Error.Class == "verification_failed" {
					result.Execution = agent.ExecutionSucceeded
					result.Verification = agent.VerificationFailed
				} else if legacy.Error != nil && (legacy.Error.Exit == app.ExitTimeout || legacy.Error.Exit == app.ExitCancellation || legacy.Error.Exit == app.ExitInterrupted || legacy.Error.Exit == app.ExitSigterm || legacy.Error.Exit == app.ExitAmbiguousOutcome) {
					result.Execution = agent.ExecutionUnknown
				} else {
					result.Execution = agent.ExecutionFailed
				}
			} else if legacy.UPID != "" && taskIDValid {
				result.Execution = agent.ExecutionRunning
			} else {
				result.Execution = agent.ExecutionUnknown
			}
			if legacy.UPID != "" && !taskIDValid {
				addAgentWarning(&result, agent.Warning{Code: "PROVIDER_TASK_ID_INVALID", Message: "provider task identifier was omitted because it was malformed or exceeded the result bound"})
				if !legacy.Waited {
					result.Error = &agent.AgentError{Code: "PROVIDER_TASK_ID_INVALID", Message: "task was submitted but its identifier cannot be safely retained for status refresh", Exit: app.ExitAmbiguousOutcome}
				}
			}
			if legacy.Error != nil {
				result.Error = &agent.AgentError{Code: stableErrorCode(legacy.Error.Exit), Message: safeMessage(legacy.Error.Detail), Exit: legacy.Error.Exit}
			}
			switch legacy.Status {
			case "verified", "no-updates":
				result.Verification = agent.VerificationPassed
			case "verification-failed":
				result.Verification = agent.VerificationFailed
			}
		} else if legacy.Success && legacy.Status == "no-updates" {
			result.Submission = agent.SubmissionNotAttempted
			result.Execution = agent.ExecutionSucceeded
			result.Verification = agent.VerificationPassed
			result.Retry = agent.RetryDoNotAutomatic
			changed := false
			result.Changed = &changed
		} else if legacy.Success && strings.Contains(strings.ToLower(legacy.Status), "already") {
			result.Submission = agent.SubmissionNotAttempted
			result.Execution = agent.ExecutionSucceeded
			changed := false
			result.Changed = &changed
			result.Retry = agent.RetryDoNotAutomatic
			if legacy.Status == "already running" || legacy.Status == "already stopped" || legacy.Status == "already paused" {
				result.Verification = agent.VerificationPassed
			}
		} else if legacy.Success {
			result.Submission = agent.SubmissionAccepted
			result.Execution = agent.ExecutionUnknown
		} else if legacy.Error != nil && legacy.Error.Class == "ambiguous_outcome" {
			result.Submission = agent.SubmissionUnknown
			result.Execution = agent.ExecutionUnknown
			result.Error = &agent.AgentError{Code: "OUTCOME_UNKNOWN", Message: safeMessage(legacy.Error.Detail), Exit: legacy.Error.Exit}
		}
	}
	if runErr == nil && !hasLegacyResult {
		// The existing handler returned nil, but did not expose a structured
		// result/task envelope. Treat the API call as accepted while retaining
		// uncertainty about remote completion and the resulting resource state.
		result.Submission = agent.SubmissionAccepted
		result.Execution = agent.ExecutionUnknown
		if operation.ProducesUPID {
			addAgentWarning(&result, agent.Warning{Code: "TASK_ID_UNAVAILABLE", Message: "operation normally returns a task identifier, but no structured task result was available"})
		} else {
			addAgentWarning(&result, agent.Warning{Code: "EXECUTION_EVIDENCE_LIMITED", Message: "handler exposed no structured execution result or postcondition"})
		}
	}
	if runErr != nil {
		result.Error = structuredError(runErr)
		if result.Submission == agent.SubmissionUnknown && errors.Is(runErr, safety.ErrAuthorizationRequired) {
			result.Submission = agent.SubmissionNotAttempted
			result.Execution = agent.ExecutionNotStarted
			result.Retry = agent.RetrySafe
		} else if result.Submission == agent.SubmissionUnknown {
			if definitiveProviderReject(runErr) || app.UPIDFromError(runErr) != "" {
				if app.UPIDFromError(runErr) != "" {
					result.Submission = agent.SubmissionAccepted
					result.TaskID, _ = safeTaskID(app.UPIDFromError(runErr))
					if app.ExitCodeFromError(runErr) == app.ExitTaskFailure {
						result.Execution = agent.ExecutionFailed
					} else {
						result.Execution = agent.ExecutionUnknown
					}
				} else {
					result.Submission = agent.SubmissionRejected
					result.Execution = agent.ExecutionNotStarted
					result.Retry = agent.RetryDoNotAutomatic
				}
			} else if app.ExitCodeFromError(runErr) == app.ExitUsage || app.ExitCodeFromError(runErr) == app.ExitConfig || app.ExitCodeFromError(runErr) == app.ExitCredential || app.ExitCodeFromError(runErr) == app.ExitUnsupportedCap || app.ExitCodeFromError(runErr) == app.ExitValidationError {
				result.Submission = agent.SubmissionNotAttempted
				result.Execution = agent.ExecutionNotStarted
				result.Retry = agent.RetrySafe
			}
		}
	}
	if result.Submission == agent.SubmissionUnknown || result.Execution == agent.ExecutionUnknown || result.Execution == agent.ExecutionRunning {
		result.Retry = agent.RetryReconcileFirst
	}
	if result.Execution == agent.ExecutionRunning || result.Execution == agent.ExecutionUnknown {
		if result.TaskID != "" {
			result.NextActions = append(result.NextActions, agent.NextAction{Operation: "agent receipt refresh", Arguments: map[string]any{"request_id": result.RequestID, "profile": result.Context.Profile}})
		} else {
			result.NextActions = append(result.NextActions, agent.NextAction{Operation: "agent receipt reconcile", Arguments: map[string]any{"request_id": result.RequestID, "profile": result.Context.Profile}})
		}
	}
	if result.Execution == agent.ExecutionFailed {
		result.Retry = agent.RetryDoNotAutomatic
	} else if result.Submission == agent.SubmissionAccepted && result.Execution == agent.ExecutionSucceeded {
		result.Retry = agent.RetryDoNotAutomatic
	}
	result.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return result
}

func definitiveProviderReject(err error) bool {
	status := app.HTTPStatusFromError(err)
	return status >= 400 && status < 500 && status != 408
}

func emitAgentFailure(w io.Writer, requestID, operation string, retry agent.Retry, exit int, code string, err error) error {
	return emitAgentFailureWithContext(w, requestID, operation, agent.ExecutionContext{}, retry, exit, code, err)
}

func emitAgentFailureWithContext(w io.Writer, requestID, operation string, executionContext agent.ExecutionContext, retry agent.Retry, exit int, code string, err error) error {
	if !agent.ValidRequestID(requestID) {
		requestID = "invalid_request"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if operation == "" {
		operation = "unknown"
	}
	r := agent.Result{
		SchemaVersion: agent.ResultSchemaVersion, RequestID: requestID, Operation: operation, Context: executionContext,
		Submission: agent.SubmissionNotAttempted, Execution: agent.ExecutionNotStarted, Verification: agent.VerificationNotRequested,
		Retry: retry, StartedAt: now, ObservedAt: now,
		Error: &agent.AgentError{Code: code, Message: safeError(err), Exit: exit},
	}
	writeErr := writeAgentResult(w, r)
	if writeErr != nil {
		return app.NewExitError(writeErr, app.ExitOutputError)
	}
	return app.MarkEmitted(app.NewExitError(err, exit))
}

func writeAgentResult(w io.Writer, r agent.Result) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("invalid agent result: %w", err)
	}
	return output.WriteJSON(w, r)
}

func structuredError(err error) *agent.AgentError {
	code := app.ExitCodeFromError(err)
	return &agent.AgentError{Code: errorCode(err), Message: safeError(err), Exit: code}
}

func readExecutionState(err error) agent.Execution {
	switch app.ExitCodeFromError(err) {
	case app.ExitUsage, app.ExitConfig, app.ExitCredential, app.ExitUnsupportedCap, app.ExitValidationError, app.ExitIncompatibility:
		if app.HTTPStatusFromError(err) == 0 {
			return agent.ExecutionNotStarted
		}
	}
	return agent.ExecutionFailed
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, safety.ErrAuthorizationRequired), errors.Is(err, safety.ErrNonInteractiveRequired):
		return "CONFIRMATION_REQUIRED"
	case errors.Is(err, safety.ErrTypeConfirmMismatch):
		return "CONFIRMATION_MISMATCH"
	case errors.Is(err, safety.ErrExpertRequired):
		return "EXPERT_MODE_REQUIRED"
	case errors.Is(err, app.ErrCredential):
		return "CREDENTIAL_ERROR"
	case errors.Is(err, app.ErrUnsupportedCap):
		return "UNSUPPORTED_CAPABILITY"
	default:
		return stableErrorCode(app.ExitCodeFromError(err))
	}
}

func stableErrorCode(exit int) string {
	switch exit {
	case app.ExitSuccess:
		return "OK"
	case app.ExitUsage:
		return "USAGE_ERROR"
	case app.ExitConfig:
		return "CONFIG_ERROR"
	case app.ExitCredential:
		return "CREDENTIAL_ERROR"
	case app.ExitAuth:
		return "AUTHENTICATION_FAILED"
	case app.ExitAuthorization:
		return "AUTHORIZATION_DENIED"
	case app.ExitNetwork:
		return "NETWORK_ERROR"
	case app.ExitTLS:
		return "TLS_ERROR"
	case app.ExitUnsupportedCap:
		return "UNSUPPORTED_CAPABILITY"
	case app.ExitPartialFailure:
		return "PARTIAL_FAILURE"
	case app.ExitProvider:
		return "PROVIDER_ERROR"
	case app.ExitNotFound:
		return "NOT_FOUND"
	case app.ExitTimeout:
		return "TIMEOUT"
	case app.ExitCancellation:
		return "CANCELLED"
	case app.ExitTaskFailure:
		return "TASK_FAILED"
	case app.ExitValidationError:
		return "VALIDATION_ERROR"
	case app.ExitAmbiguousOutcome:
		return "OUTCOME_UNKNOWN"
	case app.ExitRateLimit:
		return "RATE_LIMITED"
	case app.ExitOutputError:
		return "OUTPUT_ERROR"
	case app.ExitConflict:
		return "CONFLICT"
	default:
		return "GENERAL_ERROR"
	}
}

func safeMessage(value string) string {
	clean := output.SanitizeTerminal(redact.String(value))
	const maxErrorRunes = 4096
	runes := []rune(clean)
	if len(runes) > maxErrorRunes {
		clean = string(runes[:maxErrorRunes]) + "…[truncated]"
	}
	return clean
}

func safeTaskID(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	if len(value) > 1024 || strings.ContainsAny(value, "\x00\r\n\x1b") {
		return "", false
	}
	return value, true
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return safeMessage(err.Error())
}

// resolveExplicitProfile reads the current operator configuration once and
// returns a context identity that can be compared with a durable receipt.
func resolveExplicitProfile(name string) (*config.Config, agent.ExecutionContext, error) {
	if name == "" {
		return nil, agent.ExecutionContext{}, errors.New("an explicit --profile is required")
	}
	cfg, err := config.Read()
	if err != nil {
		return nil, agent.ExecutionContext{}, err
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return nil, agent.ExecutionContext{}, fmt.Errorf("profile %q was not found", name)
	}
	endpoint, err := normalizeEndpoint(p.Endpoint)
	if err != nil {
		return nil, agent.ExecutionContext{}, err
	}
	c := agent.ExecutionContext{Profile: name, Provider: config.NormalizeProvider(p.Provider), Endpoint: endpoint, TLSCAFile: p.CAFile, SSHHost: p.SSHHost, SSHUser: p.SSHUser, SSHKeyFile: p.SSHKeyFile, SSHPort: p.SSHPort}
	return cfg, c, nil
}

func taskExecutionStatus(ctx context.Context, cmdCtx *Context, r *agent.Receipt) error {
	if r.TaskID == "" {
		return errors.New("receipt has no provider task ID; reliable task reconciliation is unsupported")
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()
	if prov.Name() != r.Context.Provider {
		return errors.New("resolved provider does not match the receipt")
	}
	var state string
	var status string
	if inspector, ok := prov.(domain.PBSTaskInspector); ok {
		taskStatus, err := inspector.PBSTaskStatus(ctx, r.TaskID)
		if err != nil {
			return err
		}
		if taskStatus == nil || taskStatus.UPID != r.TaskID {
			return errors.New("provider task response did not match the receipt task ID")
		}
		if taskStatus.EndTime == 0 {
			state = "running"
		} else if task.Success(taskStatus.ExitStatus) {
			state, status = "succeeded", taskStatus.ExitStatus
		} else if taskStatus.ExitStatus != "" {
			state, status = "failed", taskStatus.ExitStatus
		} else {
			state = "unknown"
		}
	} else {
		inspector, ok := prov.(domain.TaskInspector)
		if !ok {
			return errors.New("provider task inspection is unsupported")
		}
		if r.Context.Node == "" {
			return errors.New("receipt does not identify the node needed for task inspection")
		}
		upidNode, ok := taskIDNode(r.TaskID)
		if !ok || upidNode != r.Context.Node {
			return errors.New("provider task ID does not identify the receipt-bound node")
		}
		taskInfo, err := inspector.Task(ctx, r.Context.Node, r.TaskID)
		if err != nil {
			return err
		}
		if taskInfo == nil || taskInfo.UPID != r.TaskID || taskInfo.Node != "" && taskInfo.Node != r.Context.Node {
			return errors.New("provider task response did not match the receipt task and node")
		}
		switch strings.ToLower(taskInfo.State) {
		case "running":
			state = "running"
		case "stopped":
			if task.Success(taskInfo.Status) {
				state, status = "succeeded", taskInfo.Status
			} else if taskInfo.Status != "" {
				state, status = "failed", taskInfo.Status
			} else {
				state = "unknown"
			}
		default:
			state = "unknown"
		}
	}
	switch state {
	case "running":
		r.Submission, r.Execution, r.Retry = agent.SubmissionAccepted, agent.ExecutionRunning, agent.RetryReconcileFirst
	case "succeeded":
		r.Submission, r.Execution, r.Retry = agent.SubmissionAccepted, agent.ExecutionSucceeded, agent.RetryDoNotAutomatic
	case "failed":
		r.Submission, r.Execution, r.Retry = agent.SubmissionAccepted, agent.ExecutionFailed, agent.RetryDoNotAutomatic
	default:
		r.Submission, r.Execution, r.Retry = agent.SubmissionUnknown, agent.ExecutionUnknown, agent.RetryReconcileFirst
	}
	r.Verification = agent.VerificationUnsupported
	r.Changed = nil
	r.Error = nil
	if status != "" && state == "failed" {
		r.Error = &agent.AgentError{Code: "TASK_FAILED", Message: safeMessage(status), Exit: app.ExitTaskFailure}
	}
	if state == "succeeded" {
		parts := strings.Fields(r.Operation)
		if len(parts) == 2 && lifecycleDesiredState(parts[0], parts[1]) != "" {
			vmid, parseErr := strconv.Atoi(r.Context.ResourceID)
			if parseErr != nil || r.Context.Node == "" {
				r.Verification = agent.VerificationUnknown
				addAgentWarning(&r.Result, agent.Warning{Code: "POSTCONDITION_UNKNOWN", Message: "task succeeded, but the receipt lacks a usable node/resource identity for postcondition inspection"})
			} else if actual, ok := currentGuestStatus(ctx, prov, parts[0], r.Context.Node, vmid); !ok {
				r.Verification = agent.VerificationUnknown
				addAgentWarning(&r.Result, agent.Warning{Code: "POSTCONDITION_UNKNOWN", Message: "task succeeded, but the provider could not establish current guest state"})
			} else if strings.EqualFold(actual, lifecycleDesiredState(parts[0], parts[1])) {
				r.Verification = agent.VerificationPassed
				addAgentWarning(&r.Result, agent.Warning{Code: "CAUSATION_NOT_ESTABLISHED", Message: "desired current state observed; this does not prove the request caused the state change"})
			} else {
				r.Verification = agent.VerificationFailed
				addAgentWarning(&r.Result, agent.Warning{Code: "POSTCONDITION_FAILED", Message: "task succeeded, but the observed guest state does not match the requested postcondition"})
			}
		}
	}
	r.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return nil
}

func taskIDNode(taskID string) (string, bool) {
	if !strings.HasPrefix(taskID, "UPID:") {
		return "", false
	}
	rest := strings.TrimPrefix(taskID, "UPID:")
	for _, separator := range []string{":", "/"} {
		if index := strings.IndexByte(rest, separator[0]); index > 0 {
			return rest[:index], true
		}
	}
	return "", false
}
