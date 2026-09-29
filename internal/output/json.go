package output

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/geoffmcc/nodex/internal/redact"
)

// WriteJSON sanitizes data with type-based redaction and writes it as
// indented JSON.
//
// Redaction happens entirely before serialization. A previous implementation
// ran the free-text regex net over the *serialized bytes*, which could rewrite
// a key or a value inside quoted syntax and leave the caller holding something
// that no longer parses. Sanitize is now key-aware and also cleans string
// leaves, so the encoder is the last step and the result is always valid.
func WriteJSON(w io.Writer, data any) error {
	sanitized := sanitizeTerminalData(redact.Sanitize(data))
	raw, err := marshalIndentNoEscape(sanitized)
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n")
	return err
}

// MarshalJSON returns indented JSON bytes for data.
func MarshalJSON(data any) ([]byte, error) {
	return marshalIndentNoEscape(data)
}

// marshalIndentNoEscape marshals data as indented JSON without Go's default
// HTML escaping. Escaping <, > and & into < and friends corrupts
// content that legitimately contains them — URL query separators, shell
// redirections, XML fragments returned by PVE endpoints — and makes
// machine-readable output harder to consume for no security benefit: JSON
// strings are not HTML contexts.
func marshalIndentNoEscape(data any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(data); err != nil {
		return nil, err
	}
	// Encode appends a newline; WriteJSON adds its own.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
