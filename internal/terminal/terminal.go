// Package terminal answers whether a file is a terminal somebody is looking
// at, and reads a line from one without showing it.
//
// Standard library only: the kernel is asked directly (an ioctl on Unix, the
// console mode on Windows), so there is no dependency and no `stty` to run.
package terminal

import (
	"context"
	"errors"
	"os"
	"strings"
)

// ErrNoEcho means the terminal's echo could not be turned off.
var ErrNoEcho = errors.New("cannot turn off echo on this terminal")

// IsTerminal reports whether f is a terminal. A pipe, a file and /dev/null
// are not.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	return isTerminal(f)
}

// Colour reports whether f is a terminal that understands ANSI colour. On
// Windows it also switches the console to understand it.
func Colour(f *os.File) bool {
	if !IsTerminal(f) {
		return false
	}
	return enableColour(f)
}

// ReadSecret reads one line from the terminal f with echo off, and puts the
// echo back before it returns, also when ctx ends first (Ctrl-C). It returns
// ErrNoEcho, having read nothing, if the echo cannot be turned off.
func ReadSecret(ctx context.Context, f *os.File) (string, error) {
	restore, err := disableEcho(f)
	if err != nil {
		return "", ErrNoEcho
	}
	defer restore()

	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := ReadLine(f)
		done <- result{line, err}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-done:
		return r.line, r.err
	}
}

// ReadLine reads up to the first newline, a byte at a time so that nothing
// after the line is consumed, and returns it without the line ending.
func ReadLine(f interface{ Read([]byte) (int, error) }) (string, error) {
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := f.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				break
			}
			b.WriteByte(buf[0])
		}
		if err != nil {
			if b.Len() > 0 {
				break
			}
			return "", err
		}
	}
	return strings.TrimRight(b.String(), "\r"), nil
}
