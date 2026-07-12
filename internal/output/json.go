package output

import (
	"encoding/json"
	"io"

	"github.com/geoffmcc/nodex/internal/redact"
)

// WriteJSON marshals data as indented JSON and writes it through redaction.
func WriteJSON(w io.Writer, data any) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	redacted := redact.Bytes(raw)
	_, err = w.Write(redacted)
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
