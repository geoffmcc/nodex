package output

import (
	"io"

	"github.com/geoffmcc/nodex/internal/redact"
)

// SanitizingWriter wraps a text output sink and applies defense-in-depth
// redaction plus terminal-control stripping to every write. It protects direct
// fmt.Fprint/Fprintf call sites that do not pass through structured formatters.
type SanitizingWriter struct {
	w io.Writer
}

// NewSanitizingWriter returns a writer that sanitizes all bytes before writing
// them to w. A nil writer is left nil by callers rather than wrapped.
func NewSanitizingWriter(w io.Writer) *SanitizingWriter {
	return &SanitizingWriter{w: w}
}

func (w *SanitizingWriter) Write(p []byte) (int, error) {
	clean := SanitizeTerminal(redact.String(string(p)))
	_, err := io.WriteString(w.w, clean)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
