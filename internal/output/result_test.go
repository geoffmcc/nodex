package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
)

func TestOperationResult_JSON(t *testing.T) {
	var buf bytes.Buffer
	r := NewOperationResult("vm start", "proxmox", "home")
	r.Target = "pve1/100"
	r.Safety = "reversible"
	r.UPID = "UPID:pve1:000E1A2B:00000123:..."
	r.Submitted = true
	r.Success = true
	r.Schema = SchemaVersionResult

	if err := WriteResult(&buf, FormatJSON, r); err != nil {
		t.Fatalf("WriteResult JSON: %v", err)
	}
	out := buf.String()

	mustContain := []string{
		`"schema":`, `"operation": "vm start"`, `"profile": "home"`,
		`"provider": "proxmox"`, `"target": "pve1/100"`,
		`"safety": "reversible"`, `"upid": "UPID:pve1:000E1A2B:00000123:..."`,
		`"submitted": true`, `"waited": false`, `"success": true`,
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("JSON output missing %q:\n%s", s, out)
		}
	}
	// Verify valid JSON (trailing newline is expected from WriteJSON).
	if !strings.HasSuffix(out, "\n") {
		t.Error("JSON output missing trailing newline")
	}
}

func TestOperationResult_JSON_WithWaitAndSuccess(t *testing.T) {
	var buf bytes.Buffer
	r := NewOperationResult("vm start", "proxmox", "home")
	r.Target = "pve1/100"
	r.Safety = "reversible"
	r.UPID = "UPID:pve1:000E1A2B:..."
	r.Submitted = true
	r.Waited = true
	r.Success = true
	r.Status = "OK"

	if err := WriteResult(&buf, FormatJSON, r); err != nil {
		t.Fatalf("WriteResult JSON: %v", err)
	}
	out := buf.String()
	mustContain := []string{`"waited": true`, `"status": "OK"`}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("JSON output missing %q:\n%s", s, out)
		}
	}
}

func TestOperationResult_JSON_Error(t *testing.T) {
	var buf bytes.Buffer
	r := NewOperationResult("vm start", "proxmox", "home")
	r.Target = "pve1/100"
	r.UPID = "UPID:pve1:000E1A2B:..."
	r.Submitted = true
	r.Waited = true
	r.Success = false
	r.Error = &ResultError{
		Class:  "provider",
		Exit:   app.ExitProvider,
		Detail: "task failed with status \"ERROR: something broke\"",
	}

	if err := WriteResult(&buf, FormatJSON, r); err != nil {
		t.Fatalf("WriteResult JSON: %v", err)
	}
	out := buf.String()
	mustContain := []string{`"success": false`, `"error":`, `"class": "provider"`,
		`"exit": 12`, `"detail": "task failed`}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("JSON error output missing %q:\n%s", s, out)
		}
	}
}

func TestOperationResult_YAML(t *testing.T) {
	var buf bytes.Buffer
	r := NewOperationResult("vm stop", "proxmox", "lab")
	r.Target = "pve2/200"
	r.Safety = "reversible"
	r.UPID = "UPID:pve2:000ABCDE:..."
	r.Submitted = true
	r.Success = true

	if err := WriteResult(&buf, FormatYAML, r); err != nil {
		t.Fatalf("WriteResult YAML: %v", err)
	}
	out := buf.String()
	mustContain := []string{"operation: vm stop", "profile: lab", "target: pve2/200"}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("YAML output missing %q:\n%s", s, out)
		}
	}
}

func TestOperationResult_Table_Submitted(t *testing.T) {
	var buf bytes.Buffer
	r := NewOperationResult("vm start", "proxmox", "home")
	r.Target = "pve1/100"
	r.UPID = "UPID:pve1:000E1A2B:..."
	r.Submitted = true
	r.Success = true

	if err := WriteResult(&buf, FormatTable, r); err != nil {
		t.Fatalf("WriteResult table: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Submitted task") || !strings.Contains(out, "UPID:pve1:000E1A2B:...") {
		t.Errorf("table output:\n%s", out)
	}
}

func TestOperationResult_Table_WaitedAndOK(t *testing.T) {
	var buf bytes.Buffer
	r := NewOperationResult("vm start", "proxmox", "home")
	r.Target = "pve1/100"
	r.UPID = "UPID:pve1:000E1A2B:..."
	r.Submitted = true
	r.Waited = true
	r.Success = true
	r.Status = "OK"

	if err := WriteResult(&buf, FormatTable, r); err != nil {
		t.Fatalf("WriteResult table: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "completed OK") {
		t.Errorf("table output:\n%s", out)
	}
}

func TestOperationResult_Table_Failed(t *testing.T) {
	var buf bytes.Buffer
	r := NewOperationResult("vm start", "proxmox", "home")
	r.Target = "pve1/100"
	r.UPID = "UPID:pve1:000E1A2B:..."
	r.Submitted = true
	r.Waited = true
	r.Success = false
	r.Error = &ResultError{Class: "provider", Exit: app.ExitProvider, Detail: "task failed"}

	if err := WriteResult(&buf, FormatTable, r); err != nil {
		t.Fatalf("WriteResult table: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "FAILED") || !strings.Contains(out, "task failed") {
		t.Errorf("table output:\n%s", out)
	}
}

func TestOperationResult_Table_NonUPID(t *testing.T) {
	var buf bytes.Buffer
	r := NewOperationResult("sdn zone create", "proxmox", "home")
	r.Target = "myzone"
	r.Safety = "disruptive"
	r.Success = true
	changed := true
	r.Changed = &changed

	if err := WriteResult(&buf, FormatTable, r); err != nil {
		t.Fatalf("WriteResult table: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "completed") {
		t.Errorf("table output:\n%s", out)
	}
}

func TestOperationResult_RedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	r := NewOperationResult("vm start", "proxmox", "home")
	r.Target = "pve1/100"
	r.UPID = "UPID:pve1:000E1A2B:..."
	r.Submitted = true
	r.Success = true
	// Embed a fake secret in UPID to verify redaction.
	r.UPID = "UPID:pve1:PVEAPIToken=user@pam!nodex=secret-key-value:000E1A2B"

	if err := WriteResult(&buf, FormatJSON, r); err != nil {
		t.Fatalf("WriteResult JSON: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "secret-key-value") {
		t.Errorf("JSON output leaked secret:\n%s", out)
	}
}

// TestOperationResult_OmittedFields verifies that zero-value fields are
// properly omitted from the JSON output.
func TestOperationResult_OmittedFields(t *testing.T) {
	var buf bytes.Buffer
	r := OperationResult{
		Schema:    SchemaVersionResult,
		Operation: "vm start",
		Provider:  "proxmox",
		Submitted: true,
		Success:   true,
	}
	if err := WriteResult(&buf, FormatJSON, r); err != nil {
		t.Fatalf("WriteResult JSON: %v", err)
	}
	out := buf.String()
	// These must NOT appear in output when zero-valued.
	for _, omit := range []string{`"profile":`, `"target":`, `"safety":`, `"upid":`,
		`"status":`, `"warnings":`, `"error":`, `"changed":`} {
		if strings.Contains(out, omit) {
			t.Errorf("JSON output included zero-value field %q:\n%s", omit, out)
		}
	}
}
