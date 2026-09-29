package output

import (
	"io"

	"gopkg.in/yaml.v3"

	"github.com/geoffmcc/nodex/internal/redact"
)

// WriteYAML sanitizes data with type-based redaction and writes it as YAML.
//
// As with WriteJSON, redaction completes before serialization so a redacted
// document is always parseable and never has a field name rewritten out from
// under the reader.
func WriteYAML(w io.Writer, data any) error {
	sanitized := sanitizeTerminalData(redact.Sanitize(data))
	raw, err := yaml.Marshal(sanitized)
	if err != nil {
		return err
	}
	_, err = w.Write(raw)
	return err
}

// MarshalYAML returns YAML bytes for data.
func MarshalYAML(data any) ([]byte, error) {
	return yaml.Marshal(data)
}
