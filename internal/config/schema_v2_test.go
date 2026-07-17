package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfigFile writes raw YAML to a temp config path and returns the path.
func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestReadFromVersion1File(t *testing.T) {
	path := writeConfigFile(t, `version: 1
current_profile: home
profiles:
  home:
    provider: proxmox
    endpoint: https://pve.example.invalid:8006
    credential_ref: file:home
`)
	cfg, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("version 1 config must load: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("expected loaded version 1, got %d", cfg.Version)
	}
	if cfg.Profiles["home"].Provider != ProviderProxmox {
		t.Errorf("expected proxmox provider, got %q", cfg.Profiles["home"].Provider)
	}
}

func TestReadFromVersion2FileWithPBSProfile(t *testing.T) {
	path := writeConfigFile(t, `version: 2
current_profile: production-pve
profiles:
  production-pve:
    provider: proxmox
    endpoint: https://pve.example.invalid:8006
    credential_ref: keyring:production-pve
  production-pbs:
    provider: pbs
    endpoint: https://pbs.example.invalid:8007
    credential_ref: keyring:production-pbs
`)
	cfg, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("version 2 config must load: %v", err)
	}
	if cfg.Version != 2 {
		t.Errorf("expected loaded version 2, got %d", cfg.Version)
	}
	if got := cfg.Profiles["production-pbs"].Provider; got != ProviderPBS {
		t.Errorf("expected pbs provider, got %q", got)
	}
}

func TestReadFromNewerVersionRejected(t *testing.T) {
	path := writeConfigFile(t, `version: 3
profiles: {}
`)
	if _, err := ReadFrom(path); err == nil {
		t.Fatal("expected newer schema version to be rejected")
	}
}

// TestUpdatePreservesVersion1 verifies that a read-modify-write of a version 1
// file does not silently migrate the file to version 2.
func TestUpdatePreservesVersion1(t *testing.T) {
	path := writeConfigFile(t, `version: 1
profiles:
  home:
    provider: proxmox
    endpoint: https://pve.example.invalid:8006
`)
	cfg, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	cfg.Profiles["second"] = Profile{Provider: ProviderProxmox}
	if err := WriteTo(cfg, path); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(path) // #nosec G304 -- test reads its own temp file
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(data), "version: 1") {
		t.Errorf("expected file to remain version 1, got:\n%s", data)
	}

	reread, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if reread.Version != 1 {
		t.Errorf("expected re-read version 1, got %d", reread.Version)
	}
	if len(reread.Profiles) != 2 {
		t.Errorf("expected 2 profiles after update, got %d", len(reread.Profiles))
	}
}

func TestValidatePBSProfileEndpointPolicy(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		valid    bool
	}{
		{"https accepted", "https://pbs.example.invalid:8007", true},
		{"http rejected", "http://pbs.example.invalid:8007", false},
		{"userinfo rejected", "https://user:pass@pbs.example.invalid:8007", false},
		{"path rejected", "https://pbs.example.invalid:8007/api2/json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Version: 2,
				Profiles: map[string]Profile{
					"backup": {Provider: ProviderPBS, Endpoint: tt.endpoint},
				},
			}
			err := Validate(cfg)
			if tt.valid && err != nil {
				t.Errorf("expected valid, got: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("expected endpoint rejection, got nil error")
			}
		})
	}
}

func TestValidatePBSCredentialRef(t *testing.T) {
	cfg := &Config{
		Version: 2,
		Profiles: map[string]Profile{
			"backup": {
				Provider:      ProviderPBS,
				Endpoint:      "https://pbs.example.invalid:8007",
				CredentialRef: "keyring:production-pbs",
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("valid pbs profile rejected: %v", err)
	}

	cfg.Profiles["backup"] = Profile{
		Provider:      ProviderPBS,
		Endpoint:      "https://pbs.example.invalid:8007",
		CredentialRef: "bogus:backend:name",
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected invalid credential_ref to be rejected")
	}
}

func TestValidateProviderNameShape(t *testing.T) {
	tests := []struct {
		provider string
		valid    bool
	}{
		{"proxmox", true},
		{"pbs", true},
		{"future-provider", true}, // unknown but well-formed: loadable, fails at use
		{"PBS", true},             // normalized to lowercase before shape check
		{"has space", false},
		{"-leading", false},
		{"bad!chars", false},
		{strings.Repeat("x", 40), false},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			cfg := &Config{
				Version: 2,
				Profiles: map[string]Profile{
					"p1": {Provider: tt.provider, Endpoint: "https://example.invalid"},
				},
			}
			err := Validate(cfg)
			if tt.valid && err != nil {
				t.Errorf("expected provider %q accepted, got: %v", tt.provider, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected provider %q rejected, got nil error", tt.provider)
			}
		})
	}
}

func TestKnownProviders(t *testing.T) {
	if !IsKnownProvider(ProviderProxmox) {
		t.Error("proxmox must be a known provider")
	}
	if !IsKnownProvider(ProviderPBS) {
		t.Error("pbs must be a known provider")
	}
	if IsKnownProvider("openstack") {
		t.Error("unexpected known provider")
	}
	got := KnownProviders()
	if len(got) != 2 || got[0] != ProviderProxmox || got[1] != ProviderPBS {
		t.Errorf("unexpected KnownProviders(): %v", got)
	}
}

// TestReadDoesNotWrite verifies that loading a version 1 config never
// modifies the file on disk (no silent rewrite on read).
func TestReadDoesNotWrite(t *testing.T) {
	content := `version: 1
profiles:
  home:
    provider: Proxmox
    endpoint: https://pve.example.invalid:8006
`
	path := writeConfigFile(t, content)
	if _, err := ReadFrom(path); err != nil {
		t.Fatalf("read: %v", err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- test reads its own temp file
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != content {
		t.Error("reading a config must not modify the file on disk")
	}
}
