package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
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
		// OSC sequences (ESC ]) — e.g. window title injection.
		{"OSC BEL terminator", "before\x1b]0;malicious title\x07after", "beforeafter"},
		{"OSC ST terminator", "before\x1b]0;payload\x1b\\after", "beforeafter"},
		// DCS sequences (ESC P) — e.g. device control injection.
		{"DCS BEL", "before\x1bP|cmd\x07after", "beforeafter"},
		{"DCS ST", "before\x1bP|cmd\x1b\\after", "beforeafter"},
		// APC sequences (ESC _).
		{"APC BEL", "before\x1b_payload\x07after", "beforeafter"},
		{"APC ST", "before\x1b_payload\x1b\\after", "beforeafter"},
		// PM sequences (ESC ^).
		{"PM BEL", "before\x1b^payload\x07after", "beforeafter"},
		{"PM ST", "before\x1b^payload\x1b\\after", "beforeafter"},
		// Lone ESC at end of string should be dropped.
		{"lone ESC", "hello\x1b", "hello"},
		// Multi-byte CSI with parameters.
		{"CSI params", "a\x1b[38;5;196mb", "ab"},
		// Mixed escape types.
		{"mixed", "\x1b[31mR\x1b]0;title\x07G\x1bP|y\x1b\\B", "RGB"},
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

func TestWriteTableRedactsAndSanitizesCells(t *testing.T) {
	var b strings.Builder
	err := WriteTable(&b, []string{"NAME"}, [][]string{{"bad\x1b[31m PVEAPIToken=user@pam!id=secret\rforge"}})
	if err != nil {
		t.Fatalf("WriteTable: %v", err)
	}
	out := b.String()
	if strings.Contains(out, "\x1b") || strings.Contains(out, "secret") || strings.Contains(out, "\r") {
		t.Fatalf("unsafe table output: %q", out)
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

func TestDomainYAMLOmitsEmptyOptionalFields(t *testing.T) {
	tests := []struct {
		name string
		data any
		want []string
		omit []string
	}{
		{
			name: "node",
			data: domain.Node{ID: "node/proxmox", Name: "proxmox", Status: "online", Role: "node", Platform: "proxmox"},
			want: []string{"id: node/proxmox", "name: proxmox", "status: online", "role: node", "platform: proxmox"},
			omit: []string{"labels:", "ip:", "version:", "uptime:"},
		},
		{
			name: "vm",
			data: domain.VM{ID: "proxmox/100", Name: "vm-one", Status: "running", Node: "proxmox", CPU: 2, Memory: 1024, Disk: 2048},
			want: []string{"id: proxmox/100", "name: vm-one", "status: running", "node: proxmox", "cpu: 2", "memory: 1024", "disk: 2048"},
			omit: []string{"labels:", "ip:", "os:", "ID:", "Name:"},
		},
		{
			name: "container",
			data: domain.Container{ID: "proxmox/200", Name: "ct-one", Status: "running", Node: "proxmox", Memory: 1024, Disk: 2048},
			want: []string{"id: proxmox/200", "name: ct-one", "status: running", "node: proxmox", "memory: 1024", "disk: 2048"},
			omit: []string{"labels:", "ip:", "os:", "ID:", "Name:"},
		},
		{
			name: "storage",
			data: domain.Storage{ID: "storage/proxmox/local-lvm", Name: "local-lvm", Type: "storage", Status: "available", Node: "proxmox", Total: 4096, Used: 1024, Avail: 3072},
			want: []string{"id: storage/proxmox/local-lvm", "name: local-lvm", "type: storage", "status: available", "node: proxmox", "total: 4096", "used: 1024", "avail: 3072"},
			omit: []string{"labels:", "content:", "ID:", "Name:"},
		},
		{
			name: "cluster",
			data: domain.Cluster{Name: "home", Version: "9.2.4", Nodes: 1},
			want: []string{"name: home", "version: 9.2.4", "nodes: 1"},
			omit: []string{"Name:", "Version:", "Nodes:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := MarshalYAML(tt.data)
			if err != nil {
				t.Fatalf("MarshalYAML: %v", err)
			}
			out := string(b)
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("YAML output missing %q: %q", want, out)
				}
			}
			for _, omit := range tt.omit {
				if strings.Contains(out, omit) {
					t.Fatalf("YAML output included %q: %q", omit, out)
				}
			}
		})
	}
}

func TestDomainYAMLIncludesNonEmptyLabels(t *testing.T) {
	b, err := MarshalYAML(domain.Storage{
		ID: "storage/proxmox/local-lvm", Name: "local-lvm", Type: "storage", Status: "available",
		Total: 4096, Used: 1024, Avail: 3072, Labels: map[string]string{"env": "test"},
	})
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, "labels:") || !strings.Contains(out, "env: test") {
		t.Fatalf("YAML output missing labels: %q", out)
	}
}

func TestDomainJSONOutputUnchanged(t *testing.T) {
	b, err := MarshalJSON(domain.Storage{
		ID: "storage/proxmox/local-lvm", Name: "local-lvm", Type: "storage", Status: "available",
		Total: 4096, Used: 1024, Avail: 3072,
	})
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	out := string(b)
	for _, want := range []string{`"id": "storage/proxmox/local-lvm"`, `"name": "local-lvm"`, `"type": "storage"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("JSON output missing %q: %q", want, out)
		}
	}
	if strings.Contains(out, "labels") {
		t.Fatalf("JSON output included empty labels: %q", out)
	}
}
