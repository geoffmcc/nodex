package cli

import "strings"

// flagSet describes the flags a command handler parses itself, which the
// global flag scanner must pass through untouched instead of treating them as
// globals or rejecting them as unknown.
type flagSet struct {
	// exact matches a space-separated --flag token as well as the inline
	// --flag=value form (handlers that accept either).
	exact []string
	// params are consumed as inline --key=value tokens only, without the
	// leading dashes in the registry. Handlers that parse --key=value styles.
	params []string
}

func (fs flagSet) owns(name string, hasInline bool) bool {
	for _, f := range fs.exact {
		if name == strings.TrimPrefix(f, "--") {
			return true
		}
	}
	if !hasInline {
		return false
	}
	for _, f := range fs.params {
		if name == f {
			return true
		}
	}
	return false
}

// handlerFlags maps command paths (joined by spaces) to the flags the
// handler owns. Keys are full leaf paths ("setup", "certification run",
// "pbs snapshot list") or dispatch sub-operations registered in
// knownDispatchCommands.
var handlerFlags = map[string]flagSet{
	"setup": {
		exact: []string{
			"--provider", "--profile", "--endpoint", "--credential-ref",
			"--ca-file", "--check", "--token", "--token-id", "--token-secret",
			"--password", "--secret",
		},
	},
	"certification run": {
		exact: []string{"--environment", "--suite", "--node", "--name", "--storage", "--vmid", "--ledger"},
	},
	"certification cleanup": {exact: []string{"--ledger"}},
	"certification report":  {exact: []string{"--ledger"}},
	"monitor check":         {exact: []string{"--target", "--environment"}},
	"container os-update":   {exact: []string{"--policy"}},
	"profile add":           {exact: []string{"--provider"}},
	"profile set-credentials": {
		exact: []string{"--backend", "--credential-name"},
	},
	"profile remove":                {exact: []string{"--remove-credential"}},
	"maintenance inventory":         {exact: maintenanceFilterFlags()},
	"maintenance status":            {exact: maintenanceFilterFlags()},
	"maintenance plan":              {exact: []string{"--environment", "--group", "--role", "--host", "--policy", "--expires-in", "--batch-size"}},
	"maintenance apply":             {exact: []string{"--plan", "--receipt-dir"}},
	"maintenance verify":            {exact: []string{"--plan", "--receipt-dir"}},
	"maintenance resume":            {exact: []string{"--plan", "--receipt"}},
	"maintenance abandon":           {exact: []string{"--receipt", "--reason"}},
	"maintenance reconcile":         {exact: []string{"--plan", "--receipt"}},
	"maintenance report":            {exact: []string{"--receipt"}},
	"firewall alias create":         {exact: []string{"--comment"}},
	"firewall ipset create":         {exact: []string{"--comment"}},
	"firewall group create":         {exact: []string{"--comment"}},
	"firewall ipset entry add":      {exact: []string{"--comment"}},
	"firewall rule create":          {params: firewallRuleParams()},
	"firewall rule update":          {params: firewallRuleParams()},
	"access acl add":                {exact: []string{"--role", "--user", "--group", "--propagate"}},
	"pbs snapshot list":             {exact: []string{"--datastore", "--namespace", "--backup-type", "--backup-id"}},
	"pbs task list":                 {exact: []string{"--running", "--errors"}},
	"pbs verify run":                {exact: []string{"--datastore"}},
	"pbs garbage-collection status": {exact: []string{"--datastore"}},
	"sdn zone create":               {params: []string{"type"}},
	"sdn vnet create":               {params: []string{"zone"}},
	"sdn subnet create":             {params: []string{"gateway"}},
}

func maintenanceFilterFlags() []string {
	return []string{"--environment", "--group", "--role", "--host", "--policy"}
}

func firewallRuleParams() []string {
	return []string{
		"action", "type", "proto", "dest", "dport", "source", "sport",
		"icmp_type", "icmp-type", "log", "comment", "iface", "macro", "enable", "pos",
	}
}

// commandFlagSet returns the flag set owned by the command the scanner is
// currently resolving. Dispatch sub-operations are resolved through
// knownDispatchCommands ('region[0]' is the operation token, e.g. "create"
// for "firewall alias create").
func commandFlagSet(path, region []string) flagSet {
	full := strings.Join(path, " ")
	if fs, ok := handlerFlags[full]; ok {
		return fs
	}
	if len(region) == 0 {
		return flagSet{}
	}
	ops, ok := knownDispatchCommands[full]
	if !ok {
		return flagSet{}
	}
	op := full + " " + region[0]
	for _, candidate := range ops {
		if candidate == op {
			if fs, ok := handlerFlags[op]; ok {
				return fs
			}
			break
		}
	}
	return flagSet{}
}

// globalBoolFlags are global flags that take no value (unless --flag=false).
var globalBoolFlags = []string{
	"--no-color", "--non-interactive", "--quiet", "--verbose", "--debug",
	"--yes", "--force", "--wait", "--expert", "--all", "--password-stdin",
}

// globalValueFlags are global flags that consume the next token as their value.
var globalValueFlags = []string{
	"--profile", "--output", "--timeout", "--limit", "--confirm-target",
}

func isGlobalBool(name string) bool {
	for _, f := range globalBoolFlags {
		if name == strings.TrimPrefix(f, "--") {
			return true
		}
	}
	return false
}

func isGlobalValue(name string) bool {
	for _, f := range globalValueFlags {
		if name == strings.TrimPrefix(f, "--") {
			return true
		}
	}
	return false
}
