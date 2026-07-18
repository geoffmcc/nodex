package output

import (
	"strings"
	"testing"
)

type namedStatus string

type namedFieldStruct struct {
	Name   string      `json:"name"`
	Status namedStatus `json:"status"`
}

// TestSanitizeTerminalDataPreservesNamedStringTypes guards against named
// string fields (status enums) being zeroed during output sanitization.
func TestSanitizeTerminalDataPreservesNamedStringTypes(t *testing.T) {
	in := namedFieldStruct{Name: "check", Status: "healthy"}
	out := sanitizeTerminalData(in)
	rs, ok := out.(namedFieldStruct)
	if !ok {
		t.Fatalf("type changed: %T", out)
	}
	if rs.Status != "healthy" {
		t.Errorf("named string field lost: %+v", rs)
	}
	if rs.Name != "check" {
		t.Errorf("plain string field lost: %+v", rs)
	}
}

func TestSanitizeTerminalDataNamedStringStillSanitized(t *testing.T) {
	in := namedFieldStruct{Status: namedStatus("bad\x1b]0;owned\x07value")}
	out := sanitizeTerminalData(in).(namedFieldStruct)
	if strings.Contains(string(out.Status), "\x1b") {
		t.Errorf("escape sequence survived in named string: %q", out.Status)
	}
}
