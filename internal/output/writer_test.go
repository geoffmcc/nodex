package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriter_NewWriter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWriter(&stdout, &stderr, FormatJSON)
	if w.Format() != FormatJSON {
		t.Errorf("Format() = %q, want %q", w.Format(), FormatJSON)
	}
	if w.Stdout() != &stdout {
		t.Error("Stdout() returned wrong writer")
	}
	if w.Stderr() != &stderr {
		t.Error("Stderr() returned wrong writer")
	}
}

func TestWriter_Diagnosticf_JSON_GoesToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWriter(&stdout, &stderr, FormatJSON)

	w.Diagnosticf("connecting to %s", "pve1")

	if stdout.Len() != 0 {
		t.Errorf("stdout is not empty in JSON mode: %q", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "connecting to pve1") {
		t.Errorf("stderr = %q, want it to contain %q", got, "connecting to pve1")
	}
}

func TestWriter_Diagnosticf_YAML_GoesToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWriter(&stdout, &stderr, FormatYAML)

	w.Diagnosticf("fetching data from %s", "node-2")

	if stdout.Len() != 0 {
		t.Errorf("stdout is not empty in YAML mode: %q", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "fetching data from node-2") {
		t.Errorf("stderr = %q, want it to contain %q", got, "fetching data from node-2")
	}
}

func TestWriter_Diagnosticf_Table_GoesToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWriter(&stdout, &stderr, FormatTable)

	w.Diagnosticf("warning: slow response from %s", "10.0.0.1")

	if stdout.Len() != 0 {
		t.Errorf("stdout is not empty in table mode: %q", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "warning: slow response from 10.0.0.1") {
		t.Errorf("stderr = %q, want it to contain %q", got, "warning: slow response from 10.0.0.1")
	}
}

func TestWriter_Diagnostic_JSON_GoesToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWriter(&stdout, &stderr, FormatJSON)

	w.Diagnostic("retrying connection")

	if stdout.Len() != 0 {
		t.Errorf("stdout is not empty in JSON mode: %q", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "retrying connection") {
		t.Errorf("stderr = %q, want it to contain %q", got, "retrying connection")
	}
}

func TestWriter_Diagnostic_YAML_GoesToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWriter(&stdout, &stderr, FormatYAML)

	w.Diagnostic("retrying connection")

	if stdout.Len() != 0 {
		t.Errorf("stdout is not empty in YAML mode: %q", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "retrying connection") {
		t.Errorf("stderr = %q, want it to contain %q", got, "retrying connection")
	}
}

func TestWriter_Diagnostic_Table_GoesToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWriter(&stdout, &stderr, FormatTable)

	w.Diagnostic("retrying connection")

	if stdout.Len() != 0 {
		t.Errorf("stdout is not empty in table mode: %q", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "retrying connection") {
		t.Errorf("stderr = %q, want it to contain %q", got, "retrying connection")
	}
}

func TestWriter_Diagnosticf_SanitizesTerminalSequences(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWriter(&stdout, &stderr, FormatJSON)

	// Attempt terminal injection via diagnostic message.
	w.Diagnosticf("host \x1b[31m%s\x1b[0m is down", "evil")

	if stdout.Len() != 0 {
		t.Errorf("stdout is not empty: %q", stdout.String())
	}
	got := stderr.String()
	if strings.Contains(got, "\x1b") {
		t.Errorf("stderr contains raw escape sequence: %q", got)
	}
	if !strings.Contains(got, "host evil is down") {
		t.Errorf("stderr = %q, want sanitized message", got)
	}
}

func TestWriter_MultipleDiagnosticsDoNotMixWithStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWriter(&stdout, &stderr, FormatJSON)

	// Simulate primary output followed by diagnostics.
	w.stdout.Write([]byte(`{"ok":true}` + "\n"))
	w.Diagnostic("step 1")
	w.Diagnostic("step 2")
	w.Diagnosticf("step %d", 3)

	stdoutLines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	stderrLines := strings.Split(strings.TrimSpace(stderr.String()), "\n")

	if len(stdoutLines) != 1 || stdoutLines[0] != `{"ok":true}` {
		t.Errorf("stdout should contain only the JSON payload, got: %q", stdout.String())
	}
	if len(stderrLines) != 3 {
		t.Errorf("stderr should contain 3 diagnostic lines, got %d: %q", len(stderrLines), stderr.String())
	}
}

func TestWriter_Diagnosticf_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	w := NewWriter(&stdout, &stderr, FormatJSON)

	w.Diagnosticf("plain message")

	if stdout.Len() != 0 {
		t.Errorf("stdout is not empty: %q", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "plain message") {
		t.Errorf("stderr = %q, want %q", got, "plain message")
	}
}
