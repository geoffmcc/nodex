package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
)

func TestConsoleCommandsAreRegistered(t *testing.T) {
	for _, name := range []string{"vm", "container"} {
		cmd, ok := GetCommand(name)
		if !ok {
			t.Fatalf("missing %s command", name)
		}
		if _, ok := cmd.sub["console"]; !ok {
			t.Fatalf("missing %s console subcommand", name)
		}
	}
}

func TestConsoleRejectsNonTTYBeforeConnecting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"vm", "console", "node1/100"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "requires an interactive TTY") {
		t.Fatalf("error = %v, want non-TTY safeguard", err)
	}
	var exitCode *app.ExitCoder
	if !errors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
		t.Fatalf("error = %v, want ExitUsage", err)
	}
}

func TestConsoleRejectsInvalidTargetBeforeTTYCheck(t *testing.T) {
	err := runVMConsole(context.Background(), &Context{Writer: &bytes.Buffer{}}, []string{"invalid"})
	if err == nil || !strings.Contains(err.Error(), "expected <node>/<vmid>") {
		t.Fatalf("error = %v, want target validation", err)
	}
}
