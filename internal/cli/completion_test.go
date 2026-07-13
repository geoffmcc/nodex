package cli

import (
	"bytes"
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
)

func TestRunCompletionScripts(t *testing.T) {
	tests := []struct {
		shell string
		want  []string
	}{
		{shell: "bash", want: []string{"_nodex_completion", "profile", "set-credentials", "complete -F _nodex_completion nodex"}},
		{shell: "zsh", want: []string{"#compdef nodex", "_nodex", "profile", "set-credentials"}},
		{shell: "fish", want: []string{"complete -c nodex", "__fish_seen_subcommand_from profile", "set-credentials"}},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), []string{"completion", tt.shell}, &stdout, &stderr)
			if err != nil {
				t.Fatalf("Run completion %s: %v", tt.shell, err)
			}
			out := stdout.String()
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("completion %s missing %q in output:\n%s", tt.shell, want, out)
				}
			}
		})
	}
}

func TestRunCompletionRejectsUnknownShell(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"completion", "powershell"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected unknown shell error")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
		t.Fatalf("error = %v, want ExitUsage", err)
	}
}
