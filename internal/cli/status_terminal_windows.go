//go:build windows

package cli

import (
	"context"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var readConsoleInput = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

// INPUT_RECORD's 4-byte-aligned union contains a 16-byte KEY_EVENT_RECORD.
// https://learn.microsoft.com/windows/console/input-record-str
type watchInputRecord struct {
	EventType       uint16
	_               uint16
	KeyDown         int32
	RepeatCount     uint16
	VirtualKeyCode  uint16
	VirtualScanCode uint16
	UnicodeChar     uint16
	ControlKeyState uint32
}

func prepareWatchConsole(in, out *os.File) (func() error, error) {
	var inputMode, outputMode uint32
	if err := windows.GetConsoleMode(windows.Handle(in.Fd()), &inputMode); err != nil {
		return nil, err
	}
	// Read native key events, rather than leaving a blocking ReadFile goroutine
	// behind. Disable Quick Edit so mouse selection cannot pause polling.
	inputMode &^= windows.ENABLE_VIRTUAL_TERMINAL_INPUT | windows.ENABLE_QUICK_EDIT_MODE
	inputMode |= windows.ENABLE_EXTENDED_FLAGS
	if err := windows.SetConsoleMode(windows.Handle(in.Fd()), inputMode); err != nil {
		return nil, err
	}
	if err := windows.GetConsoleMode(windows.Handle(out.Fd()), &outputMode); err != nil {
		return nil, err
	}
	restore := func() error { return windows.SetConsoleMode(windows.Handle(out.Fd()), outputMode) }
	return restore, windows.SetConsoleMode(windows.Handle(out.Fd()), outputMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}

func readWatchKey(ctx context.Context, in *os.File) (byte, error) {
	handle := windows.Handle(in.Fd())
	for ctx.Err() == nil {
		state, err := windows.WaitForSingleObject(handle, 50)
		if err != nil {
			return 0, err
		}
		if state != windows.WAIT_OBJECT_0 {
			continue
		}
		// This session is the sole console reader. Consume one signaled record;
		// mouse/resize/focus events never enter a blocking text read.
		var record watchInputRecord
		var read uint32
		ok, _, err := readConsoleInput.Call(uintptr(handle), uintptr(unsafe.Pointer(&record)), 1, uintptr(unsafe.Pointer(&read)))
		if ok == 0 {
			return 0, err
		}
		if read == 0 || record.EventType != 1 || record.KeyDown == 0 {
			continue
		}
		if record.VirtualKeyCode == 'C' && record.ControlKeyState&0xC != 0 {
			return 3, nil
		}
		if record.UnicodeChar > 0 && record.UnicodeChar < 128 {
			return byte(record.UnicodeChar), nil
		}
	}
	return 0, ctx.Err()
}
