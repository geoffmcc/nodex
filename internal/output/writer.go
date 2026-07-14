package output

import (
	"fmt"
	"io"
)

// Writer separates primary output (stdout) from diagnostic output (stderr).
// When the output format is machine-readable (JSON or YAML), diagnostics must
// never contaminate stdout because automation consumers parse stdout as strict
// structured data.  For human-oriented formats (table, text) diagnostics still
// go to stderr since they are supplementary messages that should not mix with
// the primary formatted output.
type Writer struct {
	stdout io.Writer
	stderr io.Writer
	format Format
}

// NewWriter creates a Writer that sends primary output to stdout and
// diagnostics to stderr.
func NewWriter(stdout, stderr io.Writer, format Format) *Writer {
	return &Writer{
		stdout: stdout,
		stderr: stderr,
		format: format,
	}
}

// Stdout returns the primary output writer.
func (w *Writer) Stdout() io.Writer {
	return w.stdout
}

// Stderr returns the diagnostic output writer.
func (w *Writer) Stderr() io.Writer {
	return w.stderr
}

// Format returns the current output format.
func (w *Writer) Format() Format {
	return w.format
}

// Diagnostic writes a diagnostic message to stderr.  In JSON and YAML modes
// this is mandatory to keep stdout clean for machine parsing.  In table mode
// diagnostics also go to stderr because they are supplementary and should not
// interleave with tabular data.
func (w *Writer) Diagnostic(msg string) {
	clean := SanitizeTerminal(msg)
	fmt.Fprintln(w.stderr, clean)
}

// Diagnosticf writes a formatted diagnostic message to stderr.
func (w *Writer) Diagnosticf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	w.Diagnostic(msg)
}
