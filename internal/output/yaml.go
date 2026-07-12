package output

import (
	"io"

	"gopkg.in/yaml.v3"

	"github.com/geoffmcc/nodex/internal/redact"
)

// WriteYAML marshals data as YAML and writes it through redaction.
func WriteYAML(w io.Writer, data any) error {
	raw, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	redacted := redact.Bytes(raw)
	_, err = w.Write(redacted)
	return err
}

// MarshalYAML returns YAML bytes for data.
func MarshalYAML(data any) ([]byte, error) {
	return yaml.Marshal(data)
}
