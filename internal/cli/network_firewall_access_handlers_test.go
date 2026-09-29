package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/safety"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// The nit asks for group/token discovery value in the users table. Crucially, a
// value the server did not report must render as "-" rather than a misleading 0.
func TestWriteAccessUsersTableShowsGroupsAndTokens(t *testing.T) {
	zero := 0
	two := 2
	var stdout bytes.Buffer
	cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: output.FormatTable}}

	err := writeAccessUsers(cmdCtx, []domain.AccessUser{
		{
			UserID:     "root@pam",
			Enable:     1,
			Groups:     []string{"admins", "ops"},
			TokenCount: &two,
		},
		{
			UserID:     "svc@pve",
			Groups:     []string{},
			TokenCount: &zero,
		},
		{UserID: "quiet@pve"},
	})
	if err != nil {
		t.Fatalf("writeAccessUsers: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{"GROUPS", "TOKENS", "admins,ops"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q: %q", want, out)
		}
	}
	if !strings.Contains(out, "root@pam") || !strings.Contains(out, "quiet@pve") {
		t.Errorf("table output missing users: %q", out)
	}
	// quiet@pve has no reported groups or tokens; it must not show a fake 0.
	quietLine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "quiet@pve") {
			quietLine = line
		}
	}
	if quietLine == "" {
		t.Fatalf("no row for quiet@pve: %q", out)
	}
	if strings.Contains(quietLine, "0") {
		t.Errorf("unreported token count rendered as 0: %q", quietLine)
	}
	if strings.Count(quietLine, "-") < 2 {
		t.Errorf("unreported groups/tokens not marked: %q", quietLine)
	}
}

func TestWriteAccessUsersStructuredOutputDistinguishesUnknownFromZero(t *testing.T) {
	zero := 0
	var stdout bytes.Buffer
	cmdCtx := &Context{Writer: &stdout, Opts: Options{Output: output.FormatJSON}}

	err := writeAccessUsers(cmdCtx, []domain.AccessUser{
		{UserID: "reported@pve", TokenCount: &zero},
		{UserID: "unknown@pve"},
	})
	if err != nil {
		t.Fatalf("writeAccessUsers: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, `"tokens": 0`) {
		t.Errorf("reported zero token count missing: %q", out)
	}
	if strings.Contains(out, `"tokens"`) && !strings.Contains(out, `"tokens": 0`) {
		t.Errorf("unreported token count should be omitted: %q", out)
	}
	if strings.Count(out, `"tokens"`) != 1 {
		t.Errorf("exactly one user should report a token count: %q", out)
	}
}

// The Tier 4 gate must run before the handler asks for a password.  Without
// --expert the command is rejected outright, so a caller must never be
// prompted for a secret the command was always going to refuse.  Stdin is an
// erroring reader: if the handler reaches secret collection, the test fails
// with that error rather than silently hanging on a real terminal.
func TestRunAccessUserCreateRejectsBeforePromptingForSecret(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmdCtx := &Context{
		Writer: &stdout,
		ErrW:   &stderr,
		// No --expert, no --yes/--force, and interactive: the only reason to
		// prompt for a password is that the gate was skipped.
		Stdin: errReader{err: errors.New("stdin must not be read")},
	}

	err := runAccessUserCreate(context.Background(), cmdCtx, []string{"newuser@pve"})

	if !errors.Is(err, safety.ErrExpertRequired) {
		t.Fatalf("want ErrExpertRequired, got %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("rejected command wrote to stderr: %q", stderr.String())
	}
}

// errReader fails any read, standing in for a stdin that would otherwise block
// on the secret prompt.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// TestCheckSecurityAdminRequiresTypeConfirmForDestructiveOps pins that Tier 4
// splits its confirmation rules. Deleting a user removes identity state that
// cannot be recovered, so --expert --yes --force must still not be enough: the
// operator has to type the userid. Creating a user and granting an ACL role
// change identity and privilege state without destroying anything, so those
// must NOT demand a typed target.
func TestCheckSecurityAdminRequiresTypeConfirmForDestructiveOps(t *testing.T) {
	t.Run("delete refuses flags alone and demands the typed userid", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmdCtx := &Context{
			Writer: &stdout,
			ErrW:   &stderr,
			Stdin:  strings.NewReader("wronguser@pve\n"),
			Opts:   Options{Expert: true, Yes: true, Force: true},
		}

		err := checkSecurityAdmin(cmdCtx, "delete user newuser@pve", "newuser@pve")

		if !errors.Is(err, safety.ErrTypeConfirmMismatch) {
			t.Fatalf("a wrong typed target must be refused with ErrTypeConfirmMismatch, got %v", err)
		}
		if !strings.Contains(stderr.String(), `Type "newuser@pve" to confirm`) {
			t.Errorf("operator was not told what to type; stderr = %q", stderr.String())
		}
	})

	t.Run("delete accepts an exact typed target", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmdCtx := &Context{
			Writer: &stdout,
			ErrW:   &stderr,
			Stdin:  strings.NewReader("newuser@pve\n"),
			Opts:   Options{Expert: true, Yes: true, Force: true},
		}

		if err := checkSecurityAdmin(cmdCtx, "delete user newuser@pve", "newuser@pve"); err != nil {
			t.Fatalf("an exact typed target must authorize the delete, got %v", err)
		}
	})

	t.Run("delete honors --confirm-target for non-interactive use", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmdCtx := &Context{
			Writer: &stdout,
			ErrW:   &stderr,
			Stdin:  errReader{err: errors.New("stdin must not be read")},
			Opts: Options{
				Expert: true, Yes: true, Force: true,
				ConfirmTarget: "newuser@pve", NonInteractive: true,
			},
		}

		if err := checkSecurityAdmin(cmdCtx, "delete user newuser@pve", "newuser@pve"); err != nil {
			t.Fatalf("--confirm-target with --yes --force must authorize, got %v", err)
		}
	})

	t.Run("create does not demand a typed target", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmdCtx := &Context{
			Writer: &stdout,
			ErrW:   &stderr,
			Stdin:  errReader{err: errors.New("stdin must not be read")},
			Opts:   Options{Expert: true, Yes: true, Force: true},
		}

		if err := checkSecurityAdmin(cmdCtx, "create user newuser@pve", ""); err != nil {
			t.Fatalf("create adds state and must be authorized by --yes --force alone, got %v", err)
		}
		if stderr.Len() != 0 {
			t.Errorf("an authorized create should not print a confirmation prompt; stderr = %q", stderr.String())
		}
	})

	// The subtests above exercise the gate directly, which would still pass if
	// runAccessUserDelete forgot to pass the target through. This one drives
	// the real handler so the wiring itself is covered: the confirmation is
	// resolved before any connection is attempted, so a wrong typed target
	// must fail with the type-confirm error rather than a connection error.
	t.Run("runAccessUserDelete demands the typed userid before connecting", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmdCtx := &Context{
			Writer: &stdout,
			ErrW:   &stderr,
			Stdin:  strings.NewReader("wronguser@pve\n"),
			Opts:   Options{Expert: true, Yes: true, Force: true},
		}

		err := runAccessUserDelete(context.Background(), cmdCtx, []string{"newuser@pve"})

		if !errors.Is(err, safety.ErrTypeConfirmMismatch) {
			t.Fatalf("delete must require typing the userid; got %v", err)
		}
	})

	t.Run("acl add does not demand a typed target", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cmdCtx := &Context{
			Writer: &stdout,
			ErrW:   &stderr,
			Stdin:  errReader{err: errors.New("stdin must not be read")},
			Opts:   Options{Expert: true, Yes: true, Force: true},
		}

		if err := checkSecurityAdmin(cmdCtx, "ACL add path=/ role=PVEAdmin", ""); err != nil {
			t.Fatalf("acl add grants privilege without destroying state, got %v", err)
		}
	})
}
