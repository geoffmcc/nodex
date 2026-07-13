package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/geoffmcc/nodex/internal/redact"
)

// TableWriter renders tabular output.
type TableWriter struct {
	w io.Writer
	t *tabwriter.Writer
}

// NewTable creates a TableWriter writing to w.
func NewTable(w io.Writer) *TableWriter {
	return &TableWriter{
		w: w,
		t: tabwriter.NewWriter(w, 0, 0, 2, ' ', 0),
	}
}

// WriteHeader writes column headers.
func (tw *TableWriter) WriteHeader(headers ...string) {
	for i, h := range headers {
		headers[i] = SanitizeTerminal(redact.String(h))
	}
	fmt.Fprintln(tw.t, strings.Join(headers, "\t"))
}

// WriteRow writes a data row.
func (tw *TableWriter) WriteRow(values ...string) {
	for i, v := range values {
		values[i] = SanitizeTerminal(redact.String(v))
	}
	fmt.Fprintln(tw.t, strings.Join(values, "\t"))
}

// Flush writes any buffered output.
func (tw *TableWriter) Flush() error {
	return tw.t.Flush()
}

// WriteTable writes headers and rows in tabular format through redaction.
func WriteTable(w io.Writer, headers []string, rows [][]string) error {
	tw := NewTable(w)
	tw.WriteHeader(headers...)
	for _, row := range rows {
		tw.WriteRow(row...)
	}
	return tw.Flush()
}

// Table formats data as a table string (for testing).
func Table(headers []string, rows [][]string) string {
	var sb strings.Builder
	_ = WriteTable(&sb, headers, rows)
	return redact.String(SanitizeTerminal(sb.String()))
}
