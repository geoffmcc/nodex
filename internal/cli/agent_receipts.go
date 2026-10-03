package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/agent"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/output"
)

type receiptListEntry struct {
	RequestID    string                 `json:"request_id"`
	ReceiptID    string                 `json:"receipt_id"`
	Operation    string                 `json:"operation"`
	Context      agent.ExecutionContext `json:"context"`
	Submission   agent.Submission       `json:"submission"`
	Execution    agent.Execution        `json:"execution"`
	Verification agent.Verification     `json:"verification"`
	Retry        agent.Retry            `json:"retry"`
	TaskID       string                 `json:"task_id,omitempty"`
	UpdatedAt    string                 `json:"updated_at"`
}

type receiptListResult struct {
	Items     []receiptListEntry `json:"items"`
	Limit     int                `json:"limit"`
	Truncated bool               `json:"truncated"`
}

func runAgentReceiptDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(errors.New("usage: nodex agent receipt <list|show|refresh|reconcile>"), app.ExitUsage)
	}
	var handler CommandFunc
	switch args[0] {
	case "list":
		handler = runAgentReceiptList
	case "show":
		handler = runAgentReceiptShow
	case "refresh":
		handler = runAgentReceiptRefresh
	case "reconcile":
		handler = runAgentReceiptReconcile
	default:
		valid := []string{"list", "show", "refresh", "reconcile"}
		sort.Strings(valid)
		return app.NewExitError(fmt.Errorf("unknown receipt operation %q (valid: %s)", args[0], strings.Join(valid, ", ")), app.ExitUsage)
	}
	return handler(ctx, cmdCtx, args[1:])
}

func runAgentReceiptList(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(errors.New("usage: nodex agent receipt list"), app.ExitUsage)
	}
	store, err := agent.DefaultStore()
	if err != nil {
		return app.NewExitError(err, app.ExitConfig)
	}
	limit := cmdCtx.Opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	receipts, total, err := store.ListRecent(limit)
	if err != nil {
		return app.NewExitError(err, app.ExitConfig)
	}
	items := make([]receiptListEntry, 0, len(receipts))
	for _, r := range receipts {
		items = append(items, receiptListEntry{
			RequestID: r.RequestID, ReceiptID: r.ReceiptID, Operation: r.Operation, Context: r.Context,
			Submission: r.Submission, Execution: r.Execution, Verification: r.Verification,
			Retry: r.Retry, TaskID: r.TaskID, UpdatedAt: r.UpdatedAt,
		})
	}
	return output.WriteJSON(cmdCtx.Writer, receiptListResult{Items: items, Limit: limit, Truncated: total > len(items)})
}

func runAgentReceiptShow(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 || !agent.ValidRequestID(args[0]) {
		return app.NewExitError(errors.New("usage: nodex agent receipt show <request-id>"), app.ExitUsage)
	}
	store, err := agent.DefaultStore()
	if err != nil {
		return app.NewExitError(err, app.ExitConfig)
	}
	receipt, err := store.Get(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitNotFound)
	}
	return output.WriteJSON(cmdCtx.Writer, receipt)
}

func runAgentReceiptRefresh(ctx context.Context, cmdCtx *Context, args []string) error {
	return reconcileAgentReceipt(ctx, cmdCtx, args, "agent receipt refresh")
}

func runAgentReceiptReconcile(ctx context.Context, cmdCtx *Context, args []string) error {
	return reconcileAgentReceipt(ctx, cmdCtx, args, "agent receipt reconcile")
}

// achievableReceiptActions returns recovery steps that can actually be carried
// out for this receipt.
//
// It never offers `agent receipt reconcile`, which is the operation that just
// failed. When task reconciliation is structurally unavailable there is no
// provider evidence that could make a repeat attempt succeed, so re-recommending
// it would leave the caller looping on a step that cannot work. Inspecting the
// stored receipt remains possible and is what a caller needs to decide what to
// do next.
func achievableReceiptActions(receipt *agent.Receipt, reconciliationAvailable bool) []agent.NextAction {
	showArgs := map[string]any{"request_id": receipt.RequestID}
	if receipt.Context.Profile != "" {
		showArgs["profile"] = receipt.Context.Profile
	}

	unresolved := receipt.Execution == agent.ExecutionUnknown || receipt.Execution == agent.ExecutionRunning
	if !unresolved {
		return nil
	}

	actions := make([]agent.NextAction, 1, 2)
	actions[0] = agent.NextAction{Operation: "agent receipt show", Arguments: showArgs}
	if !reconciliationAvailable || receipt.TaskID == "" {
		return actions
	}
	refreshArgs := map[string]any{"request_id": receipt.RequestID}
	if receipt.Context.Profile != "" {
		refreshArgs["profile"] = receipt.Context.Profile
	}
	return append(actions, agent.NextAction{Operation: "agent receipt refresh", Arguments: refreshArgs})
}

func reconcileAgentReceipt(ctx context.Context, cmdCtx *Context, args []string, operation string) error {
	if len(args) != 1 || !agent.ValidRequestID(args[0]) {
		return app.NewExitError(fmt.Errorf("usage: nodex %s <request-id>", operation), app.ExitUsage)
	}
	store, err := agent.DefaultStore()
	if err != nil {
		return app.NewExitError(err, app.ExitConfig)
	}
	lease, err := store.Lock(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitConfig)
	}
	defer func() { _ = lease.Close() }()
	receipt, err := lease.Load()
	if err != nil {
		return app.NewExitError(err, app.ExitNotFound)
	}
	if cmdCtx.Opts.Profile == "" {
		return app.NewExitError(errors.New("receipt refresh requires an explicit --profile"), app.ExitConfig)
	}
	cfg, identity, err := resolveExplicitProfile(cmdCtx.Opts.Profile)
	if err != nil {
		return app.NewExitError(err, app.ExitConfig)
	}
	if receipt.Operation == "container os-update" {
		hostName, inventoryHost, hostErr := findPVEInventoryHost(cfg, cmdCtx.Opts.Profile, receipt.Context.Node)
		if hostErr != nil {
			return app.NewExitError(hostErr, app.ExitConflict)
		}
		identity.SSHHost = inventoryHost.Address
		identity.SSHUser = inventoryHost.SSHUser
		identity.SSHKeyFile = inventoryHost.SSHKeyFile
		identity.SSHKnownHosts = inventoryHost.KnownHostsFile
		identity.SSHInventoryHost = hostName
		identity.SSHPort = inventoryHost.SSHPort
	}
	if !sameExecutionIdentity(receipt.Context, identity) {
		return app.NewExitError(errors.New("profile identity does not match the receipt; refusing to inspect a different endpoint or trust configuration"), app.ExitConflict)
	}
	cmdCtx.AgentConfig = cfg
	if receipt.Submission == agent.SubmissionNotAttempted || receipt.Submission == agent.SubmissionRejected ||
		receipt.Submission == agent.SubmissionAccepted && receipt.Execution == agent.ExecutionFailed ||
		receipt.Submission == agent.SubmissionAccepted && receipt.Execution == agent.ExecutionSucceeded && receipt.Verification != agent.VerificationUnknown && !lifecyclePath(receipt.Operation) {
		return output.WriteJSON(cmdCtx.Writer, receipt)
	}
	if err := taskExecutionStatus(ctx, cmdCtx, &receipt); err != nil {
		receipt.Error = &agent.AgentError{Code: "RECONCILIATION_UNAVAILABLE", Message: safeError(err), Exit: app.ExitCodeFromError(err)}

		// Acceptance is a historical fact: it was established when the
		// provider took the request. A later inability to observe completion
		// is a new observation about evidence, not a retraction of that fact.
		// Erasing it would replace "we know it was submitted" with "we do not
		// know anything", which is strictly less true.
		terminal := receipt.Submission == agent.SubmissionAccepted &&
			(receipt.Execution == agent.ExecutionSucceeded || receipt.Execution == agent.ExecutionFailed)
		if terminal {
			addAgentWarning(&receipt.Result, agent.Warning{Code: "REFRESH_UNAVAILABLE", Message: "the latest read-only refresh failed; the previously observed terminal execution outcome was retained"})
		} else if receipt.Submission == agent.SubmissionAccepted {
			addAgentWarning(&receipt.Result, agent.Warning{Code: "REFRESH_UNAVAILABLE", Message: "the latest read-only refresh could not observe completion; the recorded acceptance was retained and the request was not resubmitted"})
		} else {
			receipt.Submission = agent.SubmissionUnknown
			receipt.Execution = agent.ExecutionUnknown
			receipt.Verification = agent.VerificationUnknown
			receipt.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
			addAgentWarning(&receipt.Result, agent.Warning{Code: "RECONCILIATION_UNAVAILABLE", Message: "reconciliation performed no mutation; the original remote outcome remains unknown"})
		}

		// Recovery guidance must describe something that can actually be done.
		// A receipt with no task ID can never be reconciled by task inspection,
		// so re-recommending that step would send the caller in a circle.
		receipt.NextActions = achievableReceiptActions(&receipt, !errors.Is(err, errTaskReconciliationUnavailable))
		if receipt.Submission == agent.SubmissionAccepted {
			// Do not resubmit: the request may still have taken effect, and a
			// blind repeat could apply it twice.
			receipt.Retry = agent.RetryDoNotAutomatic
		} else {
			receipt.Retry = agent.RetryReconcileFirst
		}
	}
	receipt.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := lease.Save(receipt); err != nil {
		return app.NewExitError(fmt.Errorf("save refreshed receipt: %w", err), app.ExitConfig)
	}
	if err := output.WriteJSON(cmdCtx.Writer, receipt); err != nil {
		return app.NewExitError(fmt.Errorf("write refreshed receipt: %w", err), app.ExitOutputError)
	}
	if receipt.Error != nil {
		return app.MarkEmitted(app.NewExitError(errors.New(receipt.Error.Message), receipt.Error.Exit))
	}
	return nil
}

func sameExecutionIdentity(receipt, current agent.ExecutionContext) bool {
	return receipt.Profile == current.Profile && receipt.Provider == current.Provider &&
		receipt.Endpoint == current.Endpoint && receipt.TLSCAFile == current.TLSCAFile &&
		receipt.SSHHost == current.SSHHost && receipt.SSHUser == current.SSHUser &&
		receipt.SSHKeyFile == current.SSHKeyFile && receipt.SSHKnownHosts == current.SSHKnownHosts &&
		receipt.SSHInventoryHost == current.SSHInventoryHost && receipt.SSHPort == current.SSHPort
}
