//go:build linux

package cli

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/output"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// Use a disposable PTY, never the developer's terminal or infrastructure.
func TestStatusTerminalPTYRestoresAndReadsKeys(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	if value, ok := os.LookupEnv("NO_COLOR"); ok {
		if err := os.Unsetenv("NO_COLOR"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Setenv("NO_COLOR", value) })
	}
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	fd := int(master.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	colorCtx := &Context{Opts: Options{Output: output.FormatTable}, InteractiveOut: slave}
	if !statusColorEnabled(colorCtx) {
		t.Fatal("interactive PTY should enable table colors")
	}
	watchCtx := &Context{
		Opts:           Options{Output: output.FormatTable},
		InteractiveIn:  slave,
		InteractiveOut: slave,
	}
	t.Setenv("TERM", "dumb")
	err = runStatusWatch(t.Context(), watchCtx, time.Second)
	if app.ExitCodeFromError(err) != app.ExitUsage || !strings.Contains(err.Error(), "requires an interactive terminal") {
		t.Fatalf("TERM=dumb watch gate = %v, want actionable usage error", err)
	}
	t.Setenv("TERM", "xterm-256color")
	if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 80}); err != nil {
		t.Fatal(err)
	}
	before, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	terminal := &statusTerminal{in: slave, out: slave}
	defer terminal.End()
	if err := terminal.Begin(); err != nil {
		t.Fatal(err)
	}
	w, h, err := terminal.Size()
	if err != nil || w != 80 || h != 24 {
		t.Fatalf("size %d %d %v", w, h, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	for _, key := range []byte{'q', 3} {
		if _, err := master.Write([]byte{key}); err != nil {
			t.Fatal(err)
		}
		got, err := terminal.ReadKey(ctx)
		if err != nil || got != key {
			t.Fatalf("read %d want %d: %v", got, key, err)
		}
	}
	canceled, stop := context.WithCancel(t.Context())
	stop()
	if _, err := terminal.ReadKey(canceled); err != context.Canceled {
		t.Fatal("input cancellation", err)
	}
	if err := terminal.Draw("\x1b[Hfixture\x1b[J"); err != nil {
		t.Fatal(err)
	}
	if err := terminal.End(); err != nil {
		t.Fatal(err)
	}
	after, err := term.GetState(int(slave.Fd()))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("terminal not restored", err)
	}
	// A PTY is a stream: output can arrive in several reads under race/load.
	readCtx, stopRead := context.WithTimeout(t.Context(), 2*time.Second)
	defer stopRead()
	var captured strings.Builder
	for !strings.Contains(captured.String(), "\x1b[?1049l") && captured.Len() < 1024 {
		ch, err := readWatchKey(readCtx, master)
		if err != nil {
			t.Fatal("read terminal lifecycle output", err)
		}
		captured.WriteByte(ch)
	}
	text := captured.String()
	for _, sequence := range []string{"\x1b[?1049h", "\x1b[?25l", "\x1b[?25h", "\x1b[?1049l"} {
		if !strings.Contains(text, sequence) {
			t.Fatalf("missing lifecycle sequence %q in %q", sequence, text)
		}
	}
}

func TestStatusWatchRunsAndRestoresInPTY(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	if value, ok := os.LookupEnv("NO_COLOR"); ok {
		if err := os.Unsetenv("NO_COLOR"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Setenv("NO_COLOR", value) })
	}
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 80}); err != nil {
		t.Fatal(err)
	}
	terminal := &statusTerminal{in: slave, out: slave}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- watchStatus(ctx, terminal, "nodex-test-lab", true, time.Hour, time.Second, func(context.Context) *statusOverview {
			s := healthyWatchSnapshot()
			s.Containers = 1
			s.ContainerDetails = []statusGuest{{ID: "node/100", Name: "CT100", Node: "node", Status: "running"}}
			return s
		})
	}()

	readCtx, stopRead := context.WithTimeout(t.Context(), 2*time.Second)
	defer stopRead()
	var captured strings.Builder
	for !strings.Contains(captured.String(), "CT100") {
		ch, err := readWatchKey(readCtx, master)
		if err != nil {
			t.Fatal("watch did not draw guest inventory in PTY", err)
		}
		captured.WriteByte(ch)
	}
	if _, err := master.Write([]byte{'q'}); err != nil {
		t.Fatal("send watch quit key", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("watch exit", err)
		}
	case <-ctx.Done():
		t.Fatal("watch did not exit after q", ctx.Err())
	}
	for !strings.Contains(captured.String(), "\x1b[?1049l") && captured.Len() < 64*1024 {
		ch, err := readWatchKey(readCtx, master)
		if err != nil {
			t.Fatal("watch did not restore PTY screen", err)
		}
		captured.WriteByte(ch)
	}
	text := captured.String()
	if !strings.Contains(text, "\x1b[32mrunning\x1b[0m") {
		t.Fatalf("PTY watch output lacked colored running state: %q", text)
	}
	for _, sequence := range []string{"\x1b[?25h", "\x1b[?1049l"} {
		if !strings.Contains(text, sequence) {
			t.Fatalf("PTY watch output missing restore sequence %q", sequence)
		}
	}
}
