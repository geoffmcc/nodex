package main

import (
	"bytes"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
)

func TestEmitError_JSON_NonEmitted(t *testing.T) {
	var buf bytes.Buffer
	err := app.NewExitError(stderrors.New("boom"), app.ExitGeneral)
	code := emitError(err, true, &buf)
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

func TestEmitError_JSON_EmittedSuppressed(t *testing.T) {
	var buf bytes.Buffer
	inner := app.NewExitError(stderrors.New("boom"), app.ExitGeneral)
	err := app.MarkEmitted(inner)
	code := emitError(err, true, &buf)
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
	code := emitError(err, false, &buf)
	if code != app.ExitConfig {
		t.Errorf("emitError code = %d, want ExitConfig(%d)", code, app.ExitConfig)
	}
	if want := "Error: boom\n"; buf.String() != want {
		t.Errorf("emitError text = %q, want %q", buf.String(), want)
	}
}

func TestWantsJSON(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"empty", nil, false},
		{"explicit json", []string{"node", "list", "--output", "json"}, true},
		{"explicit equals json", []string{"--output=json", "node", "list"}, true},
		{"upper case json", []string{"--output", "JSON"}, true},
		{"yaml", []string{"node", "list", "--output", "yaml"}, false},
		{"json not last", []string{"--output", "json", "node", "list"}, true},
		{"default table", []string{"node", "list"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wantsJSON(tt.args); got != tt.want {
				t.Errorf("wantsJSON(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
