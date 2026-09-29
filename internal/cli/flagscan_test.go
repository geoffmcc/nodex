package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/output"
)

func TestParseGlobalRemainingPreservesCommandPath(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantArgs []string
		wantPath []string
		wantErr  string
	}{
		{
			name:     "plain command",
			args:     []string{"version"},
			wantArgs: []string{"version"},
		},
		{
			name:     "tree path plus region",
			args:     []string{"profile", "add", "backup", "--provider", "pbs"},
			wantArgs: []string{"profile", "add", "backup", "--provider", "pbs"},
		},
		{
			name:     "dispatch op path plus region",
			args:     []string{"pbs", "verify", "run", "v-daily"},
			wantArgs: []string{"pbs", "verify", "run", "v-daily"},
		},
		{
			name:     "handler-owned flags pass through",
			args:     []string{"pbs", "verify", "run", "--datastore", "vm", "job"},
			wantArgs: []string{"pbs", "verify", "run", "--datastore", "vm", "job"},
		},
		{
			name:     "interspersed globals are extracted",
			args:     []string{"--verbose", "node", "--output", "json", "status"},
			wantArgs: []string{"node", "status"},
		},
		{
			name:     "globals before and after command",
			args:     []string{"--yes", "vm", "list", "--all"},
			wantArgs: []string{"vm", "list"},
		},
		{
			name:     "help capture mid path",
			args:     []string{"vm", "snapshot", "--help"},
			wantArgs: []string{},
			wantPath: []string{"vm", "snapshot"},
		},
		{
			name:     "short help capture",
			args:     []string{"vm", "list", "-h"},
			wantArgs: []string{},
			wantPath: []string{"vm", "list"},
		},
		{
			name:    "unknown flag",
			args:    []string{"node", "status", "--bogus"},
			wantErr: "unknown flag: --bogus",
		},
		{
			name:    "missing global value",
			args:    []string{"--output"},
			wantErr: "flag needs an argument: --output",
		},
		{
			name:    "invalid output format",
			args:    []string{"--output", "csv", "version"},
			wantErr: "invalid output format",
		},
		{
			name:    "invalid timeout value",
			args:    []string{"--timeout", "abc", "version"},
			wantErr: "invalid value",
		},
		{
			name:    "zero timeout",
			args:    []string{"--timeout", "0s", "version"},
			wantErr: "timeout must be greater than zero",
		},
		{
			name:    "negative limit",
			args:    []string{"--limit", "-1", "node", "list"},
			wantErr: "limit must be non-negative",
		},
		{
			name:    "invalid bool value",
			args:    []string{"--yes=maybe", "version"},
			wantErr: "invalid value",
		},
		{
			name:     "leading separator enables literal mode",
			args:     []string{"--", "version", "postfix"},
			wantArgs: []string{"version", "postfix"},
		},
		{
			name:     "trailing separator is passed to region",
			args:     []string{"vm", "list", "--", "tail"},
			wantArgs: []string{"vm", "list", "--", "tail"},
		},
		{
			name:     "bare dash is positional",
			args:     []string{"-", "x"},
			wantArgs: []string{"-", "x"},
		},
		{
			name:     "inline global output",
			args:     []string{"--output=json", "version"},
			wantArgs: []string{"version"},
		},
		{
			name:     "bool equals false",
			args:     []string{"--yes=false", "version"},
			wantArgs: []string{"version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, helpPath, remaining, err := parseGlobal(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseGlobal(%v) error = %v, want containing %q", tt.args, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGlobal(%v) unexpected error: %v", tt.args, err)
			}
			if tt.wantPath != nil {
				if len(helpPath) != len(tt.wantPath) {
					t.Fatalf("parseGlobal(%v) helpPath = %v, want %v", tt.args, helpPath, tt.wantPath)
				}
				for i := range tt.wantPath {
					if helpPath[i] != tt.wantPath[i] {
						t.Fatalf("parseGlobal(%v) helpPath = %v, want %v", tt.args, helpPath, tt.wantPath)
					}
				}
				return
			}
			if len(helpPath) != 0 {
				t.Fatalf("parseGlobal(%v) unexpected helpPath %v", tt.args, helpPath)
			}
			if len(remaining) != len(tt.wantArgs) {
				t.Fatalf("parseGlobal(%v) remaining = %v, want %v", tt.args, remaining, tt.wantArgs)
			}
			for i := range tt.wantArgs {
				if remaining[i] != tt.wantArgs[i] {
					t.Fatalf("parseGlobal(%v) remaining = %v, want %v", tt.args, remaining, tt.wantArgs)
				}
			}
		})
	}
}

func TestParseGlobalOptionValues(t *testing.T) {
	opts, _, remaining, err := parseGlobal([]string{"--verbose", "--profile", "prod", "--timeout", "5s", "--limit", "3", "--output", "json", "node", "list"})
	if err != nil {
		t.Fatalf("parseGlobal: %v", err)
	}
	if !opts.Verbose {
		t.Error("expected Verbose true")
	}
	if opts.Profile != "prod" {
		t.Errorf("Profile = %q, want prod", opts.Profile)
	}
	if opts.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", opts.Timeout)
	}
	if opts.Limit != 3 {
		t.Errorf("Limit = %d, want 3", opts.Limit)
	}
	if opts.Output != output.FormatJSON {
		t.Errorf("Output = %v, want json", opts.Output)
	}
	if len(remaining) != 2 || remaining[0] != "node" || remaining[1] != "list" {
		t.Errorf("remaining = %v, want [node list]", remaining)
	}
}

func TestParseGlobalDefaultOutputFormat(t *testing.T) {
	opts, _, _, err := parseGlobal([]string{"version"})
	if err != nil {
		t.Fatalf("parseGlobal: %v", err)
	}
	if opts.Output != output.DefaultFormat() {
		t.Errorf("Output = %v, want default %v", opts.Output, output.DefaultFormat())
	}
}

func TestParseGlobalConfirmTargetAtAnyPosition(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
		rem  []string
	}{
		{"before command", []string{"--confirm-target", "proxmox/9999", "vm", "delete", "proxmox/9999"}, "proxmox/9999", []string{"vm", "delete", "proxmox/9999"}},
		{"inline after command path", []string{"vm", "delete", "--confirm-target=proxmox/9999", "proxmox/9999"}, "proxmox/9999", []string{"vm", "delete", "proxmox/9999"}},
		{"interspersed with handler args", []string{"vm", "delete", "--yes", "proxmox/9999", "--confirm-target", "proxmox/9999", "--force"}, "proxmox/9999", []string{"vm", "delete", "proxmox/9999"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, remaining, err := parseGlobal(tt.args)
			if err != nil {
				t.Fatalf("parseGlobal: %v", err)
			}
			if opts.ConfirmTarget != tt.want {
				t.Errorf("ConfirmTarget = %q, want %q", opts.ConfirmTarget, tt.want)
			}
			if strings.Join(remaining, " ") != strings.Join(tt.rem, " ") {
				t.Errorf("remaining = %v, want %v", remaining, tt.rem)
			}
		})
	}
}

func TestNextArg(t *testing.T) {
	args := []string{"node", "list"}
	if v, ok := nextArg(args, 0); !ok || v != "list" {
		t.Errorf("nextArg(args, 0) = %q, %v; want %q, true", v, ok, "list")
	}
	if v, ok := nextArg(args, 1); ok || v != "" {
		t.Errorf("nextArg(args, 1) = %q, %v; want empty, false", v, ok)
	}
	if v, ok := nextArg(nil, 0); ok || v != "" {
		t.Errorf("nextArg(nil, 0) = %q, %v; want empty, false", v, ok)
	}
}

func TestParseGlobalRecordsConfirmFlagPresence(t *testing.T) {
	// §12.7: parseGlobal cannot know whether a confirmation flag is
	// applicable, so it records presence separately from the parsed value.
	// `--yes=false` still counts as "passed" even though Yes is false.
	opts, _, _, err := parseGlobal([]string{"vm", "list", "--yes=false", "--confirm-target", "proxmox/1"})
	if err != nil {
		t.Fatalf("parseGlobal: %v", err)
	}
	if opts.Yes {
		t.Error("Yes = true, want false for --yes=false")
	}
	for _, flag := range []string{"--yes", "--confirm-target"} {
		if !opts.sawConfirmFlags[flag] {
			t.Errorf("sawConfirmFlags[%q] = false, want true", flag)
		}
	}
	if opts.sawConfirmFlags["--force"] {
		t.Error("sawConfirmFlags[\"--force\"] = true, want false when not passed")
	}
}

func TestParseGlobalNoConfirmFlagsWhenAbsent(t *testing.T) {
	opts, _, _, err := parseGlobal([]string{"vm", "list"})
	if err != nil {
		t.Fatalf("parseGlobal: %v", err)
	}
	if len(opts.sawConfirmFlags) != 0 {
		t.Errorf("sawConfirmFlags = %v, want empty", opts.sawConfirmFlags)
	}
}

func TestWarnInertConfirmFlagsNamesEachFlagInFixedOrder(t *testing.T) {
	// §12.7: one line per inert flag, in a deterministic order, and the exit
	// code is unchanged. Ordering is asserted so output does not depend on
	// command-line order.
	opts, _, _, err := parseGlobal([]string{"vm", "list", "--confirm-target", "proxmox/1", "--force", "--yes"})
	if err != nil {
		t.Fatalf("parseGlobal: %v", err)
	}
	meta := LookupOperation("vm list")
	if meta == nil {
		t.Fatal("LookupOperation(\"vm list\") = nil")
	}
	if !meta.Inspection {
		t.Fatalf("vm list is not read-only; test premise broken (Inspection=%v)", meta.Inspection)
	}

	var stderr bytes.Buffer
	warnInertConfirmFlags(opts, meta, &stderr)
	got := stderr.String()
	want := "warning: --yes has no effect on \"vm list\" (read-only command)\n" +
		"warning: --force has no effect on \"vm list\" (read-only command)\n" +
		"warning: --confirm-target has no effect on \"vm list\" (read-only command)\n"
	if got != want {
		t.Errorf("warning output =\n%q\nwant\n%q", got, want)
	}
}

func TestWarnInertConfirmFlagsQuietAndMutationCases(t *testing.T) {
	quietOpts, _, _, err := parseGlobal([]string{"--quiet", "vm", "list", "--yes"})
	if err != nil {
		t.Fatalf("parseGlobal: %v", err)
	}
	var stderr bytes.Buffer
	warnInertConfirmFlags(quietOpts, LookupOperation("vm list"), &stderr)
	if stderr.Len() != 0 {
		t.Errorf("--quiet should suppress the warning, got: %q", stderr.String())
	}

	mutationOpts, _, _, err := parseGlobal([]string{"vm", "start", "proxmox/1", "--yes"})
	if err != nil {
		t.Fatalf("parseGlobal: %v", err)
	}
	var mutErr bytes.Buffer
	warnInertConfirmFlags(mutationOpts, LookupOperation("vm start"), &mutErr)
	if mutErr.Len() != 0 {
		t.Errorf("mutation command must not warn, got: %q", mutErr.String())
	}

	noMeta, _, _, err := parseGlobal([]string{"vm", "list", "--yes"})
	if err != nil {
		t.Fatalf("parseGlobal: %v", err)
	}
	var nilMetaErr bytes.Buffer
	warnInertConfirmFlags(noMeta, nil, &nilMetaErr)
	if nilMetaErr.Len() != 0 {
		t.Errorf("nil metadata must not warn, got: %q", nilMetaErr.String())
	}
}

func TestWarnInertConfirmFlagsSilentForDispatchParents(t *testing.T) {
	// Several dispatch parents are Inspection:true but route to mutations
	// (e.g. `backup job` -> `backup job delete`). Their metadata describes the
	// router, not the leaf, so warning here would falsely claim --yes was
	// inert on a command that does use it. Silence is the correct answer.
	for _, path := range []string{"backup job", "ceph osd", "access user", "vm snapshot"} {
		meta := LookupOperation(path)
		if meta == nil {
			t.Fatalf("LookupOperation(%q) = nil", path)
		}
		if !meta.Inspection {
			t.Fatalf("%q is not Inspection:true; test premise broken", path)
		}
		opts, _, _, err := parseGlobal([]string{path, "--yes", "--force"})
		if err != nil {
			t.Fatalf("parseGlobal: %v", err)
		}
		var stderr bytes.Buffer
		warnInertConfirmFlags(opts, meta, &stderr)
		if stderr.Len() != 0 {
			t.Errorf("dispatch parent %q must not warn, got: %q", path, stderr.String())
		}
	}
}
