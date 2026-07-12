package cli

import (
	"bytes"
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
)

func TestRun_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Nodex") {
		t.Error("expected usage output")
	}
}

func TestRun_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Commands:") {
		t.Error("expected commands list")
	}
}

func TestRun_HelpCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"help", "version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "nodex version") {
		t.Errorf("expected version help, got: %s", out)
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"nonexistent"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
		t.Errorf("expected ExitUsage, got: %v", err)
	}
}

func TestRun_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Nodex") {
		t.Error("expected version output")
	}
}

func TestRun_ProviderSubcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// provider with no subcommand should print usage.
	err := Run(context.Background(), []string{"provider"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Subcommands:") {
		t.Error("expected subcommands list")
	}
}

func TestRun_ProviderList(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"provider", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "proxmox") {
		t.Error("expected proxmox in provider list")
	}
}

func TestRun_ProviderCapabilitiesUnknown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"provider", "capabilities", "nonexistent"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRun_ProfileSubcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Subcommands:") {
		t.Error("expected subcommands list")
	}
}

func TestRun_ProfileAddNoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "add"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestRun_ProfileShowNoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "show"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestRun_ProfileUseNoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "use"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestRun_ProfileRemoveNoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"profile", "remove"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestRun_GlobalFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--quiet", "version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRun_InvalidOutputFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--output", "xml", "version"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid output format")
	}
}
