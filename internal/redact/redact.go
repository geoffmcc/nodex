package redact

import (
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

// Patterns that indicate sensitive values.
var patterns = []*regexp.Regexp{
	// API tokens and keys
	regexp.MustCompile(`(?i)(api[_-]?token|apikey|api[_-]?key|secret[_-]?key|access[_-]?key)\s*[:=]\s*\S+`),
	// Bare token-like values (long hex/base64 strings after common markers)
	regexp.MustCompile(`(?i)(token|secret|password|passwd|pwd|credential)\s*[:=]\s*\S+`),
	// PVE API token format: user@realm!tokenid=uuid
	regexp.MustCompile(`\w+@\w+![\w-]+=[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`),
	// Bearer tokens
	regexp.MustCompile(`(?i)bearer\s+\S+`),
	// Basic auth
	regexp.MustCompile(`(?i)basic\s+[A-Za-z0-9+/=]+`),
	// File paths to credential files
	regexp.MustCompile(`(?i)file:\S+`),
	// PEM-encoded content
	regexp.MustCompile(`-----BEGIN\s+[A-Z\s]*PRIVATE KEY-----`),
}

// String redacts sensitive patterns from the input.
func String(input string) string {
	result := input
	for _, p := range patterns {
		result = p.ReplaceAllString(result, redacted)
	}
	return result
}

// Bytes redacts sensitive patterns from a byte slice.
func Bytes(input []byte) []byte {
	return []byte(String(string(input)))
}

// ContainsRedacted checks if a string contains the redacted marker.
func ContainsRedacted(s string) bool {
	return strings.Contains(s, redacted)
}
