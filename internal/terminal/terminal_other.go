//go:build !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd && !windows

package terminal

import (
	"errors"
	"os"
)

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func enableColour(*os.File) bool { return false }

func disableEcho(*os.File) (func(), error) { return nil, errors.New("not supported") }
