package cli

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Each class of error has its exit code.
func TestExitCodes(t *testing.T) {
	s := newSession(t)
	s.image("a.png")

	answer := func(status int, code, message string) {
		s.api.handle("POST /v1/resize", func(w http.ResponseWriter, r *http.Request) {
			if status == 503 {
				w.Header().Set("Retry-After", "0.01")
			}
			if code == "" {
				w.WriteHeader(status)
				fmt.Fprint(w, "<html>bad gateway</html>")
				return
			}
			apiError(w, status, code, message)
		})
	}
	tests := []struct {
		status  int
		code    string
		message string
		want    int
		stderr  string
	}{
		{400, "bad_request", "cannot decode the image", ExitFailed, "cannot decode the image (bad_request, HTTP 400)"},
		{413, "payload_too_large", "the image is too large", ExitFailed, "payload_too_large"},
		{415, "unsupported_format", "HEIC is not supported here", ExitFailed, "unsupported_format"},
		{422, "unprocessable", "cannot fit under max_bytes", ExitFailed, "unprocessable"},
		{500, "internal", "something broke", ExitFailed, "internal"},
		{401, "unauthorized", "invalid API key", ExitAuth, "photos login"},
		{402, "payment_required", "out of credits", ExitPayment, "Out of credits"},
		{403, "account_suspended", "the account is suspended", ExitPayment, "suspended"},
		{503, "busy", "all workers are busy", ExitUnavailable, "busy"},
		{503, "model_unavailable", "upscale is not available on this server", ExitFailed, "model_unavailable"},
		{502, "", "", ExitUnavailable, "HTTP 502"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d %s", tc.status, tc.code), func(t *testing.T) {
			answer(tc.status, tc.code, tc.message)
			res := s.run("resize", "--width", "10", "--retries", "1", "a.png")
			if res.code != tc.want {
				t.Errorf("exit %d, want %d\n%s", res.code, tc.want, res.stderr)
			}
			if !strings.Contains(res.stderr, tc.stderr) {
				t.Errorf("stderr %q does not say %q", res.stderr, tc.stderr)
			}
			if res.stdout != "" {
				t.Errorf("an error printed on standard output: %q", res.stdout)
			}
			if exists("a-resized.png") {
				t.Error("a file was written for a failed request")
			}
		})
	}

	// The same classes on a command that is one request.
	s.api.handle("GET /v1/account", func(w http.ResponseWriter, r *http.Request) { apiError(w, 402, "payment_required", "out of credits") })
	if res := s.run("account", "--json"); res.code != ExitPayment || res.stdout != "" {
		t.Errorf("account, out of credits: exit %d, stdout %q", res.code, res.stdout)
	}

	// Nothing listening: unreachable.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := "http://" + l.Addr().String()
	l.Close()
	res := s.run("account", "--base-url", dead, "--retries", "0")
	if res.code != ExitUnavailable || !strings.Contains(res.stderr, "cannot reach the API") {
		t.Errorf("unreachable: exit %d, stderr %q", res.code, res.stderr)
	}

	// No key at all: not signed in, and nothing is sent.
	t.Setenv("PHOTOS_API_KEY", "")
	s.api.reset()
	res = s.run("resize", "--width", "10", "a.png")
	if res.code != ExitAuth || !strings.Contains(res.stderr, "not signed in") || !strings.Contains(res.stderr, "photos login") {
		t.Errorf("no key: exit %d, stderr %q", res.code, res.stderr)
	}
	if n := len(s.api.all()); n != 0 {
		t.Errorf("%d requests without a key", n)
	}
	// Health needs none.
	if res := s.run("health"); res.code != 0 || res.stdout != "ok\n" {
		t.Errorf("health without a key: exit %d, %q %q", res.code, res.stdout, res.stderr)
	}
}

// Wrong commands are usage errors (exit 2) that name the mistake and, where
// one is close, what was meant; nothing is sent.
func TestUsageErrors(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	tests := []struct {
		args string
		says string
	}{
		{"resise a.png", `unknown command "resise" for "photos" (did you mean resize?)`},
		{"remove-bg a.png", "did you mean remove-background?"},
		{"jobs creat resize a.png", `unknown command "creat" for "photos jobs" (did you mean create?)`},
		{"frobnicate", `unknown command "frobnicate"`},
		{"resize --widht 10 a.png", "unknown flag --widht (did you mean --width?)"},
		{"resize --max_bytes 10 a.png", "unknown flag --max_bytes (did you mean --max-bytes?)"},
		{"account --jsno", "unknown flag --jsno (did you mean --json?)"},
		{"--jsno account", "unknown flag --jsno (did you mean --json?)"},
		{"resize --width ten a.png", `invalid value "ten" for flag --width`},
		{"resize --width", "flag needs an argument: --width"},
		{"resize --trim-tolerance many a.png", `invalid value "many" for flag --trim-tolerance: not a whole number`},
		{"crop --padding wide a.png", `invalid value "wide" for flag --padding: not a number`},
		{"enhance --levels=maybe a.png", `invalid value "maybe" for flag --levels: want true or false`},
		{"remove-background --crop=maybe a.png", `invalid value "maybe" for flag --crop`},
		{"watermark a.png", "--text"},
		{"watermark --text Acme --logo a.png a.png", "one of them"},
		{"expand a.png", "--aspect"},
		{"expand --aspect 1:1 --left 10 a.png", "one or the other"},
		{"expand --left -10 a.png", "cannot be negative"},
		{"select a.png", "--point"},
		{"select --point 4 a.png", "want x,y"},
		{"select --point a,b a.png", "two numbers"},
		{"select --point -4,3 a.png", "two numbers"},
		{"select --point 4,3 --exclude 1;2 a.png", `--exclude "1;2": want x,y`},
		{"select --box 1,2,3 a.png", `--box "1,2,3": want x,y,width,height`},
		{"select --box 1,2,0,4 a.png", "more than zero"},
		{"id-photo a.png", "--document"},
		{"id-photo --document us-passport --width-mm 35 a.png", "not both"},
		{"id-photo --width-mm 35 --height-mm 45 a.png", "all four sizes"},
		{"id-photos extra", "no arguments"},
		{"palette", "one image"},
		{"faces a.png a.png", "one image"},
		{"caption", "one image"},
		{"blur-face a.png", "did you mean blur-faces?"},
		{"vectorise a.png", "did you mean vectorize?"},
		{"colourize a.png", "did you mean colorize?"},
		{"fix a.png", "--marketplace"},
		{"run a.png", "--preset"},
		{"run --preset x --recipe y a.png", "one of them"},
		{"erase a.png", "--rect"},
		{"erase --rect 1,2,3 a.png", "x,y,width,height"},
		{"erase --rect a,b,c,d a.png", "four whole numbers"},
		{"check", "one image"},
		{"check a.png a.png", "one image"},
		{"label", "one to 8 images"},
		{"label a.png a.png a.png a.png a.png a.png a.png a.png a.png", "at most 8"},
		{"jobs create", "operation"},
		{"jobs create resize", "no inputs"},
		{"jobs create resize --param width a.png", "name=value"},
		{"jobs get", "one job id"},
		{"jobs wait", "job ids"},
		{"jobs delete", "id"},
		{"jobs download", "job ids"},
		{"jobs create resize --max-upload-mb 0 a.png", "at least 1"},
		{"presets one two", "one preset name at most"},
		{"completion", "bash, zsh or fish"},
		{"completion powershell", "bash, zsh or fish"},
		{"webhook verify", "--secret-env"},
		{"webhook verify --secret-env X", "--signature"},
		{"help nothing", `unknown command "nothing"`},
	}
	for _, tc := range tests {
		t.Run(tc.args, func(t *testing.T) {
			res := s.run(strings.Fields(tc.args)...)
			if res.code != ExitUsage {
				t.Errorf("exit %d, want %d", res.code, ExitUsage)
			}
			if !strings.Contains(res.stderr, tc.says) {
				t.Errorf("stderr does not say %q:\n%s", tc.says, res.stderr)
			}
			if !strings.Contains(res.stderr, "photos help") {
				t.Errorf("stderr does not point at the help:\n%s", res.stderr)
			}
			if res.stdout != "" {
				t.Errorf("standard output: %q", res.stdout)
			}
		})
	}
	if n := len(s.api.all()); n != 0 {
		t.Errorf("%d requests were made by commands that were wrong", n)
	}
}

// Every command has help with a real example, reachable as `photos help
// <command>` and as `<command> --help`, on standard output, without a key
// and without the network.
func TestHelpForEveryCommand(t *testing.T) {
	s := newSession(t)
	t.Setenv("PHOTOS_API_KEY", "")

	var walk func(cmd *Command, path []string)
	walk = func(cmd *Command, path []string) {
		name := strings.Join(path, " ")
		if len(path) > 0 {
			t.Run(name, func(t *testing.T) {
				if cmd.Summary == "" {
					t.Error("no summary")
				}
				if (cmd.Run == nil) == (len(cmd.Children) == 0) {
					t.Error("a command has either Run or Children")
				}
				if cmd.Run != nil && len(cmd.Examples) == 0 {
					t.Error("no example")
				}
				for _, example := range cmd.Examples {
					if !strings.Contains(example, "photos "+name) && !strings.Contains(example, "photos "+path[0]) {
						t.Errorf("the example does not use the command: %s", example)
					}
				}
				byHelp := s.ok(append([]string{"help"}, path...)...)
				byFlag := s.ok(append(append([]string{}, path...), "--help")...)
				if byHelp.stdout != byFlag.stdout {
					t.Error("`help <command>` and `<command> --help` differ")
				}
				for _, want := range append([]string{"Usage:\n  photos " + name, "--json"}, cmd.Examples...) {
					if !strings.Contains(byHelp.stdout, want) {
						t.Errorf("help does not contain %q:\n%s", want, byHelp.stdout)
					}
				}
				for _, f := range commandFlags(cmd) {
					if !strings.Contains(byHelp.stdout, "--"+f.Name) {
						t.Errorf("help does not list --%s", f.Name)
					}
					if f.Usage == "" {
						t.Errorf("--%s has no description", f.Name)
					}
				}
			})
		}
		for _, child := range cmd.Children {
			walk(child, append(append([]string{}, path...), child.Name))
		}
	}
	walk(Root(), nil)

	// An alias reaches the same command.
	if a, b := s.ok("bg-remove", "--help"), s.ok("remove-background", "--help"); a.stdout != b.stdout {
		t.Error("the alias bg-remove is not remove-background")
	}
	// No arguments at all: the root's help.
	if res := s.ok(); !strings.Contains(res.stdout, "remove-background") || !strings.Contains(res.stdout, "Exit codes") {
		t.Errorf("photos alone:\n%s", res.stdout)
	}
	if res := s.ok("--version"); res.stdout != "test\n" {
		t.Errorf("--version: %q", res.stdout)
	}
	if res := s.ok("version"); res.stdout != "test\n" {
		t.Errorf("version: %q", res.stdout)
	}
	if n := len(s.api.all()); n != 0 {
		t.Errorf("help made %d requests", n)
	}
}

// Data commands print the API's JSON as it came with --json, unknown fields
// and all, and something readable without.
func TestDataCommands(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.image("b.png")

	bodies := map[string]string{
		"POST /v1/check": `{"metrics":{"width":320,"height":240,"sharpness":0.82,"noise":1.5,"brightness":0.7,"border":{"mean":[250,250,250],"white":0.9}},` +
			`"findings":[{"code":"too_small","severity":"warning","message":"smaller than recommended","detail":"320 px"}],` +
			`"marketplace":{"marketplace":"amazon","passed":false,"findings":[{"code":"text_overlay","severity":"error","message":"text on the image"}],"not_checked":["subject_fill"]},"new_field":{"x":1}}`,
		"POST /v1/barcodes":      `{"barcodes":[{"format":"EAN-13","value":"5901234123457","gs1":"5901234123457"},{"format":"QR","value":"https://example.com","gs1":null}],"variant":"plain","new_field":1}`,
		"POST /v1/ocr":           `{"lines":[{"text":"FRESH MILK","score":0.98,"box":[[1,2],[3,2],[3,4],[1,4]]},{"text":"1 L","score":0.91,"box":[]}],"ms":{"ocr":12}}`,
		"POST /v1/label":         `{"label":{"headline":["FRESH MILK"],"gtin":"5901234123457","net_quantity":{"value":1,"unit":"l","ml":1000},"allergens":["milk"],"brand_new":"kept"},"barcodes":[],"text":["FRESH MILK","1 L"]}`,
		"POST /v1/identify":      `{"catalogue":"whisky","candidates":[{"product_id":"p-17","title":"Glen Example 12","score":0.93,"evidence":["barcode","age"]}],"label":{"ages":[12]},"barcodes":["5000000000000"]}`,
		"GET /v1/presets":        `[{"name":"marketplace","description":"White, square, JPEG."},{"name":"web","description":"WebP under 300 KB."}]`,
		"GET /v1/marketplaces":   `[{"name":"amazon","title":"Amazon (main image)","checked":"2026-11-06","sources":["https://example.com"],"rules":[{"check":"min_side","severity":"error"}],"fix":{"format":"jpg"},"extra":true}]`,
		"GET /v1/webhook-secret": `{"secret":"whsec_abc","header":"Photos-Signature","scheme":"t=<unix seconds>,v1=<hex>"}`,
		"POST /v1/palette":       `{"width":640,"height":480,"colors":[{"hex":"#c81018","rgb":[200,16,24],"share":0.625},{"hex":"#ffffff","rgb":[255,255,255],"share":0.375}],"later":1}`,
		"POST /v1/caption":       `{"caption":"A red shoe on a white table.","detail":"short","width":640,"height":480,"later":1}`,
		"POST /v1/faces": `{"width":512,"height":512,"faces":[{"box":{"x":180.4,"y":96.6,"width":150.2,"height":201.7},` +
			`"landmarks":{"right_eye":[220,170],"left_eye":[290,172],"nose":[255,210],"mouth_right":[228,250],"mouth_left":[284,252]},"score":0.957,"roll":-1.64}],"later":1}`,
		"GET /v1/id-photos": `[{"name":"us-passport","title":"United States passport","checked":"2026-11-12","sources":["https://example.com/us"],"notes":["neutral expression"],` +
			`"width_mm":51,"height_mm":51,"head_min_mm":25,"head_max_mm":35,"eyes_from_bottom_mm":[28,35],"background":"white","alteration":"Submit the original, unedited photo","extra":true},` +
			`{"name":"uk-passport","title":"United Kingdom passport","checked":"2026-11-12","sources":[],"notes":[],"width_mm":35,"height_mm":45,"head_min_mm":29,"head_max_mm":34,"background":"#f0f0f0"}]`,
	}
	for route, body := range bodies {
		s.api.handle(route, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, body) })
	}

	tests := []struct {
		args  string
		route string
		// human is what a person reads, in order.
		human []string
	}{
		{"check --marketplace amazon a.png", "POST /v1/check", []string{"320x240", "0.82", "too_small", "smaller than recommended (320 px)", "Marketplace amazon: does not pass", "text_overlay", "not checked: subject_fill"}},
		{"barcodes a.png", "POST /v1/barcodes", []string{"EAN-13  5901234123457\n", "QR      https://example.com\n"}},
		{"ocr a.png", "POST /v1/ocr", []string{"FRESH MILK\n1 L\n"}},
		{"label a.png b.png", "POST /v1/label", []string{"allergens     milk", "brand_new     kept", "gtin          5901234123457", "headline      FRESH MILK", "net_quantity  ml 1000; unit l; value 1"}},
		{"identify a.png", "POST /v1/identify", []string{"SCORE", "0.93   p-17     Glen Example 12  barcode, age"}},
		{"palette a.png", "POST /v1/palette", []string{"#c81018  62.5%  200,16,24\n", "#ffffff  37.5%  255,255,255\n"}},
		{"faces a.png", "POST /v1/faces", []string{"AT      SIZE     SCORE  ROLL\n", "180,97  150x202  0.96   -1.6\n"}},
		{"caption a.png", "POST /v1/caption", []string{"A red shoe on a white table.\n"}},
		{"id-photos", "GET /v1/id-photos", []string{
			"us-passport  United States passport   51x51 mm  head 25-35 mm  white    checked 2026-11-12\n",
			"uk-passport  United Kingdom passport  35x45 mm  head 29-34 mm  #f0f0f0  checked 2026-11-12\n"}},
		{"presets", "GET /v1/presets", []string{"marketplace  White, square, JPEG.\n", "web          WebP under 300 KB.\n"}},
		{"marketplaces", "GET /v1/marketplaces", []string{"amazon  Amazon (main image)  1 rules  checked 2026-11-06"}},
		{"account", "GET /v1/account", []string{"Account  acme", "Key      …WXYZ (from PHOTOS_API_KEY)", "Credits  4200", "remove-background  10", "resize             1"}},
		{"webhook secret", "GET /v1/webhook-secret", []string{"whsec_abc\n"}},
	}
	for _, tc := range tests {
		t.Run(tc.args, func(t *testing.T) {
			res := s.ok(strings.Fields(tc.args)...)
			rest := res.stdout
			for _, want := range tc.human {
				i := strings.Index(rest, want)
				if i < 0 {
					t.Fatalf("standard output does not have %q (in that order):\n%s", want, res.stdout)
				}
				rest = rest[i+len(want):]
			}

			res = s.ok(append(strings.Fields(tc.args), "--json")...)
			body, known := bodies[tc.route]
			if !known { // account: the fake's own answer
				body = `{"account":"acme","key":"key_1","credits":4200,"prices":{"remove-background":10,"resize":1},"future":true}`
			}
			var got, want any
			if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
				t.Fatalf("--json is not JSON: %q", res.stdout)
			}
			_ = json.Unmarshal([]byte(body), &want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("--json:\n%s\nwant the API's:\n%s", res.stdout, body)
			}
			if strings.Count(res.stdout, "\n") != 1 || res.stderr != "" {
				t.Errorf("--json should be one line and nothing else: %q / %q", res.stdout, res.stderr)
			}
			// Objects the library keeps raw come out byte for byte.
			if strings.HasPrefix(body, "{") && tc.args != "webhook secret" && res.stdout != body+"\n" {
				t.Errorf("--json is not the API's JSON as it came:\n%s", res.stdout)
			}
		})
	}

	// Health: the status, and exit 5 when it is not ok.
	if res := s.ok("health", "--json"); res.stdout != `{"status":"ok"}`+"\n" {
		t.Errorf("health --json: %q", res.stdout)
	}
	s.api.handle("GET /health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 503, `{"status":"loading"}`) })
	if res := s.run("health"); res.code != ExitUnavailable || res.stdout != "loading\n" {
		t.Errorf("health, loading: exit %d, %q", res.code, res.stdout)
	}
	if res := s.run("health", "--quiet"); res.code != ExitUnavailable || res.stdout != "" || res.stderr != "" {
		t.Errorf("health --quiet: exit %d, %q %q", res.code, res.stdout, res.stderr)
	}

	// Nothing found is said on standard error, and is not a failure.
	s.api.handle("POST /v1/faces", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, `{"width":8,"height":8,"faces":[]}`) })
	if res := s.ok("faces", "a.png"); res.stdout != "" || !strings.Contains(res.stderr, "No faces found.") {
		t.Errorf("faces, none found: %q %q", res.stdout, res.stderr)
	}
	if res := s.ok("faces", "--json", "a.png"); res.stdout != `{"width":8,"height":8,"faces":[]}`+"\n" || res.stderr != "" {
		t.Errorf("faces --json, none found: %q %q", res.stdout, res.stderr)
	}

	// A data command reads standard input too.
	s.api.reset()
	s.ok2(s.runIn("the bytes", "ocr", "-"))
	if got := string(s.api.only().Files["image"][0].Data); got != "the bytes" {
		t.Errorf("ocr - uploaded %q", got)
	}

	// Text from the API cannot drive the terminal.
	s.api.handle("POST /v1/ocr", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"lines":[{"text":"safe\u001b[2Jtext\u0007","score":1,"box":[]}]}`)
	})
	if res := s.ok("ocr", "a.png"); strings.ContainsAny(res.stdout, "\x1b\x07") {
		t.Errorf("control characters reached the terminal: %q", res.stdout)
	}
}

// login saves the key in a file only its owner can read, after checking it;
// logout removes it; the key is never shown and never an argument.
func TestLoginAndLogout(t *testing.T) {
	s := newSession(t)
	t.Setenv("PHOTOS_API_KEY", "")
	s.image("a.png")
	creds := filepath.Join(s.config, "credentials")

	// Not signed in yet.
	if res := s.run("account"); res.code != ExitAuth {
		t.Fatalf("before login: exit %d", res.code)
	}

	// A key the API refuses is not saved.
	res := s.runIn("pk_wrong_key_000000000000\n", "login")
	if res.code != ExitAuth || !strings.Contains(res.stderr, "nothing was saved") {
		t.Errorf("a refused key: exit %d, stderr %q", res.code, res.stderr)
	}
	if strings.Contains(res.stdout+res.stderr, "pk_wrong_key") {
		t.Errorf("the refused key was printed: %q", res.stderr)
	}
	if exists(creds) {
		t.Fatal("a refused key was saved")
	}

	// As an argument it is refused, and not repeated.
	s.api.reset()
	res = s.run("login", "pk_argument_key_0000000000")
	if res.code != ExitUsage || strings.Contains(res.stdout+res.stderr, "pk_argument") {
		t.Errorf("a key as an argument: exit %d, stderr %q", res.code, res.stderr)
	}
	if len(s.api.all()) != 0 || exists(creds) {
		t.Error("a key given as an argument was used")
	}
	if res := s.runIn("\n", "login"); res.code != ExitUsage {
		t.Errorf("an empty key: exit %d", res.code)
	}

	// The real thing, piped in.
	res = s.runIn(testKey+"\n", "login")
	if res.code != 0 {
		t.Fatalf("login: exit %d\n%s", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "Signed in as acme (key …WXYZ)") || !strings.Contains(res.stdout, creds) {
		t.Errorf("login said: %q", res.stdout)
	}
	req := s.api.only()
	if req.Path != "/v1/account" || req.Auth != "Bearer "+testKey {
		t.Errorf("login did not check the key with the account: %s", req.Path)
	}
	info, err := os.Stat(creds)
	if err != nil {
		t.Fatalf("no credentials file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("credentials file mode %o, want 600", mode)
		}
		if dir, _ := os.Stat(s.config); dir.Mode().Perm() != 0o700 {
			t.Errorf("configuration directory mode %o, want 700", dir.Mode().Perm())
		}
	}
	var saved map[string]string
	data, _ := os.ReadFile(creds)
	if err := json.Unmarshal(data, &saved); err != nil || saved["api_key"] != testKey {
		t.Errorf("the file does not hold the key")
	}
	// The API was not the default one, so the key is saved for it.
	if saved["base_url"] != s.api.srv.URL {
		t.Errorf("base_url %q, want %q", saved["base_url"], s.api.srv.URL)
	}
	if entries, _ := os.ReadDir(s.config); len(entries) != 1 {
		t.Errorf("left in the configuration directory: %v", entries)
	}

	// The saved key is used, with the API it was saved for.
	t.Setenv("PHOTOS_BASE_URL", "")
	s.api.reset()
	res = s.ok("account")
	if !strings.Contains(res.stdout, "…WXYZ (saved by photos login)") {
		t.Errorf("account: %q", res.stdout)
	}
	if got := s.api.only().Auth; got != "Bearer "+testKey {
		t.Error("the saved key was not sent")
	}
	s.ok("resize", "--width", "10", "a.png")

	// PHOTOS_API_KEY wins over it.
	t.Setenv("PHOTOS_API_KEY", "pk_env_key_00000000000000")
	t.Setenv("PHOTOS_BASE_URL", s.api.srv.URL)
	s.api.reset()
	s.run("account")
	if got := s.api.only().Auth; got != "Bearer pk_env_key_00000000000000" {
		t.Errorf("PHOTOS_API_KEY did not win: %q", got)
	}
	t.Setenv("PHOTOS_API_KEY", "")
	t.Setenv("PHOTOS_BASE_URL", "")

	// JSON for a script.
	res = s.runIn(testKey+"\n", "login", "--json", "--base-url", s.api.srv.URL)
	if got := jsonLines(t, res.stdout); len(got) != 1 || got[0]["account"] != "acme" || got[0]["key"] != "…WXYZ" {
		t.Errorf("login --json: %q", res.stdout)
	}

	// logout removes the file, and says so once.
	if res := s.ok("logout"); !strings.Contains(res.stdout, "Signed out") {
		t.Errorf("logout: %q", res.stdout)
	}
	if exists(creds) {
		t.Fatal("logout left the credentials file")
	}
	if res := s.ok("logout"); !strings.Contains(res.stdout, "No key was saved") {
		t.Errorf("second logout: %q", res.stdout)
	}
	if res := s.run("account", "--base-url", s.api.srv.URL); res.code != ExitAuth {
		t.Errorf("after logout: exit %d", res.code)
	}

	// A credentials file that is not valid does not leak into the error.
	if err := os.WriteFile(creds, []byte(`{"api_key": "`+testKey), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := s.run("account"); res.code != ExitFailed || !strings.Contains(res.stderr, "photos login") {
		t.Errorf("a broken credentials file: exit %d, %q", res.code, res.stderr)
	}
}

func jobJSON(id, status string, items ...string) string {
	done, failed, pending := 0, 0, 0
	for _, item := range items {
		switch {
		case strings.Contains(item, `"status":"done"`):
			done++
		case strings.Contains(item, `"status":"failed"`):
			failed++
		default:
			pending++
		}
	}
	return fmt.Sprintf(`{"id":%q,"operation":"resize","params":"width=64","status":%q,"created_at":1767225600,"finished_at":1767225660,`+
		`"counts":{"total":%d,"done":%d,"failed":%d,"pending":%d},"items":[%s],"later":"kept"}`,
		id, status, len(items), done, failed, pending, strings.Join(items, ","))
}

// Jobs: create with --wait polls until completed; wait, get and list show
// the job; download saves each finished item under its input's name.
func TestJobs(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.image("b.png")

	pendingItem := func(name string) string { return fmt.Sprintf(`{"input":%q,"status":"pending"}`, name) }
	doneItem := func(name, contentType string) string {
		return fmt.Sprintf(`{"input":%q,"status":"done","content_type":%q,"bytes":10,"headers":{"x-image-width":"64","x-image-height":"48"}}`, name, contentType)
	}
	failedItem := func(name string) string {
		return fmt.Sprintf(`{"input":%q,"status":"failed","error":{"code":"bad_request","message":"cannot decode the image"}}`, name)
	}

	// create --wait: queued, running, completed.
	var polls atomic.Int32
	s.api.handle("POST /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, jobJSON("job_1", "queued", pendingItem("a.png"), pendingItem("b.png")))
	})
	s.api.handle("GET /v1/jobs/job_1", func(w http.ResponseWriter, r *http.Request) {
		switch polls.Add(1) {
		case 1:
			writeJSON(w, 200, jobJSON("job_1", "running", doneItem("a.png", "image/webp"), pendingItem("b.png")))
		default:
			writeJSON(w, 200, jobJSON("job_1", "completed", doneItem("a.png", "image/webp"), doneItem("b.png", "image/webp")))
		}
	})
	res := s.ok("jobs", "create", "resize", "--param", "width=64", "--wait", "--interval", "5ms", "a.png", "b.png")
	if polls.Load() != 2 {
		t.Errorf("%d polls, want 2", polls.Load())
	}
	for _, want := range []string{"job_1: running, 1 of 2 done", "job_1: completed, 2 of 2 done"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("progress on standard error lacks %q:\n%s", want, res.stderr)
		}
	}
	for _, want := range []string{"Job        job_1", "completed: 2 done, 0 failed, 0 pending", "0  done    a.png  image/webp  10 B  64x48"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("the job as printed lacks %q:\n%s", want, res.stdout)
		}
	}

	// get --json is the API's JSON as it came.
	res = s.ok("jobs", "get", "job_1", "--json")
	if want := jobJSON("job_1", "completed", doneItem("a.png", "image/webp"), doneItem("b.png", "image/webp")) + "\n"; res.stdout != want {
		t.Errorf("jobs get --json:\n%s\nwant\n%s", res.stdout, want)
	}

	// wait: a failed item makes the exit code 1; a job that does not
	// finish in time too.
	s.api.handle("GET /v1/jobs/job_2", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, jobJSON("job_2", "completed", doneItem("a.png", "image/png"), failedItem("b.png")))
	})
	res = s.run("jobs", "wait", "job_2", "--interval", "5ms")
	if res.code != ExitFailed || !strings.Contains(res.stderr, "1 of 2 items failed") || !strings.Contains(res.stdout, "bad_request: cannot decode the image") {
		t.Errorf("wait with a failed item: exit %d\n%s\n%s", res.code, res.stdout, res.stderr)
	}
	s.api.handle("GET /v1/jobs/job_3", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, jobJSON("job_3", "running", pendingItem("a.png")))
	})
	res = s.run("jobs", "wait", "job_3", "--interval", "5ms", "--wait-timeout", "30ms")
	if res.code != ExitFailed || !strings.Contains(res.stderr, "still running after 30ms") {
		t.Errorf("wait past its timeout: exit %d, %q", res.code, res.stderr)
	}
	// No such job.
	if res := s.run("jobs", "wait", "job_none"); res.code != ExitFailed || !strings.Contains(res.stderr, "not_found") {
		t.Errorf("wait for no job: exit %d, %q", res.code, res.stderr)
	}

	// list.
	s.api.handle("GET /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `[{"id":"job_2","operation":"resize","status":"completed","created_at":1767225600,"finished_at":1767225660,"counts":{"total":2,"done":1,"failed":1,"pending":0}},`+
			`{"id":"job_1","operation":"ocr","status":"running","created_at":1767225000,"finished_at":null,"counts":{"total":2,"done":0,"failed":0,"pending":2}}]`)
	})
	res = s.ok("jobs", "list")
	for _, want := range []string{"ID", "job_2  resize     completed  1 done, 1 failed, 0 pending", "job_1  ocr        running"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("jobs list lacks %q:\n%s", want, res.stdout)
		}
	}
	var listed []map[string]any
	if err := json.Unmarshal([]byte(s.ok("jobs", "list", "--json").stdout), &listed); err != nil || len(listed) != 2 || listed[1]["id"] != "job_1" {
		t.Errorf("jobs list --json: %v %v", listed, err)
	}

	// delete, several at once, one of them unknown.
	s.api.handle("DELETE /v1/jobs/job_1", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	res = s.run("jobs", "delete", "job_1", "job_none")
	if res.code != ExitFailed || res.stdout != "Deleted job_1\n" || !strings.Contains(res.stderr, "job_none: no such thing") {
		t.Errorf("jobs delete: exit %d, %q, %q", res.code, res.stdout, res.stderr)
	}
}

// download names each result after its input, with the extension of what
// came, never replaces a file without --force, and reports the items that
// have no result.
func TestJobsDownload(t *testing.T) {
	s := newSession(t)
	items := []string{
		`{"input":"shoe.jpg","status":"done","content_type":"image/png","bytes":3}`,
		`{"input":"https://example.com/photos/shoe.png?size=large","url":"https://example.com/photos/shoe.png?size=large","status":"done","content_type":"image/webp","bytes":3}`,
		`{"input":"label.jpg","status":"done","content_type":"application/json","bytes":2}`,
		`{"input":"broken.jpg","status":"failed","error":{"code":"unsupported_format","message":"not an image"}}`,
		`{"input":"../../etc/passwd","status":"done","content_type":"image/png","bytes":3}`,
	}
	s.api.handle("GET /v1/jobs/job_9", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, jobJSON("job_9", "completed", items...))
	})
	results := map[string][2]string{
		"0": {"image/png", "one"}, "1": {"image/webp", "two"}, "2": {"application/json", "{}"}, "4": {"image/png", "four"},
	}
	for n, res := range results {
		s.api.handle("GET /v1/jobs/job_9/items/"+n, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", res[0])
			w.Header().Set("X-Image-Width", "64")
			w.Header().Set("X-Image-Height", "48")
			fmt.Fprint(w, res[1])
		})
	}

	res := s.run("jobs", "download", "job_9", "--out-dir", "got", "--json")
	if res.code != ExitFailed {
		t.Errorf("exit %d, want %d: an item failed", res.code, ExitFailed)
	}
	want := map[string]string{"got/shoe.png": "one", "got/shoe-2.webp": "two", "got/label.json": "{}", "got/passwd.png": "four"}
	files := s.files()
	if len(files) != len(want) {
		t.Errorf("files: %v", files)
	}
	for name, content := range want {
		if data, err := os.ReadFile(filepath.FromSlash(name)); err != nil || string(data) != content {
			t.Errorf("%s: %q, %v; files: %v", name, data, err, files)
		}
	}
	lines := jsonLines(t, res.stdout)
	if len(lines) != 5 {
		t.Fatalf("want 5 lines: %s", res.stdout)
	}
	if got := lines[0]; got["item"] != 0.0 || got["input"] != "shoe.jpg" || got["output"] != filepath.FromSlash("got/shoe.png") ||
		got["content_type"] != "image/png" || got["width"] != 64.0 || got["bytes"] != 3.0 {
		t.Errorf("first line: %v", got)
	}
	wantErr := map[string]any{"item": 3.0, "input": "broken.jpg", "error": map[string]any{"code": "unsupported_format", "message": "not an image (unsupported_format)"}}
	if !reflect.DeepEqual(lines[3], wantErr) {
		t.Errorf("the failed item: %v\nwant %v", lines[3], wantErr)
	}

	// Again: every file is in the way, and nothing is fetched.
	s.api.reset()
	res = s.run("jobs", "download", "job_9", "--out-dir", "got")
	if res.code != ExitFailed || strings.Count(res.stderr, "already exists") != 4 {
		t.Errorf("second download: exit %d\n%s", res.code, res.stderr)
	}
	if n := len(s.api.all()); n != 1 {
		t.Errorf("%d requests, want only the job itself", n)
	}
	// --force replaces.
	if err := os.WriteFile(filepath.FromSlash("got/shoe.png"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.run("jobs", "download", "job_9", "--out-dir", "got", "--force")
	if data, _ := os.ReadFile(filepath.FromSlash("got/shoe.png")); string(data) != "one" {
		t.Errorf("--force did not replace: %q", data)
	}
}

// create sends more than one job takes as several jobs, whole images in
// order, by size and by count, and prints each id as it starts.
func TestJobsCreateSplits(t *testing.T) {
	s := newSession(t)
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		s.write(name, bytes.Repeat([]byte{1}, 400_000))
	}
	var made atomic.Int32
	s.api.handle("POST /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, jobJSON(fmt.Sprintf("job_%d", made.Add(1)), "queued", `{"input":"x","status":"pending"}`))
	})

	// 1 MB a job: a and b together, c alone.
	res := s.ok("jobs", "create", "run", "--param", "preset=web", "--max-upload-mb", "1", "a.jpg", "b.jpg", "c.jpg")
	if res.stdout != "job_1\njob_2\n" {
		t.Errorf("stdout = %q", res.stdout)
	}
	if !strings.Contains(res.stderr, "sent as 2 jobs") || !strings.Contains(res.stderr, "photos jobs wait job_1 job_2") {
		t.Errorf("stderr = %q", res.stderr)
	}
	var sent [][]string
	for _, req := range s.api.all() {
		var names []string
		for _, f := range req.Files["image"] {
			names = append(names, f.Name)
		}
		sent = append(sent, names)
		if req.Fields["operation"][0] != "run" {
			t.Errorf("operation = %v", req.Fields["operation"])
		}
	}
	if !reflect.DeepEqual(sent, [][]string{{"a.jpg", "b.jpg"}, {"c.jpg"}}) {
		t.Errorf("jobs sent = %v", sent)
	}

	// Under the limit: one job, as before.
	s.api.reset()
	if res := s.ok("jobs", "create", "run", "a.jpg", "b.jpg", "c.jpg"); res.stdout != "job_3\n" || len(s.api.all()) != 1 {
		t.Errorf("one job: %q, %d requests", res.stdout, len(s.api.all()))
	}

	// One image larger than a job may upload: nothing is sent.
	s.api.reset()
	s.write("big.jpg", bytes.Repeat([]byte{1}, 1_100_000))
	res = s.run("jobs", "create", "run", "--max-upload-mb", "1", "a.jpg", "big.jpg")
	if res.code == 0 || !strings.Contains(res.stderr, "big.jpg is 1.1 MB, more than one job may upload") || len(s.api.all()) != 0 {
		t.Errorf("too big: exit %d, %q, %d requests", res.code, res.stderr, len(s.api.all()))
	}

	// 1001 URLs: 1000 in one job, the last in another.
	s.api.reset()
	args := []string{"jobs", "create", "resize", "--param", "width=10"}
	for i := range 1001 {
		args = append(args, fmt.Sprintf("https://example.com/%d.jpg", i))
	}
	s.ok(args...)
	var counts []int
	for _, req := range s.api.all() {
		inputs, _ := req.JSON["inputs"].([]any)
		counts = append(counts, len(inputs))
	}
	if !reflect.DeepEqual(counts, []int{1000, 1}) {
		t.Errorf("URLs per job = %v", counts)
	}

	// A job that fails to start after another started: the first id is out.
	s.api.reset()
	made.Store(0)
	s.api.handle("POST /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if made.Add(1) > 1 {
			apiError(w, 413, "payload_too_large", "request body is too large")
			return
		}
		writeJSON(w, 202, jobJSON("job_1", "queued", `{"input":"x","status":"pending"}`))
	})
	res = s.run("jobs", "create", "run", "--max-upload-mb", "1", "a.jpg", "b.jpg", "c.jpg")
	if res.code == 0 || res.stdout != "job_1\n" || !strings.Contains(res.stderr, "1 of 2 jobs started") {
		t.Errorf("second job refused: exit %d, %q, %q", res.code, res.stdout, res.stderr)
	}
}

// download waits for each job, then saves its results; several jobs share
// one directory and its names.
func TestJobsDownloadWaits(t *testing.T) {
	s := newSession(t)
	var polls atomic.Int32
	s.api.handle("GET /v1/jobs/job_a", func(w http.ResponseWriter, r *http.Request) {
		if polls.Add(1) < 3 {
			writeJSON(w, 200, jobJSON("job_a", "running", `{"input":"a.jpg","status":"pending"}`))
			return
		}
		writeJSON(w, 200, jobJSON("job_a", "completed", `{"input":"a.jpg","status":"done","content_type":"image/jpeg","bytes":1}`))
	})
	s.api.handle("GET /v1/jobs/job_b", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, jobJSON("job_b", "completed", `{"input":"other/a.jpg","status":"done","content_type":"image/jpeg","bytes":1}`))
	})
	for _, id := range []string{"job_a", "job_b"} {
		s.api.handle("GET /v1/jobs/"+id+"/items/0", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			fmt.Fprint(w, id)
		})
	}
	res := s.ok("jobs", "download", "job_a", "job_b", "--out-dir", "got", "--interval", "5ms")
	if polls.Load() != 3 {
		t.Errorf("%d looks at job_a, want 3", polls.Load())
	}
	for name, want := range map[string]string{"got/a.jpg": "job_a", "got/a-2.jpg": "job_b"} {
		if data, err := os.ReadFile(filepath.FromSlash(name)); err != nil || string(data) != want {
			t.Errorf("%s = %q, %v; stderr %s", name, data, err, res.stderr)
		}
	}

	// --no-wait saves what is finished now, and says what is not.
	s.api.handle("GET /v1/jobs/job_c", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, jobJSON("job_c", "running", `{"input":"c.jpg","status":"pending"}`))
	})
	res = s.run("jobs", "download", "job_c", "--out-dir", "got", "--no-wait")
	if res.code != ExitFailed || !strings.Contains(res.stderr, "only the finished items are saved") || !strings.Contains(res.stderr, "not finished") {
		t.Errorf("--no-wait: exit %d, %q", res.code, res.stderr)
	}
	// Past --wait-timeout: the same, after waiting.
	res = s.run("jobs", "download", "job_c", "--out-dir", "got", "--interval", "5ms", "--wait-timeout", "20ms")
	if res.code != ExitFailed || !strings.Contains(res.stderr, "only the finished items are saved") {
		t.Errorf("past the timeout: exit %d, %q", res.code, res.stderr)
	}
	// An unknown job among others: reported, the others still saved.
	res = s.run("jobs", "download", "job_none", "job_b", "--out-dir", "again")
	if res.code != ExitFailed || !strings.Contains(res.stderr, "job_none") {
		t.Errorf("unknown job: exit %d, %q", res.code, res.stderr)
	}
	if _, err := os.Stat(filepath.FromSlash("again/a.jpg")); err != nil {
		t.Errorf("job_b not saved after job_none: %v", err)
	}

	// wait takes several ids too.
	res = s.ok("jobs", "wait", "job_a", "job_b", "--interval", "5ms")
	if !strings.Contains(res.stdout, "job_a") || !strings.Contains(res.stdout, "job_b") {
		t.Errorf("wait for two:\n%s", res.stdout)
	}
}

// webhook verify: exit 0 for a genuine delivery, 1 for anything else; the
// signing key comes from the environment and is never printed.
func TestWebhookVerify(t *testing.T) {
	s := newSession(t)
	const secret = "whsec_SIGNINGKEY0123456789"
	t.Setenv("HOOK_SECRET", secret)

	body := `{"type":"job.completed","job":` + jobJSON("job_5", "completed") + `}`
	sign := func(ts int64, body, key string) string {
		mac := hmac.New(sha256.New, []byte(key))
		fmt.Fprintf(mac, "%d.%s", ts, body)
		return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
	}
	now := time.Now().Unix()
	good := sign(now, body, secret)
	path := s.write("body.json", []byte(body))

	check := func(res result) {
		t.Helper()
		if strings.Contains(res.stdout+res.stderr, secret) {
			t.Fatalf("the signing key was printed: %q %q", res.stdout, res.stderr)
		}
	}

	// From a file, and from standard input.
	res := s.run("webhook", "verify", "--secret-env", "HOOK_SECRET", "--signature", good, path)
	check(res)
	if res.code != 0 || !strings.Contains(res.stdout, "verified: job.completed  job_5") {
		t.Errorf("a genuine delivery: exit %d, %q %q", res.code, res.stdout, res.stderr)
	}
	res = s.runIn(body, "webhook", "verify", "--secret-env", "HOOK_SECRET", "--signature", good)
	check(res)
	if res.code != 0 {
		t.Errorf("from standard input: exit %d, %q", res.code, res.stderr)
	}
	res = s.runIn(body, "webhook", "verify", "--secret-env", "HOOK_SECRET", "--signature", good, "--json", "-")
	if res.code != 0 || res.stdout != body+"\n" {
		t.Errorf("--json should print the event: exit %d, %q", res.code, res.stdout)
	}
	res = s.runIn(body, "webhook", "verify", "--secret-env", "HOOK_SECRET", "--signature", good, "--quiet")
	if res.code != 0 || res.stdout != "" || res.stderr != "" {
		t.Errorf("--quiet: exit %d, %q %q", res.code, res.stdout, res.stderr)
	}

	// Everything else is exit 1.
	bad := []struct {
		name      string
		body      string
		signature string
		extra     []string
		says      string
	}{
		{"tampered body", body + " ", good, nil, "signature does not match"},
		{"wrong secret", body, sign(now, body, "another"), nil, "signature does not match"},
		{"old", body, sign(now-3600, body, secret), nil, "timestamp outside the tolerance"},
		{"old, within a wider tolerance", body, sign(now-3600, body, secret), []string{"--tolerance", "2h"}, ""},
		{"malformed header", body, "v1=abc", nil, "malformed"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			res := s.runIn(tc.body, append([]string{"webhook", "verify", "--secret-env", "HOOK_SECRET", "--signature", tc.signature}, tc.extra...)...)
			check(res)
			if tc.says == "" {
				if res.code != 0 {
					t.Errorf("exit %d, want 0: %q", res.code, res.stderr)
				}
				return
			}
			if res.code != ExitFailed || !strings.Contains(res.stderr, tc.says) || res.stdout != "" {
				t.Errorf("exit %d, stdout %q, stderr %q", res.code, res.stdout, res.stderr)
			}
		})
	}

	// The variable not being set is a usage error.
	if res := s.runIn(body, "webhook", "verify", "--secret-env", "NOT_SET_ANYWHERE", "--signature", good); res.code != ExitUsage {
		t.Errorf("an unset variable: exit %d", res.code)
	}
	// It needs neither a key nor the network.
	if n := len(s.api.all()); n != 0 {
		t.Errorf("verify made %d requests", n)
	}
}

// An organisation without catalogue matching (or without a catalogue yet)
// is told "not found" by the API: the command says what that means.
func TestIdentifyWithoutCatalogueMatching(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.api.handle("POST /v1/identify", func(w http.ResponseWriter, r *http.Request) {
		apiError(w, 404, "not_found", "not found")
	})
	res := s.run("identify", "a.png")
	if res.code != ExitFailed {
		t.Errorf("exit code = %d, want %d", res.code, ExitFailed)
	}
	for _, want := range []string{"Catalogue matching is not turned on", "Catalogues page", "not_found"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", res.stderr, want)
		}
	}
}
