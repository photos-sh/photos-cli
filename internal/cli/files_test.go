package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Results are written beside their sources, named for the operation, with
// the extension of the result's format.
func TestResultsAreNamedBesideTheirSources(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.image("b.jpg")
	s.write("sub/c.png", pngImage(8, 8))

	tests := []struct {
		args string
		want string
	}{
		{"remove-background b.jpg", "b-nobg.png"},
		{"remove-background --mask b.jpg", "b-mask.png"},
		{"remove-background --format webp a.png", "a-nobg.webp"},
		{"resize --width 10 --format jpg a.png", "a-resized.jpg"},
		{"upscale a.png", "a-upscaled.png"},
		{"erase --rect 0,0,1,1 a.png", "a-erased.png"},
		{"remove-text a.png", "a-notext.png"},
		{"run --preset marketplace a.png", "a-marketplace.png"},
		{"fix --marketplace amazon a.png", "a-amazon.png"},
		{"resize --width 10 sub/c.png", "sub/c-resized.png"},
		{"resize --width 10 --suffix _small a.png", "a_small.png"},
		{"resize --width 10 -o named.png a.png", "named.png"},
		{"resize --width 10 --out-dir out a.png", "out/a.png"},
		{"resize --width 10 --out-dir out2 --suffix -s a.png", "out2/a-s.png"},
	}
	for _, tc := range tests {
		t.Run(tc.args, func(t *testing.T) {
			res := s.ok(strings.Fields(tc.args)...)
			want := filepath.FromSlash(tc.want)
			data, err := os.ReadFile(want)
			if err != nil {
				t.Fatalf("no %s; files: %v", tc.want, s.files())
			}
			if !bytes.HasPrefix(data, []byte("\x89PNG")) {
				t.Errorf("%s is not what the API sent", tc.want)
			}
			if !strings.Contains(res.stdout, "-> "+want) {
				t.Errorf("the line does not name the output: %q", res.stdout)
			}
		})
	}
}

// A directory stands for the images in it: sorted, without what is not an
// image, without hidden files, and below it only with --recursive, where
// --out-dir keeps the layout.
func TestDirectories(t *testing.T) {
	s := newSession(t)
	for _, name := range []string{"in/b.png", "in/a.JPG", "in/notes.txt", "in/.hidden.png", "in/deep/c.webp", "in/deep/deeper/d.png", "in/.git/e.png"} {
		s.image(name)
	}

	res := s.ok("resize", "--width", "10", "--out-dir", "flat", "in")
	if got := len(s.api.all()); got != 2 {
		t.Fatalf("%d requests, want 2 (a.JPG and b.png)\n%s", got, res.stdout)
	}
	lines := strings.Split(strings.TrimSpace(res.stdout), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], filepath.FromSlash("in/a.JPG")) || !strings.HasPrefix(lines[1], filepath.FromSlash("in/b.png")) {
		t.Errorf("want a.JPG then b.png:\n%s", res.stdout)
	}

	s.api.reset()
	s.ok("resize", "--width", "10", "--out-dir", "tree", "--recursive", "in")
	if got := len(s.api.all()); got != 4 {
		t.Fatalf("--recursive: %d requests, want 4", got)
	}
	for _, want := range []string{"tree/a.png", "tree/b.png", "tree/deep/c.png", "tree/deep/deeper/d.png"} {
		if !exists(filepath.FromSlash(want)) {
			t.Errorf("no %s; files: %v", want, s.files())
		}
	}

	// A second run beside the sources leaves the first run's results alone.
	s.ok("resize", "--width", "10", "in")
	s.api.reset()
	res = s.run("resize", "--width", "10", "--force", "in")
	if got := len(s.api.all()); got != 2 {
		t.Errorf("the second run sent %d files, want 2: results of the first were taken for sources", got)
	}
	if !strings.Contains(res.stderr, "left out 2 file(s)") {
		t.Errorf("no word about the files left out: %q", res.stderr)
	}

	// An output directory inside the input is not searched.
	s.ok("resize", "--width", "10", "--recursive", "--out-dir", "in/out", "in")
	s.api.reset()
	s.ok("resize", "--width", "10", "--recursive", "--out-dir", "in/out", "--force", "in")
	if got := len(s.api.all()); got != 6 { // a, a-resized, b, b-resized, c, d: not in/out/*
		t.Errorf("searched the output directory: %d requests, want 6", got)
	}

	// Nothing to do is said, and is a failure.
	if err := os.Mkdir("empty", 0o755); err != nil {
		t.Fatal(err)
	}
	res = s.run("resize", "--width", "10", "empty")
	if res.code != ExitFailed || !strings.Contains(res.stderr, "no images") {
		t.Errorf("empty directory: exit %d, stderr %q", res.code, res.stderr)
	}
}

// Two inputs never share an output name, and an existing file is never
// replaced without --force.
func TestCollisionsAndForce(t *testing.T) {
	s := newSession(t)
	s.image("x/shoe.png")
	s.image("y/shoe.png")
	s.image("y/shoe.jpg")

	res := s.ok("resize", "--width", "10", "--out-dir", "out", "x/shoe.png", "y/shoe.png", "y/shoe.jpg")
	want := []string{"out/shoe-2.png", "out/shoe-3.png", "out/shoe.png"}
	var got []string
	for _, f := range s.files() {
		if strings.HasPrefix(f, "out/") {
			got = append(got, f)
		}
	}
	if !equal(got, want) {
		t.Fatalf("outputs %v, want %v\n%s", got, want, res.stdout)
	}

	// The file is in the way: refused before the request is made.
	before, _ := os.ReadFile(filepath.FromSlash("out/shoe.png"))
	s.api.reset()
	res = s.run("resize", "--width", "20", "--out-dir", "out", "x/shoe.png")
	if res.code != ExitFailed || !strings.Contains(res.stderr, "already exists") || !strings.Contains(res.stderr, "--force") {
		t.Fatalf("exit %d, stderr %q", res.code, res.stderr)
	}
	if n := len(s.api.all()); n != 0 {
		t.Errorf("%d requests were made for a file that could not be written", n)
	}
	after, _ := os.ReadFile(filepath.FromSlash("out/shoe.png"))
	if !bytes.Equal(before, after) {
		t.Fatal("the existing file was replaced without --force")
	}

	// The same with -o, and with a format only the answer reveals.
	s.write("taken.webp", []byte("mine"))
	if res := s.run("resize", "--width", "20", "-o", "taken.webp", "x/shoe.png"); res.code != ExitFailed {
		t.Errorf("-o onto an existing file: exit %d", res.code)
	}
	s.write("x/shoe-marketplace.png", []byte("mine"))
	if res := s.run("run", "--preset", "marketplace", "x/shoe.png"); res.code != ExitFailed || !strings.Contains(res.stderr, "already exists") {
		t.Errorf("run onto an existing file: exit %d, stderr %q", res.code, res.stderr)
	}
	if data, _ := os.ReadFile(filepath.FromSlash("x/shoe-marketplace.png")); string(data) != "mine" {
		t.Error("run replaced an existing file without --force")
	}
	if data, _ := os.ReadFile("taken.webp"); string(data) != "mine" {
		t.Error("-o replaced an existing file without --force")
	}

	// --force replaces.
	s.ok("resize", "--width", "20", "--out-dir", "out", "--force", "x/shoe.png")
	after, _ = os.ReadFile(filepath.FromSlash("out/shoe.png"))
	if bytes.Equal(before, after) {
		t.Error("--force did not replace the file")
	}
	s.ok("resize", "--width", "20", "-o", "taken.webp", "--force", "x/shoe.png")
	if data, _ := os.ReadFile("taken.webp"); string(data) == "mine" {
		t.Error("--force did not replace the -o file")
	}

	// No temporary file is left by any of it.
	for _, f := range s.files() {
		if strings.Contains(f, ".photos-") || strings.HasSuffix(f, ".tmp") {
			t.Errorf("a temporary file was left: %s", f)
		}
	}
}

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.png")

	if err := writeAtomic(path, []byte("one"), false); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(path, []byte("two"), false); !errors.Is(err, errExists) {
		t.Fatalf("second write: %v, want errExists", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "one" {
		t.Fatalf("the file was replaced: %q", data)
	}
	if err := writeAtomic(path, []byte("three"), true); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "three" {
		t.Fatalf("force did not replace: %q", data)
	}
	// A directory that is not there fails, and leaves nothing.
	if err := writeAtomic(filepath.Join(dir, "missing", "out.png"), []byte("x"), false); err == nil {
		t.Fatal("writing into a missing directory succeeded")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
}

// Standard input and standard output make the command a filter.
func TestPiping(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	image := string(pngImage(8, 8))

	// In on standard input, out on standard output: nothing else there.
	res := s.runIn(image, "resize", "--width", "10", "--height", "5", "-o", "-", "-")
	if res.code != 0 || res.stdout != string(pngImage(10, 5)) {
		t.Fatalf("exit %d; standard output is not exactly the image (%d bytes)\nstderr: %s", res.code, len(res.stdout), res.stderr)
	}
	if res.stderr != "" {
		t.Errorf("a filter should be silent: %q", res.stderr)
	}
	req := s.api.only()
	if got := string(req.Files["image"][0].Data); got != image {
		t.Error("standard input was not what was uploaded")
	}

	// "-" alone implies standard output.
	res = s.runIn(image, "resize", "--width", "10", "--height", "5", "-")
	if res.code != 0 || res.stdout != string(pngImage(10, 5)) {
		t.Errorf("exit %d; `-` without -o did not write the image to standard output", res.code)
	}

	// A file in, standard output out.
	res = s.run("resize", "--width", "10", "--height", "5", "-o", "-", "a.png")
	if res.code != 0 || res.stdout != string(pngImage(10, 5)) {
		t.Errorf("exit %d; a file to standard output failed", res.code)
	}

	// Standard input in, a named file or a directory out.
	s.ok2(s.runIn(image, "resize", "--width", "10", "-o", "from-stdin.png", "-"))
	s.ok2(s.runIn(image, "resize", "--width", "10", "--out-dir", "o", "-"))
	for _, want := range []string{"from-stdin.png", "o/stdin.png"} {
		if !exists(filepath.FromSlash(want)) {
			t.Errorf("no %s; files: %v", want, s.files())
		}
	}

	// What cannot work is a usage error, and nothing is sent.
	s.api.reset()
	for _, tc := range []struct {
		stdin string
		args  string
	}{
		{image, "resize --json -o - a.png"},          // the image and JSON on one stream
		{image, "resize --json -"},                   // the same, implied
		{image, "resize -o - a.png sub"},             // -o with a directory
		{image, "resize -o x.png a.png a.png"},       // -o with two inputs
		{image, "resize -o x.png --out-dir d a.png"}, // both
		{image, "resize - -"},                        // standard input twice
		{image, "resize - a.png"},                    // nowhere to put stdin's result
		{"", "resize"},                               // nothing
		{"", "resize https://example.com/a.jpg"},     // a URL
		{"", "resize --parallel 0 a.png"},
	} {
		if res := s.runIn(tc.stdin, strings.Fields(tc.args)...); res.code != ExitUsage {
			t.Errorf("photos %s: exit %d, want %d\n%s", tc.args, res.code, ExitUsage, res.stderr)
		}
	}
	if n := len(s.api.all()); n != 0 {
		t.Errorf("%d requests were made by commands that were wrong", n)
	}

	// Empty standard input is a failed file, not a request.
	if res := s.runIn("", "resize", "-o", "x.png", "-"); res.code != ExitFailed || !strings.Contains(res.stderr, "empty") {
		t.Errorf("empty standard input: exit %d, %q", res.code, res.stderr)
	}
}

// ok2 fails the test unless the result is a success.
func (s *session) ok2(res result) {
	s.t.Helper()
	if res.code != 0 {
		s.t.Fatalf("exit %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
}

// --json: one object per file and nothing else on standard output.
func TestJSONShapesOfImageCommands(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.image("b.png")

	res := s.ok("resize", "--width", "30", "--height", "20", "--format", "jpg", "--json", "a.png")
	want := map[string]any{
		"input": "a.png", "output": "a-resized.jpg", "content_type": "image/jpeg",
		"width": 30.0, "height": 20.0, "quality": 85.0, "bytes": float64(len(pngImage(30, 20))),
	}
	lines := jsonLines(t, res.stdout)
	if len(lines) != 1 || !reflect.DeepEqual(lines[0], want) {
		t.Errorf("resize: %v\nwant %v", lines, want)
	}
	if strings.Count(res.stdout, "\n") != 1 {
		t.Errorf("want one line, got %q", res.stdout)
	}

	// The operation's extras.
	res = s.ok("remove-text", "--json", "a.png")
	if got := jsonLines(t, res.stdout)[0]; got["removed"] != 3.0 || got["kept"] != 1.0 {
		t.Errorf("remove-text: %v", got)
	}
	res = s.ok("fix", "--marketplace", "amazon", "--json", "a.png")
	got := jsonLines(t, res.stdout)[0]
	if got["passed"] != false || !reflect.DeepEqual(got["findings"], []any{"too_small", "text_overlay"}) {
		t.Errorf("fix: %v", got)
	}
	// A fix that passes still says so, with an empty list.
	s.api.handle("POST /v1/fix", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("X-Marketplace-Passed", "true")
		w.Header().Set("X-Marketplace-Findings", "")
		_, _ = w.Write([]byte("jpeg"))
	})
	res = s.ok("fix", "--marketplace", "amazon", "--json", "b.png")
	got = jsonLines(t, res.stdout)[0]
	if got["passed"] != true || !reflect.DeepEqual(got["findings"], []any{}) {
		t.Errorf("fix that passes: %v", got)
	}

	// Several files: JSON Lines, in the order of the inputs, a failure
	// among them with the API's code.
	s.api.handle("POST /v1/upscale", func(w http.ResponseWriter, r *http.Request) {
		apiError(w, 422, "unprocessable", "the image is too large to upscale")
	})
	s.image("c.png")
	s.api.handle("POST /v1/resize", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("width") == "13" && strings.Contains(r.Header.Get("Content-Type"), "multipart") {
			// Only b.png is broken; find it by its part's file name.
			_ = r.ParseMultipartForm(1 << 20)
			if _, h, err := r.FormFile("image"); err == nil && h.Filename == "b.png" {
				apiError(w, 400, "bad_request", "cannot decode the image")
				return
			}
		}
		answerImage(w, r)
	})
	res = s.run("resize", "--width", "13", "--out-dir", "j", "--json", "a.png", "b.png", "c.png")
	if res.code != ExitFailed {
		t.Fatalf("exit %d, want %d", res.code, ExitFailed)
	}
	lines = jsonLines(t, res.stdout)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d: %s", len(lines), res.stdout)
	}
	if lines[0]["input"] != "a.png" || lines[1]["input"] != "b.png" || lines[2]["input"] != "c.png" {
		t.Errorf("not in the order of the inputs: %s", res.stdout)
	}
	wantErr := map[string]any{"input": "b.png", "error": map[string]any{
		"code": "bad_request", "message": "cannot decode the image (bad_request, HTTP 400)", "status": 400.0,
	}}
	if !reflect.DeepEqual(lines[1], wantErr) {
		t.Errorf("the failed file: %v\nwant %v", lines[1], wantErr)
	}
	if lines[0]["output"] != filepath.FromSlash("j/a.png") || lines[2]["output"] != filepath.FromSlash("j/c.png") {
		t.Errorf("the others were not written: %s", res.stdout)
	}
	// The error is also said on standard error, for a person.
	if !strings.Contains(res.stderr, "b.png: cannot decode the image") || !strings.Contains(res.stderr, "3 files: 2 done, 1 failed") {
		t.Errorf("stderr: %q", res.stderr)
	}
}

// One file failing does not stop the others, each gets its line, and a file
// that is not there fails alone.
func TestSeveralFilesOneFailing(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.image("c.png")

	res := s.run("resize", "--width", "10", "a.png", "missing.png", "c.png")
	if res.code != ExitFailed {
		t.Fatalf("exit %d, want %d", res.code, ExitFailed)
	}
	out := strings.Split(strings.TrimSpace(res.stdout), "\n")
	if len(out) != 2 || !strings.HasPrefix(out[0], "a.png") || !strings.HasPrefix(out[1], "c.png") {
		t.Errorf("standard output:\n%s", res.stdout)
	}
	if !strings.Contains(res.stderr, "missing.png: no such file") && !strings.Contains(res.stderr, "missing.png: The system cannot find") {
		t.Errorf("stderr: %q", res.stderr)
	}
	if !exists("a-resized.png") || !exists("c-resized.png") {
		t.Errorf("the good files were not written: %v", s.files())
	}

	// --quiet: nothing but the error.
	res = s.run("resize", "--width", "10", "--force", "--quiet", "a.png", "missing.png", "c.png")
	if res.stdout != "" {
		t.Errorf("--quiet printed on standard output: %q", res.stdout)
	}
	if lines := strings.Split(strings.TrimSpace(res.stderr), "\n"); len(lines) != 1 || !strings.Contains(lines[0], "missing.png") {
		t.Errorf("--quiet should leave the error alone on standard error: %q", res.stderr)
	}
}

// With several failures the exit code is the most severe one's.
func TestTheMostSevereFailureDecidesTheExitCode(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.image("b.png")
	s.image("c.png")
	s.api.handle("POST /v1/resize", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		_, h, _ := r.FormFile("image")
		switch h.Filename {
		case "a.png":
			apiError(w, 400, "bad_request", "cannot decode the image")
		case "b.png":
			w.Header().Set("Retry-After", "0.01")
			apiError(w, 503, "busy", "all workers are busy")
		default:
			answerImage(w, r)
		}
	})
	res := s.run("resize", "--width", "10", "--retries", "1", "--parallel", "1", "a.png", "b.png", "c.png")
	if res.code != ExitUnavailable {
		t.Errorf("exit %d, want %d (busy is more severe than a bad file)\n%s", res.code, ExitUnavailable, res.stderr)
	}
	if !exists("c-resized.png") {
		t.Error("the good file was not written")
	}
}

// A refused key fails every file alike, so the rest are not sent.
func TestARefusedKeyStopsTheBatch(t *testing.T) {
	s := newSession(t)
	var names []string
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		names = append(names, s.image(n+".png"))
	}
	var calls atomic.Int32
	s.api.handle("POST /v1/resize", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		apiError(w, 401, "unauthorized", "invalid API key")
	})
	res := s.run(append([]string{"resize", "--width", "10", "--parallel", "1"}, names...)...)
	if res.code != ExitAuth {
		t.Fatalf("exit %d, want %d", res.code, ExitAuth)
	}
	if n := calls.Load(); n > 2 {
		t.Errorf("%d files were sent with a key that had been refused", n)
	}
	if !strings.Contains(res.stderr, "not tried") || !strings.Contains(res.stderr, "photos login") {
		t.Errorf("stderr: %q", res.stderr)
	}
}

// Results come out in the order of the inputs, whatever order they finish
// in, and --parallel bounds how many are sent at once.
func TestParallelKeepsTheOrder(t *testing.T) {
	s := newSession(t)
	var names []string
	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		names = append(names, s.image(n+".png"))
	}
	var running, peak atomic.Int32
	s.api.handle("POST /v1/resize", func(w http.ResponseWriter, r *http.Request) {
		now := running.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		_ = r.ParseMultipartForm(1 << 20)
		_, h, _ := r.FormFile("image")
		// The first files take the longest.
		time.Sleep(time.Duration('g'-h.Filename[0]) * 8 * time.Millisecond)
		running.Add(-1)
		answerImage(w, r)
	})
	res := s.ok(append([]string{"resize", "--width", "10", "--parallel", "3", "--json"}, names...)...)
	lines := jsonLines(t, res.stdout)
	if len(lines) != len(names) {
		t.Fatalf("%d lines, want %d", len(lines), len(names))
	}
	for i, line := range lines {
		if line["input"] != names[i] {
			t.Errorf("line %d is %v, want %s", i, line["input"], names[i])
		}
	}
	if p := peak.Load(); p > 3 || p < 2 {
		t.Errorf("%d requests at once, want 3 at most (and more than one)", p)
	}
}

// Ctrl-C drops the requests in flight, writes nothing partial, and exits
// 130.
func TestCancelLeavesNoPartialFile(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.image("b.png")

	started := make(chan struct{}, 8)
	s.api.handle("POST /v1/resize", func(w http.ResponseWriter, r *http.Request) {
		// Half an answer, then nothing until the client goes away.
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", "100000")
		_, _ = w.Write(pngImage(8, 8)[:20])
		w.(http.Flusher).Flush()
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan result, 1)
	go func() { done <- s.runCtx(ctx, "", "resize", "--width", "10", "a.png", "b.png") }()
	<-started
	<-started
	cancel()

	var res result
	select {
	case res = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the command did not stop when cancelled")
	}
	if res.code != ExitInterrupted {
		t.Errorf("exit %d, want %d\n%s", res.code, ExitInterrupted, res.stderr)
	}
	if got, want := s.files(), []string{"a.png", "b.png"}; !equal(got, want) {
		t.Errorf("files after the interruption: %v, want only %v", got, want)
	}
	if !strings.Contains(res.stderr, "interrupted") {
		t.Errorf("stderr: %q", res.stderr)
	}
}
