//go:build linux

package terminal

import "syscall"

const (
	ioctlGet = syscall.TCGETS
	ioctlSet = syscall.TCSETS
)
