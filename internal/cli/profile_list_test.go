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

// writeTestConfig installs a config with one profile whose credential_ref
// points at a named credential store.
//
// cfgDir is the directory config.Dir() resolved to, not an assumed XDG path:
// the product only honours XDG_CONFIG_HOME on linux, so a fixture written to
// the XDG layout is invisible on darwin and windows.
func writeTestConfig(t *testing.T, cfgDir string) {
	t.Helper()
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	body := "" +
		"version: 1\n" +
		"current_profile: production\n" +
		"profiles:\n" +
		"  production:\n" +
		"    provider: proxmox\n" +
		"    endpoint: https://pve.example.com:8006\n" +
		"    credential_ref: file:production-secret-store\n" +
		"    ssh_host: pve.example.com\n" +
		"    ssh_user: root\n" +
		"    ssh_port: 22\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// TestRun_ProfileListJSONKeys pins §10.4. config.Profile carried only YAML
// tags, so `profile list --output json` serialized it under its Go field
// names: "Provider", "Endpoint", "CredentialRef". A dashboard reading
// `profile.provider` saw nothing, and the JSON key set for the same value
// differed from the YAML one.
func TestRun_ProfileListJSONKeys(t *testing.T) {
	_, cfgDir := isolatedConfigDir(t)
	writeTestConfig(t, cfgDir)

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--output", "json", "profile", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("profile list: %v (stderr %q)", err, stderr.String())
	}

	var entries []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		t.Fatalf("profile list --output json is not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %s", len(entries), stdout.String())
	}
	e := entries[0]
	for _, key := range []string{"name", "current", "provider", "endpoint", "credential_ref"} {
		if _, ok := e[key]; !ok {
			t.Errorf("missing snake_case key %q in %v", key, e)
		}
	}
	// The Go field names must not leak into the document.
	for _, bad := range []string{"Name", "Current", "Provider", "Endpoint", "CredentialRef"} {
		if _, ok := e[bad]; ok {
			t.Errorf("PascalCase key %q present in JSON output: %v", bad, e)
		}
	}
}

// TestRun_ProfileListCredentialRefRedacted pins that the credential-store
// reference is masked, and that masking it once does not corrupt the document
// when the serialized bytes pass through the redaction net again on the way to
// stdout.
func TestRun_ProfileListCredentialRefRedacted(t *testing.T) {
	_, cfgDir := isolatedConfigDir(t)
	writeTestConfig(t, cfgDir)

	var stdout, stderr bytes.Buffer

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()
			if err := Run(context.Background(), []string{"--output", format, "profile", "list"}, &stdout, &stderr); err != nil {
				t.Fatalf("profile list --output %s: %v (stderr %q)", format, err, stderr.String())
			}
			out := stdout.String()
			if strings.Count(out, "[REDACTED]") != 1 {
				t.Errorf("want exactly one redaction marker, got %d in:\n%s", strings.Count(out, "[REDACTED]"), out)
			}
			// A second pass appended a stray quoted marker after the value,
			// which is what made "credential_ref: [REDACTED]'[REDACTED]'".
			if strings.Contains(out, "[REDACTED]'[REDACTED]'") {
				t.Errorf("double redaction in %s output:\n%s", format, out)
			}

			// The document must still parse in its declared format.
			switch format {
			case "json":
				var v []map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &v); err != nil {
					t.Errorf("invalid JSON: %v\n%s", err, out)
				}
			case "yaml":
				var v []map[string]any
				if err := yaml.Unmarshal(stdout.Bytes(), &v); err != nil {
					t.Errorf("invalid YAML: %v\n%s", err, out)
				}
			}
		})
	}
}
