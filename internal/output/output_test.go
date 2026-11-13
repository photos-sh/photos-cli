package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"plain text":               "plain text",
		"tab\tand\nnewline":        "tab\tand\nnewline",
		"clear\x1b[2Jscreen":       "clear[2Jscreen",
		"bell\x07 and title\x1b]0": "bell and title]0",
		"c1 \u009b31m control":     "c1 31m control",
		"šđčćž €":                  "šđčćž €",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	if got := OneLine("a\nb\tc"); got != "a b c" {
		t.Errorf("OneLine: %q", got)
	}
}

func TestSize(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 999: "999 B", 1000: "1.0 kB", 34_100: "34.1 kB", 2_300_000: "2.3 MB", 5_000_000_000: "5.0 GB"} {
		if got := Size(n); got != want {
			t.Errorf("Size(%d) = %q, want %q", n, got, want)
		}
	}
}

// What goes where: results on standard output, the rest on standard error,
// and --quiet and --json silence what they should.
func TestPrinterStreams(t *testing.T) {
	say := func(p *Printer) {
		p.Data("data")
		p.Info("info")
		p.Note("note")
		p.Warn("warn")
		p.Error("error")
	}
	var out, errOut bytes.Buffer
	say(New(&out, &errOut))
	if out.String() != "data\ninfo\n" || errOut.String() != "note\nwarning: warn\nphotos: error\n" {
		t.Errorf("plain: %q / %q", out.String(), errOut.String())
	}

	out.Reset()
	errOut.Reset()
	p := New(&out, &errOut)
	p.Quiet = true
	say(p)
	if out.String() != "data\n" || errOut.String() != "photos: error\n" {
		t.Errorf("--quiet: %q / %q", out.String(), errOut.String())
	}

	out.Reset()
	errOut.Reset()
	p = New(&out, &errOut)
	p.JSON = true
	p.Info("info")
	_ = p.JSONLine(map[string]string{"url": "https://example.com/?a=1&b=<2>"})
	_ = p.Raw([]byte("{\"as\": \"it came\"}\n"))
	if out.String() != `{"url":"https://example.com/?a=1&b=<2>"}`+"\n"+`{"as": "it came"}`+"\n" {
		t.Errorf("--json: %q", out.String())
	}

	// Not a terminal: no colour, whatever is asked.
	if got := New(&out, &errOut).Paint(Red, "x"); got != "x" {
		t.Errorf("colour in a buffer: %q", got)
	}
}

func TestTable(t *testing.T) {
	var out bytes.Buffer
	New(&out, &out).Table([]string{"ID", "STATUS"}, [][]string{{"job_1", "done"}, {"job_22\nx", "failed"}})
	want := "ID        STATUS\njob_1     done\njob_22 x  failed\n"
	if out.String() != want {
		t.Errorf("table:\n%s\nwant\n%s", out.String(), want)
	}
	if strings.Contains(out.String(), "\t") {
		t.Error("tabs left in the table")
	}
}
