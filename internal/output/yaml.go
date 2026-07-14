package output

import (
	"io"

	"gopkg.in/yaml.v3"

	"github.com/geoffmcc/nodex/internal/redact"
)

// WriteYAML sanitizes data with type-based redaction, marshals it as YAML,
// applies regex defense-in-depth, and writes the result.
func WriteYAML(w io.Writer, data any) error {
	sanitized := redact.Sanitize(data)
	raw, err := yaml.Marshal(sanitized)
	if err != nil {
		return err
	}
	clean := redact.Bytes(raw)
	_, err = w.Write(clean)
	return err
}

// MarshalYAML returns YAML bytes for data.
func MarshalYAML(data any) ([]byte, error) {
	return yaml.Marshal(data)
}
