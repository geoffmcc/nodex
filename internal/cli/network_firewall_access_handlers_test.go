package cli

import (
	"bytes"
	"strings"
	"testing"

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
