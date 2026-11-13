package cli

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// Every command parses its flags and makes the request the API documents:
// the method, the path, the query (the API's parameter names, with
// underscores), the key, and the multipart parts.
func TestEachCommandMakesItsRequest(t *testing.T) {
	s := newSession(t)
	s.image("a.png")
	s.image("b.png")
	s.image("mask.png")
	s.image("logo.png")
	s.image("sky.png")
	s.write("shop.toml", []byte("[output]\nformat = \"webp\"\n"))

	// Data endpoints answer with the smallest valid body.
	for route, body := range map[string]string{
		"POST /v1/check":         `{"metrics":{"width":8,"height":8,"sharpness":0.5,"border":{}},"findings":[]}`,
		"POST /v1/barcodes":      `{"barcodes":[],"variant":"none"}`,
		"POST /v1/ocr":           `{"lines":[]}`,
		"POST /v1/label":         `{"label":{},"barcodes":[],"text":[]}`,
		"POST /v1/identify":      `{"catalogue":"whisky","candidates":[],"label":{},"barcodes":[]}`,
		"POST /v1/palette":       `{"width":8,"height":8,"colors":[]}`,
		"POST /v1/faces":         `{"width":8,"height":8,"faces":[]}`,
		"POST /v1/caption":       `{"caption":"A shoe.","detail":"short","width":8,"height":8}`,
		"GET /v1/presets":        `[]`,
		"GET /v1/marketplaces":   `[]`,
		"GET /v1/id-photos":      `[]`,
		"GET /v1/jobs":           `[]`,
		"GET /v1/webhook-secret": `{"secret":"whsec_1","header":"Photos-Signature","scheme":"v1"}`,
		"GET /v1/jobs/job_1":     `{"id":"job_1","operation":"resize","params":"","status":"completed","created_at":1,"counts":{"total":0},"items":[]}`,
	} {
		s.api.handle(route, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, body) })
	}
	s.api.handle("DELETE /v1/jobs/job_1", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	s.api.handle("POST /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, `{"id":"job_1","operation":"resize","params":"","status":"queued","created_at":1,"counts":{"total":2,"pending":2},"items":[]}`)
	})

	tests := []struct {
		args   string
		method string
		path   string
		query  string
		// files are the multipart file parts, as "field=name"; fields the
		// text parts, as "field=value".
		files  []string
		fields []string
	}{
		{"remove-background a.png", "POST", "/v1/remove-background", "", []string{"image=a.png"}, nil},
		{"bg-remove --format jpg --quality 90 --background white --crop --padding 40 a.png", "POST", "/v1/remove-background",
			"background=white&crop=true&format=jpg&padding=40&quality=90", []string{"image=a.png"}, nil},
		{"remove-background --mask a.png", "POST", "/v1/remove-background", "mask=true", []string{"image=a.png"}, nil},
		// Staging the cutout.
		{"remove-background --width 80 --height 60 --fill 0.8 --align bottom --shadow contact --reflection --background white a.png", "POST", "/v1/remove-background",
			"align=bottom&background=white&fill=0.8&height=60&reflection=true&shadow=contact&width=80", []string{"image=a.png"}, nil},
		{"remove-background --blur 40 a.png", "POST", "/v1/remove-background", "blur=40", []string{"image=a.png"}, nil},
		{"remove-background --background-image sky.png --shadow drop a.png", "POST", "/v1/remove-background", "shadow=drop",
			[]string{"image=a.png", "background=sky.png"}, nil},

		{"resize --width 800 --height 600 --fit cover --enlarge --format webp --quality 82 --max-bytes 300000 --background #ffffff a.png",
			"POST", "/v1/resize", "background=%23ffffff&enlarge=true&fit=cover&format=webp&height=600&max_bytes=300000&quality=82&width=800",
			[]string{"image=a.png"}, nil},
		// Flags may follow the file.
		{"resize a.png --width 100", "POST", "/v1/resize", "width=100", []string{"image=a.png"}, nil},
		{"resize --width=100 a.png", "POST", "/v1/resize", "width=100", []string{"image=a.png"}, nil},
		{"resize --trim --trim-tolerance 20 --dpi 300 a.png", "POST", "/v1/resize", "dpi=300&trim=true&trim_tolerance=20", []string{"image=a.png"}, nil},
		// A tolerance of 0 is a value, and is sent; one not given is not.
		{"resize --trim --trim-tolerance 0 a.png", "POST", "/v1/resize", "trim=true&trim_tolerance=0", []string{"image=a.png"}, nil},
		{"resize --trim a.png", "POST", "/v1/resize", "trim=true", []string{"image=a.png"}, nil},

		{"upscale --scale 2.5 --format png --quality 70 a.png", "POST", "/v1/upscale", "format=png&quality=70&scale=2.5", []string{"image=a.png"}, nil},

		{"erase --rect 1,2,30,40 --rect 5,6,7,8 --format jpg --quality 80 a.png", "POST", "/v1/erase",
			"format=jpg&quality=80&rect=1%2C2%2C30%2C40&rect=5%2C6%2C7%2C8", []string{"image=a.png"}, nil},
		{"erase --mask mask.png a.png", "POST", "/v1/erase", "", []string{"image=a.png", "mask=mask.png"}, nil},

		{"remove-text --region all --matching sale|www --format png --quality 60 a.png", "POST", "/v1/remove-text",
			"format=png&matching=sale%7Cwww&quality=60&region=all", []string{"image=a.png"}, nil},

		{"enhance a.png", "POST", "/v1/enhance", "", []string{"image=a.png"}, nil},
		{"enhance --auto --levels=false --white-balance --brightness 10 --contrast -20 --saturation -100 --temperature 5.5 --sharpen 30 --format jpg --quality 90 a.png",
			"POST", "/v1/enhance", "auto=true&brightness=10&contrast=-20&format=jpg&levels=false&quality=90&saturation=-100&sharpen=30&temperature=5.5&white_balance=true",
			[]string{"image=a.png"}, nil},

		{"crop --aspect 4:5 a.png", "POST", "/v1/crop", "aspect=4%3A5", []string{"image=a.png"}, nil},
		{"crop --width 40 --height 50 --zoom --padding 0.6 --focus face --format webp --quality 80 a.png", "POST", "/v1/crop",
			"focus=face&format=webp&height=50&padding=0.6&quality=80&width=40&zoom=true", []string{"image=a.png"}, nil},
		{"crop --aspect 1:1 --zoom --padding 0 a.png", "POST", "/v1/crop", "aspect=1%3A1&padding=0&zoom=true", []string{"image=a.png"}, nil},

		{"expand --aspect 16:9 a.png", "POST", "/v1/expand", "aspect=16%3A9", []string{"image=a.png"}, nil},
		{"expand --left 10 --right 20 --top 30 --bottom 40 --fill white --format png --quality 70 a.png", "POST", "/v1/expand",
			"bottom=40&fill=white&format=png&left=10&quality=70&right=20&top=30", []string{"image=a.png"}, nil},

		{"watermark --text Acme --color black --position top-left --scale 15 --opacity 80 --margin 0 --format jpg --quality 85 a.png", "POST", "/v1/watermark",
			"color=black&format=jpg&margin=0&opacity=80&position=top-left&quality=85&scale=15&text=Acme", []string{"image=a.png"}, nil},
		{"watermark --logo logo.png --tile a.png", "POST", "/v1/watermark", "tile=true", []string{"image=a.png", "watermark=logo.png"}, nil},

		{"denoise --strength 60 --format png --quality 70 a.png", "POST", "/v1/denoise", "format=png&quality=70&strength=60", []string{"image=a.png"}, nil},
		{"colorize --strength 70 --format webp --quality 75 a.png", "POST", "/v1/colorize", "format=webp&quality=75&strength=70", []string{"image=a.png"}, nil},

		{"select --point 4,3 a.png", "POST", "/v1/select", "point=4%2C3", []string{"image=a.png"}, nil},
		{"select --point 4,3 --point 5.5,2 --exclude 1,1 --exclude 7,7 --box 1,2,5,4 --pick largest --crop --padding 8 --background white --format jpg --quality 90 a.png",
			"POST", "/v1/select",
			"background=white&box=1%2C2%2C5%2C4&crop=true&exclude=1%2C1&exclude=7%2C7&format=jpg&padding=8&pick=largest&point=4%2C3&point=5.5%2C2&quality=90",
			[]string{"image=a.png"}, nil},
		{"select --box 0,0,5,4 --mask a.png", "POST", "/v1/select", "box=0%2C0%2C5%2C4&mask=true", []string{"image=a.png"}, nil},

		{"blur-faces a.png", "POST", "/v1/blur-faces", "", []string{"image=a.png"}, nil},
		{"blur-faces --mode fill --strength 80 --padding 0 --color #202020 --min-score 0.4 --format jpg --quality 88 a.png", "POST", "/v1/blur-faces",
			"color=%23202020&format=jpg&min_score=0.4&mode=fill&padding=0&quality=88&strength=80", []string{"image=a.png"}, nil},
		{"restore-faces --strength 70 --min-score 0.5 --format png --quality 80 a.png", "POST", "/v1/restore-faces",
			"format=png&min_score=0.5&quality=80&strength=70", []string{"image=a.png"}, nil},

		{"id-photo --document us-passport a.png", "POST", "/v1/id-photo", "document=us-passport", []string{"image=a.png"}, nil},
		{"id-photo --width-mm 35 --height-mm 45 --head-min-mm 32 --head-max-mm 36 --eyes-min-mm 28.5 --eyes-max-mm 34 --dpi 600 --background white --format png --quality 95 a.png",
			"POST", "/v1/id-photo",
			"background=white&dpi=600&eyes_max_mm=34&eyes_min_mm=28.5&format=png&head_max_mm=36&head_min_mm=32&height_mm=45&quality=95&width_mm=35",
			[]string{"image=a.png"}, nil},

		{"vectorize a.png", "POST", "/v1/vectorize", "", []string{"image=a.png"}, nil},
		{"vectorize --mode mono --curves polygon --detail 5 --speckle 0 a.png", "POST", "/v1/vectorize", "curves=polygon&detail=5&mode=mono&speckle=0", []string{"image=a.png"}, nil},

		{"run --preset marketplace a.png", "POST", "/v1/run", "preset=marketplace", []string{"image=a.png"}, nil},
		{"run --recipe shop.toml a.png", "POST", "/v1/run", "", []string{"image=a.png", "recipe=recipe.toml"}, nil},

		{"fix --marketplace amazon a.png", "POST", "/v1/fix", "marketplace=amazon", []string{"image=a.png"}, nil},

		{"check --marketplace etsy --subject --text a.png", "POST", "/v1/check", "marketplace=etsy&subject=true&text=true", []string{"image=a.png"}, nil},
		{"barcodes --all-formats --engine zbar a.png", "POST", "/v1/barcodes", "all_formats=true&engine=zbar", []string{"image=a.png"}, nil},
		{"ocr a.png", "POST", "/v1/ocr", "", []string{"image=a.png"}, nil},
		{"label a.png b.png", "POST", "/v1/label", "", []string{"image=a.png", "image=b.png"}, nil},
		{"identify --catalogue whisky --top 3 a.png b.png", "POST", "/v1/identify", "catalogue=whisky&top=3", []string{"image=a.png", "image=b.png"}, nil},
		{"palette a.png", "POST", "/v1/palette", "", []string{"image=a.png"}, nil},
		{"palette --colors 3 a.png", "POST", "/v1/palette", "colors=3", []string{"image=a.png"}, nil},
		{"faces --min-score 0.4 a.png", "POST", "/v1/faces", "min_score=0.4", []string{"image=a.png"}, nil},
		{"caption a.png", "POST", "/v1/caption", "", []string{"image=a.png"}, nil},
		{"caption --detail paragraph a.png", "POST", "/v1/caption", "detail=paragraph", []string{"image=a.png"}, nil},

		{"presets", "GET", "/v1/presets", "", nil, nil},
		{"marketplaces", "GET", "/v1/marketplaces", "", nil, nil},
		{"id-photos", "GET", "/v1/id-photos", "", nil, nil},
		{"account", "GET", "/v1/account", "", nil, nil},
		{"health", "GET", "/health", "", nil, nil},
		{"webhook secret", "GET", "/v1/webhook-secret", "", nil, nil},

		{"jobs list", "GET", "/v1/jobs", "", nil, nil},
		{"jobs get job_1", "GET", "/v1/jobs/job_1", "", nil, nil},
		{"jobs delete job_1", "DELETE", "/v1/jobs/job_1", "", nil, nil},
		{"jobs create resize --param width=800 --param max-bytes=300000 --webhook-url https://example.com/hook a.png b.png", "POST", "/v1/jobs", "",
			[]string{"image=a.png", "image=b.png"},
			[]string{"operation=resize", "params=max_bytes=300000&width=800", "webhook_url=https://example.com/hook"}},
		{"jobs create run --recipe shop.toml a.png", "POST", "/v1/jobs", "",
			[]string{"image=a.png", "recipe=recipe.toml"}, []string{"operation=run"}},
		// The operations added with the editing tools; a parameter given
		// twice is sent twice.
		{"jobs create select --param point=4,3 --param point=5,2 --param crop=true a.png", "POST", "/v1/jobs", "",
			[]string{"image=a.png"}, []string{"operation=select", "params=crop=true&point=4%2C3&point=5%2C2"}},
		{"jobs create id-photo --param document=us-passport a.png", "POST", "/v1/jobs", "",
			[]string{"image=a.png"}, []string{"operation=id-photo", "params=document=us-passport"}},
		{"jobs create blur-faces --param mode=fill --param min-score=0.4 a.png b.png", "POST", "/v1/jobs", "",
			[]string{"image=a.png", "image=b.png"}, []string{"operation=blur-faces", "params=min_score=0.4&mode=fill"}},
	}
	for _, tc := range tests {
		t.Run(tc.args, func(t *testing.T) {
			s.api.reset()
			// --out-dir keeps the image commands from finding their own
			// results in the way on a second run.
			args := strings.Fields(tc.args)
			if imageEndpoints[tc.path] {
				args = append(args, "--out-dir", t.TempDir())
			}
			res := s.run(args...)
			if res.code != 0 {
				t.Fatalf("exit %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
			}
			req := s.api.only()
			if req.Method != tc.method || req.Path != tc.path {
				t.Errorf("request: %s %s, want %s %s", req.Method, req.Path, tc.method, tc.path)
			}
			if got := req.Query.Encode(); got != tc.query {
				t.Errorf("query: %s, want %s", got, tc.query)
			}
			if tc.path == "/health" {
				if req.Auth != "" {
					t.Errorf("health sent a key")
				}
			} else if req.Auth != "Bearer "+testKey {
				t.Errorf("Authorization header is wrong")
			}
			if !strings.HasPrefix(req.Agent, "photos-cli/test photos-sh-go/") {
				t.Errorf("User-Agent: %q", req.Agent)
			}

			var files, fields []string
			for _, field := range []string{"image", "mask", "recipe", "background", "watermark"} {
				for _, up := range req.Files[field] {
					files = append(files, field+"="+up.Name)
				}
			}
			for _, field := range []string{"operation", "params", "webhook_url", "url"} {
				for _, v := range req.Fields[field] {
					fields = append(fields, field+"="+v)
				}
			}
			if !reflect.DeepEqual(files, tc.files) {
				t.Errorf("file parts: %v, want %v", files, tc.files)
			}
			if !reflect.DeepEqual(fields, tc.fields) {
				t.Errorf("text parts: %v, want %v", fields, tc.fields)
			}
		})
	}
}

// What is uploaded is the file's bytes, and the recipe's text.
func TestUploadsCarryTheFiles(t *testing.T) {
	s := newSession(t)
	s.write("a.png", []byte("the image"))
	s.write("mask.png", []byte("the mask"))
	s.write("r.toml", []byte("[output]\n"))

	s.ok("erase", "--mask", "mask.png", "--rect", "0,0,1,1", "-o", "out.png", "a.png")
	req := s.api.only()
	if got := string(req.Files["image"][0].Data); got != "the image" {
		t.Errorf("image part: %q", got)
	}
	if got := string(req.Files["mask"][0].Data); got != "the mask" {
		t.Errorf("mask part: %q", got)
	}

	s.api.reset()
	s.ok("run", "--recipe", "r.toml", "-o", "out2.png", "a.png")
	if got := string(s.api.only().Files["recipe"][0].Data); got != "[output]\n" {
		t.Errorf("recipe part: %q", got)
	}
}

// A job of URLs is sent as JSON, with the parameters as an object.
func TestJobsCreateWithURLs(t *testing.T) {
	s := newSession(t)
	s.api.handle("POST /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 202, `{"id":"job_7","operation":"remove-background","params":"format=png","status":"queued","created_at":1,"counts":{"total":2,"pending":2},"items":[]}`)
	})
	res := s.ok("jobs", "create", "remove-background", "--param", "format=png", "--param", "crop=true",
		"https://example.com/1.jpg", "https://example.com/2.jpg")
	if res.stdout != "job_7\n" {
		t.Errorf("standard output should be the id alone, got %q", res.stdout)
	}
	req := s.api.only()
	want := map[string]any{
		"operation": "remove-background",
		"params":    map[string]any{"format": "png", "crop": "true"},
		"inputs":    []any{"https://example.com/1.jpg", "https://example.com/2.jpg"},
	}
	if !reflect.DeepEqual(req.JSON, want) {
		t.Errorf("body: %v, want %v", req.JSON, want)
	}
}

// --base-url wins over PHOTOS_BASE_URL; --timeout and --retries reach the
// library.
func TestBaseURLTimeoutAndRetries(t *testing.T) {
	s := newSession(t)
	other := newFakeAPI(t)
	s.ok("account", "--base-url", other.srv.URL+"/")
	if len(other.all()) != 1 || len(s.api.all()) != 0 {
		t.Fatalf("--base-url did not win: %d and %d requests", len(other.all()), len(s.api.all()))
	}
	// Before the command works too.
	s.ok("--base-url", other.srv.URL, "account")
	if len(other.all()) != 2 {
		t.Fatalf("--base-url before the command was not used")
	}

	busy := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0.01")
		apiError(w, 503, "busy", "all workers are busy")
	}
	s.api.handle("GET /v1/account", busy)
	for retries, want := range map[string]int{"0": 1, "2": 3} {
		s.api.reset()
		res := s.run("account", "--retries", retries)
		if res.code != ExitUnavailable {
			t.Errorf("--retries %s: exit %d, want %d", retries, res.code, ExitUnavailable)
		}
		if got := len(s.api.all()); got != want {
			t.Errorf("--retries %s: %d requests, want %d", retries, got, want)
		}
	}

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	s.api.handle("GET /v1/account", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	res := s.run("account", "--timeout", "50ms")
	if res.code != ExitUnavailable || !strings.Contains(res.stderr, "timed out") {
		t.Errorf("--timeout: exit %d, stderr %q", res.code, res.stderr)
	}

	for _, bad := range [][]string{{"account", "--timeout", "-1s"}, {"account", "--retries", "-1"}, {"account", "--base-url", "ftp://x"}, {"account", "--timeout", "soon"}} {
		if res := s.run(bad...); res.code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", bad, res.code, ExitUsage)
		}
	}
}
