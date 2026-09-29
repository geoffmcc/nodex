package main

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/output"
)

func TestEmitError_JSON_NonEmitted(t *testing.T) {
	var buf bytes.Buffer
	err := app.NewExitError(stderrors.New("boom"), app.ExitGeneral)
	code := emitError(err, output.FormatJSON, &buf)
	if code != app.ExitGeneral {
		t.Errorf("emitError code = %d, want ExitGeneral(%d)", code, app.ExitGeneral)
	}
	if buf.Len() == 0 {
		t.Fatal("expected a JSON error document on stderr")
	}
	if !strings.Contains(buf.String(), "boom") {
		t.Errorf("error document missing message, got %q", buf.String())
	}
}

// TestEmitError_JSON_ErrorKeyIsAlwaysAnObject pins the unified envelope. The
// `error` key used to be a string on the pre-flight path and an object on the
// mutation path, so a consumer had to branch on its type before reading it.
func TestEmitError_JSON_ErrorKeyIsAlwaysAnObject(t *testing.T) {
	var buf bytes.Buffer
	err := app.NewExitError(stderrors.New("usage: nodex ceph status <node>"), app.ExitUsage)
	emitError(err, output.FormatJSON, &buf)

	var doc struct {
		Schema int `json:"schema"`
		Error  struct {
			Class  string `json:"class"`
			Exit   int    `json:"exit"`
			Detail string `json:"detail"`
		} `json:"error"`
		Exit int `json:"exit"`
	}
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("error envelope is not valid JSON: %v\n%s", err, buf.String())
	}
	if doc.Schema != output.SchemaVersionResult {
		t.Errorf("schema = %d, want %d", doc.Schema, output.SchemaVersionResult)
	}
	if doc.Error.Class != "error" {
		t.Errorf("error.class = %q, want %q", doc.Error.Class, "error")
	}
	if doc.Error.Exit != app.ExitUsage {
		t.Errorf("error.exit = %d, want %d", doc.Error.Exit, app.ExitUsage)
	}
	if doc.Error.Detail != "usage: nodex ceph status <node>" {
		t.Errorf("error.detail = %q, want the message", doc.Error.Detail)
	}
	if doc.Exit != app.ExitUsage {
		t.Errorf("exit = %d, want %d", doc.Exit, app.ExitUsage)
	}
}

// TestEmitError_JSON_DoesNotHTMLEscape verifies usage text keeps its angle
// brackets instead of being emitted as \u003cnode\u003e, which a dashboard
// rendering doc.error would display literally.
func TestEmitError_JSON_DoesNotHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	err := app.NewExitError(stderrors.New("usage: nodex ceph status <node>"), app.ExitUsage)
	emitError(err, output.FormatJSON, &buf)
	if strings.Contains(buf.String(), `\u003c`) {
		t.Errorf("JSON output HTML-escaped angle brackets: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "<node>") {
		t.Errorf("JSON output lost the literal usage text: %s", buf.String())
	}
}

// TestEmitError_YAML_Structured pins that a YAML consumer gets a structured
// document rather than a bare "Error:" prose line.
func TestEmitError_YAML_Structured(t *testing.T) {
	var buf bytes.Buffer
	err := app.NewExitError(stderrors.New("usage: nodex ceph status <node>"), app.ExitUsage)
	code := emitError(err, output.FormatYAML, &buf)
	if code != app.ExitUsage {
		t.Errorf("emitError code = %d, want ExitUsage(%d)", code, app.ExitUsage)
	}

	var doc struct {
		Schema int `yaml:"schema"`
		Error  struct {
			Class  string `yaml:"class"`
			Exit   int    `yaml:"exit"`
			Detail string `yaml:"detail"`
		} `yaml:"error"`
		Exit int `yaml:"exit"`
	}
	if err := yaml.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("error envelope is not valid YAML: %v\n%s", err, buf.String())
	}
	if doc.Schema != output.SchemaVersionResult {
		t.Errorf("schema = %d, want %d", doc.Schema, output.SchemaVersionResult)
	}
	if doc.Error.Detail != "usage: nodex ceph status <node>" {
		t.Errorf("error.detail = %q, want the message", doc.Error.Detail)
	}
	if doc.Exit != app.ExitUsage {
		t.Errorf("exit = %d, want %d", doc.Exit, app.ExitUsage)
	}
}

func TestEmitError_YAML_EmittedSuppressed(t *testing.T) {
	var buf bytes.Buffer
	inner := app.NewExitError(stderrors.New("boom"), app.ExitGeneral)
	err := app.MarkEmitted(inner)
	code := emitError(err, output.FormatYAML, &buf)
	if code != app.ExitGeneral {
		t.Errorf("emitError code = %d, want ExitGeneral(%d)", code, app.ExitGeneral)
	}
	if buf.Len() != 0 {
		t.Errorf("emitted error must not produce a second document, got %q", buf.String())
	}
}

func TestEmitError_Text(t *testing.T) {
	var buf bytes.Buffer
	err := app.NewExitError(stderrors.New("boom"), app.ExitConfig)
	code := emitError(err, output.FormatTable, &buf)
	if code != app.ExitConfig {
		t.Errorf("emitError code = %d, want ExitConfig(%d)", code, app.ExitConfig)
	}
	if want := "Error: boom\n"; buf.String() != want {
		t.Errorf("emitError text = %q, want %q", buf.String(), want)
	}
}

func TestWantsFormat(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want output.Format
	}{
		{"empty", nil, output.FormatTable},
		{"explicit json", []string{"node", "list", "--output", "json"}, output.FormatJSON},
		{"explicit equals json", []string{"--output=json", "node", "list"}, output.FormatJSON},
		{"upper case json", []string{"node", "list", "--output", "JSON"}, output.FormatJSON},
		{"yaml", []string{"node", "list", "--output", "yaml"}, output.FormatYAML},
		{"yml", []string{"node", "list", "--output=yml"}, output.FormatYAML},
		{"json not last", []string{"--output", "json", "node", "list"}, output.FormatJSON},
		{"default table", []string{"node", "list"}, output.FormatTable},
		{"unknown value", []string{"node", "list", "--output", "toml"}, output.FormatTable},
		{"dangling flag", []string{"node", "list", "--output"}, output.FormatTable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wantsFormat(tt.args); got != tt.want {
				t.Errorf("wantsFormat(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
