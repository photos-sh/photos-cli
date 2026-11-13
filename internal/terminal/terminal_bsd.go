//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package terminal

import "syscall"

// The BSDs spell the terminal-attributes ioctls differently.
const (
	ioctlGet = syscall.TIOCGETA
	ioctlSet = syscall.TIOCSETA
)
