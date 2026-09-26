package cli

import (
	"bytes"
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
)

func TestParseLogArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		def      int
		wantNode string
		wantLast int
		wantGrep string
		follow   bool
	}{
		{name: "node only uses default cap", args: []string{"proxmox"}, def: 50, wantNode: "proxmox", wantLast: 50},
		{name: "last overrides default", args: []string{"proxmox", "--last", "200"}, def: 50, wantNode: "proxmox", wantLast: 200},
		{name: "last inline form", args: []string{"--last=5", "proxmox"}, def: 50, wantNode: "proxmox", wantLast: 5},
		{name: "last before node", args: []string{"--last", "5", "proxmox"}, def: 50, wantNode: "proxmox", wantLast: 5},
		{name: "last zero disables cap", args: []string{"proxmox", "--last", "0"}, def: 50, wantNode: "proxmox", wantLast: 0},
		{name: "grep", args: []string{"proxmox", "--grep", "corosync"}, def: 50, wantNode: "proxmox", wantLast: 50, wantGrep: "corosync"},
		{name: "grep inline alternation", args: []string{"proxmox", "--grep=a|b"}, def: 50, wantNode: "proxmox", wantLast: 50, wantGrep: "a|b"},
		{name: "follow", args: []string{"proxmox", "--follow"}, def: 50, wantNode: "proxmox", wantLast: 50, follow: true},
		{name: "combined", args: []string{"proxmox", "--last", "10", "--grep", "pveproxy", "--follow"}, def: 50, wantNode: "proxmox", wantLast: 10, wantGrep: "pveproxy", follow: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseLogArgs(tt.args, tt.def)
			if err != nil {
				t.Fatalf("parseLogArgs(%v): %v", tt.args, err)
			}
			if opts.node != tt.wantNode {
				t.Errorf("node = %q, want %q", opts.node, tt.wantNode)
			}
			if opts.last != tt.wantLast {
				t.Errorf("last = %d, want %d", opts.last, tt.wantLast)
			}
			gotGrep := ""
			if opts.grep != nil {
				gotGrep = opts.grep.String()
			}
			if gotGrep != tt.wantGrep {
				t.Errorf("grep = %q, want %q", gotGrep, tt.wantGrep)
			}
			if opts.follow != tt.follow {
				t.Errorf("follow = %v, want %v", opts.follow, tt.follow)
			}
		})
	}
}

func TestParseLogArgsRejects(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no args", args: nil},
		{name: "empty node", args: []string{""}},
		{name: "two positionals", args: []string{"proxmox", "extra"}},
		{name: "negative last", args: []string{"proxmox", "--last", "-1"}},
		{name: "non-numeric last", args: []string{"proxmox", "--last", "many"}},
		{name: "last without value", args: []string{"proxmox", "--last"}},
		{name: "grep without value", args: []string{"proxmox", "--grep"}},
		{name: "invalid regexp", args: []string{"proxmox", "--grep", "([unclosed"}},
		{name: "unknown flag", args: []string{"proxmox", "--since", "1h"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseLogArgs(tt.args, 50); err == nil {
				t.Fatalf("parseLogArgs(%v) succeeded, want usage error", tt.args)
			}
		})
	}
}

func TestFilterSyslog(t *testing.T) {
	entries := []domain.SyslogEntry{
		{N: 1, Text: "system startup"},
		{N: 2, Text: "corosync stopped"},
		{N: 3, Text: "pveproxy restarted"},
	}
	t.Run("nil pattern is a no-op", func(t *testing.T) {
		got := filterSyslog(entries, nil)
		if len(got) != len(entries) {
			t.Fatalf("len = %d, want %d", len(got), len(entries))
		}
	})
	t.Run("filters by regexp", func(t *testing.T) {
		opts, err := parseLogArgs([]string{"n", "--grep", "corosync|pveproxy"}, 50)
		if err != nil {
			t.Fatalf("parseLogArgs: %v", err)
		}
		got := filterSyslog(entries, opts.grep)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].N != 2 || got[1].N != 3 {
			t.Fatalf("got lines %d,%d want 2,3", got[0].N, got[1].N)
		}
	})
	t.Run("no matches yields empty", func(t *testing.T) {
		opts, err := parseLogArgs([]string{"n", "--grep", "zzz"}, 50)
		if err != nil {
			t.Fatalf("parseLogArgs: %v", err)
		}
		if got := filterSyslog(entries, opts.grep); len(got) != 0 {
			t.Fatalf("len = %d, want 0", len(got))
		}
	})
}

func TestTailSyslog(t *testing.T) {
	entries := []domain.SyslogEntry{{N: 1}, {N: 2}, {N: 3}, {N: 4}, {N: 5}}
	tests := []struct {
		name  string
		n     int
		wantN []int64
	}{
		{name: "keeps trailing entries", n: 2, wantN: []int64{4, 5}},
		{name: "n equal to length is a no-op", n: 5, wantN: []int64{1, 2, 3, 4, 5}},
		{name: "n above length is a no-op", n: 99, wantN: []int64{1, 2, 3, 4, 5}},
		{name: "zero disables the cap", n: 0, wantN: []int64{1, 2, 3, 4, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tailSyslog(entries, tt.n)
			if len(got) != len(tt.wantN) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.wantN))
			}
			for i, want := range tt.wantN {
				if got[i].N != want {
					t.Fatalf("entry %d = %d, want %d", i, got[i].N, want)
				}
			}
		})
	}
}

// seedLogE2E configures the e2e mock profile used by the syslog commands.
func seedLogE2E(t *testing.T) {
	t.Helper()
	isolateConfigAndHome(t)
	t.Setenv("NODEX_E2E_TOKEN", "e2e-token")
	cfg := config.DefaultConfig()
	cfg.CurrentProfile = "e2e"
	cfg.Profiles["e2e"] = config.Profile{
		Provider:      e2eMockProviderName,
		Endpoint:      "https://e2e.example.invalid",
		CredentialRef: "env:e2e",
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

func TestLogFlagsEndToEnd(t *testing.T) {
	seedLogE2E(t)

	tests := []struct {
		name     string
		args     []string
		want     []string
		notWant  []string
		wantExit int
	}{
		{
			name: "default output",
			args: []string{"--output", "json", "log", "e2e-node"},
			want: []string{`"n": 1`, `"t": "system startup"`, `"n": 2`},
		},
		{
			name:    "grep selects matching entries",
			args:    []string{"--output", "json", "log", "e2e-node", "--grep", "disk"},
			want:    []string{`"n": 2`, `"t": "disk failure"`},
			notWant: []string{`"n": 1`},
		},
		{
			name: "last bounds output",
			args: []string{"--output", "json", "log", "e2e-node", "--last", "1"},
			want: []string{`"n": 2`},
		},
		{
			// Nits #19: a log limit must mean "most recent N", not the N
			// oldest since boot.
			name:    "global limit means most recent entries",
			args:    []string{"--output", "json", "--limit", "1", "log", "e2e-node"},
			want:    []string{`"n": 2`},
			notWant: []string{`"n": 1`},
		},
		{
			name:     "unknown flag rejected",
			args:     []string{"log", "e2e-node", "--since", "1h"},
			wantExit: app.ExitUsage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if tt.wantExit != 0 {
				if err == nil {
					t.Fatalf("Run(%v) succeeded, want exit %d", tt.args, tt.wantExit)
				}
				var exitErr *app.ExitCoder
				if !stderrors.As(err, &exitErr) {
					t.Fatalf("Run(%v) error = %T, want *app.ExitCoder", tt.args, err)
				}
				if exitErr.ExitCode != tt.wantExit {
					t.Fatalf("exit = %d, want %d", exitErr.ExitCode, tt.wantExit)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run(%v): %v stderr=%q", tt.args, err, stderr.String())
			}
			out := stdout.String()
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("Run(%v) output missing %q:\n%s", tt.args, want, out)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(out, notWant) {
					t.Fatalf("Run(%v) output unexpectedly contains %q:\n%s", tt.args, notWant, out)
				}
			}
		})
	}
}
