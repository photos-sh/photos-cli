package terminal

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestReadLine(t *testing.T) {
	r := strings.NewReader("first line\r\nsecond\n")
	for _, want := range []string{"first line", "second"} {
		got, err := ReadLine(r)
		if err != nil || got != want {
			t.Errorf("ReadLine = %q, %v; want %q", got, err, want)
		}
	}
	if _, err := ReadLine(r); err == nil {
		t.Error("no error at the end")
	}
	// A last line without a newline is still a line.
	if got, err := ReadLine(strings.NewReader("no newline")); err != nil || got != "no newline" {
		t.Errorf("ReadLine = %q, %v", got, err)
	}
}

// A pipe, a file and the null device are not terminals, and echo cannot be
// turned off on them.
func TestNotATerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()

	for name, f := range map[string]*os.File{"pipe": r, "null device": null, "nil": nil} {
		if IsTerminal(f) {
			t.Errorf("%s is reported as a terminal", name)
		}
		if Colour(f) {
			t.Errorf("%s is reported as taking colour", name)
		}
	}
	if _, err := ReadSecret(context.Background(), r); err != ErrNoEcho {
		t.Errorf("ReadSecret on a pipe: %v, want ErrNoEcho", err)
	}
}
