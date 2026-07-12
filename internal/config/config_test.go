package config

import (
	"os"
	"path/filepath"
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
	data, err := os.ReadFile(path)
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
