package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

// The editing tools: what each names its result, what it says about it to a
// person and to a script, and which exit code that is.

// Each tool writes its result beside the source under its own name, says on
// its line what it found, and with --json adds the same facts to the object,
// and nothing when it has nothing to report.
func TestEditingToolsReportWhatTheyFound(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.image("logo.png")

	const idSizes = "--width-mm 35 --height-mm 45 --head-min-mm 32 --head-max-mm 36"
	idFacts := map[string]any{"passed": false, "findings": []any{"low_resolution", "background_not_plain"}, "head_mm": 34.2}
	tests := []struct {
		args string
		// output is the file written; contentType what the API said it is.
		output      string
		contentType string
		// human is what the line says after the file's size; "" for nothing.
		human string
		// facts are what --json has beside the file's own fields.
		facts map[string]any
	}{
		{"enhance a.png", "a-enhanced.png", "image/png", "", nil},
		{"expand --aspect 1:1 a.png", "a-expanded.png", "image/png", "", nil},
		{"watermark --text Acme a.png", "a-watermarked.png", "image/png", "", nil},
		{"watermark --logo logo.png --format jpg a.png", "a-watermarked.jpg", "image/jpeg", "", nil},
		{"denoise a.png", "a-denoised.png", "image/png", "", nil},
		{"colorize a.png", "a-colorized.png", "image/png", "", nil},
		{"remove-background --width 40 --height 30 a.png", "a-nobg.png", "image/png", "", nil},

		{"crop --aspect 1:1 a.png", "a-cropped.png", "image/png", "  subject found", map[string]any{"subject_found": true}},
		{"blur-faces a.png", "a-nofaces.png", "image/png", "  faces hidden: 2", map[string]any{"faces": 2.0}},
		{"restore-faces a.png", "a-restored.png", "image/png", "  faces restored: 3, skipped: 1", map[string]any{"faces": 3.0, "faces_skipped": 1.0}},
		{"select --point 1,1 a.png", "a-selected.png", "image/png", "  score: 0.93", map[string]any{"score": 0.934}},
		{"select --box 1,1,4,4 --mask a.png", "a-mask.png", "image/png", "  score: 0.93", map[string]any{"score": 0.934}},
		{"select --point 1,1 --format webp a.png", "a-selected.webp", "image/webp", "  score: 0.93", map[string]any{"score": 0.934}},
		{"vectorize a.png", "a-vector.svg", "image/svg+xml", "  paths: 23", map[string]any{"paths": 23.0}},

		// An id photo is a JPEG, named for its document; its findings are
		// listed, and never called a verdict.
		{"id-photo --document us-passport a.png", "a-us-passport.jpg", "image/jpeg",
			"  head: 34.2 mm  findings: low_resolution, background_not_plain", idFacts},
		{"id-photo " + idSizes + " a.png", "a-id.jpg", "image/jpeg",
			"  head: 34.2 mm  findings: low_resolution, background_not_plain", idFacts},
		{"id-photo --document uk-passport --format png a.png", "a-uk-passport.png", "image/png",
			"  head: 34.2 mm  findings: low_resolution, background_not_plain", idFacts},
	}
	for _, tc := range tests {
		t.Run(tc.args, func(t *testing.T) {
			res := s.ok(strings.Fields(tc.args)...)
			data, err := os.ReadFile(tc.output)
			if err != nil {
				t.Fatalf("no %s; files: %v", tc.output, s.files())
			}
			if tc.contentType == "image/svg+xml" {
				if string(data) != svgImage {
					t.Errorf("%s is not the SVG the API sent: %q", tc.output, data)
				}
			} else if !strings.HasPrefix(string(data), "\x89PNG") {
				t.Errorf("%s is not what the API sent", tc.output)
			}
			line := strings.TrimSuffix(res.stdout, "\n")
			if strings.Contains(line, "\n") || !strings.HasPrefix(line, "a.png -> "+tc.output+"  ") {
				t.Fatalf("want one line naming %s, got %q", tc.output, res.stdout)
			}
			// What follows the size is what the tool found, and nothing
			// else.
			if after := afterSize(t, line, tc.contentType); after != tc.human {
				t.Errorf("the line says %q after the size, want %q:\n%s", after, tc.human, line)
			}

			// For a script: the same facts, and no others.
			res = s.ok(append(strings.Fields(tc.args), "--json", "--force")...)
			lines := jsonLines(t, res.stdout)
			if len(lines) != 1 {
				t.Fatalf("want one JSON line, got %q", res.stdout)
			}
			got := lines[0]
			if got["input"] != "a.png" || got["output"] != tc.output || got["content_type"] != tc.contentType {
				t.Errorf("--json: %v", got)
			}
			facts := map[string]any{}
			for k, v := range got {
				switch k {
				case "input", "output", "content_type", "width", "height", "quality", "bytes":
				default:
					facts[k] = v
				}
			}
			want := tc.facts
			if want == nil {
				want = map[string]any{}
			}
			if !reflect.DeepEqual(facts, want) {
				t.Errorf("--json reports %v, want %v", facts, want)
			}
		})
	}

	// An SVG goes down a pipe like any other result.
	s.api.reset()
	if res := s.ok("vectorize", "-o", "-", "a.png"); res.stdout != svgImage {
		t.Errorf("vectorize -o - wrote %q", res.stdout)
	}
	// A result of an earlier run beside its source is not sent again.
	s.image("pics/x.png")
	s.image("pics/y.png")
	s.ok("blur-faces", "pics")
	s.api.reset()
	s.ok("blur-faces", "--force", "pics")
	if got := len(s.api.all()); got != 2 { // x.png and y.png, not x-nofaces.png
		t.Errorf("the second run sent %d files, want 2", got)
	}
}

// afterSize is what a file's line says after the type and the size: what
// the tool found.
func afterSize(t *testing.T, line, contentType string) string {
	t.Helper()
	_, rest, ok := strings.Cut(line, "  "+contentType+"  ")
	if !ok {
		t.Fatalf("the line does not give the type %s: %q", contentType, line)
	}
	if i := strings.Index(rest, "  "); i >= 0 {
		return rest[i:]
	}
	return ""
}

// What a tool finds is not what the command exits with: nothing found, or
// something found wrong, is said on the line and the exit code stays 0, as
// with check and fix. Only a request the API refuses is a failure.
func TestFindingNothingIsNotAFailure(t *testing.T) {
	s := newSession(t)
	s.image("a.png")

	// answer makes an endpoint answer with a PNG and these headers.
	answer := func(path string, headers map[string]string) {
		s.api.handle("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			for name, value := range headers {
				w.Header().Set(name, value)
			}
			_, _ = w.Write(pngImage(8, 8))
		})
	}
	answer("/v1/blur-faces", map[string]string{"X-Faces": "0"})
	answer("/v1/restore-faces", map[string]string{"X-Faces": "0", "X-Faces-Skipped": "2"})
	answer("/v1/crop", map[string]string{"X-Subject-Found": "false"})
	answer("/v1/remove-background", map[string]string{"X-Subject-Found": "false"})
	answer("/v1/id-photo", map[string]string{"X-Id-Photo-Passed": "true", "X-Id-Photo-Findings": "", "X-Id-Photo-Head-Mm": "33"})

	tests := []struct {
		args  string
		human string
		facts map[string]any
	}{
		{"blur-faces a.png", "  no faces found", map[string]any{"faces": 0.0}},
		{"restore-faces a.png", "  faces restored: 0, skipped: 2", map[string]any{"faces": 0.0, "faces_skipped": 2.0}},
		{"crop --aspect 1:1 a.png", "  no subject found", map[string]any{"subject_found": false}},
		{"remove-background --width 40 --height 30 a.png", "  no subject found", map[string]any{"subject_found": false}},
		// No findings is said as that: the tool cannot approve a photo.
		{"id-photo --document us-passport a.png", "  head: 33 mm  no findings", map[string]any{"passed": true, "findings": []any{}, "head_mm": 33.0}},
	}
	for _, tc := range tests {
		t.Run(tc.args, func(t *testing.T) {
			res := s.run(append(strings.Fields(tc.args), "--out-dir", t.TempDir())...)
			if res.code != ExitOK {
				t.Fatalf("exit %d, want 0\n%s", res.code, res.stderr)
			}
			if !strings.HasSuffix(res.stdout, tc.human+"\n") {
				t.Errorf("the line does not end with %q: %q", tc.human, res.stdout)
			}
			if strings.Contains(res.stdout, "passes") {
				t.Errorf("an id photo is never said to pass: %q", res.stdout)
			}
			res = s.run(append(strings.Fields(tc.args), "--out-dir", t.TempDir(), "--json")...)
			if res.code != ExitOK {
				t.Fatalf("--json: exit %d, want 0\n%s", res.code, res.stderr)
			}
			got := jsonLines(t, res.stdout)[0]
			for k, want := range tc.facts {
				if !reflect.DeepEqual(got[k], want) {
					t.Errorf("--json: %s is %v, want %v", k, got[k], want)
				}
			}
		})
	}

	// The API refusing the photo is a failure, and writes nothing.
	refused := []struct {
		args    string
		path    string
		status  int
		code    string
		message string
	}{
		{"select --point 1,1 a.png", "/v1/select", 422, "unprocessable", "nothing was found at the points"},
		{"select --point 900,900 a.png", "/v1/select", 400, "bad_request", "the point 900,900 is outside the photo"},
		{"id-photo --document us-passport a.png", "/v1/id-photo", 422, "unprocessable", "no face was found"},
		{"denoise a.png", "/v1/denoise", 413, "payload_too_large", "photos up to 12 megapixels"},
		{"caption a.png", "/v1/caption", 503, "model_unavailable", "caption is not available on this server"},
	}
	for _, tc := range refused {
		t.Run(tc.args+" refused", func(t *testing.T) {
			s.api.handle("POST "+tc.path, func(w http.ResponseWriter, r *http.Request) { apiError(w, tc.status, tc.code, tc.message) })
			dir := t.TempDir()
			args := append(strings.Fields(tc.args), "--retries", "0")
			if imageEndpoints[tc.path] {
				args = append(args, "--out-dir", dir)
			}
			res := s.run(args...)
			if res.code != ExitFailed {
				t.Errorf("exit %d, want %d", res.code, ExitFailed)
			}
			if !strings.Contains(res.stderr, tc.message) || !strings.Contains(res.stderr, tc.code) {
				t.Errorf("stderr does not give the API's reason: %q", res.stderr)
			}
			if res.stdout != "" {
				t.Errorf("standard output: %q", res.stdout)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("a file was written for a refused request: %v", entries)
			}
		})
	}
}

// A file a flag names (a logo, a background) that is not there fails once,
// before anything is sent.
func TestFilesNamedByFlags(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.image("b.png")
	s.write("logo.png", []byte("the logo"))
	s.write("sky.png", []byte("the sky"))
	if err := os.Mkdir("dir", 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		args string
		code int
		says string
	}{
		{"watermark --logo missing.png a.png b.png", ExitFailed, "--logo missing.png"},
		{"watermark --logo dir a.png", ExitUsage, "--logo dir is a directory"},
		{"remove-background --background-image missing.png a.png", ExitFailed, "--background-image missing.png"},
		{"remove-background --background-image dir a.png", ExitUsage, "is a directory"},
	} {
		res := s.run(strings.Fields(tc.args)...)
		if res.code != tc.code || !strings.Contains(res.stderr, tc.says) {
			t.Errorf("photos %s: exit %d, want %d; stderr %q", tc.args, res.code, tc.code, res.stderr)
		}
		if n := strings.Count(res.stderr, "photos:"); n != 1 {
			t.Errorf("photos %s: %d errors, want one for the flag, not one per image:\n%s", tc.args, n, res.stderr)
		}
	}
	if n := len(s.api.all()); n != 0 {
		t.Fatalf("%d requests were made", n)
	}

	// What is uploaded is the file's bytes, under the part the API names.
	s.ok("watermark", "--logo", "logo.png", "-o", "marked.png", "a.png")
	if got := string(s.api.only().Files["watermark"][0].Data); got != "the logo" {
		t.Errorf("watermark part: %q", got)
	}
	s.api.reset()
	s.ok("remove-background", "--background-image", "sky.png", "-o", "staged.png", "a.png")
	if got := string(s.api.only().Files["background"][0].Data); got != "the sky" {
		t.Errorf("background part: %q", got)
	}
}

// download reports for a job's items what the matching command reports,
// and names an SVG and a data result by what they are.
func TestJobsDownloadOfTheEditingTools(t *testing.T) {
	s := newSession(t)

	tests := []struct {
		operation   string
		contentType string
		headers     map[string]string
		output      string
		human       string
		facts       map[string]any
	}{
		{"vectorize", "image/svg+xml", map[string]string{"X-Paths": "12"}, "shoe.svg", "  paths: 12", map[string]any{"paths": 12.0}},
		{"blur-faces", "image/png", map[string]string{"X-Faces": "4"}, "shoe.png", "  faces hidden: 4", map[string]any{"faces": 4.0}},
		{"restore-faces", "image/png", map[string]string{"X-Faces": "1", "X-Faces-Skipped": "2"}, "shoe.png",
			"  faces restored: 1, skipped: 2", map[string]any{"faces": 1.0, "faces_skipped": 2.0}},
		{"select", "image/png", map[string]string{"X-Select-Score": "0.871"}, "shoe.png", "  score: 0.87", map[string]any{"score": 0.871}},
		{"crop", "image/png", map[string]string{"X-Subject-Found": "false"}, "shoe.png", "  no subject found", map[string]any{"subject_found": false}},
		{"id-photo", "image/jpeg", map[string]string{"X-Id-Photo-Passed": "false", "X-Id-Photo-Findings": "head_tilted", "X-Id-Photo-Head-Mm": "31.5"},
			"shoe.jpg", "  head: 31.5 mm  findings: head_tilted", map[string]any{"passed": false, "findings": []any{"head_tilted"}, "head_mm": 31.5}},
		// An engine that does not keep the headers on job items: the file
		// is saved, and nothing is made up about it.
		{"blur-faces", "image/png", nil, "shoe.png", "", map[string]any{}},
		// Data results are JSON files.
		{"caption", "application/json", nil, "shoe.json", "", map[string]any{}},
		{"palette", "application/json", nil, "shoe.json", "", map[string]any{}},
		{"faces", "application/json", nil, "shoe.json", "", map[string]any{}},
	}
	for n, tc := range tests {
		t.Run(fmt.Sprintf("%s %d", tc.operation, n), func(t *testing.T) {
			id := fmt.Sprintf("job_%d", n)
			s.api.handle("GET /v1/jobs/"+id, func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, 200, fmt.Sprintf(`{"id":%q,"operation":%q,"params":"","status":"completed","created_at":1,"finished_at":2,`+
					`"counts":{"total":1,"done":1,"failed":0,"pending":0},"items":[{"input":"shoe.jpg","status":"done","content_type":%q,"bytes":6}]}`,
					id, tc.operation, tc.contentType))
			})
			s.api.handle("GET /v1/jobs/"+id+"/items/0", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				for name, value := range tc.headers {
					w.Header().Set(name, value)
				}
				fmt.Fprint(w, "result")
			})

			dir := filepath.Join("got", id)
			res := s.ok("jobs", "download", id, "--out-dir", dir)
			want := filepath.Join(dir, tc.output)
			if data, err := os.ReadFile(want); err != nil || string(data) != "result" {
				t.Fatalf("%s: %q, %v; files: %v", want, data, err, s.files())
			}
			line := strings.TrimSuffix(res.stdout, "\n")
			if !strings.HasPrefix(line, "shoe.jpg -> "+want+"  ") {
				t.Errorf("the line does not name %s: %q", want, res.stdout)
			}
			if after := afterSize(t, line, tc.contentType); after != tc.human {
				t.Errorf("the line says %q after the size, want %q", after, tc.human)
			}

			res = s.ok("jobs", "download", id, "--out-dir", dir, "--force", "--json")
			got := jsonLines(t, res.stdout)[0]
			facts := map[string]any{}
			for k, v := range got {
				switch k {
				case "item", "input", "output", "content_type", "width", "height", "quality", "bytes":
				default:
					facts[k] = v
				}
			}
			if !reflect.DeepEqual(facts, tc.facts) {
				t.Errorf("--json reports %v, want %v", facts, tc.facts)
			}
		})
	}
}

// jobs create takes every operation the editing tools added, under the
// name its command has.
func TestJobsCreateTakesTheNewOperations(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.api.handle("POST /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, `{"id":"job_1","operation":"x","params":"","status":"queued","created_at":1,"counts":{"total":1,"pending":1},"items":[]}`)
	})
	root := Root()
	// The words of the help: names, with their dashes.
	named := map[string]bool{}
	for _, word := range strings.FieldsFunc(s.ok("jobs", "create", "--help").stdout, func(r rune) bool {
		return r != '-' && !unicode.IsLetter(r)
	}) {
		named[word] = true
	}
	for _, operation := range []string{
		"enhance", "crop", "palette", "watermark", "expand", "vectorize", "faces", "blur-faces",
		"id-photo", "denoise", "colorize", "select", "restore-faces", "caption",
	} {
		if root.lookup(operation) == nil {
			t.Errorf("%s is an operation and not a command", operation)
		}
		s.api.reset()
		if res := s.ok("jobs", "create", operation, "a.png"); res.stdout != "job_1\n" {
			t.Errorf("jobs create %s: %q", operation, res.stdout)
		}
		if got := s.api.only().Fields["operation"]; !reflect.DeepEqual(got, []string{operation}) {
			t.Errorf("jobs create %s sent the operation %v", operation, got)
		}
		if !named[operation] {
			t.Errorf("the help of jobs create does not name %s", operation)
		}
	}

	// With URLs the job is JSON, where an object cannot say a parameter
	// twice: the points still arrive, both of them.
	s.api.reset()
	s.ok("jobs", "create", "select", "--param", "point=4,3", "--param", "point=5,2", "--param", "pick=largest", "https://example.com/1.jpg")
	want := map[string]any{
		"operation": "select",
		"params":    "pick=largest&point=4%2C3&point=5%2C2",
		"inputs":    []any{"https://example.com/1.jpg"},
	}
	if got := s.api.only().JSON; !reflect.DeepEqual(got, want) {
		t.Errorf("body: %v, want %v", got, want)
	}
}

// The flags of options whose zero is a value of its own leave the option
// alone unless they are given.
func TestOptionalFlagValues(t *testing.T) {
	var (
		n *int
		f *float64
		b *bool
	)
	if got := (optionalInt{&n}).String() + (optionalFloat{&f}).String() + (optionalBool{&b}).String(); got != "" {
		t.Errorf("flags not given have a default: %q", got)
	}
	// The flag package asks a zero value of the type for its text.
	if got := (optionalInt{}).String() + (optionalFloat{}).String() + (optionalBool{}).String(); got != "" {
		t.Errorf("zero values have a text: %q", got)
	}
	if err := (optionalInt{&n}).Set("0"); err != nil || n == nil || *n != 0 {
		t.Errorf("optionalInt 0: %v, %v", n, err)
	}
	if err := (optionalFloat{&f}).Set("0.5"); err != nil || f == nil || *f != 0.5 {
		t.Errorf("optionalFloat 0.5: %v, %v", f, err)
	}
	if err := (optionalBool{&b}).Set("false"); err != nil || b == nil || *b {
		t.Errorf("optionalBool false: %v, %v", b, err)
	}
	for _, bad := range []string{"", "x", "1.5"} {
		if err := (optionalInt{&n}).Set(bad); err == nil {
			t.Errorf("optionalInt took %q", bad)
		}
	}
	for _, bad := range []string{"", "x", "NaN", "Inf", "1e999"} {
		if err := (optionalFloat{&f}).Set(bad); err == nil {
			t.Errorf("optionalFloat took %q", bad)
		}
	}
	if err := (optionalBool{&b}).Set("maybe"); err == nil {
		t.Error("optionalBool took maybe")
	}
}
