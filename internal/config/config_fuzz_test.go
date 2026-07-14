package config

import (
	"testing"
)

// FuzzValidateEndpoint tests endpoint URL validation at the trust boundary
// where user-supplied URLs enter the configuration system.
// It must never panic, must not allocate unbounded memory, and must return
// consistent results (valid or error, never ambiguous).
func FuzzValidateEndpoint(f *testing.F) {
	// Seed corpus with known-good and known-bad endpoints.
	seeds := []string{
		"https://pve.example.com:8006",
		"https://pve.example.com",
		"http://pve.example.com:8006",
		"https://user:pass@pve.example.com:8006",
		"https://pve.example.com:8006/path",
		"https://pve.example.com:8006?token=secret",
		"https://pve.example.com:8006#frag",
		"",
		"not-a-url",
		"ftp://pve.example.com",
		"https://pve.example.com:8006/",
		"HTTPS://PVE.EXAMPLE.COM:8006",
		"https://a",
		"https://1.2.3.4:8006",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, endpoint string) {
		// Guard against excessively large inputs that might cause
		// the URL parser to allocate unbounded memory.
		if len(endpoint) > 2048 {
			return
		}

		// ValidateEndpoint must never panic.
		_ = ValidateEndpoint(endpoint)
	})
}

// FuzzProfileNameValidate tests profile name validation through the config
// Validate function. Profile names come from user input and must be
// safely validated against the profile regex.
func FuzzProfileNameValidate(f *testing.F) {
	seeds := []string{
		"home",
		"test-profile",
		"profile_1",
		"a",
		"123abc",
		"",
		"-bad",
		"_bad",
		"has spaces",
		"too!long@name#here$",
		"../escape",
		".hidden",
		"A_really-long_profile_name_thats_MAX_64_characters_ok_maybe_longer",
		"x",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 256 {
			return
		}

		// Validate with a minimal valid config containing the fuzzed name.
		cfg := &Config{
			Version: 1,
			Profiles: map[string]Profile{
				name: {Provider: "proxmox", Endpoint: "https://example.com"},
			},
		}
		// Must never panic. The result can be nil (valid) or an error.
		_ = Validate(cfg)
	})
}
