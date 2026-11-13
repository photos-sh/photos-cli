//go:build linux || darwin || dragonfly || freebsd || netbsd || openbsd

package terminal

import (
	"os"
	"syscall"
	"unsafe"
)

func ioctl(fd, request uintptr, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(unsafe.Pointer(t)))
	if errno != 0 {
		return errno
	}
	return nil
}

// A terminal is a file descriptor that answers the terminal-attributes
// ioctl. (A character device is not enough: /dev/null is one.)
func isTerminal(f *os.File) bool {
	var t syscall.Termios
	return ioctl(f.Fd(), ioctlGet, &t) == nil
}

func enableColour(*os.File) bool { return os.Getenv("TERM") != "dumb" }

func disableEcho(f *os.File) (restore func(), err error) {
	var old syscall.Termios
	if err := ioctl(f.Fd(), ioctlGet, &old); err != nil {
		return nil, err
	}
	quiet := old
	quiet.Lflag &^= syscall.ECHO
	quiet.Lflag |= syscall.ICANON | syscall.ISIG
	if err := ioctl(f.Fd(), ioctlSet, &quiet); err != nil {
		return nil, err
	}
	return func() { _ = ioctl(f.Fd(), ioctlSet, &old) }, nil
}
