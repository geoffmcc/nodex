package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/geoffmcc/nodex/internal/agent"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/safety"
	"github.com/geoffmcc/nodex/internal/version"
)

const AgentContractVersion = "1.0.0"

type AgentContract struct {
	ContractVersion  string            `json:"contract_version"`
	NodeXVersion     string            `json:"nodex_version"`
	InputSchemas     []string          `json:"input_schema_versions"`
	OutputSchemas    []string          `json:"output_schema_versions"`
	Features         []string          `json:"features"`
	OutcomeSemantics map[string]string `json:"outcome_semantics"`
	RetryRules       []string          `json:"retry_rules"`
	Safety           []string          `json:"safety_instructions"`
	TrustBoundary    string            `json:"trust_boundary"`
}

type AgentArgument struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Choices     []string `json:"choices,omitempty"`
	Description string   `json:"description,omitempty"`
	Syntax      string   `json:"syntax"`
}

type AgentFlag struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	ValueSyntax string   `json:"value_syntax,omitempty"`
	Default     any      `json:"default,omitempty"`
	Choices     []string `json:"choices,omitempty"`
	Required    bool     `json:"required"`
	Minimum     any      `json:"minimum,omitempty"`
	Maximum     any      `json:"maximum,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Format      string   `json:"format,omitempty"`
	Description string   `json:"description,omitempty"`
}

type OperationContract struct {
	ID                string          `json:"id"`
	Path              string          `json:"path"`
	Aliases           []string        `json:"aliases,omitempty"`
	Description       string          `json:"description"`
	ArgumentSyntax    string          `json:"argument_syntax"`
	Arguments         []AgentArgument `json:"arguments"`
	Flags             []AgentFlag     `json:"flags"`
	InputSchema       map[string]any  `json:"input_schema"`
	Constraints       []string        `json:"constraints,omitempty"`
	TargetRequirement string          `json:"target_requirement"`
	LocalSideEffects  []string        `json:"local_side_effects"`
	RemoteSideEffects []string        `json:"remote_side_effects"`
	SafetyTier        string          `json:"safety_tier"`
	Risks             []string        `json:"risks"`
	Confirmation      []string        `json:"confirmation_requirements"`
	// ConfirmationTargetFormat documents how to derive the exact
	// --confirm-target value for type-in confirmation operations. It describes
	// the format only; the runtime value is resolved from command arguments at
	// execution time and is deliberately not predicted here.
	ConfirmationTargetFormat string   `json:"confirmation_target_format,omitempty"`
	ProviderInterface        string   `json:"provider_interface,omitempty"`
	Interactive              bool     `json:"interactive"`
	SubmitsTask              bool     `json:"submits_task"`
	Verification             []string `json:"verification"`
	Recovery                 []string `json:"recovery"`
	StructuredOutput         []string `json:"structured_output"`
	LegacyOutputModes        []string `json:"legacy_output_modes"`
	AgentSupported           bool     `json:"agent_supported"`
	AgentUnsupportedWhy      string   `json:"agent_unsupported_reason,omitempty"`
	ProviderSupportNote      string   `json:"provider_support_note"`
	ProviderPermissionNote   string   `json:"provider_permission_note"`
}

type OperationSummary struct {
	ID                  string   `json:"id"`
	Path                string   `json:"path"`
	Aliases             []string `json:"aliases,omitempty"`
	Description         string   `json:"description"`
	ArgumentSyntax      string   `json:"argument_syntax"`
	TargetRequirement   string   `json:"target_requirement"`
	LocalSideEffects    []string `json:"local_side_effects"`
	RemoteSideEffects   []string `json:"remote_side_effects"`
	SafetyTier          string   `json:"safety_tier"`
	Risks               []string `json:"risks"`
	Confirmation        []string `json:"confirmation_requirements"`
	ProviderInterface   string   `json:"provider_interface,omitempty"`
	ProviderSupport     string   `json:"provider_support"`
	ProviderPermission  string   `json:"provider_permission"`
	Interactive         bool     `json:"interactive"`
	SubmitsTask         bool     `json:"submits_task"`
	Verification        []string `json:"verification"`
	Recovery            []string `json:"recovery"`
	StructuredOutput    []string `json:"structured_output"`
	LegacyOutputModes   []string `json:"legacy_output_modes"`
	AgentSupported      bool     `json:"agent_supported"`
	AgentUnsupportedWhy string   `json:"agent_unsupported_reason,omitempty"`
}

func currentAgentContract() AgentContract {
	return AgentContract{
		ContractVersion: AgentContractVersion,
		NodeXVersion:    version.Current().Version,
		InputSchemas:    []string{"1"},
		OutputSchemas:   []string{fmt.Sprint(agent.ResultSchemaVersion)},
		Features: []string{
			"offline operation discovery", "opt-in --agent JSON execution",
			"explicit profile and endpoint binding for remote requests", "durable mutation receipts",
			"request ID deduplication", "read-only task refresh and reconciliation",
		},
		OutcomeSemantics: map[string]string{
			"submission":   "for mutations, accepted means the provider accepted the request, not that execution completed; read-only operations report not_attempted",
			"execution":    "running/succeeded/failed/unknown is reported independently of submission",
			"verification": "passed requires a provider-backed postcondition; task completion alone is not verification",
			"changed":      "true/false is reported only when evidence establishes the fact; otherwise null",
			"receipt":      "a local diagnostic/deduplication record, not proof of authorization",
		},
		RetryRules: []string{
			"For mutations, a repeated request ID with identical normalized input returns the existing receipt and never resubmits.",
			"For mutations, a repeated request ID with different input or context is rejected.",
			"For unknown or running operations, refresh/reconcile before considering a separately identified request.",
			"Transport loss and provider server errors remain unknown unless a definitive provider rejection response was received.",
			"An accepted task is not cancelled when observation stops; use the provider's explicit task-cancellation command where available.",
			"NodeX cannot guarantee exactly-once provider execution when the provider has no idempotency key.",
		},
		Safety: []string{
			"Use an explicit --profile for every remote operation in --agent mode.",
			"Pass the existing --yes, --force, --expert, and --confirm-target acknowledgements required by the operation; NodeX never inserts them.",
			"A confirmation flag acknowledges intent only; it does not authenticate a human approver.",
			"Do not disable TLS verification or weaken SSH host-key controls to make an operation succeed.",
			"Use operation list/describe before execution; provider implementation support is not current authorization or readiness.",
		},
		TrustBoundary: "Agent mode is a predictable CLI interface, not an isolation boundary against an agent that can invoke arbitrary local commands or edit NodeX files.",
	}
}

func runAgentContract(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex agent contract"), app.ExitUsage)
	}
	return output.WriteJSON(cmdCtx.Writer, currentAgentContract())
}

func runOperationList(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex operation list"), app.ExitUsage)
	}
	ops := Operations()
	contracts := make([]OperationSummary, 0, len(ops))
	for _, op := range ops {
		contracts = append(contracts, summarizeOperation(op))
	}
	return output.WriteJSON(cmdCtx.Writer, contracts)
}

func summarizeOperation(op OperationMeta) OperationSummary {
	contract := describeOperation(op, false)
	providerSupport := op.ProviderSupportNote
	if providerSupport == "" {
		providerSupport = "static implementation metadata only"
	}
	providerPermission := "checked by the existing handler at execution time"
	if strings.HasPrefix(op.ProviderSupportNote, "Unsupported:") {
		providerPermission = "not checked; the unsupported operation makes no provider request"
	}
	return OperationSummary{
		ID: contract.ID, Path: contract.Path, Aliases: contract.Aliases,
		Description: contract.Description, ArgumentSyntax: contract.ArgumentSyntax,
		TargetRequirement: contract.TargetRequirement, SafetyTier: contract.SafetyTier,
		LocalSideEffects: contract.LocalSideEffects, RemoteSideEffects: contract.RemoteSideEffects,
		Risks: contract.Risks, ProviderSupport: providerSupport,
		ProviderPermission: providerPermission,
		Confirmation:       contract.Confirmation, ProviderInterface: contract.ProviderInterface,
		Interactive: contract.Interactive, SubmitsTask: contract.SubmitsTask,
		Verification: contract.Verification, Recovery: contract.Recovery,
		StructuredOutput: contract.StructuredOutput, AgentSupported: contract.AgentSupported,
		LegacyOutputModes:   contract.LegacyOutputModes,
		AgentUnsupportedWhy: contract.AgentUnsupportedWhy,
	}
}

func runOperationDescribe(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex operation describe <operation path>"), app.ExitUsage)
	}
	path := strings.Join(args, " ")
	op := LookupOperation(path)
	if op == nil {
		return app.NewExitError(fmt.Errorf("unknown operation %q", path), app.ExitNotFound)
	}
	return output.WriteJSON(cmdCtx.Writer, describeOperation(*op, true))
}

func describeOperation(op OperationMeta, detailed bool) OperationContract {
	entry := leafHelp[op.Path]
	usage := strings.TrimSpace(entry.usage)
	remoteProfileRequired := requiresRemoteProfile(op)
	flags := operationFlags(op.Path, usage, remoteProfileRequired)
	for i := range flags {
		switch flags[i].Name {
		case "--yes":
			flags[i].Required = op.SafetyTier >= safety.TierReversible
		case "--force":
			flags[i].Required = op.SafetyTier >= safety.TierDisruptive
		case "--confirm-target":
			flags[i].Required = op.RequiresTypeConfirm
		case "--expert":
			flags[i].Required = op.RequiresExpert
		case "--target":
			flags[i].Required = op.Path == "monitor check"
		case "--datastore":
			if op.Path == "pbs verify run" {
				flags[i].Required = false // one branch of an exclusive job-ID/datastore choice
			}
		}
	}
	args := parseArgumentSyntax(op.Path, usage)
	localEffects, remoteEffects := sideEffects(op)
	supported, unsupportedWhy, interactive := agentModeSupport(op)
	confirmation := []string{}
	if op.SafetyTier >= safety.TierReversible {
		confirmation = append(confirmation, "--yes")
	}
	if op.SafetyTier >= safety.TierDisruptive {
		confirmation = append(confirmation, "--force")
	}
	if op.RequiresTypeConfirm {
		confirmation = append(confirmation, "--confirm-target <exact target>")
	}
	if op.RequiresExpert {
		confirmation = append(confirmation, "--expert")
	}
	verification := []string{"submission outcome only; operation-specific postcondition verification is not generally available"}
	permissionNote := "Current provider permissions are determined only when the existing handler makes its request."
	if strings.HasPrefix(op.ProviderSupportNote, "Unsupported:") {
		verification = []string{"not applicable; operation is rejected as unsupported before submission"}
		permissionNote = "Not checked; the unsupported operation makes no provider request."
	}
	if op.ProducesUPID {
		verification = []string{"provider task status can be refreshed when the returned task identifier and task inspection capability are available", "task completion does not prove the target postcondition"}
	}
	if lifecyclePath(op.Path) && op.ProviderSupportNote == "" {
		verification = []string{"after task success, current guest state is inspected on the receipt-bound node", "observed desired state proves current state only, not that this request caused the change; VMID reuse cannot be excluded by providers that expose no stable guest UUID"}
	}
	switch op.Path {
	case "vm create":
		verification = []string{"after task success, VM config is read back and CPU, memory, and any requested disk storage/size are compared with the request"}
	case "container create":
		verification = []string{"after task success, container config is read back and requested CPU, memory, swap, unprivileged status, and any requested rootfs storage/size are compared with the request"}
	}
	var recovery []string
	switch {
	case strings.HasPrefix(op.Path, "maintenance "):
		switch op.Path {
		case "maintenance inventory", "maintenance status":
			recovery = []string{"use the maintenance inventory/status commands; agent receipts are not used"}
		case "maintenance plan":
			recovery = []string{"inspect the immutable plan and use maintenance report/verify/apply with its native receipt"}
		default:
			recovery = []string{"use the plan-bound maintenance report/verify/resume/reconcile/abandon workflow; agent receipts are not used"}
		}
	case strings.HasPrefix(op.Path, "certification ") && !op.Inspection:
		recovery = []string{"use certification report/cleanup and its dedicated ledger; agent receipts are not used"}
	case !supported:
		recovery = []string{"no agent receipt is created; use the operation's existing human/native workflow"}
	case op.Inspection:
		recovery = []string{"repeat the read-only inspection with a new request ID if a fresh observation is required"}
	default:
		recovery = []string{"agent receipt show", "agent receipt reconcile performs read-only reconciliation and never resubmits"}
	}
	inputSchema := operationInputSchema(op, args, flags)
	if !detailed {
		args = nil
		flags = nil
		inputSchema = nil
	}
	providerSupportNote := op.ProviderSupportNote
	if providerSupportNote == "" {
		providerSupportNote = "Provider implementation support is described by provider_interface/capabilities; this is not a live readiness or authorization check."
	}
	return OperationContract{
		ID: strings.ReplaceAll(op.Path, " ", "."), Path: op.Path, Aliases: OperationAliases(op),
		Description: op.Description, ArgumentSyntax: usage, Arguments: args, Flags: flags, InputSchema: inputSchema,
		Constraints:       operationConstraints(op),
		TargetRequirement: targetRequirement(op), LocalSideEffects: localEffects, RemoteSideEffects: remoteEffects,
		SafetyTier: op.SafetyTier.String(), Risks: riskNames(op.RiskDimensions), Confirmation: confirmation,
		ConfirmationTargetFormat: op.ConfirmTargetFormat,
		ProviderInterface:        op.CapabilityInterface, Interactive: interactive, SubmitsTask: op.ProducesUPID,
		Verification: verification, Recovery: recovery, StructuredOutput: agentStructuredOutput(supported), LegacyOutputModes: append([]string(nil), op.OutputModes...),
		AgentSupported: supported, AgentUnsupportedWhy: unsupportedWhy,
		ProviderSupportNote:    providerSupportNote,
		ProviderPermissionNote: permissionNote,
	}
}

func agentStructuredOutput(supported bool) []string {
	if supported {
		return []string{"agent-json-v1"}
	}
	return []string{}
}

func operationFlags(path, usage string, profileRequired bool) []AgentFlag {
	flags := handlerFlags[path]
	result := make([]AgentFlag, 0, len(flags.exact)+len(flags.params)+len(globalValueFlags)+len(globalBoolFlags))
	for _, name := range flags.exact {
		spec := handlerFlagDefinition(path, name)
		spec.Required = flagRequiredInUsage(usage, name)
		result = append(result, spec)
	}
	for _, name := range flags.params {
		flag := "--" + name
		spec := handlerFlagDefinition(path, flag)
		spec.ValueSyntax = flag + "=<value>"
		spec.Required = flagRequiredInUsage(usage, flag)
		result = append(result, spec)
	}
	for _, name := range globalValueFlags {
		switch name {
		case "--output":
			result = append(result, AgentFlag{Name: name, Type: "string", Default: "json", Choices: []string{"json"}, Description: "Agent mode rejects explicit non-JSON output."})
		case "--timeout":
			result = append(result, AgentFlag{Name: name, Type: "string", Format: "duration", ValueSyntax: "Go duration; must be greater than 0", Default: "30s"})
		case "--limit":
			limit := AgentFlag{Name: name, Type: "integer", Minimum: 0, ValueSyntax: "minimum 0; 0 means no limit", Default: 0}
			if path == "agent receipt list" {
				limit.Default = 20
				limit.Maximum = 100
				limit.ValueSyntax = "minimum 0; 0 selects the default 20; maximum 100"
			}
			result = append(result, limit)
		case "--request-id":
			result = append(result, AgentFlag{Name: name, Type: "string", Pattern: `^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`, ValueSyntax: "generated if omitted; otherwise ^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$"})
		default:
			required := name == "--profile" && profileRequired
			result = append(result, AgentFlag{Name: name, Type: "string", Required: required, Description: "Global CLI option; remote operations require an explicit --profile."})
		}
	}
	for _, name := range globalBoolFlags {
		if name == "--all" || name == "--password-stdin" {
			continue
		}
		spec := AgentFlag{Name: name, Type: "boolean", Default: false, Required: flagRequiredInUsage(usage, name), Description: "Existing global CLI option; safety flags are never added automatically."}
		if name == "--agent" {
			spec.Required, spec.Default = true, true
		}
		if name == "--non-interactive" {
			spec.Default = true
		}
		result = append(result, spec)
	}
	unique := make(map[string]AgentFlag, len(result))
	for _, flag := range result {
		if previous, ok := unique[flag.Name]; ok {
			previous.Required = previous.Required || flag.Required
			if previous.Default == nil {
				previous.Default = flag.Default
			}
			if len(previous.Choices) == 0 {
				previous.Choices = flag.Choices
			}
			if previous.ValueSyntax == "" {
				previous.ValueSyntax = flag.ValueSyntax
			}
			if previous.Minimum == nil {
				previous.Minimum = flag.Minimum
			}
			if previous.Maximum == nil {
				previous.Maximum = flag.Maximum
			}
			if previous.Pattern == "" {
				previous.Pattern = flag.Pattern
			}
			if previous.Format == "" {
				previous.Format = flag.Format
			}
			unique[flag.Name] = previous
			continue
		}
		unique[flag.Name] = flag
	}
	result = result[:0]
	for _, flag := range unique {
		result = append(result, flag)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func handlerFlagDefinition(path, name string) AgentFlag {
	spec := AgentFlag{Name: name, Type: "string", Description: "Validated by the existing operation handler."}
	switch name {
	case "--check", "--running", "--errors", "--follow", "--remove-credential", "--propagate":
		spec.Type = "boolean"
	case "--batch-size", "--vmid":
		spec.Type, spec.Minimum, spec.ValueSyntax = "integer", 1, "minimum 1"
	case "--expires-in":
		spec.Type, spec.ValueSyntax = "string", "Go duration"
	case "--last":
		spec.Type, spec.Minimum, spec.ValueSyntax = "integer", 0, "minimum 0; 0 uses the handler's maximum"
	case "--pos":
		spec.Type, spec.Minimum = "integer", 0
	case "--cores":
		spec.Type, spec.Minimum, spec.Maximum = "integer", 1, domain.MaxCreateCores
		spec.ValueSyntax = "integer number of virtual cores"
		if path == "vm create" {
			spec.Default = domain.DefaultVMCreateCores
		} else {
			spec.Description = "Optional; when omitted Proxmox's allocation across available host CPUs is retained."
		}
	case "--memory":
		spec.Type, spec.Minimum, spec.Maximum = "integer", 16, domain.MaxCreateMemoryMiB
		spec.ValueSyntax = "memory in MiB"
		spec.Default = domain.DefaultVMCreateMemoryMiB
	case "--swap":
		spec.Type, spec.Minimum, spec.Maximum = "integer", 0, domain.MaxCreateMemoryMiB
		spec.ValueSyntax = "swap in MiB"
		spec.Default = domain.DefaultContainerCreateSwapMiB
	case "--disk-size", "--rootfs-size":
		spec.Type, spec.Minimum, spec.Maximum = "integer", 1, domain.MaxCreateDiskSizeGiB
		spec.ValueSyntax = "size in GiB"
		if name == "--disk-size" {
			spec.Default = domain.DefaultVMCreateDiskSizeGiB
			spec.Description = "Default applies only when disk storage is supplied; otherwise no boot disk is created."
		} else {
			spec.Description = "Requires positional storage or --rootfs-storage; when omitted Proxmox's rootfs-size default applies."
		}
	case "--disk-storage":
		spec.ValueSyntax = "Proxmox storage ID"
		spec.Description = "VM boot-disk storage; cannot be combined with the positional disk-storage argument."
	case "--rootfs-storage":
		spec.ValueSyntax = "Proxmox storage ID"
		spec.Description = "Container rootfs storage; cannot be combined with the positional storage argument."
	case "--enable":
		spec.Type, spec.Choices = "integer", []string{"0", "1"}
	}
	if strings.Contains(path, "firewall rule") && name == "--action" {
		spec.Choices = []string{"accept", "deny", "reject"}
	}
	if strings.Contains(path, "firewall rule") && name == "--type" {
		spec.Choices = []string{"in", "out", "group"}
	}
	if path == "pbs snapshot list" && name == "--backup-type" {
		spec.Choices = []string{"vm", "ct", "host"}
	}
	if path == "container os-update" && name == "--policy" {
		spec.Choices = []string{"approved-full-upgrade"}
	}
	return spec
}

func flagRequiredInUsage(usage, flag string) bool {
	for _, token := range strings.Fields(usage) {
		if strings.HasPrefix(token, "[") {
			continue
		}
		token = strings.Trim(token, "]")
		if token == flag || strings.HasPrefix(token, flag+"=") {
			return true
		}
	}
	return false
}

func operationInputSchema(op OperationMeta, args []AgentArgument, flags []AgentFlag) map[string]any {
	prefix := make([]any, 0, len(args))
	requiredArgs := 0
	variadic := false
	for _, argument := range args {
		item := map[string]any{"type": "string", "title": argument.Name, "description": argument.Description}
		if len(argument.Choices) > 0 {
			item["enum"] = argument.Choices
		}
		prefix = append(prefix, item)
		if argument.Required {
			requiredArgs++
		}
		if strings.Contains(argument.Syntax, "...") {
			variadic = true
		}
	}
	argsSchema := map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Positional arguments are parsed by the existing CLI handler."}
	if len(prefix) > 0 {
		argsSchema["prefixItems"] = prefix
		argsSchema["minItems"] = requiredArgs
		if !variadic {
			argsSchema["maxItems"] = len(prefix)
		}
	}
	if op.Path == "pbs verify run" {
		argsSchema["minItems"] = 0
		argsSchema["maxItems"] = 1
	}
	flagProperties := make(map[string]any, len(flags))
	var requiredFlags []string
	for _, flag := range flags {
		property := map[string]any{"type": flag.Type}
		if flag.Default != nil {
			property["default"] = flag.Default
		}
		if flag.Minimum != nil {
			property["minimum"] = flag.Minimum
		}
		if flag.Maximum != nil {
			property["maximum"] = flag.Maximum
		}
		if flag.Pattern != "" {
			property["pattern"] = flag.Pattern
		}
		if flag.Format != "" {
			property["format"] = flag.Format
		}
		if len(flag.Choices) > 0 {
			property["enum"] = flag.Choices
		}
		if flag.ValueSyntax != "" {
			property["description"] = flag.ValueSyntax
		}
		flagProperties[flag.Name] = property
		if flag.Required {
			requiredFlags = append(requiredFlags, flag.Name)
		}
	}
	flagsSchema := map[string]any{"type": "object", "properties": flagProperties, "additionalProperties": false}
	if len(requiredFlags) > 0 {
		flagsSchema["required"] = requiredFlags
	}
	properties := map[string]any{"arguments": argsSchema, "flags": flagsSchema}
	root := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "properties": properties, "additionalProperties": false}
	var required []string
	if requiredArgs > 0 || variadic {
		required = append(required, "arguments")
	}
	if len(requiredFlags) > 0 {
		required = append(required, "flags")
	}
	if len(required) > 0 {
		root["required"] = required
	}
	if op.Path == "pbs verify run" {
		root["oneOf"] = []any{
			map[string]any{
				"required": []string{"arguments"},
				"properties": map[string]any{
					"arguments": map[string]any{"minItems": 1, "maxItems": 1},
					"flags":     map[string]any{"not": map[string]any{"required": []string{"--datastore"}}},
				},
			},
			map[string]any{
				"required": []string{"arguments", "flags"},
				"properties": map[string]any{
					"arguments": map[string]any{"maxItems": 0},
					"flags":     map[string]any{"required": []string{"--datastore"}},
				},
			},
		}
	}
	return root
}

func parseArgumentSyntax(operation, usage string) []AgentArgument {
	if usage == "" {
		return []AgentArgument{}
	}
	var result []AgentArgument
	tokens := strings.Fields(usage)
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token == "|" || token == "..." {
			continue
		}
		if strings.HasPrefix(token, "--") || strings.Contains(token, "--") {
			if usageFlagTakesValue(token) && !strings.Contains(token, "=") && i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "-") && tokens[i+1] != "|" {
				i++ // the following token is the flag's value, not a positional argument
			}
			continue
		}
		required := strings.HasPrefix(token, "<") && !strings.Contains(token, "]")
		if !strings.HasPrefix(token, "[") && !strings.HasPrefix(token, "<") {
			required = true
		}
		name := strings.Trim(token, "<>[]")
		if name == "" || strings.HasPrefix(name, "--") {
			continue
		}
		if strings.Contains(name, ">/<") {
			name = "target"
		}
		kind := "string"
		var choices []string
		if strings.Contains(name, "|") {
			choices = strings.Split(name, "|")
			kind = "string enum"
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "vmid") || name == "id" || name == "pos" || name == "size" || strings.Contains(lower, "port") || strings.Contains(lower, "batch") {
			kind = "string validated as an integer by the handler"
		}
		if !strings.HasPrefix(token, "<") && !strings.HasPrefix(token, "[") && !strings.Contains(name, "=") {
			kind = "literal or positional value as shown in argument_syntax"
		}
		if operation == "firewall rule create" && (name == "cluster|node|vm" || name == "cluster" || name == "node" || name == "vm") {
			name, kind, choices = "scope", "string enum", []string{"cluster", "node", "vm"}
		}
		result = append(result, AgentArgument{Name: name, Type: kind, Required: required, Choices: choices, Syntax: token, Description: "Accepted position and constraints are enforced by the existing CLI handler."})
	}
	return result
}

func usageFlagTakesValue(token string) bool {
	name := strings.Trim(token, "[]")
	name, _, _ = strings.Cut(name, "=")
	name = strings.TrimLeft(name, "-")
	return !isGlobalBool(name) && !optionIsBoolean("--"+name)
}

func targetRequirement(op OperationMeta) string {
	if op.Path == "certification report" {
		return "none; reads the local sanitized certification ledger"
	}
	if op.Path == "agent receipt refresh" || op.Path == "agent receipt reconcile" {
		return "explicit --profile must still match the receipt's provider, endpoint, and trust identity"
	}
	if op.Path == "profile test" || op.Path == "profile diagnose-permissions" {
		return "explicit --profile; any positional profile identifier must match it exactly"
	}
	if op.Path == "environment health" || op.Path == "environment backup-health" {
		return "named environment selects one or more configured provider profiles; agent execution is unsupported because the target set may fan out"
	}
	if op.Path == "monitor check" {
		return "exactly one configured target selected with --target; --environment fan-out is not supported in agent mode"
	}
	if strings.HasSuffix(op.Path, " migrate") || strings.HasSuffix(op.Path, " clone") || op.Path == "vm disk move" || op.Path == "backup create" || op.Path == "backup restore" {
		return "explicit profile and complete source/destination arguments; resolved secondary targets are listed in context.related_targets before submission"
	}
	if op.Path == "doctor" {
		return "checks all configured profiles and is unsupported in agent mode because it fans out across provider endpoints"
	}
	if strings.HasPrefix(op.Path, "maintenance ") {
		return "targets are defined by the configured maintenance inventory and, where applicable, an immutable plan/receipt; these workflows retain their native recovery interface"
	}
	if op.Scope == ScopeSystem && op.CapabilityInterface == "" {
		return "none; local or configuration-scoped operation"
	}
	if op.Inspection {
		return "explicit profile for remote operations; target arguments follow the operation syntax"
	}
	if op.Scope == ScopeCluster || op.Scope == ScopeFirewall || op.Scope == ScopeAccess || op.Scope == ScopeSDN || op.Scope == ScopeCeph || op.Scope == ScopeBackup || op.Scope == ScopeRepl {
		return "explicit profile and endpoint-bound cluster/provider target; include resource/node identifiers required by the operation"
	}
	return "explicit profile plus exact node/resource identifier required by the operation"
}

func sideEffects(op OperationMeta) ([]string, []string) {
	if _, dispatch := knownDispatchCommands[op.Path]; dispatch {
		return []string{}, []string{}
	}
	if strings.HasPrefix(op.ProviderSupportNote, "Unsupported:") {
		return []string{}, []string{}
	}
	switch op.Path {
	case "environment health", "environment backup-health", "monitor check", "profile test", "profile diagnose-permissions", "doctor":
		return []string{}, []string{"remote_read"}
	case "certification run", "certification cleanup":
		return []string{"local_certification_ledger_write"}, []string{"remote_mutation"}
	case "maintenance inventory", "maintenance status", "maintenance verify":
		return []string{}, []string{"remote_read"}
	case "maintenance plan":
		return []string{"local_immutable_plan_write"}, []string{"remote_read"}
	case "maintenance policy plan":
		return []string{"local_immutable_plan_write"}, []string{"remote_read"}
	case "maintenance apply", "maintenance resume":
		return []string{"local_maintenance_receipt_write"}, []string{"remote_mutation"}
	case "maintenance policy apply", "maintenance policy restore":
		return []string{"local_maintenance_receipt_write"}, []string{"remote_mutation"}
	case "maintenance reconcile":
		return []string{"local_maintenance_receipt_write"}, []string{"remote_read"}
	case "maintenance abandon":
		return []string{"local_maintenance_receipt_write"}, []string{}
	case "storage upload":
		return []string{"local_file_read"}, []string{"remote_mutation"}
	case "cluster join":
		return []string{}, []string{"remote_read"}
	case "agent receipt refresh", "agent receipt reconcile":
		return []string{"local_agent_receipt_write"}, []string{"remote_read"}
	case "agent receipt show", "agent receipt list":
		return []string{"local_receipt_read", "local_receipt_directory_create_if_missing"}, []string{}
	case "maintenance report":
		return []string{"local_maintenance_receipt_read"}, []string{}
	}
	if op.Inspection {
		if op.CapabilityInterface == "" {
			return []string{"local_read"}, []string{}
		}
		return []string{}, []string{"remote_read"}
	}
	if op.Scope == ScopeProfile || op.Scope == ScopeSystem && op.CapabilityInterface == "" {
		return []string{"local_configuration_or_receipt_write"}, []string{}
	}
	if op.Path == "storage download" {
		return []string{"local_file_write"}, []string{"remote_read"}
	}
	return []string{}, []string{"remote_mutation"}
}

func operationConstraints(op OperationMeta) []string {
	constraints := []string{"conflicting repeated global or operation flags are rejected", "--all and --password-stdin are incompatible with --agent"}
	if !op.Waitable {
		constraints = append(constraints, "--wait is not accepted for this operation")
	}
	if op.RequiresExpert {
		constraints = append(constraints, "--expert is required by the existing handler")
	}
	if op.RequiresTypeConfirm {
		constraints = append(constraints, "--confirm-target must match exactly the target string computed by the existing handler, after the required confirmation flags")
		if op.ConfirmTargetFormat != "" {
			// Publish the derivation rule so a caller can construct the value.
			// Describe the format only; the runtime target is not knowable here.
			constraints = append(constraints, "confirmation target format: "+op.ConfirmTargetFormat)
			constraints = append(constraints, "the confirmation target is resolved from the command arguments at runtime and is not predicted by this contract")
		}
	}
	if op.SafetyTier >= safety.TierDisruptive {
		constraints = append(constraints, "--force requires --yes")
	}
	if op.RequiresExpert {
		constraints = append(constraints, "--expert is additionally required; it does not replace the existing confirmation flags")
	}
	switch op.Path {
	case "pbs verify run":
		constraints = append(constraints, "provide exactly one positional job ID or --datastore <store>")
	case "firewall rule create":
		constraints = append(constraints, "action and type are required; cluster scope takes no resource ID, node scope requires a node, and VM scope requires node/VMID; remaining fields are validated by the existing firewall handler")
	case "firewall rule update", "firewall rule delete":
		constraints = append(constraints, "cluster scope uses a position; node scope requires node and position; VM scope requires node/VMID and position; scope-target is omitted only for cluster scope")
	case "access acl add":
		constraints = append(constraints, "--role is required and at least one of --user or --group is required")
	case "monitor check":
		constraints = append(constraints, "agent mode requires one --target and rejects unbounded target/environment fan-out")
	case "maintenance apply", "maintenance resume", "maintenance reconcile":
		constraints = append(constraints, "a valid immutable maintenance plan and its existing receipt are required")
	case "maintenance policy apply", "maintenance policy restore":
		constraints = append(constraints, "a valid immutable security policy plan is required")
		constraints = append(constraints, "--confirm-target must equal the policy plan ID after --yes --force")
	}
	return constraints
}

func lifecyclePath(path string) bool {
	words := strings.Fields(path)
	return len(words) == 2 && (words[0] == "vm" || words[0] == "container") && lifecycleDesiredState(words[0], words[1]) != ""
}

func agentModeSupport(op OperationMeta) (bool, string, bool) {
	if op.Path == "agent receipt refresh" || op.Path == "agent receipt reconcile" {
		return true, "", false
	}
	if strings.HasPrefix(op.ProviderSupportNote, "Unsupported:") {
		return false, strings.TrimSpace(strings.TrimPrefix(op.ProviderSupportNote, "Unsupported:")), false
	}
	interactive := op.Path == "vm console" || op.Path == "container console" || op.Path == "setup" || op.Path == "access user create" || op.SecuritySensitivity == SecCredentials
	if interactive {
		return false, "requires interactive terminal input or secret collection", true
	}
	switch op.Path {
	case "init", "profile add", "profile set-credentials", "profile use", "profile remove", "profile import", "storage download":
		return false, "local configuration/credential or local-file side effects are intentionally excluded from agent execution", false
	case "certification run", "certification cleanup":
		return false, "uses the dedicated certification authorization and recovery ledger; use its existing workflow", false
	case "cluster join":
		return false, "the existing handler performs a preflight and refuses execution without a safe peer credential", false
	case "environment health", "environment backup-health":
		return false, "may fan out across multiple configured provider profiles; agent mode requires one immutable explicit target context", false
	case "doctor":
		return false, "checks every configured profile and fans out across multiple provider endpoints", false
	case "maintenance inventory", "maintenance status":
		return false, "operates on configured multi-host inventory without one explicit immutable target context", false
	case "maintenance verify", "maintenance plan", "maintenance apply", "maintenance resume", "maintenance reconcile", "maintenance abandon", "maintenance policy plan", "maintenance policy apply", "maintenance policy restore":
		return false, "uses immutable maintenance plans and their existing durable receipt/recovery controls", false
	}
	if !op.Inspection && op.CapabilityInterface == "" {
		return false, "no provider-bound execution contract is available for this local mutation", false
	}
	return true, "", false
}

func riskNames(risks []RiskDimension) []string {
	out := make([]string, len(risks))
	for i, risk := range risks {
		out[i] = string(risk)
	}
	return out
}
