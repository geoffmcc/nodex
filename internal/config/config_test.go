package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}
	if cfg.Profiles == nil {
		t.Error("expected non-nil profiles map")
	}
}

func TestValidateNil(t *testing.T) {
	err := Validate(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestValidateVersion(t *testing.T) {
	cfg := &Config{Version: 99}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for wrong version")
	}
}

func TestValidateProfileName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"home", true},
		{"test-profile", true},
		{"profile_1", true},
		{"a", true},
		{"123abc", true},
		{"", false},
		{"-bad", false},
		{"_bad", false},
		{"has spaces", false},
		{"too!long@name#here$", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Version: 1,
				Profiles: map[string]Profile{
					tt.name: {Provider: "proxmox", Endpoint: "https://example.com"},
				},
			}
			err := Validate(cfg)
			if tt.valid && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("expected invalid, got nil error")
			}
		})
	}
}

func TestValidateProviderNormalized(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Profiles: map[string]Profile{
			"test": {Provider: "Proxmox", Endpoint: "https://example.com"},
		},
	}
	err := Validate(cfg)
	if err != nil {
		t.Errorf("provider case should be accepted: %v", err)
	}
}

func TestValidateMissingProvider(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Profiles: map[string]Profile{
			"test": {Endpoint: "https://example.com"},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for missing provider")
	}
}

func TestValidateCurrentProfileNotFound(t *testing.T) {
	cfg := &Config{
		Version:        1,
		CurrentProfile: "missing",
		Profiles:       map[string]Profile{},
	}
	err := Validate(cfg)
	if err == nil {
		t.Error("expected error for missing current profile")
	}
}

func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.CurrentProfile = "home"
	cfg.Profiles["home"] = Profile{
		Provider:      "proxmox",
		Endpoint:      "https://prox.example.com:8006",
		CredentialRef: "file:home",
	}

	if err := WriteTo(cfg, path); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Verify file exists and has content.
	data, err := os.ReadFile(path) // #nosec G304 -- path is a test-owned temp file.
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("config file is empty")
	}

	// Read it back.
	got, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}

	if got.Version != cfg.Version {
		t.Errorf("version: got %d, want %d", got.Version, cfg.Version)
	}
	if got.CurrentProfile != cfg.CurrentProfile {
		t.Errorf("current_profile: got %q, want %q", got.CurrentProfile, cfg.CurrentProfile)
	}
	if got.Profiles["home"].Provider != "proxmox" {
		t.Errorf("provider: got %q, want %q", got.Profiles["home"].Provider, "proxmox")
	}
}

func TestReadNonExistent(t *testing.T) {
	_, err := ReadFrom("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for non-existent config")
	}
}

func TestValidateCurrentProfile(t *testing.T) {
	cfg := &Config{
		Version:        1,
		CurrentProfile: "valid",
		Profiles: map[string]Profile{
			"valid": {Provider: "proxmox", Endpoint: "https://example.com"},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("expected valid config, got: %v", err)
	}
}

func TestProfileNames(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Profiles: map[string]Profile{
			"alpha": {Provider: "proxmox"},
			"beta":  {Provider: "proxmox"},
		},
	}
	names := ProfileNames(cfg)
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}

func TestNormalizeProvider(t *testing.T) {
	if got := NormalizeProvider("Proxmox"); got != "proxmox" {
		t.Errorf("expected proxmox, got %s", got)
	}
}

func TestValidateEndpointPolicy(t *testing.T) {
	if err := ValidateEndpoint("https://pve.example.com:8006"); err != nil {
		t.Fatalf("expected valid endpoint: %v", err)
	}
	for _, endpoint := range []string{
		"http://pve.example.com:8006",
		"https://user:pass@pve.example.com:8006",
		"https://pve.example.com:8006/path",
		"https://pve.example.com:8006?token=secret",
	} {
		if err := ValidateEndpoint(endpoint); err == nil {
			t.Fatalf("ValidateEndpoint(%q) succeeded, want error", endpoint)
		}
	}
}

func TestProfileNamesSorted(t *testing.T) {
	cfg := &Config{Profiles: map[string]Profile{"b": {}, "a": {}}}
	names := ProfileNames(cfg)
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("ProfileNames = %v, want sorted", names)
	}
}

func TestUpdateConcurrentMutations(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if err := WriteTo(DefaultConfig(), path); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	var wg sync.WaitGroup
	for _, name := range []string{"a", "b", "c", "d"} {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Update(func(cfg *Config) error {
				cfg.Profiles[name] = Profile{Provider: "proxmox"}
				if cfg.CurrentProfile == "" {
					cfg.CurrentProfile = name
				}
				return nil
			})
		}()
	}
	wg.Wait()
	cfg, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	if len(cfg.Profiles) != 4 {
		t.Fatalf("profiles = %v, want 4", cfg.Profiles)
	}
}
