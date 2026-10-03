package cli

import (
	"bytes"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/safety"
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

func TestCheckDestructiveReportsEOFAsUsageNotNetwork(t *testing.T) {
	// Evidence 66: `container snapshot delete <t> <name> --yes --force` with
	// stdin at /dev/null typed nothing, so ReadString returned EOF. The
	// untyped wrap fell through to the exit-code string heuristics, and
	// classifyNetwork matched the literal "EOF" in "read confirmation: EOF",
	// reporting exit 7 / class "network" for what is really a missing
	// confirmation. A network class invites an agent to retry or report an
	// outage instead of supplying the confirmation.
	var stderr bytes.Buffer
	cmdCtx := &Context{
		Opts:  Options{Yes: true, Force: true},
		ErrW:  &stderr,
		Stdin: strings.NewReader(""), // /dev/null: EOF, no input at all
	}

	err := checkDestructive(cmdCtx, "snapshot nx4-desc", "nx4-desc")
	if err == nil {
		t.Fatal("checkDestructive = nil, want refusal when stdin is EOF")
	}
	if got := app.ExitCodeFromError(err); got != app.ExitUsage {
		t.Errorf("ExitCodeFromError = %d, want %d (ExitUsage): %v", got, app.ExitUsage, err)
	}
	// The typed ExitCoder must win over the string heuristics, which match the
	// literal "EOF" in classifyNetwork. Note IsNetworkError is deliberately NOT
	// asserted here: a genuine TLS failure (peer closes mid-handshake) also
	// surfaces as a bare io.EOF, so that matcher must stay substring-based for
	// the transport paths. Typing the error is what keeps confirmation out of it.
	if !strings.Contains(err.Error(), "confirmation") {
		t.Errorf("error should name confirmation as the failure, got: %v", err)
	}
}

func TestCheckDestructiveTreatsUnterminatedLineAsConfirmation(t *testing.T) {
	// Piped stdin often omits the trailing newline (`printf '%s' proxmox/9999`).
	// The value was still read, so it must confirm rather than fail on EOF.
	var stderr bytes.Buffer
	cmdCtx := &Context{
		Opts:  Options{Yes: true, Force: true},
		ErrW:  &stderr,
		Stdin: strings.NewReader("proxmox/9999"), // no trailing newline
	}

	if err := checkDestructive(cmdCtx, "VM proxmox/9999", "proxmox/9999"); err != nil {
		t.Fatalf("checkDestructive = %v, want nil (target read without trailing newline)", err)
	}
}

func TestCheckDestructiveRejectsUnterminatedMismatch(t *testing.T) {
	// The unterminated-read tolerance must not accept a wrong value.
	var stderr bytes.Buffer
	cmdCtx := &Context{
		Opts:  Options{Yes: true, Force: true},
		ErrW:  &stderr,
		Stdin: strings.NewReader("wrong-target"), // no trailing newline
	}

	err := checkDestructive(cmdCtx, "VM proxmox/9999", "proxmox/9999")
	if err == nil {
		t.Fatal("checkDestructive = nil, want mismatch refusal without trailing newline")
	}
	if !stderrors.Is(err, safety.ErrTypeConfirmMismatch) {
		t.Errorf("want ErrTypeConfirmMismatch, got: %v", err)
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
