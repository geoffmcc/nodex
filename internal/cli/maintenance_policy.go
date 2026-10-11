package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/ansible"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/maintenance"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
)

// runSecurityPolicyAnsible is the only seam from policy plan/apply/restore to
// the fixed embedded Ansible allowlist.
var runSecurityPolicyAnsible = func(ctx context.Context, request ansible.RunRequest) (*ansible.RunResult, error) {
	detection, err := ansible.Detect(ctx)
	if err != nil {
		return nil, app.NewExitError(fmt.Errorf("security policy requires Ansible: %w", err), app.ExitIncompatibility)
	}
	return (&ansible.Runner{Exe: detection.Path}).Run(ctx, request)
}

type policyInspection struct {
	Distribution               string   `json:"distribution"`
	DistributionRelease        string   `json:"distribution_release"`
	AllowedOrigins             []string `json:"allowed_origins"`
	OriginsPatterns            []string `json:"origins_patterns"`
	NonSecurityOriginCount     int      `json:"nonsecurity_origin_count"`
	PeriodicUpdatePackageLists string   `json:"periodic_update_package_lists"`
	PeriodicUnattendedUpgrade  string   `json:"periodic_unattended_upgrade"`
	AutomaticReboot            string   `json:"automatic_reboot"`
	APTConfigRC                int      `json:"apt_config_rc"`
	ManagedFileExists          bool     `json:"managed_file_exists"`
	ManagedFileIsSymlink       bool     `json:"managed_file_is_symlink"`
	ManagedFileOwned           bool     `json:"managed_file_owned"`
	ManagedFileChecksum        string   `json:"managed_file_checksum"`
	ManagedFileContentB64      string   `json:"managed_file_content_b64"`
	PackageInstalled           bool     `json:"package_installed"`
	TimerEnabled               string   `json:"timer_enabled"`
	TimerEnabledRC             int      `json:"timer_enabled_rc"`
	TimerActive                string   `json:"timer_active"`
	TimerActiveRC              int      `json:"timer_active_rc"`
}

func runMaintenancePolicyDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance policy <plan|apply|restore>"), app.ExitUsage)
	}
	switch args[0] {
	case "plan":
		return runMaintenancePolicyPlan(ctx, cmdCtx, args[1:])
	case "apply":
		return runMaintenancePolicyApply(ctx, cmdCtx, args[1:])
	case "restore":
		return runMaintenancePolicyRestore(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance policy <plan|apply|restore>"), app.ExitUsage)
	}
}

func runMaintenancePolicyPlan(ctx context.Context, cmdCtx *Context, args []string) error {
	filters, rest, err := parseMaintenanceFilters(args)
	if err != nil {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance policy plan [--expires-in <duration>] [--environment <env>] [--group <group>] [--role <role>] [--host <name>]"), app.ExitUsage)
	}
	expiresIn := maintenance.SecurityPolicyTTL
	for i := 0; i < len(rest); i++ {
		if rest[i] != "--expires-in" || i+1 >= len(rest) {
			return app.NewExitError(fmt.Errorf("usage: nodex maintenance policy plan [--expires-in <duration>] [--environment <env>] [--group <group>] [--role <role>] [--host <name>]"), app.ExitUsage)
		}
		duration, parseErr := time.ParseDuration(rest[i+1])
		if parseErr != nil || duration < 10*time.Minute || duration > 24*time.Hour {
			return app.NewExitError(fmt.Errorf("--expires-in must be a duration between 10m and 24h"), app.ExitUsage)
		}
		expiresIn = duration
		i++
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	selected, err := selectInventoryHosts(cfg, filters)
	if err != nil {
		return err
	}
	explicit := make(map[string]bool, len(filters.hosts))
	for _, name := range filters.hosts {
		explicit[name] = true
	}
	eligible := make(map[string]config.InventoryHost)
	excluded := make([]maintenance.SecurityPolicyExclusion, 0)
	for name, host := range selected {
		reason := ""
		switch host.Role {
		case config.RolePVE, config.RolePBS, config.RoleDNS:
			reason = "infrastructure roles are excluded from unattended security updates"
		case "":
			reason = "host has no declared role"
		case "generic":
			if !host.UnattendedSecurityUpdates {
				reason = "host is not opted in (set unattended_security_updates: true)"
			}
		default:
			if !host.UnattendedSecurityUpdates {
				reason = "host is not opted in (set unattended_security_updates: true)"
			}
		}
		if reason != "" {
			if explicit[name] {
				return app.NewExitError(fmt.Errorf("host %q is not eligible: %s", name, reason), app.ExitConfig)
			}
			excluded = append(excluded, maintenance.SecurityPolicyExclusion{Name: name, Role: host.Role, Reason: reason})
			continue
		}
		eligible[name] = host
	}
	if len(eligible) == 0 {
		return app.NewExitError(fmt.Errorf("no opted-in, non-infrastructure hosts match the policy plan; set unattended_security_updates: true in inventory"), app.ExitNotFound)
	}
	now := time.Now()
	planID, err := maintenance.NewPlanID()
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	plan := maintenance.SecurityPolicyPlan{
		Schema: maintenance.SecurityPolicyPlanSchema, PlanID: planID,
		CreatedAt: now.Unix(), ExpiresAt: now.Add(expiresIn).Unix(),
		Operation: "configure", RebootPolicy: maintenance.RebootPolicyNever,
		SafetyClassification: "disruptive", ExcludedHosts: excluded,
	}

	names := sortedInventoryNames(eligible)
	specs := hostSpecs(eligible)
	inspectionRun, runErr := runSecurityPolicyAnsible(ctx, ansible.RunRequest{Operation: "inspect-security-policy", Hosts: specs})
	for _, name := range names {
		host := eligible[name]
		policyHost := securityPolicyHostFromInventory(name, host)
		inspection, inspectErr := securityPolicyEvidence(inspectionRun, name)
		if runErr != nil {
			inspectErr = runErr
		}
		if inspectErr != nil {
			policyHost.Blockers = append(policyHost.Blockers, redact.String(output.SanitizeTerminal(inspectErr.Error())))
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("host %s policy inspection failed", name))
			plan.Hosts = append(plan.Hosts, policyHost)
			continue
		}
		populateSecurityPolicyHost(&policyHost, inspection)
		if hostBlockers := policyHost.Blockers; len(hostBlockers) > 0 {
			for _, blocker := range hostBlockers {
				plan.Blockers = append(plan.Blockers, fmt.Sprintf("host %s: %s", name, blocker))
			}
		} else if inspection.NonSecurityOriginCount > 0 {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("host %s has %d non-security unattended-upgrade origin(s); the Nodex drop-in will suppress them while enabled", name, inspection.NonSecurityOriginCount))
		}
		plan.Hosts = append(plan.Hosts, policyHost)
	}
	if err := plan.Finalize(); err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	if err := writeSecurityPolicyPlan(cmdCtx, plan); err != nil {
		return err
	}
	if len(plan.Blockers) > 0 {
		return app.NewExitError(fmt.Errorf("policy plan %s has %d blocker(s); apply will refuse until they clear", plan.PlanID, len(plan.Blockers)), app.ExitPartialFailure)
	}
	return nil
}

func sortedInventoryNames(hosts map[string]config.InventoryHost) []string {
	names := make([]string, 0, len(hosts))
	for name := range hosts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func securityPolicyHostFromInventory(name string, host config.InventoryHost) maintenance.SecurityPolicyHost {
	return maintenance.SecurityPolicyHost{
		Name: name, Address: host.Address, Role: host.Role, Environment: host.Environment,
		MaintenanceGroup: host.MaintenanceGroup, Criticality: orDefault(host.Criticality, config.CriticalityStandard),
		SSHUser: host.SSHUser, SSHPort: host.SSHPort, KeyConfigured: host.SSHKeyFile != "",
		KnownHostsConfigured: host.KnownHostsFile != "",
	}
}

func securityPolicyEvidence(run *ansible.RunResult, host string) (policyInspection, error) {
	if run == nil {
		return policyInspection{}, fmt.Errorf("ansible returned no policy inspection result")
	}
	var summary *ansible.HostResult
	for i := range run.Hosts {
		if run.Hosts[i].Host == host {
			summary = &run.Hosts[i]
			break
		}
	}
	if summary == nil || summary.Failures > 0 || summary.Unreachable > 0 {
		return policyInspection{}, fmt.Errorf("policy inspection did not successfully inspect the host")
	}
	var found *ansible.TaskOutcome
	for i := range run.TaskOutcomes[host] {
		outcome := &run.TaskOutcomes[host][i]
		if outcome.EvidenceID != ansible.SecurityPolicyEvidenceInspect {
			continue
		}
		if found != nil {
			return policyInspection{}, fmt.Errorf("policy inspection returned duplicate evidence")
		}
		found = outcome
	}
	if found == nil || !ansible.EvidenceOutcomeUsable("inspect-security-policy", *found) || found.Message == "" {
		return policyInspection{}, fmt.Errorf("policy inspection evidence is missing or unusable")
	}
	var result policyInspection
	if err := json.Unmarshal([]byte(found.Message), &result); err != nil {
		return policyInspection{}, fmt.Errorf("policy inspection evidence could not be decoded")
	}
	if result.APTConfigRC != 0 {
		return policyInspection{}, fmt.Errorf("apt-config inspection failed")
	}
	if result.NonSecurityOriginCount < 0 {
		return policyInspection{}, fmt.Errorf("APT reported an invalid origin count")
	}
	return result, nil
}

func populateSecurityPolicyHost(host *maintenance.SecurityPolicyHost, inspection policyInspection) {
	host.Distribution, host.DistributionRelease = inspection.Distribution, inspection.DistributionRelease
	host.BeforeConfigExists = inspection.ManagedFileExists
	host.BeforePackageInstalled = inspection.PackageInstalled
	var err error
	host.BeforePackageListsPeriodic, err = parseAPTBoolean(inspection.PeriodicUpdatePackageLists, "APT::Periodic::Update-Package-Lists")
	if err != nil {
		host.Blockers = append(host.Blockers, "effective APT package-list schedule could not be determined")
	}
	host.BeforeUnattendedPeriodic, err = parseAPTBoolean(inspection.PeriodicUnattendedUpgrade, "APT::Periodic::Unattended-Upgrade")
	if err != nil {
		host.Blockers = append(host.Blockers, "effective unattended-upgrade schedule could not be determined")
	}
	host.BeforeAutomaticReboot, err = parseAPTBoolean(inspection.AutomaticReboot, "Unattended-Upgrade::Automatic-Reboot")
	if err != nil {
		host.Blockers = append(host.Blockers, "effective unattended-upgrade reboot policy could not be determined")
	}
	host.BeforeTimerEnabled = normalizePolicyUnitState(inspection.TimerEnabled, inspection.TimerEnabledRC, inspection.PackageInstalled)
	host.BeforeTimerActive = normalizePolicyUnitState(inspection.TimerActive, inspection.TimerActiveRC, inspection.PackageInstalled)
	host.BeforeNonSecurityOriginCount = inspection.NonSecurityOriginCount
	host.BeforeSecurityAllowedOrigins, err = parseSecurityOriginLines(inspection.AllowedOrigins, "Unattended-Upgrade::Allowed-Origins::")
	if err != nil {
		host.Blockers = append(host.Blockers, "effective allowed-origin policy could not be safely parsed")
	}
	host.BeforeOriginsPatterns, err = parseSecurityOriginLines(inspection.OriginsPatterns, "Unattended-Upgrade::Origins-Pattern::")
	if err != nil {
		host.Blockers = append(host.Blockers, "effective origin-pattern policy could not be safely parsed")
	}
	host.SecurityAllowedOrigins = append([]string(nil), host.BeforeSecurityAllowedOrigins...)
	host.SecurityOriginsPatterns = append([]string(nil), host.BeforeOriginsPatterns...)
	if !strings.EqualFold(inspection.Distribution, "Debian") && !strings.EqualFold(inspection.Distribution, "Ubuntu") {
		host.Blockers = append(host.Blockers, "only Debian and Ubuntu are supported")
	}
	if len(host.SecurityAllowedOrigins)+len(host.SecurityOriginsPatterns) == 0 {
		patterns, fallbackErr := defaultSecurityAPTOrigins(inspection.Distribution, inspection.DistributionRelease)
		if fallbackErr != nil {
			host.Blockers = append(host.Blockers, "no default security repository could be derived from the host release")
		} else {
			host.SecurityOriginsPatterns = patterns
		}
	}
	if !validPolicyTimerState(host.BeforeTimerEnabled, true) || !validPolicyTimerState(host.BeforeTimerActive, false) {
		host.Blockers = append(host.Blockers, "the existing unattended-upgrade timer state is unsupported or could not be determined")
	}
	if inspection.ManagedFileExists {
		if inspection.ManagedFileIsSymlink {
			host.Blockers = append(host.Blockers, "the reserved Nodex policy path is a symlink and will not be followed")
			return
		}
		content, decodeErr := base64.StdEncoding.DecodeString(inspection.ManagedFileContentB64)
		if !inspection.ManagedFileOwned || decodeErr != nil || len(content) == 0 || len(content) > 64*1024 {
			host.Blockers = append(host.Blockers, "the reserved Nodex policy file could not be safely read")
		} else {
			host.BeforeConfigChecksum = maintenance.SecurityPolicyConfigChecksum(string(content))
			if inspection.ManagedFileChecksum == "" || host.BeforeConfigChecksum != inspection.ManagedFileChecksum {
				host.Blockers = append(host.Blockers, "the reserved Nodex policy file changed during inspection")
			} else if err := maintenance.ValidateSecurityPolicyConfig(string(content)); err != nil {
				host.Blockers = append(host.Blockers, "the reserved policy path contains non-canonical content and will not be overwritten")
			} else {
				host.BeforeConfig = string(content)
			}
		}
	} else if inspection.ManagedFileChecksum != "" || inspection.ManagedFileContentB64 != "" {
		host.Blockers = append(host.Blockers, "the reserved Nodex policy file changed during inspection")
	}
	if len(host.Blockers) > 0 {
		return
	}
	content, buildErr := maintenance.BuildSecurityPolicyConfig(host.SecurityAllowedOrigins, host.SecurityOriginsPatterns)
	if buildErr != nil {
		host.Blockers = append(host.Blockers, "no effective security-only APT origins were found")
		return
	}
	host.AfterConfig = content
	host.AfterConfigChecksum = maintenance.SecurityPolicyConfigChecksum(content)
}

func defaultSecurityAPTOrigins(distribution, release string) ([]string, error) {
	if release == "" || len(release) > 64 {
		return nil, fmt.Errorf("invalid distribution release")
	}
	for _, r := range release {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
		default:
			return nil, fmt.Errorf("invalid distribution release")
		}
	}
	switch strings.ToLower(distribution) {
	case "debian":
		return []string{"origin=Debian,codename=${distro_codename}-security,label=Debian-Security"}, nil
	case "ubuntu":
		return []string{
			"origin=Ubuntu,codename=${distro_codename}-security,label=Ubuntu",
			"origin=UbuntuESMApps,codename=${distro_codename}-apps-security,label=UbuntuESMApps",
			"origin=UbuntuESM,codename=${distro_codename}-infra-security,label=UbuntuESM",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported distribution %q", distribution)
	}
}

func parseSecurityOriginLines(lines []string, key string) ([]string, error) {
	origins := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, key) {
			return nil, fmt.Errorf("unexpected APT origin line")
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, key))
		if strings.HasSuffix(value, ";") {
			value = strings.TrimSpace(strings.TrimSuffix(value, ";"))
		}
		parsed, err := strconv.Unquote(value)
		if err != nil || !maintenance.IsSecurityAPTOrigin(parsed) {
			return nil, fmt.Errorf("invalid security APT origin")
		}
		origins = append(origins, parsed)
	}
	return origins, nil
}

func parseAPTBoolean(line, key string) (bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return false, nil
	}
	if !strings.HasPrefix(line, key+" ") {
		return false, fmt.Errorf("APT setting %q has an unexpected name", key)
	}
	value := strings.TrimSpace(strings.TrimPrefix(line, key))
	if strings.HasSuffix(value, ";") {
		value = strings.TrimSpace(strings.TrimSuffix(value, ";"))
	}
	value, err := strconv.Unquote(value)
	if err != nil {
		return false, fmt.Errorf("APT setting %q has an invalid value", key)
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("APT setting %q is not boolean", key)
	}
	return parsed, nil
}

func normalizePolicyUnitState(state string, rc int, packageInstalled bool) string {
	state = strings.TrimSpace(state)
	if state == "" {
		if rc != 0 && !packageInstalled {
			return "not-found"
		}
		return "unknown"
	}
	return state
}

func validPolicyTimerState(value string, enabledState bool) bool {
	if enabledState {
		return value == "enabled" || value == "disabled" || value == "not-found"
	}
	return value == "active" || value == "inactive" || value == "not-found"
}

func securityPolicyHostsMatchInventory(planHost maintenance.SecurityPolicyHost, host config.InventoryHost, requireOptIn bool) bool {
	return planHost.Address == host.Address && planHost.Role == host.Role &&
		planHost.Environment == host.Environment && planHost.MaintenanceGroup == host.MaintenanceGroup &&
		planHost.Criticality == orDefault(host.Criticality, config.CriticalityStandard) &&
		planHost.SSHUser == host.SSHUser && planHost.SSHPort == host.SSHPort &&
		planHost.KeyConfigured == (host.SSHKeyFile != "") &&
		planHost.KnownHostsConfigured == (host.KnownHostsFile != "") &&
		(!requireOptIn || host.UnattendedSecurityUpdates)
}

func writeSecurityPolicyPlan(cmdCtx *Context, plan maintenance.SecurityPolicyPlan) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, plan)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, plan)
	default:
		fmt.Fprintf(cmdCtx.Writer, "Policy plan: %s\n", plan.PlanID)
		fmt.Fprintf(cmdCtx.Writer, "Operation:   %s unattended security updates\n", plan.Operation)
		fmt.Fprintf(cmdCtx.Writer, "Reboots:     %s\n", plan.RebootPolicy)
		fmt.Fprintf(cmdCtx.Writer, "Digest:      %s\n", plan.Digest)
		rows := make([][]string, 0, len(plan.Hosts))
		for _, host := range plan.Hosts {
			state := "change"
			if host.BeforeConfig == host.AfterConfig && host.BeforePackageInstalled &&
				host.BeforePackageListsPeriodic && host.BeforeUnattendedPeriodic && !host.BeforeAutomaticReboot &&
				host.BeforeTimerEnabled == "enabled" && host.BeforeTimerActive == "active" {
				state = "already configured"
			}
			periodic := fmt.Sprintf("lists=%t unattended=%t", host.BeforePackageListsPeriodic, host.BeforeUnattendedPeriodic)
			reboot := fmt.Sprintf("%t => false", host.BeforeAutomaticReboot)
			rows = append(rows, []string{host.Name, host.Role, host.Distribution, strconv.FormatBool(host.BeforePackageInstalled), host.BeforeTimerEnabled + "/" + host.BeforeTimerActive, periodic, reboot, state})
		}
		if err := output.WriteTable(cmdCtx.Writer, []string{"HOST", "ROLE", "OS", "PACKAGE", "TIMER ENABLED/ACTIVE", "APT PERIODIC", "AUTO-REBOOT", "PLAN"}, rows); err != nil {
			return err
		}
		for _, host := range plan.Hosts {
			if host.AfterConfig != "" && host.BeforeConfig != host.AfterConfig {
				fmt.Fprintf(cmdCtx.Writer, "\n--- %s: %s\n+++ %s: %s\n", host.Name, ansible.SecurityPolicyConfigPath, host.Name, ansible.SecurityPolicyConfigPath)
				writeSecurityPolicyDiff(cmdCtx.Writer, host.BeforeConfig, host.AfterConfig)
			}
		}
		for _, excluded := range plan.ExcludedHosts {
			fmt.Fprintf(cmdCtx.Writer, "\nExcluded %s (%s): %s\n", excluded.Name, excluded.Role, excluded.Reason)
		}
		for _, warning := range plan.Warnings {
			fmt.Fprintf(cmdCtx.Writer, "\nWarning: %s\n", warning)
		}
		for _, blocker := range plan.Blockers {
			fmt.Fprintf(cmdCtx.Writer, "\nBlocker: %s\n", blocker)
		}
		fmt.Fprintf(cmdCtx.Writer, "\nTo apply: nodex --yes --force --confirm-target %s maintenance policy apply --plan <file>\n", plan.PlanID)
		return nil
	}
}

func writeSecurityPolicyDiff(w io.Writer, before, after string) {
	if before == "" {
		for _, line := range strings.Split(strings.TrimSuffix(after, "\n"), "\n") {
			fmt.Fprintf(w, "+%s\n", line)
		}
		return
	}
	for _, line := range strings.Split(strings.TrimSuffix(before, "\n"), "\n") {
		fmt.Fprintf(w, "-%s\n", line)
	}
	for _, line := range strings.Split(strings.TrimSuffix(after, "\n"), "\n") {
		fmt.Fprintf(w, "+%s\n", line)
	}
}

func runMaintenancePolicyApply(ctx context.Context, cmdCtx *Context, args []string) error {
	return runMaintenancePolicyMutation(ctx, cmdCtx, args, "apply")
}

func runMaintenancePolicyRestore(ctx context.Context, cmdCtx *Context, args []string) error {
	return runMaintenancePolicyMutation(ctx, cmdCtx, args, "restore")
}

type policyMutationArgs struct {
	planPath   string
	receiptDir string
}

func parsePolicyMutationArgs(args []string, action string) (policyMutationArgs, error) {
	var parsed policyMutationArgs
	for i := 0; i < len(args); i++ {
		if args[i] != "--plan" && args[i] != "--receipt-dir" {
			return parsed, fmt.Errorf("unknown policy %s argument %q", action, args[i])
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			return parsed, fmt.Errorf("%s requires a value", args[i])
		}
		if args[i] == "--plan" {
			parsed.planPath = args[i+1]
		} else {
			parsed.receiptDir = args[i+1]
		}
		i++
	}
	if parsed.planPath == "" {
		return parsed, fmt.Errorf("--plan is required")
	}
	if parsed.receiptDir == "" {
		parsed.receiptDir = filepath.Join(filepath.Dir(parsed.planPath), ".nodex-policy-receipts")
	}
	return parsed, nil
}

func runMaintenancePolicyMutation(ctx context.Context, cmdCtx *Context, args []string, action string) error {
	usage := fmt.Errorf("usage: nodex maintenance policy %s --plan <file> [--receipt-dir <dir>] --yes --force --confirm-target <plan-id>", action)
	parsed, err := parsePolicyMutationArgs(args, action)
	if err != nil {
		return app.NewExitError(usage, app.ExitUsage)
	}
	var plan maintenance.SecurityPolicyPlan
	if action == "restore" {
		plan, err = maintenance.LoadSecurityPolicyPlanForRestore(parsed.planPath)
	} else {
		plan, err = maintenance.LoadSecurityPolicyPlanFile(parsed.planPath, time.Now())
	}
	if err != nil {
		return app.NewExitError(fmt.Errorf("load security policy plan: %w", err), app.ExitValidationError)
	}
	if len(plan.Blockers) > 0 {
		return app.NewExitError(fmt.Errorf("policy plan %s has %d blocker(s); %s refused", plan.PlanID, len(plan.Blockers), action), app.ExitValidationError)
	}
	for _, host := range plan.Hosts {
		if len(host.Blockers) > 0 || host.AfterConfig == "" {
			return app.NewExitError(fmt.Errorf("policy plan %s contains a blocked host %q; %s refused", plan.PlanID, host.Name, action), app.ExitValidationError)
		}
	}
	if !cmdCtx.Opts.Yes || !cmdCtx.Opts.Force || cmdCtx.Opts.ConfirmTarget != plan.PlanID {
		return app.NewExitError(fmt.Errorf("confirmation refused: %s requires --yes --force --confirm-target %s", action, plan.PlanID), app.ExitUsage)
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	selected := make(map[string]config.InventoryHost, len(plan.Hosts))
	for _, planned := range plan.Hosts {
		if cfg.Inventory == nil {
			return app.NewExitError(fmt.Errorf("inventory is not configured"), app.ExitConfig)
		}
		host, ok := cfg.Inventory.Hosts[planned.Name]
		if !ok || !securityPolicyHostsMatchInventory(planned, host, action == "apply") {
			return app.NewExitError(fmt.Errorf("inventory changed for policy host %q; create a new plan", planned.Name), app.ExitConflict)
		}
		selected[planned.Name] = host
	}

	lockPath := maintenance.SecurityPolicyReceiptPath(parsed.receiptDir, plan.PlanID, action)
	if lockPath == "" {
		return app.NewExitError(fmt.Errorf("invalid policy receipt path"), app.ExitValidationError)
	}
	policyLockPath := filepath.Join(parsed.receiptDir, plan.PlanID+".policy.lock")
	lock, err := config.Lock(policyLockPath)
	if err != nil {
		return app.NewExitError(fmt.Errorf("lock policy receipt: %w", err), app.ExitConflict)
	}
	defer func() { _ = config.Unlock(lock) }()
	var priorRestoreReceipt *maintenance.Receipt
	if _, statErr := os.Lstat(lockPath); statErr == nil {
		if action != "restore" {
			return app.NewExitError(fmt.Errorf("%s receipt already exists at %s; inspect it and create a new plan", action, lockPath), app.ExitConflict)
		}
		existing, loadErr := maintenance.LoadReceipt(lockPath)
		if loadErr != nil {
			return app.NewExitError(fmt.Errorf("load prior restore receipt: %w", loadErr), app.ExitValidationError)
		}
		if err := existing.VerifyForIdentity(plan.PlanID, plan.Digest); err != nil {
			return app.NewExitError(err, app.ExitConflict)
		}
		plannedHosts := make(map[string]bool, len(plan.Hosts))
		for _, planned := range plan.Hosts {
			plannedHosts[planned.Name] = true
		}
		for _, receiptHost := range existing.Hosts {
			if !plannedHosts[receiptHost.Host] {
				return app.NewExitError(fmt.Errorf("restore receipt contains unplanned host %q", receiptHost.Host), app.ExitConflict)
			}
		}
		if existing.State == "succeeded" || existing.State == "abandoned" {
			return app.NewExitError(fmt.Errorf("restore receipt is already terminal: %s", existing.State), app.ExitConflict)
		}
		priorRestoreReceipt = &existing
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect policy receipt: %w", statErr)
	}

	applyReceiptPath := maintenance.SecurityPolicyReceiptPath(parsed.receiptDir, plan.PlanID, "apply")
	var applyReceipt maintenance.Receipt
	if action == "restore" {
		applyReceipt, err = maintenance.LoadReceipt(applyReceiptPath)
		if err != nil {
			return app.NewExitError(fmt.Errorf("restore requires the original apply receipt: %w", err), app.ExitConflict)
		}
		if err := applyReceipt.VerifyForIdentity(plan.PlanID, plan.Digest); err != nil {
			return app.NewExitError(err, app.ExitConflict)
		}
	}

	hosts := hostSpecs(selected)
	preflight, err := runSecurityPolicyAnsible(ctx, ansible.RunRequest{Operation: "inspect-security-policy", Hosts: hosts})
	if err != nil {
		return err
	}
	inspections := make(map[string]policyInspection, len(plan.Hosts))
	for _, planned := range plan.Hosts {
		inspection, inspectErr := securityPolicyEvidence(preflight, planned.Name)
		if inspectErr != nil {
			return app.NewExitError(fmt.Errorf("revalidate policy host %q: %w", planned.Name, inspectErr), app.ExitConflict)
		}
		inspections[planned.Name] = inspection
		if action == "apply" {
			if !policyStateMatchesPlanBefore(inspection, planned) {
				return app.NewExitError(fmt.Errorf("policy state changed on host %q; create a new policy plan", planned.Name), app.ExitConflict)
			}
		} else if !policyStateCanRestore(inspection, planned) && !policyStateMatchesPlanRestored(inspection, planned) {
			return app.NewExitError(fmt.Errorf("policy state on host %q matches neither the applied plan nor its prior state; restore refused", planned.Name), app.ExitConflict)
		}
	}

	receipt := maintenance.NewReceiptForIdentity(plan.PlanID, plan.Digest, time.Now())
	if priorRestoreReceipt != nil {
		receipt = *priorRestoreReceipt
		receipt.State, receipt.Error = "running", ""
		receipt.Events = append(receipt.Events, maintenance.ReceiptEvent{State: "restore_retry_started", At: time.Now().Unix()})
	}
	for _, planned := range plan.Hosts {
		state := "scheduled"
		verification := "pending"
		if action == "restore" && policyStateMatchesPlanRestored(inspections[planned.Name], planned) {
			state, verification = "succeeded", "succeeded"
		}
		hostReceipt := findReceiptHost(&receipt, planned.Name)
		if hostReceipt == nil {
			receipt.Hosts = append(receipt.Hosts, maintenance.HostReceipt{Host: planned.Name})
			hostReceipt = findReceiptHost(&receipt, planned.Name)
		}
		hostReceipt.Operation = "unattended-security-policy-" + action
		hostReceipt.State, hostReceipt.Success, hostReceipt.Verification = state, state == "succeeded", verification
		hostReceipt.FailuresDetail = nil
		hostReceipt.Warnings = nil
	}
	if action == "restore" {
		receipt.Events = append(receipt.Events, maintenance.ReceiptEvent{State: "apply_receipt_state_" + applyReceipt.State, At: time.Now().Unix()})
	}
	if err := persistPolicyReceipt(lockPath, &receipt); err != nil {
		return fmt.Errorf("write policy receipt: %w", err)
	}

	for _, planned := range plan.Hosts {
		hostReceipt := findReceiptHost(&receipt, planned.Name)
		if hostReceipt == nil {
			return app.NewExitError(fmt.Errorf("policy receipt is missing host %q", planned.Name), app.ExitValidationError)
		}
		if hostReceipt.State == "succeeded" {
			continue
		}
		host := selected[planned.Name]
		hostReceipt.State = "running"
		receipt.Events = append(receipt.Events, maintenance.ReceiptEvent{Host: planned.Name, State: "running", At: time.Now().Unix()})
		if err := persistPolicyReceipt(lockPath, &receipt); err != nil {
			return fmt.Errorf("persist policy receipt: %w", err)
		}
		var request ansible.RunRequest
		if action == "apply" {
			request = ansible.RunRequest{
				Operation: "apply-security-policy", Hosts: hostSpecs(map[string]config.InventoryHost{planned.Name: host}),
				PolicyPlanID: plan.PlanID, PolicyConfig: planned.AfterConfig,
				PolicyBeforeExists: planned.BeforeConfigExists, PolicyBeforeChecksum: planned.BeforeConfigChecksum,
			}
		} else {
			request = ansible.RunRequest{
				Operation: "restore-security-policy", Hosts: hostSpecs(map[string]config.InventoryHost{planned.Name: host}),
				PolicyPlanID: plan.PlanID, PolicyBeforeExists: planned.BeforeConfigExists,
				PolicyBeforeChecksum:  planned.BeforeConfigChecksum,
				PolicyCurrentExists:   inspections[planned.Name].ManagedFileExists,
				PolicyCurrentChecksum: inspections[planned.Name].ManagedFileChecksum,
				PolicyTimerWasEnabled: planned.BeforeTimerEnabled, PolicyTimerWasActive: planned.BeforeTimerActive,
			}
		}
		result, runErr := runSecurityPolicyAnsible(ctx, request)
		if runErr != nil || !policyRunHostSucceeded(result, planned.Name, ansible.SecurityPolicyEvidenceApply, ansible.SecurityPolicyEvidenceRestore) {
			hostReceipt.State, hostReceipt.Success, hostReceipt.Verification = policyFailureState(result, runErr), false, "unknown"
			hostReceipt.FailuresDetail = []string{safePolicyRunError(result, runErr)}
			receipt.State = hostReceipt.State
			receipt.Error = fmt.Sprintf("host %s policy %s did not complete with verified Ansible evidence", planned.Name, action)
			markPolicyHostsNotStarted(&receipt, planned.Name)
			if err := persistPolicyReceipt(lockPath, &receipt); err != nil {
				return err
			}
			_ = writeSecurityPolicyReceipt(cmdCtx, receipt)
			return app.NewExitError(fmt.Errorf("policy %s stopped; receipt: %s", action, lockPath), app.ExitAmbiguousOutcome)
		}
		verification, verifyErr := runSecurityPolicyAnsible(ctx, ansible.RunRequest{
			Operation: "inspect-security-policy", Hosts: hostSpecs(map[string]config.InventoryHost{planned.Name: host}),
		})
		verified := false
		var verifiedState policyInspection
		if verifyErr == nil {
			actual, inspectErr := securityPolicyEvidence(verification, planned.Name)
			if inspectErr == nil {
				verifiedState = actual
				if action == "apply" {
					verified = policyStateMatchesPlanApplied(actual, planned)
				} else {
					verified = policyStateMatchesPlanRestored(actual, planned)
				}
			}
		}
		if !verified {
			hostReceipt.State, hostReceipt.Success, hostReceipt.Verification = "unknown", false, "failed"
			hostReceipt.FailuresDetail = []string{"post-change policy state did not match the plan"}
			receipt.State = "unknown"
			receipt.Error = fmt.Sprintf("host %s policy %s postcondition could not be verified", planned.Name, action)
			markPolicyHostsNotStarted(&receipt, planned.Name)
			if err := persistPolicyReceipt(lockPath, &receipt); err != nil {
				return err
			}
			_ = writeSecurityPolicyReceipt(cmdCtx, receipt)
			return app.NewExitError(fmt.Errorf("policy %s verification failed; receipt: %s", action, lockPath), app.ExitPartialFailure)
		}
		hostReceipt.State, hostReceipt.Success, hostReceipt.Verification = "succeeded", true, "succeeded"
		hostReceipt.Changed = 1
		hostReceipt.Warnings = append(hostReceipt.Warnings, fmt.Sprintf("APT periodic: update-lists=%t unattended-upgrade=%t automatic-reboot=%t; apt-daily-upgrade.timer enabled=%s active=%s; package installed=%t", aptBooleanValue(verifiedState.PeriodicUpdatePackageLists, "APT::Periodic::Update-Package-Lists"), aptBooleanValue(verifiedState.PeriodicUnattendedUpgrade, "APT::Periodic::Unattended-Upgrade"), aptBooleanValue(verifiedState.AutomaticReboot, "Unattended-Upgrade::Automatic-Reboot"), verifiedState.TimerEnabled, verifiedState.TimerActive, verifiedState.PackageInstalled))
		if action == "restore" {
			hostReceipt.Warnings = append(hostReceipt.Warnings, "unattended-upgrades package was retained; the prior policy file and timer state were restored")
		}
		receipt.Events = append(receipt.Events, maintenance.ReceiptEvent{Host: planned.Name, State: "succeeded", At: time.Now().Unix()})
		if err := persistPolicyReceipt(lockPath, &receipt); err != nil {
			return fmt.Errorf("persist policy receipt: %w", err)
		}
	}
	receipt.State, receipt.Error = "succeeded", ""
	if err := persistPolicyReceipt(lockPath, &receipt); err != nil {
		return err
	}
	return writeSecurityPolicyReceipt(cmdCtx, receipt)
}

func policyRunHostSucceeded(result *ansible.RunResult, host string, evidenceIDs ...string) bool {
	if result == nil || !result.EvidenceComplete {
		return false
	}
	for _, summary := range result.Hosts {
		if summary.Host == host && (summary.Failures != 0 || summary.Unreachable != 0 || summary.Failed) {
			return false
		}
	}
	for _, id := range evidenceIDs {
		for _, outcome := range result.TaskOutcomes[host] {
			if outcome.EvidenceID == id && ansible.EvidenceOutcomeUsable(result.Operation, outcome) {
				return true
			}
		}
	}
	return false
}

func policyFailureState(result *ansible.RunResult, err error) string {
	if err != nil || result == nil || result.ParseError != "" || !result.EvidenceComplete {
		return "unknown"
	}
	for _, host := range result.Hosts {
		if host.Failures > 0 {
			return "failed"
		}
		if host.Unreachable > 0 || host.Failed {
			return "unknown"
		}
	}
	return "failed"
}

func safePolicyRunError(result *ansible.RunResult, err error) string {
	if err != nil {
		return redact.String(output.SanitizeTerminal(err.Error()))
	}
	if result != nil && result.ParseError != "" {
		return redact.String(output.SanitizeTerminal(result.ParseError))
	}
	return "Ansible returned incomplete or failed policy evidence"
}

func policyStateMatchesPlanBefore(actual policyInspection, planned maintenance.SecurityPolicyHost) bool {
	periodicLists, unattended, automaticReboot, settingsErr := policyAPTSettings(actual)
	if settingsErr != nil || actual.Distribution != planned.Distribution || actual.DistributionRelease != planned.DistributionRelease || actual.PackageInstalled != planned.BeforePackageInstalled ||
		periodicLists != planned.BeforePackageListsPeriodic || unattended != planned.BeforeUnattendedPeriodic || automaticReboot != planned.BeforeAutomaticReboot ||
		actual.NonSecurityOriginCount != planned.BeforeNonSecurityOriginCount ||
		normalizePolicyUnitState(actual.TimerEnabled, actual.TimerEnabledRC, actual.PackageInstalled) != planned.BeforeTimerEnabled ||
		normalizePolicyUnitState(actual.TimerActive, actual.TimerActiveRC, actual.PackageInstalled) != planned.BeforeTimerActive ||
		actual.ManagedFileExists != planned.BeforeConfigExists || actual.ManagedFileIsSymlink || (actual.ManagedFileExists && !actual.ManagedFileOwned) {
		return false
	}
	allowed, err1 := parseSecurityOriginLines(actual.AllowedOrigins, "Unattended-Upgrade::Allowed-Origins::")
	patterns, err2 := parseSecurityOriginLines(actual.OriginsPatterns, "Unattended-Upgrade::Origins-Pattern::")
	if err1 != nil || err2 != nil || !sameStrings(allowed, planned.BeforeSecurityAllowedOrigins) || !sameStrings(patterns, planned.BeforeOriginsPatterns) {
		return false
	}
	if planned.BeforeConfigExists {
		return actual.ManagedFileOwned && actual.ManagedFileChecksum == planned.BeforeConfigChecksum
	}
	return !actual.ManagedFileExists && actual.ManagedFileChecksum == "" && actual.ManagedFileContentB64 == ""
}

func policyStateMatchesPlanApplied(actual policyInspection, planned maintenance.SecurityPolicyHost) bool {
	periodicLists, unattended, automaticReboot, settingsErr := policyAPTSettings(actual)
	if settingsErr != nil || actual.Distribution != planned.Distribution || actual.DistributionRelease != planned.DistributionRelease || !actual.PackageInstalled || !actual.ManagedFileOwned || actual.ManagedFileIsSymlink ||
		!periodicLists || !unattended || automaticReboot ||
		actual.ManagedFileChecksum != planned.AfterConfigChecksum || !actual.ManagedFileExists ||
		!timerEnabled(actual.TimerEnabled) || strings.ToLower(strings.TrimSpace(actual.TimerActive)) != "active" || actual.NonSecurityOriginCount != 0 {
		return false
	}
	allowed, err1 := parseSecurityOriginLines(actual.AllowedOrigins, "Unattended-Upgrade::Allowed-Origins::")
	patterns, err2 := parseSecurityOriginLines(actual.OriginsPatterns, "Unattended-Upgrade::Origins-Pattern::")
	return err1 == nil && err2 == nil && sameStrings(allowed, planned.SecurityAllowedOrigins) && sameStrings(patterns, planned.SecurityOriginsPatterns)
}

func policyStateCanRestore(actual policyInspection, planned maintenance.SecurityPolicyHost) bool {
	if actual.Distribution != planned.Distribution || actual.DistributionRelease != planned.DistributionRelease || actual.ManagedFileIsSymlink {
		return false
	}
	currentIsBefore := (planned.BeforeConfigExists && actual.ManagedFileExists && actual.ManagedFileOwned && actual.ManagedFileChecksum == planned.BeforeConfigChecksum) ||
		(!planned.BeforeConfigExists && !actual.ManagedFileExists && actual.ManagedFileChecksum == "")
	currentIsAfter := actual.ManagedFileExists && actual.ManagedFileOwned && actual.ManagedFileChecksum == planned.AfterConfigChecksum
	if !currentIsBefore && !currentIsAfter {
		return false
	}
	enabled := normalizePolicyUnitState(actual.TimerEnabled, actual.TimerEnabledRC, actual.PackageInstalled)
	active := normalizePolicyUnitState(actual.TimerActive, actual.TimerActiveRC, actual.PackageInstalled)
	if !validPolicyTimerState(enabled, true) || !validPolicyTimerState(active, false) {
		return false
	}
	wasEnabled := timerEnabled(planned.BeforeTimerEnabled)
	currentEnabledIsBefore := timerEnabled(enabled) == wasEnabled
	currentEnabledIsApplied := timerEnabled(enabled)
	wasActive := strings.EqualFold(planned.BeforeTimerActive, "active")
	currentActiveIsBefore := strings.EqualFold(active, "active") == wasActive
	currentActiveIsApplied := strings.EqualFold(active, "active")
	return (currentEnabledIsBefore || currentEnabledIsApplied) && (currentActiveIsBefore || currentActiveIsApplied)
}

func policyStateMatchesPlanRestored(actual policyInspection, planned maintenance.SecurityPolicyHost) bool {
	if actual.Distribution != planned.Distribution || actual.DistributionRelease != planned.DistributionRelease ||
		(planned.BeforePackageInstalled && !actual.PackageInstalled) || actual.ManagedFileExists != planned.BeforeConfigExists || actual.ManagedFileIsSymlink {
		return false
	}
	periodicLists, unattended, automaticReboot, settingsErr := policyAPTSettings(actual)
	if settingsErr != nil || (planned.BeforePackageInstalled && (periodicLists != planned.BeforePackageListsPeriodic || unattended != planned.BeforeUnattendedPeriodic)) ||
		automaticReboot != planned.BeforeAutomaticReboot {
		return false
	}
	if planned.BeforeConfigExists {
		if !actual.ManagedFileOwned || actual.ManagedFileChecksum != planned.BeforeConfigChecksum {
			return false
		}
	} else if actual.ManagedFileChecksum != "" || actual.ManagedFileContentB64 != "" {
		return false
	}
	if timerEnabled(actual.TimerEnabled) != timerEnabled(planned.BeforeTimerEnabled) {
		return false
	}
	if (strings.ToLower(strings.TrimSpace(actual.TimerActive)) == "active") != (strings.ToLower(strings.TrimSpace(planned.BeforeTimerActive)) == "active") {
		return false
	}
	return true
}

func timerEnabled(state string) bool {
	return state == "enabled" || state == "enabled-runtime"
}

func aptBooleanValue(line, key string) bool {
	value, err := parseAPTBoolean(line, key)
	return err == nil && value
}

func policyAPTSettings(actual policyInspection) (periodicLists, unattended, automaticReboot bool, err error) {
	if periodicLists, err = parseAPTBoolean(actual.PeriodicUpdatePackageLists, "APT::Periodic::Update-Package-Lists"); err != nil {
		return false, false, false, err
	}
	if unattended, err = parseAPTBoolean(actual.PeriodicUnattendedUpgrade, "APT::Periodic::Unattended-Upgrade"); err != nil {
		return false, false, false, err
	}
	if automaticReboot, err = parseAPTBoolean(actual.AutomaticReboot, "Unattended-Upgrade::Automatic-Reboot"); err != nil {
		return false, false, false, err
	}
	return periodicLists, unattended, automaticReboot, nil
}

func sameStrings(a, b []string) bool {
	left, right := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(left)
	sort.Strings(right)
	left, right = uniqueStrings(left), uniqueStrings(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func uniqueStrings(sorted []string) []string {
	result := sorted[:0]
	for _, value := range sorted {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func findReceiptHost(receipt *maintenance.Receipt, name string) *maintenance.HostReceipt {
	if receipt == nil {
		return nil
	}
	for i := range receipt.Hosts {
		if receipt.Hosts[i].Host == name {
			return &receipt.Hosts[i]
		}
	}
	return nil
}

func markPolicyHostsNotStarted(receipt *maintenance.Receipt, failedHost string) {
	for i := range receipt.Hosts {
		if receipt.Hosts[i].Host != failedHost && receipt.Hosts[i].State == "scheduled" {
			receipt.Hosts[i].State = "not_started"
			receipt.Hosts[i].Verification = "unknown"
		}
	}
}

func persistPolicyReceipt(path string, receipt *maintenance.Receipt) error {
	receipt.UpdatedAt = time.Now().Unix()
	if err := receipt.Finalize(); err != nil {
		return err
	}
	return maintenance.SaveReceipt(path, *receipt)
}

func writeSecurityPolicyReceipt(cmdCtx *Context, receipt maintenance.Receipt) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, receipt)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, receipt)
	default:
		rows := make([][]string, 0, len(receipt.Hosts))
		for _, host := range receipt.Hosts {
			detail := append(append([]string(nil), host.Warnings...), host.FailuresDetail...)
			rows = append(rows, []string{host.Host, host.State, host.Verification, strings.Join(detail, "; ")})
		}
		fmt.Fprintf(cmdCtx.Writer, "Policy plan: %s\nReceipt: %s\nState: %s\n", receipt.PlanID, receipt.ReceiptID, receipt.State)
		return output.WriteTable(cmdCtx.Writer, []string{"HOST", "STATE", "VERIFICATION", "DETAIL"}, rows)
	}
}
