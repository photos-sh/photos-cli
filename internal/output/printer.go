// Package output writes what the commands have to say.
//
// Three rules hold it together:
//
// Standard output carries the result and nothing else: the lines a person
// reads, or with --json the JSON a script reads (one compact object per line
// for files, the API's own JSON for data). An image written with `-o -` has
// standard output to itself.
//
// Progress, warnings and errors go to standard error, so a pipe never
// carries them. --quiet silences everything there but errors.
//
// Colour is for a terminal: never in a pipe or a file, never with NO_COLOR.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/photos-sh/photos-cli/internal/terminal"
)

// Printer writes results and messages. It is safe for several goroutines.
type Printer struct {
	out io.Writer
	err io.Writer

	// JSON makes the output machine-readable; see the package comment.
	JSON bool
	// Quiet silences everything but errors and the data that was asked for.
	Quiet bool

	colourOut bool
	colourErr bool

	mu sync.Mutex
}

// New builds a printer on the two streams.
func New(out, errOut io.Writer) *Printer {
	return &Printer{
		out:       out,
		err:       errOut,
		colourOut: colourAllowed(out),
		colourErr: colourAllowed(errOut),
	}
}

// colourAllowed: only on a terminal, and not when NO_COLOR is set to
// anything (https://no-color.org).
func colourAllowed(w io.Writer) bool {
	if v, set := os.LookupEnv("NO_COLOR"); set && v != "" {
		return false
	}
	f, ok := w.(*os.File)
	return ok && terminal.Colour(f)
}

// Colours, as SGR codes.
const (
	Red    = "31"
	Green  = "32"
	Yellow = "33"
	Bold   = "1"
	Dim    = "2"
)

// Paint colours text for standard output, when that is a terminal.
func (p *Printer) Paint(code, text string) string { return paint(p.colourOut, code, text) }

func paint(on bool, code, text string) string {
	if !on || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// Data writes a line of the answer to standard output. It is printed even
// with --quiet: it is what was asked for.
func (p *Printer) Data(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.out, fmt.Sprintf(format, args...))
}

// Info writes a line about something done (a file written, a job deleted) to
// standard output. Nothing with --json, where the JSON says it, or --quiet.
func (p *Printer) Info(format string, args ...any) {
	if p.JSON || p.Quiet {
		return
	}
	p.Data(format, args...)
}

// Note writes progress to standard error, unless --quiet.
func (p *Printer) Note(format string, args ...any) {
	if p.Quiet {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.err, fmt.Sprintf(format, args...))
}

// Warn writes a warning to standard error, unless --quiet.
func (p *Printer) Warn(format string, args ...any) {
	if p.Quiet {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.err, paint(p.colourErr, Yellow, "warning:")+" "+fmt.Sprintf(format, args...))
}

// Error writes an error to standard error. --quiet does not silence it.
func (p *Printer) Error(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintln(p.err, paint(p.colourErr, Red, "photos:")+" "+fmt.Sprintf(format, args...))
}

// JSONLine writes v as one compact line of JSON on standard output.
func (p *Printer) JSONLine(v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// A URL with a query string is a normal value here; "&" for "&" is
	// one somebody has to un-escape by hand.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.out.Write(buf.Bytes())
	return err
}

// Raw writes JSON from the API as it came, ending the line.
func (p *Printer) Raw(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	data = bytes.TrimRight(data, "\r\n")
	if _, err := p.out.Write(data); err != nil {
		return err
	}
	_, err := p.out.Write([]byte{'\n'})
	return err
}

// Bytes writes binary data (an image) to standard output.
func (p *Printer) Bytes(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.out.Write(data)
	return err
}

// Table writes aligned columns to standard output. The headers may be nil.
// Cells are cleaned and kept on one line.
func (p *Printer) Table(headers []string, rows [][]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w := tabwriter.NewWriter(p.out, 0, 8, 2, ' ', 0)
	if len(headers) > 0 {
		fmt.Fprintln(w, strings.Join(headers, "\t"))
	}
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = OneLine(cell)
		}
		fmt.Fprintln(w, strings.TrimRight(strings.Join(cells, "\t"), "\t"))
	}
	w.Flush()
}

// Size is a byte count for a person: "812 B", "34.1 kB", "2.3 MB".
func Size(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1f kB", float64(n)/1000)
	case n < 1000*1000*1000:
		return fmt.Sprintf("%.1f MB", float64(n)/(1000*1000))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1000*1000*1000))
}
