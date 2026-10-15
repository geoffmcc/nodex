package cli

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/term"
)

type statusTerminal struct {
	in, out       *os.File
	inputState    *term.State
	restoreOutput func() error
	screen        bool
}

func (t *statusTerminal) Begin() error {
	state, err := term.MakeRaw(int(t.in.Fd()))
	if err != nil {
		return err
	}
	t.inputState = state
	cleanup, err := prepareWatchConsole(t.in, t.out)
	t.restoreOutput = cleanup
	if err != nil {
		return err
	}
	t.screen = true
	_, err = io.WriteString(t.out, "\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H")
	return err
}
func (t *statusTerminal) End() error {
	var errs []error
	if t.screen {
		_, err := io.WriteString(t.out, "\x1b[0m\x1b[?25h\x1b[?1049l")
		errs = append(errs, err)
		t.screen = false
	}
	if t.restoreOutput != nil {
		errs = append(errs, t.restoreOutput())
		t.restoreOutput = nil
	}
	if t.inputState != nil {
		errs = append(errs, term.Restore(int(t.in.Fd()), t.inputState))
		t.inputState = nil
	}
	return errors.Join(errs...)
}
func (t *statusTerminal) ReadKey(ctx context.Context) (byte, error) { return readWatchKey(ctx, t.in) }
func (t *statusTerminal) Size() (int, int, error)                   { return term.GetSize(int(t.out.Fd())) }
func (t *statusTerminal) Draw(frame string) error                   { _, err := io.WriteString(t.out, frame); return err }
