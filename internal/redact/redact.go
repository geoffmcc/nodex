package redact

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

// ---------------------------------------------------------------------------
// Type-based redaction (primary)
// ---------------------------------------------------------------------------

// Secret is a string that never leaks into serialized, formatted, or logged
// output.  Every public rendering path returns [REDACTED]; the real value is
// only available through the explicit Raw method, which callers must use
// deliberately and only at the last possible moment before constructing an
// authenticated request.
type Secret string

// MarshalJSON implements json.Marshaler.
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(redacted) }

// MarshalYAML implements yaml.Marshaler by way of the standard MarshalYAML
// method (gopkg.in/yaml.v3).
func (s Secret) MarshalYAML() (any, error) { return redacted, nil }

// String implements fmt.Stringer (%s, %v).
func (s Secret) String() string { return redacted }

// GoString implements fmt.GoStringer (%#v).
func (s Secret) GoString() string { return redacted }

// Raw returns the real secret value.  Only use this when you are about to
// place the value into an Authorization header or equivalent; never log,
// serialize, or expose it.
func (s Secret) Raw() string { return string(s) }

// IsZero reports whether the secret is empty.
func (s Secret) IsZero() bool { return s == "" }

// Redactable is implemented by types that can produce a safely-redacted copy
// of themselves.  The returned value must be suitable for serialization;
// every sensitive field must be replaced with the [REDACTED] marker.
type Redactable interface {
	Redacted() any
}

// Sanitize returns a deep copy of v in which every Redactable value has been
// replaced by its Redacted() form.  Maps, slices, arrays, structs, and
// pointers are walked recursively.
func Sanitize(v any) any {
	return sanitize(v, true)
}

// sanitize does the recursive work.  The checkRedactable flag controls
// whether we test the top-level value for the Redactable interface.
// When a Redactable value returns its Redacted() form, we recursively
// walk the result but do NOT re-test it for Redactable (otherwise
// a type whose Redacted() returns the same concrete type would recurse
// infinitely).
func sanitize(v any, checkRedactable bool) any {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)

	// 0. Handle nil pointers and nil interfaces early so we never
	//    call methods on nil receivers.
	if rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
	}

	// 1. If the value itself is a Secret, redact it immediately.
	if _, ok := v.(Secret); ok {
		return Secret(redacted)
	}

	// 2. If allowed and the value implements Redactable, replace it and
	//    walk the result without re-checking Redactable on the top.
	if checkRedactable {
		if r, ok := v.(Redactable); ok {
			return sanitize(r.Redacted(), false)
		}
	}

	// 3. Dereference pointers (keep checkRedactable on the pointed-to value).
	if rv.Kind() == reflect.Ptr {
		return sanitize(rv.Elem().Interface(), checkRedactable)
	}

	// 3. Walk aggregate types.
	switch rv.Kind() {
	case reflect.Map:
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			sk := sanitize(iter.Key().Interface(), true)
			sv := sanitize(iter.Value().Interface(), true)
			if sk != nil && sv != nil {
				out.SetMapIndex(reflect.ValueOf(sk), reflect.ValueOf(sv))
			}
		}
		return out.Interface()

	case reflect.Slice:
		if rv.IsNil() {
			return nil
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Cap())
		for i := 0; i < rv.Len(); i++ {
			sv := sanitize(rv.Index(i).Interface(), true)
			if sv != nil {
				out.Index(i).Set(reflect.ValueOf(sv))
			}
		}
		return out.Interface()

	case reflect.Array:
		out := reflect.New(rv.Type()).Elem()
		for i := 0; i < rv.Len(); i++ {
			sv := sanitize(rv.Index(i).Interface(), true)
			if sv != nil {
				out.Index(i).Set(reflect.ValueOf(sv))
			}
		}
		return out.Interface()

	case reflect.Struct:
		// Walk exported, settable fields.
		out := reflect.New(rv.Type()).Elem()
		for i := 0; i < rv.NumField(); i++ {
			f := rv.Field(i)
			of := out.Field(i)
			if !f.CanInterface() || !of.CanSet() {
				continue
			}
			// Check for Secret-typed fields: unconditionally redact.
			if f.Type() == reflect.TypeOf(Secret("")) {
				of.Set(reflect.ValueOf(Secret(redacted)))
				continue
			}
			sanitized := sanitize(f.Interface(), true)
			if sanitized == nil {
				continue
			}
			sv := reflect.ValueOf(sanitized)
			// If the field expects a pointer but we got a value, wrap it.
			if of.Kind() == reflect.Ptr && sv.Kind() != reflect.Ptr {
				if sv.CanAddr() {
					sv = sv.Addr()
				} else {
					ptr := reflect.New(sv.Type())
					ptr.Elem().Set(sv)
					sv = ptr
				}
			}
			// If the field expects a value but we got a pointer, dereference.
			if of.Kind() != reflect.Ptr && sv.Kind() == reflect.Ptr && !sv.IsNil() {
				sv = sv.Elem()
			}
			if sv.Type().AssignableTo(of.Type()) {
				of.Set(sv)
			}
		}
		return out.Interface()

	case reflect.String:
		return v

	case reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return sanitize(rv.Elem().Interface(), checkRedactable)

	default:
		return v
	}
}

// ---------------------------------------------------------------------------
// Regex-based redaction (defense-in-depth for free-text output)
// ---------------------------------------------------------------------------

// pattern pairs a matching regexp with its replacement template. Most
// patterns replace the whole match with the redaction marker; structured
// (JSON/YAML) field patterns preserve the surrounding syntax so redacted
// documents remain parseable.
type pattern struct {
	re          *regexp.Regexp
	replacement string
}

// Patterns that indicate sensitive values.
// These are applied to free-text output (stdout, stderr, logs, error messages)
// AFTER type-based redaction. They serve as defense-in-depth.
var patterns = []pattern{
	// API tokens and keys in key=value or key:value form.
	{regexp.MustCompile(`(?i)(api[_-]?token|apikey|api[_-]?key|secret[_-]?key|access[_-]?key)\s*[:=]\s*\S+`), redacted},
	// Bare token-like values after common markers.
	{regexp.MustCompile(`(?i)(token|secret|password|passwd|pwd|credential)\s*[:=]\s*\S+`), redacted},
	// PVE API token format in Authorization header or standalone: PVEAPIToken=user@realm!id=uuid
	{regexp.MustCompile(`(?i)PVEAPIToken=\S+`), redacted},
	// PBS API token format in Authorization header or standalone: PBSAPIToken=user@realm!id:uuid
	{regexp.MustCompile(`(?i)PBSAPIToken=\S+`), redacted},
	// Proxmox token grammar, PVE form: user@realm!tokenid=uuid
	{regexp.MustCompile(`[A-Za-z0-9._-]+@[A-Za-z0-9._-]+![A-Za-z0-9._-]+=\S+`), redacted},
	// Proxmox token grammar, PBS form: user@realm!tokenid:uuid. The secret
	// tail must look like one (8+ alphanumeric/hyphen chars): PVE and PBS
	// task UPIDs legitimately end in "user@realm!tokenid:" as a field
	// terminator, and a bare \S+ here would corrupt every UPID in output.
	{regexp.MustCompile(`[A-Za-z0-9._-]+@[A-Za-z0-9._-]+![A-Za-z0-9._-]+:[A-Za-z0-9-]{8,}`), redacted},
	// JSON/YAML field patterns: "password": "value", "token_secret": "value".
	// The replacement keeps the key and quoting so JSON/YAML documents stay
	// structurally valid after redaction. Plain "tokenid"/"token_id" is
	// deliberately not matched: PVE token IDs are identifiers the API and UI
	// display (and Nodex's own credential prompt echoes them); only secrets
	// are hidden.
	{regexp.MustCompile(`(?i)"(token[_-]?(?:secret|value)|password|secret|credential)"\s*:\s*"[^"]*"`), `"$1": "` + redacted + `"`},
	{regexp.MustCompile(`(?i)'(token[_-]?(?:secret|value)|password|secret|credential)'\s*:\s*'[^']*'`), `'$1': '` + redacted + `'`},
	// Bearer tokens: Bearer eyJ...
	{regexp.MustCompile(`(?i)bearer\s+\S+`), redacted},
	// Basic auth: Basic base64string
	{regexp.MustCompile(`(?i)basic\s+[A-Za-z0-9+/=]+`), redacted},
	// Credential-file references: file:profile
	{regexp.MustCompile(`(?i)"?credential[_-]?ref"?\s*[:=]\s*"?file:\S+`), redacted},
	// Environment variable patterns: NODEX_*_TOKEN_SECRET=..., NODEX_*_PASSWORD=...
	{regexp.MustCompile(`(?i)(NODEX|TOKEN|PASSWORD|SECRET|CREDENTIAL)_[A-Za-z0-9_]*=\S+`), redacted},
	// CSRF and session tokens: PVEAuthCookie, PBSAuthCookie, CSRFPreventionToken
	{regexp.MustCompile(`(?i)(PVEAuthCookie|PBSAuthCookie|CSRFPreventionToken)=\S+`), redacted},
	// PEM-encoded private key content.
	{regexp.MustCompile(`-----BEGIN\s+[A-Z\s]*PRIVATE KEY-----`), redacted},
}

// String redacts sensitive patterns from the input.  This is defense-in-depth
// for free-text strings that have not been through the type-based Sanitize
// path.
func String(input string) string {
	result := input
	for _, p := range patterns {
		result = p.re.ReplaceAllString(result, p.replacement)
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

// Format safely formats a value for logging/debug output.  If the value
// implements Redactable, its Redacted() form is used first; otherwise
// the value is stringified as-is.  The result is then run through regex
// redaction for defense-in-depth.
func Format(v any) string {
	return String(fmt.Sprintf("%v", Sanitize(v)))
}
