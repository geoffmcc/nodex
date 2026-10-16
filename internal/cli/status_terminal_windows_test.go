//go:build windows

package cli

import (
	"context"
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Opt in only in a disposable console/ConPTY; never inject input into a user's
// normal interactive shell. The regular suite still runs fake-terminal tests.
func TestStatusTerminalWindowsConsole(t *testing.T) {
	if os.Getenv("NODEX_TEST_WATCH_TERMINAL") != "1" {
		t.Skip("requires an isolated console with NODEX_TEST_WATCH_TERMINAL=1")
	}
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	var inputBefore, outputBefore uint32
	if err := windows.GetConsoleMode(windows.Handle(in.Fd()), &inputBefore); err != nil {
		t.Fatal(err)
	}
	if err := windows.GetConsoleMode(windows.Handle(out.Fd()), &outputBefore); err != nil {
		t.Fatal(err)
	}
	terminal := &statusTerminal{in: in, out: out}
	defer terminal.End()
	if err := terminal.Begin(); err != nil {
		t.Fatal(err)
	}
	writeInput := windows.NewLazySystemDLL("kernel32.dll").NewProc("WriteConsoleInputW")
	for _, key := range []uint16{'q', 3} {
		record := watchInputRecord{EventType: 1, KeyDown: 1, RepeatCount: 1, UnicodeChar: key}
		var written uint32
		ok, _, err := writeInput.Call(uintptr(in.Fd()), uintptr(unsafe.Pointer(&record)), 1, uintptr(unsafe.Pointer(&written)))
		if ok == 0 || written != 1 {
			t.Fatalf("write console input: %v", err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		got, err := terminal.ReadKey(ctx)
		cancel()
		if err != nil || uint16(got) != key {
			t.Fatalf("read key %d want %d: %v", got, key, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := terminal.ReadKey(ctx); err != context.Canceled {
		t.Fatal("input cancellation", err)
	}
	if err := terminal.End(); err != nil {
		t.Fatal(err)
	}
	var inputAfter, outputAfter uint32
	if err := windows.GetConsoleMode(windows.Handle(in.Fd()), &inputAfter); err != nil {
		t.Fatal(err)
	}
	if err := windows.GetConsoleMode(windows.Handle(out.Fd()), &outputAfter); err != nil {
		t.Fatal(err)
	}
	if inputBefore != inputAfter || outputBefore != outputAfter {
		t.Fatal("console modes not restored")
	}
}
