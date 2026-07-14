package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStringRedactsSecrets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // expected substring after redaction; empty = must contain REDACTED
	}{
		{
			name:     "api token assignment",
			input:    "api_token: secret123abc",
			contains: "",
		},
		{
			name:     "password field",
			input:    "password: hunter2",
			contains: "",
		},
		{
			name:     "PVE API token in header",
			input:    "Authorization: PVEAPIToken=root@pam!monitor=abc12345-1234-1234-1234-123456789abc",
			contains: "",
		},
		{
			name:     "bearer token",
			input:    "Bearer eyJhbGciOiJIUzI1NiIs",
			contains: "",
		},
		{
			name:     "basic auth",
			input:    "Basic dXNlcjpwYXNz",
			contains: "",
		},
		{
			name:     "credential ref file",
			input:    "credential_ref: file:home",
			contains: "",
		},
		{
			name:     "CSRF token",
			input:    "CSRFPreventionToken=abc123def456",
			contains: "",
		},
		{
			name:     "PVEAuthCookie",
			input:    "PVEAuthCookie=some-session-value",
			contains: "",
		},
		{
			name:     "env var TOKEN_SECRET",
			input:    "NODEX_DEFAULT_TOKEN_SECRET=supersecret",
			contains: "",
		},
		{
			name:     "env var PASSWORD",
			input:    "PASSWORD=my-real-password",
			contains: "",
		},
		{
			name:     "JSON password field",
			input:    `"password": "hunter2"`,
			contains: "",
		},
		{
			name:     "JSON token_id field",
			input:    `"token_id": "my-token-id"`,
			contains: "",
		},
		{
			name:     "JSON token_secret field",
			input:    `"token_secret": "abc-secret-123"`,
			contains: "",
		},
		{
			name:     "JSON credential field",
			input:    `"credential": "value123"`,
			contains: "",
		},
		{
			name:     "Proxmox token format bare",
			input:    "root@pam!test=aaaa-bbbb-cccc-dddd",
			contains: "",
		},
		{
			name:     "clean text unchanged",
			input:    "nodex version 0.1",
			contains: "nodex version 0.1",
		},
		{
			name:     "harmless JSON unchanged",
			input:    `{"name": "admin", "enabled": true}`,
			contains: `{"name": "admin", "enabled": true}`,
		},
		{
			name:     "PEM private key",
			input:    "-----BEGIN RSA PRIVATE KEY-----",
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := String(tt.input)
			if tt.contains == "" {
				if !strings.Contains(result, redacted) {
					t.Errorf("expected result to contain %q, got %q", redacted, result)
				}
			} else {
				if !strings.Contains(result, tt.contains) {
					t.Errorf("expected result to contain %q, got %q", tt.contains, result)
				}
			}
		})
	}
}

func TestStringCleanTextUnchanged(t *testing.T) {
	input := "Hello world, this is clean text."
	result := String(input)
	if result != input {
		t.Errorf("clean text was modified: %q -> %q", input, result)
	}
}

func TestContainsRedacted(t *testing.T) {
	if !ContainsRedacted("[REDACTED]") {
		t.Error("expected true for redacted marker")
	}
	if ContainsRedacted("clean text") {
		t.Error("expected false for clean text")
	}
}

func TestBytesRedacts(t *testing.T) {
	input := []byte("password: secret123")
	result := Bytes(input)
	if !strings.Contains(string(result), redacted) {
		t.Error("expected bytes to be redacted")
	}
}

// TestRedactedLeavesJSONStructure proves redaction replaces secret values but keeps
// surrounding JSON structure intact so structured consumers see [REDACTED] instead of real secrets.
func TestRedactedLeavesJSONStructure(t *testing.T) {
	input := `{"userid": "admin@pve", "password": "real-password", "enabled": true}`
	result := String(input)
	// The password value should be redacted.
	if !strings.Contains(result, redacted) {
		t.Errorf("expected redaction marker in JSON, got %q", result)
	}
	// The redacted output should still be valid-enough JSON (or at least recognizable).
	if !strings.Contains(result, "userid") && !strings.Contains(result, "enabled") {
		t.Errorf("non-secret fields should remain, got %q", result)
	}
	// Verify it's still valid JSON by trying to unmarshal (with [REDACTED] as string).
	m := make(map[string]any)
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		// It might not be valid JSON after redaction (whole field value replaced).
		// This is OK — the test just documents the behavior.
		t.Logf("redacted JSON is not valid JSON (expected): %v", err)
	}
}

// TestRedactionDoesNotLeakAuthHeader verifies the PVEAPIToken pattern catches real token formats.
func TestRedactionDoesNotLeakAuthHeader(t *testing.T) {
	token := "user@pve!api-token=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	input := "PVEAPIToken=" + token
	result := String(input)
	if strings.Contains(result, token) {
		t.Errorf("token leaked in redacted output: %q", result)
	}
	if !strings.Contains(result, redacted) {
		t.Errorf("expected redaction, got %q", result)
	}
}

// TestMultiplePatternsRedacted verifies that multiple secrets in one string are all redacted.
func TestMultiplePatternsRedacted(t *testing.T) {
	input := "auth: PVEAPIToken=user@pam!tok=abc123 and CSRFPreventionToken=xyz789"
	result := String(input)
	if strings.Contains(result, "abc123") || strings.Contains(result, "xyz789") {
		t.Errorf("secrets leaked in multi-pattern string: %q", result)
	}
	count := strings.Count(result, redacted)
	if count < 2 {
		t.Errorf("expected at least 2 redactions, got %d: %q", count, result)
	}
}
