package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/output"
)

func TestStatusColorPolicy(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		noColor, table, tty  bool
		noColorEnv, dumbTerm bool
		want                 bool
	}{
		{name: "interactive table", table: true, tty: true, want: true},
		{name: "explicit no color", noColor: true, table: true, tty: true},
		{name: "json output", tty: true},
		{name: "redirected output", table: true},
		{name: "NO_COLOR", table: true, tty: true, noColorEnv: true},
		{name: "dumb terminal", table: true, tty: true, dumbTerm: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusColorPolicy(tc.noColor, tc.table, tc.tty, tc.noColorEnv, tc.dumbTerm); got != tc.want {
				t.Fatalf("statusColorPolicy = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStatusColorEnabledForRedirectedAndWriterOutput(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	if value, ok := os.LookupEnv("NO_COLOR"); ok {
		if err := os.Unsetenv("NO_COLOR"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Setenv("NO_COLOR", value) })
	}

	redirected, err := os.CreateTemp(t.TempDir(), "status-output-*")
	if err != nil {
		t.Fatal(err)
	}
	defer redirected.Close()

	ctx := &Context{Opts: Options{Output: output.FormatTable}, InteractiveOut: redirected}
	if statusColorEnabled(ctx) {
		t.Fatal("redirected table output enabled ANSI colors")
	}
	ctx.InteractiveOut = &strings.Builder{}
	if statusColorEnabled(ctx) {
		t.Fatal("writer output enabled ANSI colors without a terminal")
	}
}

func TestStatusColorTextStylesOnlyStateWords(t *testing.T) {
	plain := "VMs: 1 running, 0 stopped, 1 paused; storage available; unavailable section"
	styled := statusColorText(plain, true)
	for _, want := range []string{"\x1b[32mrunning\x1b[0m", "\x1b[31mstopped\x1b[0m", "\x1b[33mpaused\x1b[0m", "\x1b[32mavailable\x1b[0m", "\x1b[31munavailable\x1b[0m"} {
		if !strings.Contains(styled, want) {
			t.Fatalf("styled status missing %q: %q", want, styled)
		}
	}
	if got := statusColorText(plain, false); got != plain {
		t.Fatalf("disabled colors changed output: %q", got)
	}
}

func TestStatusColorTextColorsGuestStateWithoutColoringName(t *testing.T) {
	plain := "  101  stopped  running  [node-unavailable]"
	styled := statusColorText(plain, true)
	if !strings.Contains(styled, "  stopped  \x1b[32mrunning\x1b[0m  [node-unavailable]") {
		t.Fatalf("guest name/node were colored instead of just state: %q", styled)
	}
	plain = "  102  paused-vm  stopped"
	styled = statusColorText(plain, true)
	if !strings.Contains(styled, "paused-vm  \x1b[31mstopped\x1b[0m") {
		t.Fatalf("guest status not colored: %q", styled)
	}
}

func TestStatusColorTextColorsTaskWarningsYellow(t *testing.T) {
	plain := "  pve-nodex-lab  vzstart  CT 9911  completed  WARNINGS: 1  WARN: Systemd 259 detected. You may need to enable nesting."
	styled := statusColorText(plain, true)
	for _, want := range []string{
		"\x1b[33mWARNINGS:\x1b[0m 1",
		"\x1b[33mWARN:\x1b[0m Systemd 259 detected. You may need to enable nesting.",
	} {
		if !strings.Contains(styled, want) {
			t.Fatalf("task warning not styled yellow (%q): %q", want, styled)
		}
	}
}
