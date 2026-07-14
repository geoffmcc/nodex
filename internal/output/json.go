package output

import (
	"encoding/json"
	"io"

	"github.com/geoffmcc/nodex/internal/redact"
)

// WriteJSON sanitizes data with type-based redaction, marshals it as
// indented JSON, applies regex defense-in-depth, and writes the result.
func WriteJSON(w io.Writer, data any) error {
	sanitized := redact.Sanitize(data)
	raw, err := json.MarshalIndent(sanitized, "", "  ")
	if err != nil {
		return err
	}
	clean := redact.Bytes(raw)
	_, err = w.Write(clean)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n")
	return err
}

// MarshalJSON returns indented JSON bytes for data.
func MarshalJSON(data any) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}
