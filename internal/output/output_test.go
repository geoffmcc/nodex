package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}

	if err := WriteJSON(&buf, data); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"key": "value"`) {
		t.Errorf("expected JSON output, got: %s", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("expected trailing newline")
	}
}

func TestWriteYAML(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}

	if err := WriteYAML(&buf, data); err != nil {
		t.Fatalf("WriteYAML: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "key: value") {
		t.Errorf("expected YAML output, got: %s", out)
	}
}

func TestWriteTable(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"NAME", "STATUS"}
	rows := [][]string{
		{"server-1", "running"},
		{"server-2", "stopped"},
	}

	if err := WriteTable(&buf, headers, rows); err != nil {
		t.Fatalf("WriteTable: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "NAME") {
		t.Error("expected headers in output")
	}
	if !strings.Contains(out, "server-1") {
		t.Error("expected data in output")
	}
}

func TestSanitizeTerminal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"CSI reset", "hello\x1b[0m world", "hello world"},
		{"CSI color", "hello\x1b[31m world", "hello world"},
		{"CSI bold", "hello\x1b[1m world", "hello world"},
		{"multiple escapes", "\x1b[31mred\x1b[0m normal", "red normal"},
		{"empty", "", ""},
		{"no escapes", "no escapes here", "no escapes here"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeTerminal(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeTerminal(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDefaultFormat(t *testing.T) {
	// DefaultFormat should return either table or json (depends on terminal).
	f := DefaultFormat()
	if f != FormatTable && f != FormatJSON {
		t.Errorf("DefaultFormat() = %q, want table or json", f)
	}
}

func TestFormatter_Format(t *testing.T) {
	var buf bytes.Buffer
	f := New(&buf, FormatJSON, false)
	if f.Format() != FormatJSON {
		t.Errorf("Format() = %q, want %q", f.Format(), FormatJSON)
	}
}

func TestMarshalJSON(t *testing.T) {
	data := map[string]int{"a": 1}
	b, err := MarshalJSON(data)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !strings.Contains(string(b), `"a": 1`) {
		t.Errorf("unexpected output: %s", b)
	}
}

func TestMarshalYAML(t *testing.T) {
	data := map[string]int{"a": 1}
	b, err := MarshalYAML(data)
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if !strings.Contains(string(b), "a: 1") {
		t.Errorf("unexpected output: %s", b)
	}
}
