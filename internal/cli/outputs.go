package cli

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/photos-sh/photos-cli/internal/output"
)

// errExists means the output file is already there and --force was not given.
var errExists = errors.New("already exists (use --force to replace it)")

// writeAtomic writes data to path so that the file is either complete or
// absent: into a temporary file beside it, then moved into place. Without
// force an existing file is never replaced, also not one that appears while
// this runs.
func writeAtomic(path string, data []byte, force bool) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".photos-*.tmp")
	if err != nil {
		return pathError(err)
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(name)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return pathError(err)
	}
	if err = tmp.Close(); err != nil {
		return pathError(err)
	}
	// CreateTemp makes it 0600; a result is an ordinary file.
	if err = os.Chmod(name, 0o644); err != nil {
		return pathError(err)
	}

	if force {
		return renameErr(os.Rename(name, path))
	}
	// A hard link fails if the name is taken, which makes "do not replace"
	// one step. Where links are not possible, look first and then move.
	if lerr := os.Link(name, path); lerr == nil {
		os.Remove(name)
		return nil
	} else if errors.Is(lerr, os.ErrExist) {
		return errExists
	}
	if _, serr := os.Lstat(path); serr == nil {
		return errExists
	}
	return renameErr(os.Rename(name, path))
}

func renameErr(err error) error {
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err
	}
	return err
}

// exists reports whether something is at path.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// extensionFor is the file extension for a content type: ".png", ".jpg",
// ".json"; fallback when the type is not one it knows.
func extensionFor(contentType, fallback string) string {
	media, _, _ := mime.ParseMediaType(contentType)
	switch strings.ToLower(media) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/avif":
		return ".avif"
	case "image/gif":
		return ".gif"
	case "image/tiff":
		return ".tiff"
	case "image/svg+xml":
		return ".svg"
	case "application/json":
		return ".json"
	}
	return fallback
}

// formatExtension is the extension of a --format value, "" for none.
func formatExtension(format string) string {
	switch strings.ToLower(format) {
	case "png":
		return ".png"
	case "jpg", "jpeg":
		return ".jpg"
	case "webp":
		return ".webp"
	case "avif":
		return ".avif"
	}
	return ""
}

// sameFormatExtension predicts the extension of a result that keeps the
// photo's own format unless --format says otherwise; "" when it cannot tell.
func sameFormatExtension(format, sourceExt string) string {
	if ext := formatExtension(format); ext != "" || format != "" {
		return ext
	}
	switch sourceExt {
	case ".jpg", ".jpeg":
		return ".jpg"
	case ".png", ".webp", ".avif":
		return sourceExt
	}
	return ""
}

// namer hands out output names in one run so that no two inputs share one:
// the second "shoe" in a directory becomes "shoe-2".
type namer struct {
	taken map[string]bool
}

// claim returns stem, or stem-2, stem-3…, not yet claimed in dir. Names are
// compared without their case, as macOS and Windows compare them.
func (n *namer) claim(dir, stem string) string {
	if n.taken == nil {
		n.taken = map[string]bool{}
	}
	candidate := stem
	for i := 2; ; i++ {
		key := strings.ToLower(filepath.Join(dir, candidate))
		if !n.taken[key] {
			n.taken[key] = true
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", stem, i)
	}
}

// fileResult is what one file became: a line for a person, or with --json
// one object on its own line.
type fileResult struct {
	// Item is the item's number, for results of a batch job.
	Item        *int   `json:"item,omitempty"`
	Input       string `json:"input"`
	Output      string `json:"output"`
	ContentType string `json:"content_type"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Quality     int    `json:"quality,omitempty"`
	Bytes       int    `json:"bytes"`
	// What remove-text and fix report beside the image.
	Removed  *int      `json:"removed,omitempty"`
	Kept     *int      `json:"kept,omitempty"`
	Passed   *bool     `json:"passed,omitempty"`
	Findings *[]string `json:"findings,omitempty"`
	// SubjectFound is whether crop found a subject (or, with --focus face,
	// a face), and false when remove-background was asked for a canvas and
	// the photo has no subject. Absent when the API did not say.
	SubjectFound *bool `json:"subject_found,omitempty"`
	// Paths is how many paths vectorize's SVG has.
	Paths *int `json:"paths,omitempty"`
	// Faces is how many faces blur-faces hid or restore-faces restored;
	// FacesSkipped how many restore-faces left alone.
	Faces        *int `json:"faces,omitempty"`
	FacesSkipped *int `json:"faces_skipped,omitempty"`
	// Score is how sure the model is of select's mask, 0 to 1.
	Score *float64 `json:"score,omitempty"`
	// HeadMm is the head's height in an id photo, in millimetres. Passed and
	// Findings are id-photo's too: what it could see is wrong.
	HeadMm *float64 `json:"head_mm,omitempty"`
}

// fileFailure is the --json line of a file that failed.
type fileFailure struct {
	Item  *int      `json:"item,omitempty"`
	Input string    `json:"input"`
	Error errorJSON `json:"error"`
}

type errorJSON struct {
	// Code is the API's error code, or one of the CLI's own: unreachable,
	// timeout, file_exists, failed.
	Code    string `json:"code"`
	Message string `json:"message"`
	// Status is the HTTP status, for an error the API answered with.
	Status int `json:"status,omitempty"`
}

// line is the result for a person.
func (r *fileResult) line(p *output.Printer, width int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-*s -> %s", width, output.OneLine(r.Input), p.Paint(output.Bold, output.OneLine(r.Output)))
	if r.Width > 0 && r.Height > 0 {
		fmt.Fprintf(&b, "  %dx%d", r.Width, r.Height)
	}
	fmt.Fprintf(&b, "  %s  %s", r.ContentType, output.Size(int64(r.Bytes)))
	if r.Removed != nil && r.Kept != nil {
		fmt.Fprintf(&b, "  text removed: %d, kept: %d", *r.Removed, *r.Kept)
	}
	if r.SubjectFound != nil {
		if *r.SubjectFound {
			b.WriteString("  subject found")
		} else {
			b.WriteString("  " + p.Paint(output.Yellow, "no subject found"))
		}
	}
	if r.Paths != nil {
		fmt.Fprintf(&b, "  paths: %d", *r.Paths)
	}
	switch {
	case r.Faces != nil && r.FacesSkipped != nil:
		fmt.Fprintf(&b, "  faces restored: %d, skipped: %d", *r.Faces, *r.FacesSkipped)
	case r.Faces != nil && *r.Faces == 0:
		// Said loudly: every face in the photo still shows.
		b.WriteString("  " + p.Paint(output.Yellow, "no faces found"))
	case r.Faces != nil:
		fmt.Fprintf(&b, "  faces hidden: %d", *r.Faces)
	}
	if r.Score != nil {
		fmt.Fprintf(&b, "  score: %s", round(*r.Score))
	}
	switch {
	case r.HeadMm != nil:
		// An id photo. "Passes" would promise more than the tool can see:
		// an empty list is not an approval.
		fmt.Fprintf(&b, "  head: %s mm", num(*r.HeadMm))
		switch {
		case r.Findings != nil && len(*r.Findings) > 0:
			b.WriteString("  " + p.Paint(output.Yellow, "findings: "+output.OneLine(strings.Join(*r.Findings, ", "))))
		case r.Passed != nil && !*r.Passed:
			b.WriteString("  " + p.Paint(output.Yellow, "findings: not listed"))
		default:
			b.WriteString("  " + p.Paint(output.Green, "no findings"))
		}
	case r.Passed != nil && *r.Passed:
		b.WriteString("  " + p.Paint(output.Green, "passes"))
	case r.Passed != nil:
		verdict := "does not pass"
		if r.Findings != nil && len(*r.Findings) > 0 {
			verdict += ": " + output.OneLine(strings.Join(*r.Findings, ", "))
		}
		b.WriteString("  " + p.Paint(output.Yellow, verdict))
	}
	return b.String()
}

// task is one file to produce.
type task struct {
	// display names the input in the output.
	display string
	// item is the item's number in a batch job, or nil.
	item *int
	// do produces the file. A result whose Output is "-" went to standard
	// output and gets no line.
	do func(ctx context.Context) (*fileResult, error)
}

// runTasks produces the files, a few at once, and reports each on its own
// line, in the order given. It keeps going after a failure, except one that
// every other file would meet too (the key refused, no credits left). The
// exit code is that of the most severe failure.
func runTasks(ctx context.Context, env *Env, tasks []task, parallel int, noun string) error {
	p := env.Printer
	width := 0
	for _, t := range tasks {
		width = max(width, len(t.display))
	}
	width = min(width, 48)

	type outcome struct {
		index int
		res   *fileResult
		err   error
	}
	// inner is cancelled by Ctrl-C (through ctx) and by a failure that makes
	// the rest pointless.
	inner, abort := context.WithCancel(ctx)
	defer abort()

	indexes := make(chan int)
	outcomes := make(chan outcome)
	var workers sync.WaitGroup
	for w := 0; w < max(1, min(parallel, len(tasks))); w++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range indexes {
				res, err := tasks[i].do(inner)
				outcomes <- outcome{index: i, res: res, err: err}
			}
		}()
	}
	go func() {
		defer close(indexes)
		for i := range tasks {
			select {
			case indexes <- i:
			case <-inner.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(outcomes)
	}()

	var (
		done, failed int
		worst        error
		worstCode    int
	)
	report := func(o outcome) {
		t := tasks[o.index]
		if o.err != nil {
			if errors.Is(o.err, context.Canceled) {
				return // interrupted or given up: counted below
			}
			failed++
			if code := ExitCode(o.err); code > worstCode {
				worst, worstCode = o.err, code
				if code == ExitAuth || code == ExitPayment {
					abort()
				}
			}
			if p.JSON {
				failure := fileFailure{Item: t.item, Input: t.display, Error: errorJSON{Code: errorCode(o.err), Message: describe(o.err)}}
				failure.Error.Status = httpStatus(o.err)
				_ = p.JSONLine(failure)
			}
			p.Error("%s: %s", output.OneLine(t.display), describe(o.err))
			return
		}
		done++
		if p.JSON {
			_ = p.JSONLine(o.res)
		} else if o.res.Output != "-" {
			p.Info("%s", o.res.line(p, width))
		}
	}

	// Results are reported in the order of the inputs, each as soon as every
	// one before it is in.
	waiting := map[int]outcome{}
	next := 0
	for o := range outcomes {
		waiting[o.index] = o
		for {
			ready, ok := waiting[next]
			if !ok {
				break
			}
			delete(waiting, next)
			report(ready)
			next++
		}
	}
	for i := next; i < len(tasks); i++ { // only after an interruption
		if o, ok := waiting[i]; ok {
			report(o)
		}
	}

	rest := len(tasks) - done - failed
	if ctx.Err() != nil {
		p.Error("interrupted: %d of %d %s done", done, len(tasks), noun)
		return &exitError{code: ExitInterrupted}
	}
	if len(tasks) > 1 && (failed > 0 || rest > 0) {
		summary := fmt.Sprintf("%d %s: %d done, %d failed", len(tasks), noun, done, failed)
		if rest > 0 {
			summary += fmt.Sprintf(", %d not tried", rest)
		}
		p.Note("%s", summary)
	}
	if worst != nil {
		if h := hint(worst); h != "" {
			p.Note("%s", h)
		}
		return &exitError{code: worstCode}
	}
	return nil
}
