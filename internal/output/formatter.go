package output

import (
	"fmt"
	"io"
	"os"

	"github.com/geoffmcc/nodex/internal/redact"
)

// Format represents the output format type.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// Formatter handles structured output rendering.
type Formatter struct {
	format Format
	w      io.Writer
	color  bool
}

// New creates a Formatter writing to w in the given format.
func New(w io.Writer, format Format, color bool) *Formatter {
	return &Formatter{format: format, w: w, color: color}
}

// DefaultFormat returns table for TTY, json for non-TTY.
func DefaultFormat() Format {
	if isTerminal(os.Stdout) {
		return FormatTable
	}
	return FormatJSON
}

// Format returns the current output format.
func (f *Formatter) Format() Format {
	return f.format
}

// WriteRaw writes unformatted data through the redaction pipeline.
func (f *Formatter) WriteRaw(data []byte) error {
	redacted := redact.Bytes(data)
	_, err := f.w.Write(redacted)
	return err
}

// WriteString writes a string through the redaction pipeline.
func (f *Formatter) WriteString(s string) error {
	redacted := redact.String(s)
	_, err := fmt.Fprint(f.w, redacted)
	return err
}
