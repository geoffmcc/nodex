package cli

import (
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
