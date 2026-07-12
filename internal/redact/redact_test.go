package redact

import (
	"strings"
	"testing"
)

func TestStringRedactsSecrets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // expected redacted marker
	}{
		{
			name:     "api token assignment",
			input:    "api_token: secret123abc",
			contains: redacted,
		},
		{
			name:     "password field",
			input:    "password: hunter2",
			contains: redacted,
		},
		{
			name:     "PVE API token",
			input:    "Authorization: root@pam!monitor=abc12345-1234-1234-1234-123456789abc",
			contains: redacted,
		},
		{
			name:     "bearer token",
			input:    "Bearer eyJhbGciOiJIUzI1NiIs",
			contains: redacted,
		},
		{
			name:     "clean text unchanged",
			input:    "nodex version 0.1",
			contains: "nodex version 0.1",
		},
		{
			name:     "credential ref",
			input:    "credential_ref: file:home",
			contains: redacted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := String(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected result to contain %q, got %q", tt.contains, result)
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
