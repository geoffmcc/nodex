package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// helpEntry carries per-leaf help text used by printCommandHelp.
//
//   - desc:     one-line description; empty means the tree command's short text
//     (dispatch operations always need a desc).
//   - usage:    the argument portion that follows "nodex <path>"; empty means
//     the command takes no arguments.
//   - examples: optional example invocations (the "nodex <path>" prefix is
//     prepended automatically).
type helpEntry struct {
	desc     string
	usage    string
	examples []string
}

// leafHelp seeds per-leaf help with concrete usage text. Commands absent from
// this map fall back to a generic "nodex <path> [arguments]" line.
var leafHelp = map[string]helpEntry{
	// Lifecycle / configuration leaves (tree-registered).
	"init":                         {usage: ""},
	"setup":                        {usage: "[--provider name] [--profile name] [--endpoint https://host:port] [--credential-ref backend:name] [--ca-file path] [--check]", examples: []string{"--provider proxmox --endpoint https://nodex.local:8006"}},
	"completion":                   {usage: "<bash|zsh|fish|powershell>"},
	"version":                      {usage: ""},
	"version parse":                {usage: "<version>", examples: []string{"8.2.4"}},
	"version compare":              {usage: "<v1> <v2>", examples: []string{"8.2.4 8.2.5"}},
	"doctor":                       {usage: ""},
	"status":                       {usage: ""},
	"node list":                    {usage: ""},
	"node show":                    {usage: "<name>"},
	"node status":                  {usage: "<name>"},
	"node services":                {usage: "<node>"},
	"node network":                 {usage: "<node>"},
	"node dns":                     {usage: "<node>"},
	"node time":                    {usage: "<node>"},
	"node disks":                   {usage: "<node>"},
	"node certificates":            {usage: "<node>"},
	"node subscription":            {usage: "<node>"},
	"node updates":                 {usage: "<node>"},
	"vm list":                      {usage: "", desc: "List all virtual machines"},
	"vm show":                      {usage: "<id>"},
	"vm config":                    {usage: "<node>/<vmid>"},
	"vm snapshots":                 {usage: "<node>/<vmid>"},
	"vm snapshot-config":           {usage: "<node>/<vmid> <name>"},
	"vm start":                     {usage: "<node>/<vmid>"},
	"vm stop":                      {usage: "<node>/<vmid>"},
	"vm shutdown":                  {usage: "<node>/<vmid>"},
	"vm reset":                     {usage: "<node>/<vmid>"},
	"vm reboot":                    {usage: "<node>/<vmid>"},
	"vm suspend":                   {usage: "<node>/<vmid>"},
	"vm resume":                    {usage: "<node>/<vmid>"},
	"vm pause":                     {usage: "<node>/<vmid>"},
	"vm unpause":                   {usage: "<node>/<vmid>"},
	"vm update":                    {usage: "<node>/<vmid> <key=value>"},
	"vm delete":                    {usage: "<node>/<vmid>"},
	"vm cloud-init":                {usage: "<node>/<vmid>"},
	"vm template":                  {usage: "<node>/<vmid>"},
	"vm migrate":                   {usage: "<node>/<vmid> <target> [online]"},
	"vm clone":                     {usage: "<node>/<vmid> <new-vmid> <name> [storage]"},
	"vm create":                    {usage: "<node> <vmid> [name] [iso] [disk-storage]"},
	"vm console":                   {usage: "<node>/<vmid>"},
	"task list":                    {usage: "<node>"},
	"task show":                    {usage: "<node> <upid>"},
	"container list":               {usage: ""},
	"container show":               {usage: "<id>"},
	"container config":             {usage: "<node>/<vmid>"},
	"container snapshots":          {usage: "<node>/<vmid>"},
	"container snapshot-config":    {usage: "<node>/<vmid> <name>"},
	"container start":              {usage: "<node>/<vmid>"},
	"container stop":               {usage: "<node>/<vmid>"},
	"container shutdown":           {usage: "<node>/<vmid>"},
	"container reboot":             {usage: "<node>/<vmid>"},
	"container suspend":            {usage: "<node>/<vmid>"},
	"container resume":             {usage: "<node>/<vmid>"},
	"container update":             {usage: "<node>/<vmid> <key=value>"},
	"container os-update":          {usage: "<node>/<vmid> --policy security-only|full-upgrade"},
	"container delete":             {usage: "<node>/<vmid>"},
	"container template":           {usage: "<node>/<vmid>"},
	"container migrate":            {usage: "<node>/<vmid> <target>"},
	"container clone":              {usage: "<node>/<vmid> <new-vmid> <name> [storage]"},
	"container create":             {usage: "<node> <vmid> <ostemplate> [hostname] [storage]"},
	"container restore":            {usage: "<node> <vmid> <archive> [storage]"},
	"container console":            {usage: "<node>/<vmid>"},
	"storage list":                 {usage: ""},
	"storage show":                 {usage: "<name>"},
	"storage content":              {usage: "<node> <storage>"},
	"storage upload":               {usage: "<node> <storage> <local-file>"},
	"storage download":             {usage: "<node> <storage> <volume-id> <local-path>"},
	"storage delete":               {usage: "<node> <storage> <volume-id>"},
	"cluster status":               {usage: ""},
	"cluster log":                  {usage: ""},
	"cluster init":                 {usage: "<name> <bind-address>"},
	"cluster join":                 {usage: "<node-address> <fingerprint>"},
	"event list":                   {usage: ""},
	"log":                          {usage: "<node>"},
	"pools list":                   {usage: ""},
	"network show":                 {usage: "<node>"},
	"network apply":                {usage: "<node>"},
	"network revert":               {usage: "<node>"},
	"monitor targets":              {usage: ""},
	"monitor check":                {usage: "[--target <name>] [--environment <name>]"},
	"backup list":                  {usage: "<node>"},
	"backup content":               {usage: "<node> <storage>"},
	"backup create":                {usage: "<node>/<vmid> <storage> <mode>"},
	"backup restore":               {usage: "<node> <new-vmid> <archive> [storage]"},
	"firewall list":                {usage: ""},
	"firewall aliases":             {usage: ""},
	"firewall ipsets":              {usage: ""},
	"firewall security-groups":     {usage: ""},
	"firewall options":             {usage: ""},
	"firewall node-rules":          {usage: "<node>"},
	"firewall vm-rules":            {usage: "<node>/<vmid>"},
	"ha list":                      {usage: ""},
	"ha groups":                    {usage: ""},
	"ha status":                    {usage: ""},
	"ha current":                   {usage: ""},
	"sdn zones":                    {usage: ""},
	"sdn vnets":                    {usage: ""},
	"environment list":             {usage: ""},
	"environment health":           {usage: "<name>"},
	"environment backup-health":    {usage: "<name>"},
	"pbs status":                   {usage: ""},
	"pbs version":                  {usage: ""},
	"pbs subscription":             {usage: ""},
	"pbs certificates":             {usage: ""},
	"ceph status":                  {usage: "<node>"},
	"replication list":             {usage: ""},
	"replication show":             {usage: "<id>"},
	"replication create":           {usage: "<id> <guest> <type> <target> [key=value...]"},
	"replication update":           {usage: "<id> [key=value...]"},
	"replication delete":           {usage: "<id>"},
	"replication schedule":         {usage: "<node> <id>"},
	"profile list":                 {usage: ""},
	"profile show":                 {usage: "<name>"},
	"profile use":                  {usage: "<name>"},
	"profile current":              {usage: ""},
	"profile test":                 {usage: "[name]"},
	"profile add":                  {usage: "<name> [--provider <provider>]"},
	"profile set-credentials":      {usage: "<name> [--backend file|keyring] [--credential-name name]"},
	"profile remove":               {usage: "<name> [--remove-credential]"},
	"profile export":               {usage: "<name>"},
	"profile import":               {usage: "<name>"},
	"profile diagnose-permissions": {usage: "<profile>"},
	"provider list":                {usage: ""},
	"provider capabilities":        {usage: "<name>"},
	"certification run":            {usage: "--environment <name> --suite <readonly|disposable-mutations> --node <node> --vmid <id> --name nodex-cert-<name> --storage <storage> [--ledger <path>]"},
	"certification cleanup":        {usage: "[--ledger <path>]"},
	"certification report":         {usage: "[--ledger <path>]"},

	// Dispatch group leaves.
	"vm snapshot create":          {desc: "Create a VM snapshot", usage: "<node>/<vmid> <name> [description]"},
	"vm snapshot delete":          {desc: "Delete a VM snapshot", usage: "<node>/<vmid> <name>"},
	"vm snapshot rollback":        {desc: "Roll back a VM to a snapshot", usage: "<node>/<vmid> <name>"},
	"container snapshot create":   {desc: "Create a container snapshot", usage: "<node>/<vmid> <name> [description]"},
	"container snapshot delete":   {desc: "Delete a container snapshot", usage: "<node>/<vmid> <name>"},
	"container snapshot rollback": {desc: "Roll back a container to a snapshot", usage: "<node>/<vmid> <name>"},
	"vm disk resize":              {desc: "Resize a VM disk", usage: "<node>/<vmid> <disk> <size>"},
	"vm disk move":                {desc: "Move a VM disk to another storage", usage: "<node>/<vmid> <disk> <storage>"},
	"firewall rule create": {
		desc:  "Create a firewall rule",
		usage: "cluster --action <accept|deny|reject> --type <in|out|group> [options]",
	},
	"firewall rule update":        {desc: "Update a firewall rule", usage: "cluster <pos> [key=value ...]"},
	"firewall rule delete":        {desc: "Delete a firewall rule", usage: "cluster <pos>"},
	"firewall alias create":       {desc: "Create a firewall alias", usage: "<name> <cidr> [--comment \"...\"]"},
	"firewall alias delete":       {desc: "Delete a firewall alias", usage: "<name>"},
	"firewall ipset create":       {desc: "Create a firewall IP set", usage: "<name> [--comment \"...\"]"},
	"firewall ipset entry add":    {desc: "Add an entry to a firewall IP set", usage: "<name> <cidr> [--comment \"...\"]"},
	"firewall ipset entry remove": {desc: "Remove an entry from a firewall IP set", usage: "<name> <cidr>"},
	"firewall ipset delete":       {desc: "Delete a firewall IP set", usage: "<name>"},
	"firewall group create":       {desc: "Create a firewall security group", usage: "<name> [--comment \"...\"]"},
	"firewall group delete":       {desc: "Delete a firewall security group", usage: "<name>"},
	"firewall options update":     {desc: "Update firewall options", usage: "enable=<0|1> [policy_in=<a>] [policy_out=<a>] [log_in_drop=<0|1>] [log_ratelimit=<s>] [nf_conntrack_max=<n>] [digest=<s>]"},
	"backup job list":             {desc: "List backup job schedules", usage: ""},
	"backup job show":             {desc: "Show a backup job schedule", usage: "<id>"},
	"backup job create":           {desc: "Create a backup job schedule", usage: "storage=<name> mode=<mode> starttime=<HH:MM> [node=<n>] [vmid=<id>] [dow=<d>] [compress=<c>] [comment=<c>] [mailnotification=<m>] [mailto=<m>] [prune-backups=<p>] [pool=<p>]"},
	"backup job update":           {desc: "Update a backup job schedule", usage: "<id> [key=value ...]"},
	"backup job delete":           {desc: "Delete a backup job schedule", usage: "<id>"},
	"sdn zone create":             {desc: "Create an SDN zone", usage: "<name> --type <type>"},
	"sdn zone delete":             {desc: "Delete an SDN zone", usage: "<name>"},
	"sdn vnet create":             {desc: "Create an SDN VNet", usage: "<name> --zone <zone>"},
	"sdn vnet delete":             {desc: "Delete an SDN VNet", usage: "<name>"},
	"sdn subnet create":           {desc: "Create an SDN subnet", usage: "<vnet> <cidr> --gateway <gw>"},
	"sdn subnet delete":           {desc: "Delete an SDN subnet", usage: "<vnet> <subnet>"},
	"sdn controller create":       {desc: "Create an SDN controller", usage: "<name>"},
	"sdn controller delete":       {desc: "Delete an SDN controller", usage: "<name>"},
	"ceph osd list":               {desc: "List Ceph OSDs", usage: "<node>"},
	"ceph osd create":             {desc: "Create a Ceph OSD", usage: "<node> <dev>"},
	"ceph osd out":                {desc: "Mark a Ceph OSD out", usage: "<node> <id>"},
	"ceph osd in":                 {desc: "Mark a Ceph OSD in", usage: "<node> <id>"},
	"ceph osd destroy":            {desc: "Destroy a Ceph OSD", usage: "<node> <id>"},
	"ceph mon list":               {desc: "List Ceph monitors", usage: "<node>"},
	"ceph pool list":              {desc: "List Ceph pools", usage: "<node>"},
	"ceph pool create":            {desc: "Create a Ceph pool", usage: "<node> <name> [key=value...]"},
	"ceph pool destroy":           {desc: "Destroy a Ceph pool", usage: "<node> <name>"},
	"access user create":          {desc: "Create an access user", usage: "<userid> [email=<e>] [firstname=<f>] [lastname=<l>] [comment=<c>]", examples: []string{"tom email=tom@example.com"}},
	"access user delete":          {desc: "Delete an access user", usage: "<userid>"},
	"access users list":           {desc: "List access users", usage: ""},
	"access groups list":          {desc: "List access groups", usage: ""},
	"access roles list":           {desc: "List access roles", usage: ""},
	"access acl list":             {desc: "List ACL entries", usage: ""},
	"access acl add":              {desc: "Add an ACL entry", usage: "<path> --role <role> [--user <id>] [--group <id>] [--propagate]"},
	"access domains list":         {desc: "List authentication domains", usage: ""},
	"access tokens list":          {desc: "List API tokens for a user", usage: "<user>"},

	// PBS dispatch leaves.
	"pbs datastore list":            {desc: "List PBS datastores", usage: ""},
	"pbs datastore show":            {desc: "Show PBS datastore details", usage: "<datastore>"},
	"pbs snapshot list":             {desc: "List PBS backup snapshots", usage: "--datastore <store> [--namespace <ns>] [--backup-type vm|ct|host] [--backup-id <id>]"},
	"pbs task list":                 {desc: "List PBS tasks", usage: "[--running] [--errors]"},
	"pbs task show":                 {desc: "Show a PBS task", usage: "<upid>"},
	"pbs task log":                  {desc: "Show a PBS task log", usage: "<upid>"},
	"pbs verify list":               {desc: "List PBS verification jobs", usage: ""},
	"pbs verify run":                {desc: "Run PBS verification", usage: "<job-id> | --datastore <store>"},
	"pbs prune list":                {desc: "List PBS prune jobs", usage: ""},
	"pbs prune run":                 {desc: "Run a PBS prune job", usage: "<job-id>"},
	"pbs sync list":                 {desc: "List PBS sync jobs", usage: ""},
	"pbs sync run":                  {desc: "Run a PBS sync job", usage: "<job-id>"},
	"pbs garbage-collection status": {desc: "Show PBS garbage collection status", usage: "[--datastore <store>]"},
	"pbs garbage-collection run":    {desc: "Run PBS garbage collection", usage: "<datastore>"},

	// Maintenance leaves.
	"maintenance inventory": {usage: "[--environment <env>] [--group <group>] [--role <role>] [--host <name>]"},
	"maintenance status":    {usage: "[--environment <env>] [--group <group>] [--role <role>] [--host <name>]"},
	"maintenance plan":      {usage: "--policy security-only|approved-full-upgrade [--expires-in <duration>] [--batch-size <n>] [--environment <env>] [--group <group>] [--role <role>] [--host <name>]"},
	"maintenance apply":     {usage: "--plan <file> [--receipt-dir <dir>]"},
	"maintenance verify":    {usage: "--plan <file>"},
	"maintenance resume":    {usage: "--plan <file> --receipt <file>"},
	"maintenance reconcile": {usage: "--plan <file> --receipt <file>"},
	"maintenance abandon":   {usage: "--receipt <file> --reason <reason> --yes --force --confirm-target <receipt-id>"},
	"maintenance report":    {usage: "--receipt <file>"},
}

// flagDisplay returns the flags a command owns as a sort-stable list of
// --flag tokens (value flags beyond the exact sets are noted inline).
func flagDisplay(fs flagSet) []string {
	out := make([]string, 0, len(fs.exact)+len(fs.params))
	out = append(out, fs.exact...)
	for _, p := range fs.params {
		out = append(out, "--"+p+"=<value>")
	}
	sort.Strings(out)
	return out
}

// printCommandHelp prints help for a command path of arbitrary depth. It
// resolves tree commands, dispatch groups, and dispatch operations, and
// returns false when the path does not exist.
func printCommandHelp(w io.Writer, path []string) bool {
	if len(path) == 0 {
		printUsage(w)
		return true
	}

	if isDispatchOp(strings.Join(path, " ")) {
		printDispatchOpHelp(w, strings.Join(path, " "))
		return true
	}

	cur, ok := commands[path[0]]
	if !ok {
		fmt.Fprintf(w, "Unknown command: %s\n", strings.Join(path, " "))
		return false
	}
	resolved := []string{path[0]}
	for _, p := range path[1:] {
		if cur.sub == nil {
			break
		}
		next, ok := cur.sub[p]
		if !ok {
			break
		}
		cur = next
		resolved = append(resolved, p)
	}
	full := strings.Join(resolved, " ")

	if ops, ok := knownDispatchCommands[full]; ok {
		printDispatchGroupHelp(w, full, cur, ops)
		return true
	}
	if cur.sub != nil {
		printTreeGroupHelp(w, full, cur)
		return true
	}
	printLeafHelp(w, full, cur)
	return true
}

// isDispatchOp reports whether full is one of the registered dispatch
// operations (e.g. "firewall rule create").
func isDispatchOp(full string) bool {
	for _, ops := range knownDispatchCommands {
		for _, op := range ops {
			if op == full {
				return true
			}
		}
	}
	return false
}

func printDispatchGroupHelp(w io.Writer, full string, cmd *command, ops []string) {
	desc := ""
	if cmd != nil {
		desc = cmd.short
	}
	fmt.Fprintf(w, "nodex %s — %s\n", full, desc)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Usage: nodex %s <operation> [args]\n", full)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Operations:")
	sorted := append([]string(nil), ops...)
	sort.Strings(sorted)
	for _, op := range sorted {
		entry := leafHelp[op]
		opName := strings.TrimPrefix(op, full+" ")
		fmt.Fprintf(w, "  %-20s %s\n", opName, entry.desc)
		fmt.Fprintln(w, "    "+renderUsage(op, entry))
	}
}

func printTreeGroupHelp(w io.Writer, full string, cmd *command) {
	fmt.Fprintf(w, "nodex %s — %s\n", full, cmd.short)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Usage: nodex %s <subcommand> [args]\n", full)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	names := make([]string, 0, len(cmd.sub))
	for name := range cmd.sub {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sub := cmd.sub[name]
		fmt.Fprintf(w, "  %-14s %s\n", name, sub.short)
	}
}

func printDispatchOpHelp(w io.Writer, full string) {
	entry := leafHelp[full]
	fmt.Fprintf(w, "nodex %s — %s\n", full, entry.desc)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+renderUsage(full, entry))
	if fs := handlerFlags[full]; len(fs.exact) > 0 || len(fs.params) > 0 {
		fmt.Fprintf(w, "\nFlags: %s\n", strings.Join(flagDisplay(fs), " "))
	}
	printExamples(w, full, entry)
}

func printLeafHelp(w io.Writer, full string, cmd *command) {
	entry, known := leafHelp[full]
	desc := entry.desc
	if desc == "" {
		desc = cmd.short
	}
	fmt.Fprintf(w, "nodex %s — %s\n", full, desc)
	fmt.Fprintln(w)
	if !known {
		fmt.Fprintf(w, "Usage: nodex %s [arguments]\n", full)
		return
	}
	fmt.Fprintln(w, "  "+renderUsage(full, entry))
	if fs := handlerFlags[full]; len(fs.exact) > 0 || len(fs.params) > 0 {
		fmt.Fprintf(w, "\nFlags: %s\n", strings.Join(flagDisplay(fs), " "))
	}
	printExamples(w, full, entry)
}

func renderUsage(full string, entry helpEntry) string {
	if entry.usage == "" {
		return fmt.Sprintf("Usage: nodex %s", full)
	}
	return fmt.Sprintf("Usage: nodex %s %s", full, entry.usage)
}

func printExamples(w io.Writer, full string, entry helpEntry) {
	if len(entry.examples) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	for _, ex := range entry.examples {
		fmt.Fprintf(w, "  nodex %s %s\n", full, ex)
	}
}

// printSubcommandUsage prints the subcommand usage for a group command to the
// given writer (used when a subcommand is required but missing).
func printSubcommandUsage(w io.Writer, cmd *command) {
	fmt.Fprintf(w, "Usage: nodex %s <subcommand> [args]\n", cmd.name)
	fmt.Fprintln(w)
	if ops, ok := knownDispatchCommands[cmd.name]; ok {
		fmt.Fprintln(w, "Operations:")
		sorted := append([]string(nil), ops...)
		sort.Strings(sorted)
		for _, op := range sorted {
			opName := strings.TrimPrefix(op, cmd.name+" ")
			fmt.Fprintf(w, "  %-20s %s\n", opName, leafHelp[op].desc)
		}
		return
	}
	fmt.Fprintln(w, "Subcommands:")
	names := make([]string, 0, len(cmd.sub))
	for name := range cmd.sub {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sub := cmd.sub[name]
		fmt.Fprintf(w, "  %-14s %s\n", name, sub.short)
	}
}
