package cli

import (
	"bytes"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
)

func TestCheckDestructiveHonorsConfirmTargetWithPipedStdin(t *testing.T) {
	// #16: `nodex --yes --force --confirm-target proxmox/9999 vm delete proxmox/9999`
	// with piped stdin used to fall through to a TTY read and fail with EOF.
	var stderr bytes.Buffer
	cmdCtx := &Context{
		Opts: Options{
			Yes:           true,
			Force:         true,
			ConfirmTarget: "proxmox/9999",
		},
		ErrW:  &stderr,
		Stdin: strings.NewReader(""), // piped stdin, not a TTY, not --non-interactive
	}

	if err := checkDestructive(cmdCtx, "VM proxmox/9999", "proxmox/9999"); err != nil {
		t.Fatalf("checkDestructive = %v, want nil (confirm-target authorizes)", err)
	}
}

func TestCheckDestructiveRefusesConfirmTargetWithoutFlags(t *testing.T) {
	// #16/#39: --confirm-target alone must not authorize.
	var stderr bytes.Buffer
	cmdCtx := &Context{
		Opts:  Options{ConfirmTarget: "proxmox/9999"},
		ErrW:  &stderr,
		Stdin: strings.NewReader(""),
	}

	err := checkDestructive(cmdCtx, "VM proxmox/9999", "proxmox/9999")
	if err == nil {
		t.Fatal("checkDestructive = nil, want refusal without --yes --force")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
		t.Errorf("expected ExitUsage, got: %v", err)
	}
	if !strings.Contains(err.Error(), "confirmation refused") {
		t.Errorf("expected refusal message, got: %v", err)
	}
}

func TestCheckDestructiveRefusesConfirmTargetMismatch(t *testing.T) {
	// #39: any mismatch = refusal, even with --yes --force.
	var stderr bytes.Buffer
	cmdCtx := &Context{
		Opts: Options{
			Yes:           true,
			Force:         true,
			ConfirmTarget: "proxmox/1000",
		},
		ErrW:  &stderr,
		Stdin: strings.NewReader(""),
	}

	err := checkDestructive(cmdCtx, "VM proxmox/9999", "proxmox/9999")
	if err == nil {
		t.Fatal("checkDestructive = nil, want refusal on target mismatch")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
		t.Errorf("expected ExitUsage, got: %v", err)
	}
}

func TestCheckDestructiveHonorsConfirmTargetWithNonInteractive(t *testing.T) {
	var stderr bytes.Buffer
	cmdCtx := &Context{
		Opts: Options{
			NonInteractive: true,
			Yes:            true,
			Force:          true,
			ConfirmTarget:  "proxmox/9999",
		},
		ErrW:  &stderr,
		Stdin: strings.NewReader(""),
	}

	if err := checkDestructive(cmdCtx, "VM proxmox/9999", "proxmox/9999"); err != nil {
		t.Fatalf("checkDestructive = %v, want nil", err)
	}
}

func TestCheckDestructiveRefusesNonInteractiveWithoutConfirmTarget(t *testing.T) {
	var stderr bytes.Buffer
	cmdCtx := &Context{
		Opts: Options{
			NonInteractive: true,
			Yes:            true,
			Force:          true,
		},
		ErrW:  &stderr,
		Stdin: strings.NewReader(""),
	}

	err := checkDestructive(cmdCtx, "VM proxmox/9999", "proxmox/9999")
	if err == nil {
		t.Fatal("checkDestructive = nil, want refusal in non-interactive mode")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
		t.Errorf("expected ExitUsage, got: %v", err)
	}
}

func TestCheckDestructiveInteractiveStillPromptsForTypeIn(t *testing.T) {
	// Interactive users (no --confirm-target) keep the TTY type-in path.
	var stderr bytes.Buffer
	cmdCtx := &Context{
		Opts: Options{
			Yes:   true,
			Force: true,
		},
		ErrW:  &stderr,
		Stdin: strings.NewReader("proxmox/9999\n"),
	}

	if err := checkDestructive(cmdCtx, "VM proxmox/9999", "proxmox/9999"); err != nil {
		t.Fatalf("checkDestructive = %v, want nil after typing target", err)
	}
	if !strings.Contains(stderr.String(), "Type \"proxmox/9999\" to confirm") {
		t.Errorf("expected type-in prompt on stderr, got: %q", stderr.String())
	}
}
