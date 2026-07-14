package redact

import (
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

// Patterns that indicate sensitive values.
// These are applied to all output (stdout, stderr, logs, error messages).
var patterns = []*regexp.Regexp{
	// API tokens and keys in key=value or key:value form.
	regexp.MustCompile(`(?i)(api[_-]?token|apikey|api[_-]?key|secret[_-]?key|access[_-]?key)\s*[:=]\s*\S+`),
	// Bare token-like values after common markers.
	regexp.MustCompile(`(?i)(token|secret|password|passwd|pwd|credential)\s*[:=]\s*\S+`),
	// PVE API token format in Authorization header or standalone: PVEAPIToken=user@realm!id=uuid
	regexp.MustCompile(`(?i)PVEAPIToken=\S+`),
	// Proxmox token ID format: user@realm!tokenid=uuid
	regexp.MustCompile(`[A-Za-z0-9._-]+@[A-Za-z0-9._-]+![A-Za-z0-9._-]+=\S+`),
	// JSON/YAML field patterns: "password": "value", "token_secret": "value"
	regexp.MustCompile(`(?i)"(token[_-]?(id|secret|value)?|password|secret|credential)"\s*:\s*"[^"]*"`),
	regexp.MustCompile(`(?i)'(token[_-]?(id|secret|value)?|password|secret|credential)'\s*:\s*'[^']*'`),
	// Bearer tokens: Bearer eyJ...
	regexp.MustCompile(`(?i)bearer\s+\S+`),
	// Basic auth: Basic base64string
	regexp.MustCompile(`(?i)basic\s+[A-Za-z0-9+/=]+`),
	// Credential-file references: file:profile
	regexp.MustCompile(`(?i)"?credential[_-]?ref"?\s*[:=]\s*"?file:\S+`),
	// Environment variable patterns: NODEX_*_TOKEN_SECRET=..., NODEX_*_PASSWORD=...
	regexp.MustCompile(`(?i)(NODEX|TOKEN|PASSWORD|SECRET|CREDENTIAL)_[A-Za-z0-9_]*=\S+`),
	// CSRF and session tokens: PVEAuthCookie, CSRFPreventionToken
	regexp.MustCompile(`(?i)(PVEAuthCookie|CSRFPreventionToken)=\S+`),
	// PEM-encoded private key content.
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
