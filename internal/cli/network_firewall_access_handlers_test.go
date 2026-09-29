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
