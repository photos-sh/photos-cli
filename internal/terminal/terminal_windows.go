//go:build windows

package terminal

import (
	"os"
	"syscall"
)

const (
	enableEchoInput                 = 0x0004
	enableVirtualTerminalProcessing = 0x0004
)

var procSetConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")

func setConsoleMode(h syscall.Handle, mode uint32) error {
	r, _, err := procSetConsoleMode.Call(uintptr(h), uintptr(mode))
	if r == 0 {
		return err
	}
	return nil
}

// A console is a handle that has a console mode.
func isTerminal(f *os.File) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode) == nil
}

// enableColour asks the console to read ANSI escape codes, which Windows 10
// and later do on request.
func enableColour(f *os.File) bool {
	h := syscall.Handle(f.Fd())
	var mode uint32
	if syscall.GetConsoleMode(h, &mode) != nil {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	return setConsoleMode(h, mode|enableVirtualTerminalProcessing) == nil
}

func disableEcho(f *os.File) (restore func(), err error) {
	h := syscall.Handle(f.Fd())
	var old uint32
	if err := syscall.GetConsoleMode(h, &old); err != nil {
		return nil, err
	}
	if err := setConsoleMode(h, old&^enableEchoInput); err != nil {
		return nil, err
	}
	return func() { _ = setConsoleMode(h, old) }, nil
}
