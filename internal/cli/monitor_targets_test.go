package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// writeMonitorConfig installs a config declaring the given monitor targets as
// a "monitoring.targets" mapping.
//
// cfgDir is the directory config.Dir() resolved to, not an assumed XDG path:
// the product only honours XDG_CONFIG_HOME on linux, so a fixture written to
// the XDG layout is invisible on darwin and windows.
func writeMonitorConfig(t *testing.T, cfgDir string, targets map[string]map[string]string) {
	t.Helper()
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	var b strings.Builder
	if len(targets) == 0 {
		b.WriteString("version: 1\n")
		writeMonitorConfigFile(t, cfgDir, b.String())
		return
	}
	// The monitoring section requires schema version 2.
	b.WriteString("version: 2\n")
	b.WriteString("monitoring:\n  targets:\n")
	for name, fields := range targets {
		b.WriteString("    " + name + ":\n")
		for k, v := range fields {
			b.WriteString("      " + k + ": " + v + "\n")
		}
	}
	writeMonitorConfigFile(t, cfgDir, b.String())
}

func writeMonitorConfigFile(t *testing.T, cfgDir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// TestRun_MonitorTargetsEmitsList pins §12.6. `monitor targets` was the only
// list-shaped command returning an object, so a consumer iterating the result
// got a silent no-op instead of an error.
func TestRun_MonitorTargetsEmitsList(t *testing.T) {
	_, cfgDir := isolatedConfigDir(t)
	writeMonitorConfig(t, cfgDir, map[string]map[string]string{
		"pve": {"type": "https", "address": "https://pve.example.com:8006"},
	})

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := Run(context.Background(), []string{"--output", format, "monitor", "targets"}, &stdout, &stderr); err != nil {
				t.Fatalf("monitor targets --output %s: %v (stderr %q)", format, err, stderr.String())
			}
			out := stdout.String()
			if strings.HasPrefix(strings.TrimSpace(out), "{") {
				t.Errorf("output is an object, want a list:\n%s", out)
			}

			var entries []map[string]any
			var err error
			if format == "json" {
				err = json.Unmarshal([]byte(out), &entries)
			} else {
				err = yaml.Unmarshal([]byte(out), &entries)
			}
			if err != nil {
				t.Fatalf("invalid %s: %v\n%s", format, err, out)
			}
			if len(entries) != 1 {
				t.Fatalf("got %d entries, want 1:\n%s", len(entries), out)
			}
			// The target's fields must be flattened, not nested under a
			// wrapper key, and the name must be carried in the entry.
			e := entries[0]
			if e["name"] != "pve" {
				t.Errorf("name = %v, want pve", e["name"])
			}
			if e["type"] != "https" {
				t.Errorf("type = %v, want https (got %v)", e["type"], e)
			}
			for _, bad := range []string{"MonitorTarget"} {
				if _, ok := e[bad]; ok {
					t.Errorf("nested wrapper key %q present: %v", bad, e)
				}
			}
		})
	}
}

// TestRun_MonitorTargetsEmptyIsList pins that "nothing configured" is still a
// list, and that it says so on stderr. Previously this was `{}` with exit 0,
// indistinguishable from a healthy configuration.
func TestRun_MonitorTargetsEmptyIsList(t *testing.T) {
	_, cfgDir := isolatedConfigDir(t)
	writeMonitorConfig(t, cfgDir, nil)

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := Run(context.Background(), []string{"--output", format, "monitor", "targets"}, &stdout, &stderr); err != nil {
				t.Fatalf("monitor targets --output %s: %v", format, err)
			}
			out := strings.TrimSpace(stdout.String())
			if out != "[]" {
				t.Errorf("empty result = %q, want []", out)
			}
			if !strings.Contains(stderr.String(), "no monitor targets configured") {
				t.Errorf("missing empty-result note on stderr, got %q", stderr.String())
			}
		})
	}
}
