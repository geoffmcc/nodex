package redact

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Regex-based redaction tests (defense-in-depth)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Secret type tests
// ---------------------------------------------------------------------------

func TestSecretJSONMarshal(t *testing.T) {
	s := Secret("super-secret-value")
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(Secret): %v", err)
	}
	if string(data) != `"[REDACTED]"` {
		t.Errorf("json.Marshal(Secret) = %s, want %q", string(data), `"[REDACTED]"`)
	}
	// Make sure the raw value is NOT in the JSON.
	if strings.Contains(string(data), "super-secret-value") {
		t.Error("raw secret leaked in JSON marshal")
	}
}

func TestSecretYAMLMarshal(t *testing.T) {
	s := Secret("super-secret-value")
	data, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("yaml.Marshal(Secret): %v", err)
	}
	if !strings.Contains(string(data), redacted) {
		t.Errorf("yaml.Marshal(Secret) missing redacted marker: %s", string(data))
	}
	if strings.Contains(string(data), "super-secret-value") {
		t.Error("raw secret leaked in YAML marshal")
	}
}

func TestSecretString(t *testing.T) {
	s := Secret("my-password")
	if s.String() != redacted {
		t.Errorf("Secret.String() = %q, want %q", s.String(), redacted)
	}
	// %s should also redact.
	formatted := fmt.Sprintf("%s", s)
	if formatted != redacted {
		t.Errorf("fmt.Sprintf(%%s, Secret) = %q, want %q", formatted, redacted)
	}
}

func TestSecretGoString(t *testing.T) {
	s := Secret("my-password")
	// %#v should NOT leak the raw value.
	formatted := fmt.Sprintf("%#v", s)
	if formatted != redacted {
		t.Errorf("fmt.Sprintf(%%#v, Secret) = %q, want %q", formatted, redacted)
	}
	if strings.Contains(formatted, "my-password") {
		t.Error("raw secret leaked via GoString")
	}
}

func TestSecretVVerb(t *testing.T) {
	s := Secret("my-password")
	// %v should also redact.
	formatted := fmt.Sprintf("%v", s)
	if formatted != redacted {
		t.Errorf("fmt.Sprintf(%%v, Secret) = %q, want %q", formatted, redacted)
	}
}

func TestSecretRaw(t *testing.T) {
	s := Secret("my-password")
	if s.Raw() != "my-password" {
		t.Errorf("Secret.Raw() = %q, want %q", s.Raw(), "my-password")
	}
}

func TestSecretIsZero(t *testing.T) {
	var s Secret
	if !s.IsZero() {
		t.Error("empty Secret.IsZero() should be true")
	}
	s = Secret("val")
	if s.IsZero() {
		t.Error("non-empty Secret.IsZero() should be false")
	}
}

// ---------------------------------------------------------------------------
// Redactable and Sanitize tests
// ---------------------------------------------------------------------------

// testStruct is a sample struct with a Secret field for testing.
type testStruct struct {
	Name     string `json:"name"`
	Token    Secret `json:"token"`
	Password Secret `json:"password,omitempty"`
	Enabled  bool   `json:"enabled"`
	Count    int    `json:"count"`
}

// Ensure testStruct implements Redactable.
func (t testStruct) Redacted() any {
	return testStruct{
		Name:     t.Name,
		Token:    Secret(redacted),
		Password: Secret(redacted),
		Enabled:  t.Enabled,
		Count:    t.Count,
	}
}

func TestSanitizeStruct(t *testing.T) {
	ts := testStruct{
		Name:     "admin",
		Token:    Secret("real-token"),
		Password: Secret("real-password"),
		Enabled:  true,
		Count:    42,
	}
	sanitized := Sanitize(ts).(testStruct)

	if sanitized.Name != "admin" {
		t.Errorf("Name = %q, want %q", sanitized.Name, "admin")
	}
	if sanitized.Token.Raw() != redacted {
		t.Errorf("Token = %q, want %q", sanitized.Token.Raw(), redacted)
	}
	if sanitized.Password.Raw() != redacted {
		t.Errorf("Password = %q, want %q", sanitized.Password.Raw(), redacted)
	}
	if sanitized.Enabled != true {
		t.Error("Enabled should remain true")
	}
	if sanitized.Count != 42 {
		t.Errorf("Count = %d, want 42", sanitized.Count)
	}
}

func TestSanitizeJSONValidAfterRedaction(t *testing.T) {
	ts := testStruct{
		Name:     "admin",
		Token:    Secret("real-token"),
		Password: Secret("real-password"),
		Enabled:  true,
		Count:    42,
	}
	sanitized := Sanitize(ts)

	data, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("JSON marshal after sanitize failed: %v", err)
	}

	// Verify it's still valid JSON.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("redacted output is not valid JSON: %v\noutput: %s", err, data)
	}

	// Secrets must be absent from JSON.
	raw := string(data)
	if strings.Contains(raw, "real-token") {
		t.Error("Token leaked in JSON output")
	}
	if strings.Contains(raw, "real-password") {
		t.Error("Password leaked in JSON output")
	}

	// Non-secret fields must be present.
	if m["name"] != "admin" {
		t.Errorf("name = %v, want admin", m["name"])
	}
}

func TestSanitizeYAMLValidAfterRedaction(t *testing.T) {
	ts := testStruct{
		Name:     "admin",
		Token:    Secret("real-token"),
		Password: Secret("real-password"),
		Enabled:  true,
		Count:    42,
	}
	sanitized := Sanitize(ts)

	data, err := yaml.Marshal(sanitized)
	if err != nil {
		t.Fatalf("YAML marshal after sanitize failed: %v", err)
	}

	// Verify it's still valid YAML.
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("redacted output is not valid YAML: %v\noutput: %s", err, data)
	}

	// Secrets must be absent.
	raw := string(data)
	if strings.Contains(raw, "real-token") {
		t.Error("Token leaked in YAML output")
	}
	if strings.Contains(raw, "real-password") {
		t.Error("Password leaked in YAML output")
	}
}

func TestSanitizeNestedStruct(t *testing.T) {
	type inner struct {
		Secret Secret `json:"secret"`
		Value  string `json:"value"`
	}
	type outer struct {
		Name  string `json:"name"`
		Inner inner  `json:"inner"`
	}
	o := outer{
		Name: "test",
		Inner: inner{
			Secret: Secret("nested-secret"),
			Value:  "regular-value",
		},
	}
	sanitized := Sanitize(o).(outer)

	if sanitized.Inner.Secret.Raw() != redacted {
		t.Errorf("nested Secret = %q, want %q", sanitized.Inner.Secret.Raw(), redacted)
	}
	if sanitized.Inner.Value != "regular-value" {
		t.Errorf("nested Value = %q, want regular-value", sanitized.Inner.Value)
	}
}

func TestSanitizeSliceOfSecrets(t *testing.T) {
	type item struct {
		Key   string `json:"key"`
		Value Secret `json:"value"`
	}
	items := []item{
		{Key: "k1", Value: Secret("s1")},
		{Key: "k2", Value: Secret("s2")},
	}
	sanitized := Sanitize(items).([]item)

	for i, it := range sanitized {
		if it.Value.Raw() != redacted {
			t.Errorf("item[%d].Value = %q, want %q", i, it.Value.Raw(), redacted)
		}
	}

	data, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("slice JSON marshal: %v", err)
	}
	if strings.Contains(string(data), "s1") || strings.Contains(string(data), "s2") {
		t.Error("secrets leaked in slice JSON")
	}
}

func TestSanitizeMapOfSecrets(t *testing.T) {
	m := map[string]Secret{
		"token":  Secret("abc123"),
		"secret": Secret("xyz789"),
	}
	sanitized := Sanitize(m).(map[string]Secret)

	for k, v := range sanitized {
		if v.Raw() != redacted {
			t.Errorf("map[%s] = %q, want %q", k, v.Raw(), redacted)
		}
	}

	data, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("map JSON marshal: %v", err)
	}
	if strings.Contains(string(data), "abc123") || strings.Contains(string(data), "xyz789") {
		t.Error("secrets leaked in map JSON")
	}
}

func TestSanitizeNilValue(t *testing.T) {
	result := Sanitize(nil)
	if result != nil {
		t.Errorf("Sanitize(nil) = %v, want nil", result)
	}
}

func TestSanitizeNilPointer(t *testing.T) {
	var ts *testStruct
	result := Sanitize(ts)
	if result != nil {
		t.Errorf("Sanitize(nil *testStruct) = %v, want nil", result)
	}
}

func TestSanitizeNilSlice(t *testing.T) {
	var s []testStruct
	result := Sanitize(s)
	if result != nil {
		t.Errorf("Sanitize(nil slice) = %v, want nil", result)
	}
}

func TestSanitizeNonRedactableString(t *testing.T) {
	result := Sanitize("hello")
	if result != "hello" {
		t.Errorf("Sanitize(hello) = %v, want hello", result)
	}
}

func TestSanitizeInt(t *testing.T) {
	result := Sanitize(42)
	if result != 42 {
		t.Errorf("Sanitize(42) = %v, want 42", result)
	}
}

// errorWithSecrets is a test error type carrying secret fields.
type errorWithSecrets struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Token   Secret `json:"token"`
}

func (e errorWithSecrets) Error() string {
	return fmt.Sprintf("error %d: %s", e.Code, e.Message)
}

func (e errorWithSecrets) Redacted() any {
	return errorWithSecrets{
		Code:    e.Code,
		Message: e.Message,
		Token:   Secret(redacted),
	}
}

func TestSanitizeErrorObject(t *testing.T) {
	e := errorWithSecrets{Code: 500, Message: "internal error", Token: Secret("leaked-token")}
	sanitized := Sanitize(e).(errorWithSecrets)

	if sanitized.Token.Raw() != redacted {
		t.Errorf("error Token = %q, want %q", sanitized.Token.Raw(), redacted)
	}
	if sanitized.Code != 500 {
		t.Errorf("error Code = %d, want 500", sanitized.Code)
	}
}

func TestSanitizeDoesNotCorruptEscaping(t *testing.T) {
	ts := testStruct{
		Name:     "test\nwith\nnewlines",
		Token:    Secret("tok\nwith\nnewlines"),
		Password: Secret("pass\r\nwith\rcrlf"),
		Enabled:  true,
		Count:    1,
	}
	sanitized := Sanitize(ts)

	data, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("JSON marshal with escaping: %v", err)
	}
	// The redacted JSON should still be valid.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("JSON with escaping is invalid after redaction: %v\noutput: %s", err, data)
	}
	// Name should preserve its escaping in JSON.
	if m["name"] != "test\nwith\nnewlines" {
		t.Errorf("name escaping corrupted: %v", m["name"])
	}
}

func TestSanitizeArrayOfStructs(t *testing.T) {
	type entry struct {
		ID   int    `json:"id"`
		Pass Secret `json:"pass"`
	}
	arr := [3]entry{
		{ID: 1, Pass: Secret("a")},
		{ID: 2, Pass: Secret("b")},
		{ID: 3, Pass: Secret("c")},
	}
	sanitized := Sanitize(arr).([3]entry)

	for i, e := range sanitized {
		if e.Pass.Raw() != redacted {
			t.Errorf("arr[%d].Pass = %q, want %q", i, e.Pass.Raw(), redacted)
		}
	}
}

func TestSanitizeEmptySlice(t *testing.T) {
	s := []testStruct{}
	result := Sanitize(s)
	sl, ok := result.([]testStruct)
	if !ok {
		t.Fatalf("expected []testStruct, got %T", result)
	}
	if len(sl) != 0 {
		t.Errorf("empty slice len = %d, want 0", len(sl))
	}
}

func TestSanitizeInterfaceContainingRedactable(t *testing.T) {
	var v any = testStruct{Name: "iface", Token: Secret("secret"), Enabled: true}
	result := Sanitize(v)
	ts, ok := result.(testStruct)
	if !ok {
		t.Fatalf("expected testStruct from interface{}, got %T", result)
	}
	if ts.Token.Raw() != redacted {
		t.Errorf("interface Token = %q, want %q", ts.Token.Raw(), redacted)
	}
}

// ---------------------------------------------------------------------------
// Debug non-leakage tests
// ---------------------------------------------------------------------------

func TestSecretDoesNotAppearInDebugFormat(t *testing.T) {
	s := Secret("debug-should-not-see-this")

	// All format verbs should hide the secret.
	for _, verb := range []string{"%s", "%v", "%#v", "%q"} {
		formatted := fmt.Sprintf(verb, s)
		if strings.Contains(formatted, "debug-should-not-see-this") {
			t.Errorf("verb %s leaked secret: %q", verb, formatted)
		}
	}
}

func TestSecretDoesNotAppearInError(t *testing.T) {
	s := Secret("error-secret")
	err := fmt.Errorf("some error with secret: %v", s)
	if strings.Contains(err.Error(), "error-secret") {
		t.Errorf("error message leaked secret: %q", err.Error())
	}
	if !strings.Contains(err.Error(), redacted) {
		t.Errorf("error message missing redacted marker: %q", err.Error())
	}
}

func TestSanitizeThenJSONDoesNotLeak(t *testing.T) {
	type config struct {
		APIToken Secret `json:"api_token"`
		Host     string `json:"host"`
	}
	c := config{APIToken: Secret("very-secret-token-value"), Host: "pve.example.com"}
	sanitized := Sanitize(c)

	data, _ := json.Marshal(sanitized)
	raw := string(data)
	if strings.Contains(raw, "very-secret-token-value") {
		t.Errorf("JSON leaked secret: %s", raw)
	}
}

func TestSanitizePointerToStruct(t *testing.T) {
	ts := &testStruct{
		Name:     "ptr",
		Token:    Secret("ptr-secret"),
		Password: Secret("ptr-pass"),
		Enabled:  true,
	}
	sanitized := Sanitize(ts).(testStruct)

	if sanitized.Token.Raw() != redacted {
		t.Errorf("pointer Token = %q, want %q", sanitized.Token.Raw(), redacted)
	}
	if sanitized.Password.Raw() != redacted {
		t.Errorf("pointer Password = %q, want %q", sanitized.Password.Raw(), redacted)
	}
}

func TestFormatFunctionRedacts(t *testing.T) {
	ts := testStruct{Name: "test", Token: Secret("format-secret")}
	formatted := Format(ts)
	if strings.Contains(formatted, "format-secret") {
		t.Errorf("Format leaked secret: %q", formatted)
	}
	if !strings.Contains(formatted, redacted) {
		t.Errorf("Format missing redacted marker: %q", formatted)
	}
}

// ---------------------------------------------------------------------------
// RedactedLeavesJSONStructure — updated for type-based redaction
// ---------------------------------------------------------------------------

func TestTypeBasedRedactionPreservesJSONStructure(t *testing.T) {
	ts := testStruct{
		Name:     "admin@pve",
		Password: Secret("real-password"),
		Enabled:  true,
	}
	sanitized := Sanitize(ts)

	data, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("type-based JSON marshal failed: %v", err)
	}

	// Must be valid JSON.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("type-based output is not valid JSON: %v\noutput: %s", err, data)
	}

	raw := string(data)
	// Non-secret fields must be intact.
	if !strings.Contains(raw, "admin@pve") {
		t.Errorf("non-secret field lost in redaction: %s", raw)
	}
	// Secrets must be absent.
	if strings.Contains(raw, "real-password") {
		t.Error("password leaked in type-based JSON output")
	}
	// Redacted marker must be present.
	if !strings.Contains(raw, redacted) {
		t.Error("type-based JSON missing redacted marker")
	}
}

func TestTypeBasedRedactionPreservesYAMLStructure(t *testing.T) {
	ts := testStruct{
		Name:     "admin@pve",
		Password: Secret("real-password"),
		Enabled:  true,
	}
	sanitized := Sanitize(ts)

	data, err := yaml.Marshal(sanitized)
	if err != nil {
		t.Fatalf("type-based YAML marshal failed: %v", err)
	}

	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("type-based output is not valid YAML: %v\noutput: %s", err, data)
	}

	raw := string(data)
	if strings.Contains(raw, "real-password") {
		t.Error("password leaked in type-based YAML output")
	}
}

func TestTypeBasedRedactionDefenseInDepth(t *testing.T) {
	// Even after type-based redaction, if somehow a secret string sneaks
	// into free text, the regex defense-in-depth should catch it.
	raw := fmt.Sprintf("some log: PVEAPIToken=%s", "user@pam!test=some-uuid-val")
	redacted := String(raw)
	if strings.Contains(redacted, "some-uuid-val") {
		t.Errorf("regex defense-in-depth failed to redact: %q", redacted)
	}
}
