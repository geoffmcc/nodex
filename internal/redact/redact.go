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
//
// Sanitize is additionally *key-aware*: a value stored under a map key or
// struct field whose name denotes a secret (see secretKeyName) is replaced by
// the redaction marker even when the value is a plain string.  This is what
// makes structured output safe without relying on byte-level regex rewriting
// of an already-serialized document: the key survives, only the value is
// removed, so JSON and YAML stay syntactically valid and machine-readable.
func Sanitize(v any) any {
	return sanitize(v, true)
}

// secretLabelAlt is the set of field/key name fragments that denote a secret.
// It is the single source of truth shared by the key-aware walk below and the
// free-text patterns in String.
const secretLabelAlt = `api[_-]?token|apikey|api[_-]?key|secret[_-]?key|access[_-]?key|access[_-]?token|auth[_-]?token|token|secret|password|passwd|pwd|credential|credentials|passphrase|private[_-]?key|authorization`

// secretLabel is secretLabelAlt wrapped in a capture group so the
// free-text patterns can preserve the matched key in their replacement.
const secretLabel = `(` + secretLabelAlt + `)`

// secretKeyName matches a map key or serialized field name that denotes a
// secret.  The separators bound the fragment so that unrelated names such as
// "tokenizer" or "monkey" are not treated as secrets.
var secretKeyName = regexp.MustCompile(`(?i)(^|[._-])(?:` + secretLabelAlt + `)($|[._-])`)

// nonSecretKeys are names that look sensitive but are identifiers or policy
// metadata that PVE, PBS, and nodex legitimately display.  Redacting them
// would destroy useful output (and, historically, did: `tokenid` was being
// replaced alongside real token secrets).
var nonSecretKeys = map[string]bool{
	"tokenid":         true,
	"token_id":        true,
	"token_name":      true,
	"tokentype":       true,
	"token_type":      true,
	"credential_type": true,
	"key_type":        true,
	"key_size":        true,
	"alg":             true,
	"password_policy": true,
}

// isSecretKey reports whether a map key or serialized field name denotes a
// secret.
func isSecretKey(name string) bool {
	if nonSecretKeys[strings.ToLower(name)] {
		return false
	}
	return secretKeyName.MatchString(name)
}

var secretType = reflect.TypeOf(Secret(""))

// fieldName resolves the name a struct field is serialized under, preferring
// the JSON tag, then the YAML tag, and finally the Go field name.  Matching
// on the serialized name means redaction follows exactly what a consumer of
// the document sees.
func fieldName(f reflect.StructField) string {
	for _, tagName := range []string{"json", "yaml"} {
		tag := f.Tag.Get(tagName)
		if tag == "" || tag == "-" {
			continue
		}
		if name, _, _ := strings.Cut(tag, ","); name != "" {
			return name
		}
	}
	return f.Name
}

// redactTo replaces the value at rv with the redaction marker, converting
// where that is meaningful.  Numeric and boolean values are deliberately left
// alone: a count or a flag under a sensitive-looking name is metadata, not a
// secret, and silently rewriting it would corrupt structured output.
func redactTo(rv reflect.Value) {
	if !rv.CanSet() {
		return
	}
	if rv.Type() == secretType {
		rv.Set(reflect.ValueOf(Secret(redacted)))
		return
	}
	switch rv.Kind() {
	case reflect.String:
		rv.SetString(redacted)
	case reflect.Interface:
		if rv.NumMethod() == 0 {
			rv.Set(reflect.ValueOf(redacted))
		} else {
			rv.Set(reflect.Zero(rv.Type()))
		}
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
		rv.Set(reflect.Zero(rv.Type()))
	}
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
	if rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
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
	if rv.Kind() == reflect.Pointer {
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
			if sk == nil || sv == nil {
				continue
			}
			// Key-aware redaction: a secret-named key loses its value even
			// when the value is a plain string.
			if key, ok := sk.(string); ok && isSecretKey(key) {
				slot := reflect.New(rv.Type().Elem()).Elem()
				slot.Set(reflect.ValueOf(sv))
				redactTo(slot)
				sv = slot.Interface()
			}
			out.SetMapIndex(reflect.ValueOf(sk), reflect.ValueOf(sv))
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
			if of.Kind() == reflect.Pointer && sv.Kind() != reflect.Pointer {
				if sv.CanAddr() {
					sv = sv.Addr()
				} else {
					ptr := reflect.New(sv.Type())
					ptr.Elem().Set(sv)
					sv = ptr
				}
			}
			// If the field expects a value but we got a pointer, dereference.
			if of.Kind() != reflect.Pointer && sv.Kind() == reflect.Pointer && !sv.IsNil() {
				sv = sv.Elem()
			}
			if sv.Type().AssignableTo(of.Type()) {
				of.Set(sv)
			}
			// Key-aware redaction applied last so that a Secret-typed field
			// still round-trips as a Secret while a plain string under a
			// secret-named field is masked.
			if isSecretKey(fieldName(rv.Type().Field(i))) {
				redactTo(of)
			}
		}
		return out.Interface()

	case reflect.String:
		// String leaves still get the free-text net, because a credential can
		// be embedded inside an otherwise harmless field (captured stdout, a
		// server message, a log line). Doing it here rather than after
		// serialization is what makes structured output safe: the value is
		// cleaned while it is still a Go string, so the encoder can always
		// emit valid JSON/YAML around it.
		//
		// The result must keep the field's exact type, so a named string type
		// (e.g. `type Status string`) is rebuilt rather than replaced by a
		// bare string — otherwise the value stops being assignable to the
		// field and silently zeroes it.
		raw := rv.String()
		cleaned := String(raw)
		if cleaned == raw {
			return v
		}
		out := reflect.New(rv.Type()).Elem()
		out.SetString(cleaned)
		return out.Interface()

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
//
// These are applied to free-text output (stdout, stderr, logs, error messages)
// AFTER type-based redaction.  They serve as defense-in-depth; the primary
// mechanism is the type-based, key-aware Sanitize walk above.
//
// Two properties are maintained deliberately:
//
//  1. Every replacement is structure-preserving.  A pattern rewrites only the
//     value, never the key or the surrounding punctuation, so a redacted
//     document remains parseable and a redacted message keeps its grammar.
//     (Rewriting a whole match with the marker deleted the key as well,
//     producing output like `" [REDACTED]": "hunter2"`, which is not valid
//     JSON and which told the reader nothing.)
//
//  2. The generic `key: value` form requires a *secret-shaped* value — one
//     containing a digit or an auth/base64 character.  This is what stops the
//     defense-in-depth net from destroying diagnostics, where a sensitive
//     word appears as prose: "read password: EOF" must survive, while
//     "password: hunter2" and "api_token: a1b2c3d4e5f6" must not.
//
//     The equals form and the credential grammars stay unconditional, because
//     `key=value` is assignment syntax rather than prose and those match with
//     high confidence.  Values under a secret-named key in structured output
//     are covered by Sanitize's key-aware walk regardless of their shape.
var patterns = []pattern{
	// Proxmox token grammar, PVE form: user@realm!tokenid=uuid.
	{regexp.MustCompile(`[A-Za-z0-9._-]+@[A-Za-z0-9._-]+![A-Za-z0-9._-]+=\S+`), redacted},
	// Proxmox token grammar, PBS form: user@realm!tokenid:uuid. The secret
	// tail must look like one (8+ alphanumeric/hyphen chars): PVE and PBS
	// task UPIDs legitimately end in "user@realm!tokenid:" as a field
	// terminator, and a bare \S+ here would corrupt every UPID in output.
	{regexp.MustCompile(`[A-Za-z0-9._-]+@[A-Za-z0-9._-]+![A-Za-z0-9._-]+:[A-Za-z0-9-]{8,}`), redacted},
	// PVE API token: PVEAPIToken=user@realm!id=uuid
	{regexp.MustCompile(`(?i)(PVEAPIToken)=\S+`), `$1=` + redacted},
	// PBS API token: PBSAPIToken=user@realm!id:uuid
	{regexp.MustCompile(`(?i)(PBSAPIToken)=\S+`), `$1=` + redacted},
	// CSRF and session cookies: PVEAuthCookie, PBSAuthCookie,
	// CSRFPreventionToken. Kept ahead of the generic key=value rule so the
	// match is exact and cannot be cut short by a more permissive pattern.
	{regexp.MustCompile(`(?i)(PVEAuthCookie|PBSAuthCookie|CSRFPreventionToken)=\S+`), `$1=` + redacted},
	// Environment assignments: NODEX_*_TOKEN_SECRET=..., NODEX_*_PASSWORD=...
	{regexp.MustCompile(`(?i)\b(NODEX|TOKEN|PASSWORD|SECRET|CREDENTIAL)(_[A-Za-z0-9_]*)=\S+`), `$1$2=` + redacted},
	// PEM-encoded private key content.
	{regexp.MustCompile(`-----BEGIN\s+[A-Z\s]*PRIVATE KEY-----`), redacted},
	// Bearer tokens: Bearer eyJ...
	{regexp.MustCompile(`(?i)\b(bearer)\s+\S+`), `$1 ` + redacted},
	// Basic auth: Basic base64string
	{regexp.MustCompile(`(?i)\b(basic)\s+[A-Za-z0-9+/=]+`), `$1 ` + redacted},
}

// labelRule redacts the value introduced by a secret-named key. Unlike the
// mechanical patterns above it keeps the key, the separator, and the value's
// original quoting style, because these are the forms that actually appear in
// serialized documents and in prose.
//
// requireSecretShape makes the colon form conditional on the value looking
// like a credential, so diagnostics such as "read password: EOF" survive while
// "password: hunter2" and "api_token: a1b2c3d4" do not. It is disabled for the
// equals form ("=" is assignment syntax, not prose) and for credential_ref,
// which is always masked.
type labelRule struct {
	re                 *regexp.Regexp
	requireSecretShape bool
}

// labelRules are applied by redactLabelled after the mechanical patterns.
// Quoted and bare values are matched separately: a rule that accepts both
// would let the quoted form swallow the bare one and defeat the
// secret-shape guard that protects prose.
var labelRules = []labelRule{
	// Quoted structured fields: "password": "value", "token_secret": "value",
	// 'credential_ref': 'value'. Unconditional — a quoted value under a
	// secret-named key is masked whatever it contains.
	{
		re: regexp.MustCompile(`(?i)(["']?\b(?:` + secretLabelAlt + `)[_-]?(?:secret|value|ref)?["']?)(\s*[:=]\s*)("[^"]*"|'[^']*')`),
	},
	// Bare assignment form: key=value. Unconditional — "=" is assignment
	// syntax and does not occur in the prose this net must not damage.
	{
		re: regexp.MustCompile(`(?i)\b` + secretLabel + `(\s*=\s*)([^\s,]+)`),
	},
	// Bare prose form: key: value. The value must look like a secret
	// (contains a digit or an auth/base64 character) so that a diagnostic
	// such as "read password: EOF" or "invalid secret: too short" is left
	// intact. A short, purely alphabetic value is a word, not a credential.
	{
		re:                 regexp.MustCompile(`(?i)\b` + secretLabel + `(\s*:\s*)([^\s,]+)`),
		requireSecretShape: true,
	},
	// Credential-file references: credential_ref: file:profile. Always
	// masked regardless of the store name's shape, quoted or bare.
	{
		re: regexp.MustCompile(`(?i)\b(credential[_-]?ref["']?)(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,]*)`),
	},
}

// secretShape matches a value that looks like a credential rather than a
// word: it contains a digit or an auth/base64 character.
var secretShape = regexp.MustCompile(`[0-9+/=@]`)

// isRedactedValue reports whether a captured value is already the redaction
// marker, with optional JSON or YAML quoting.
//
// Redaction has to be idempotent. Output passes through the net more than
// once: the structured writers redact before marshaling, and
// output.SanitizingWriter then redacts the serialized bytes on the way to
// stdout. Without this guard the credential_ref rule re-matched the already
// redacted text — its value group can match zero characters, so on
// "credential_ref: '[REDACTED]'" it replaced the label and separator, matched
// nothing, and left the quoted marker behind as:
// credential_ref: [REDACTED]'[REDACTED]'
func isRedactedValue(v string) bool {
	return strings.Trim(v, `"'`) == redacted
}

// redactLabelled applies the label rules. A match whose value is already
// redacted is returned byte-identical.
func redactLabelled(s string) string {
	for _, rule := range labelRules {
		s = rule.re.ReplaceAllStringFunc(s, func(m string) string {
			g := rule.re.FindStringSubmatch(m)
			if len(g) < 4 {
				return m
			}
			label, sep, val := g[1], g[2], g[3]
			if isRedactedValue(val) {
				return m
			}
			// An empty bare value carries no secret; leaving it alone keeps
			// the rule from inventing a redaction in "credential_ref: ".
			if val == "" {
				return m
			}
			if rule.requireSecretShape && !secretShape.MatchString(val) {
				return m
			}
			switch val[0] {
			case '"':
				return label + sep + `"` + redacted + `"`
			case '\'':
				return label + sep + `'` + redacted + `'`
			default:
				return label + sep + redacted
			}
		})
	}
	return s
}

// String redacts sensitive patterns from the input.  This is defense-in-depth
// for free-text strings that have not been through the type-based Sanitize
// path.
func String(input string) string {
	result := input
	for _, p := range patterns {
		result = p.re.ReplaceAllString(result, p.replacement)
	}
	return redactLabelled(result)
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
