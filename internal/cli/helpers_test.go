package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// testKey stands in for an API key. No output of any command may contain
// it: run checks that on every invocation.
const testKey = "pk_test_SECRET0123456789abcdefWXYZ"

// request is one request the fake API received.
type request struct {
	Method string
	Path   string
	Query  url.Values
	Auth   string
	Agent  string
	// Fields are the text parts of a multipart body; Files its file parts,
	// by field name, in order.
	Fields map[string][]string
	Files  map[string][]upload
	// JSON is a JSON body.
	JSON map[string]any
}

type upload struct {
	Name string
	Data []byte
}

// fakeAPI is an in-process stand-in for the Photos.sh API.
type fakeAPI struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	requests []request
	// handlers answer "METHOD /path"; anything else gets the default for
	// its kind of endpoint.
	handlers map[string]http.HandlerFunc
}

// pngImage is a real PNG of the given size.
func pngImage(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 5), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// imageEndpoints are the endpoints that answer with an image.
var imageEndpoints = map[string]bool{
	"/v1/remove-background": true, "/v1/resize": true, "/v1/upscale": true, "/v1/erase": true,
	"/v1/remove-text": true, "/v1/run": true, "/v1/fix": true,
	"/v1/enhance": true, "/v1/crop": true, "/v1/watermark": true, "/v1/expand": true,
	"/v1/vectorize": true, "/v1/blur-faces": true, "/v1/id-photo": true, "/v1/denoise": true,
	"/v1/colorize": true, "/v1/select": true, "/v1/restore-faces": true,
}

// svgImage is what the fake vectorize answers with.
const svgImage = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 48"><path d="M0 0h64v48H0z"/></svg>`

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t, handlers: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// handle sets the answer to "METHOD /path".
func (f *fakeAPI) handle(route string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[route] = h
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	rec := request{
		Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(),
		Auth: r.Header.Get("Authorization"), Agent: r.Header.Get("User-Agent"),
		Fields: map[string][]string{}, Files: map[string][]upload{},
	}
	// The body is read here for the record, and put back for the handler.
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	media, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch media {
	case "multipart/form-data":
		mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(part)
			if part.FileName() != "" {
				rec.Files[part.FormName()] = append(rec.Files[part.FormName()], upload{Name: part.FileName(), Data: data})
			} else {
				rec.Fields[part.FormName()] = append(rec.Fields[part.FormName()], string(data))
			}
		}
	case "application/json":
		_ = json.Unmarshal(body, &rec.JSON)
	}

	f.mu.Lock()
	f.requests = append(f.requests, rec)
	h := f.handlers[r.Method+" "+r.URL.Path]
	f.mu.Unlock()

	switch {
	case h != nil:
		h(w, r)
	case r.URL.Path == "/health":
		writeJSON(w, 200, `{"status":"ok"}`)
	case r.Header.Get("Authorization") != "Bearer "+testKey:
		apiError(w, 401, "unauthorized", "invalid API key")
	case imageEndpoints[r.URL.Path]:
		answerImage(w, r)
	case r.URL.Path == "/v1/account":
		writeJSON(w, 200, `{"account":"acme","key":"key_1","credits":4200,"prices":{"remove-background":10,"resize":1},"future":true}`)
	default:
		apiError(w, 404, "not_found", "no such thing")
	}
}

// answerImage answers like an image endpoint: a PNG (or, with ?format=, a
// file claiming to be that format) of ?width= × ?height= pixels (default
// 64 × 48), with the headers of the operation. An id photo is a JPEG unless
// ?format= says otherwise, and vectorize answers with an SVG.
func answerImage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	width, height := 64, 48
	fmt.Sscan(q.Get("width"), &width)
	fmt.Sscan(q.Get("height"), &height)
	contentType := "image/png"
	format := q.Get("format")
	if format == "" && r.URL.Path == "/v1/id-photo" {
		format = "jpg"
	}
	switch format {
	case "jpg":
		contentType = "image/jpeg"
		w.Header().Set("X-Image-Quality", "85")
	case "webp":
		contentType = "image/webp"
		w.Header().Set("X-Image-Quality", "80")
	case "avif":
		contentType = "image/avif"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Image-Width", fmt.Sprint(width))
	w.Header().Set("X-Image-Height", fmt.Sprint(height))
	w.Header().Set("Server-Timing", "decode;dur=0.1")
	switch r.URL.Path {
	case "/v1/remove-text":
		w.Header().Set("X-Text-Removed", "3")
		w.Header().Set("X-Text-Kept", "1")
	case "/v1/fix":
		w.Header().Set("X-Marketplace-Passed", "false")
		w.Header().Set("X-Marketplace-Findings", "too_small,text_overlay")
	case "/v1/crop":
		w.Header().Set("X-Subject-Found", "true")
	case "/v1/blur-faces":
		w.Header().Set("X-Faces", "2")
	case "/v1/restore-faces":
		w.Header().Set("X-Faces", "3")
		w.Header().Set("X-Faces-Skipped", "1")
	case "/v1/select":
		w.Header().Set("X-Select-Score", "0.934")
	case "/v1/id-photo":
		w.Header().Set("X-Id-Photo-Passed", "false")
		w.Header().Set("X-Id-Photo-Findings", "low_resolution,background_not_plain")
		w.Header().Set("X-Id-Photo-Head-Mm", "34.2")
	case "/v1/vectorize":
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("X-Paths", "23")
		_, _ = io.WriteString(w, svgImage)
		return
	}
	_, _ = w.Write(pngImage(width, height))
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func apiError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, fmt.Sprintf(`{"error":{"code":%q,"message":%q}}`, code, message))
}

// all returns the requests received so far.
func (f *fakeAPI) all() []request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]request(nil), f.requests...)
}

// only returns the single request received, failing the test otherwise.
func (f *fakeAPI) only() request {
	f.t.Helper()
	reqs := f.all()
	if len(reqs) != 1 {
		f.t.Fatalf("got %d requests, want 1: %+v", len(reqs), paths(reqs))
	}
	return reqs[0]
}

func (f *fakeAPI) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
}

func paths(reqs []request) []string {
	out := make([]string, len(reqs))
	for i, r := range reqs {
		out[i] = r.Method + " " + r.Path + "?" + r.Query.Encode()
	}
	return out
}

// result is what one invocation of the CLI did.
type result struct {
	stdout string
	stderr string
	code   int
}

// session is a CLI pointed at a fake API, with its own configuration
// directory and working directory.
type session struct {
	t   *testing.T
	api *fakeAPI
	// dir is a scratch directory; the working directory during the test.
	dir string
	// config is the configuration directory (PHOTOS_CONFIG_DIR).
	config string
}

// newSession starts a fake API and points the environment at it, with the
// test key in PHOTOS_API_KEY.
func newSession(t *testing.T) *session {
	t.Helper()
	s := &session{t: t, api: newFakeAPI(t), dir: t.TempDir(), config: filepath.Join(t.TempDir(), "config")}
	t.Setenv("PHOTOS_CONFIG_DIR", s.config)
	t.Setenv("PHOTOS_BASE_URL", s.api.srv.URL)
	t.Setenv("PHOTOS_API_KEY", testKey)
	t.Setenv("NO_COLOR", "1")

	// Relative paths in the tests are relative to the scratch directory.
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(s.dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return s
}

// run runs the CLI with nothing on standard input.
func (s *session) run(args ...string) result {
	s.t.Helper()
	return s.runCtx(context.Background(), "", args...)
}

// runIn runs the CLI with stdin on standard input.
func (s *session) runIn(stdin string, args ...string) result {
	s.t.Helper()
	return s.runCtx(context.Background(), stdin, args...)
}

func (s *session) runCtx(ctx context.Context, stdin string, args ...string) result {
	s.t.Helper()
	var out, errOut bytes.Buffer
	env := &Env{Version: "test", Out: &out, Err: &errOut, In: strings.NewReader(stdin)}
	code := Main(ctx, env, args)
	res := result{stdout: out.String(), stderr: errOut.String(), code: code}
	// The rule that holds for every command: the key is never printed.
	if strings.Contains(res.stdout, testKey) || strings.Contains(res.stderr, testKey) {
		s.t.Fatalf("photos %s printed the API key:\nstdout: %s\nstderr: %s", strings.Join(args, " "), res.stdout, res.stderr)
	}
	return res
}

// ok runs the CLI and fails the test unless it exits 0.
func (s *session) ok(args ...string) result {
	s.t.Helper()
	res := s.run(args...)
	if res.code != 0 {
		s.t.Fatalf("photos %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), res.code, res.stdout, res.stderr)
	}
	return res
}

// write makes a file in the scratch directory; name uses slashes.
func (s *session) write(name string, data []byte) string {
	s.t.Helper()
	path := filepath.FromSlash(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		s.t.Fatal(err)
	}
	return path
}

// image makes a small PNG in the scratch directory.
func (s *session) image(name string) string {
	return s.write(name, pngImage(8, 8))
}

// files lists every file below the scratch directory, with slashes, sorted.
func (s *session) files() []string {
	s.t.Helper()
	var names []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		s.t.Fatal(err)
	}
	return names
}

// jsonLines decodes standard output as JSON Lines.
func jsonLines(t *testing.T, out string) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("standard output is not JSON Lines: %q: %v", line, err)
		}
		lines = append(lines, v)
	}
	return lines
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
