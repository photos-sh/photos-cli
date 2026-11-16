package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A recipe's readings are on the file's line, and in --json.
func TestRunShowsWhatARecipeRead(t *testing.T) {
	s := newSession(t)
	s.write("ham.png", []byte("the image"))
	s.write("r.toml", []byte("[barcodes]\n[text]\n"))
	s.api.handle("POST /v1/run", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Barcodes", `[{"format":"EAN-13","value":"3850104010017","gs1":"3850104010017"}]`)
		w.Header().Set("X-Text", `["Pr\u0161ut","500 g"]`)
		_, _ = w.Write([]byte("png"))
	})

	res := s.ok("run", "--recipe", "r.toml", "-o", "out.png", "ham.png")
	for _, want := range []string{"barcodes: EAN-13 3850104010017", "text: 2 lines", "\n    Pršut\n    500 g"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, res.stdout)
		}
	}

	res = s.ok("--json", "run", "--recipe", "r.toml", "-o", "out2.png", "ham.png")
	var line struct {
		Barcodes []struct{ Format, Value string } `json:"barcodes"`
		Text     []string                         `json:"text"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &line); err != nil {
		t.Fatalf("%v: %s", err, res.stdout)
	}
	if len(line.Barcodes) != 1 || line.Barcodes[0].Value != "3850104010017" || strings.Join(line.Text, "|") != "Pršut|500 g" {
		t.Errorf("json = %+v", line)
	}
}

// Found nothing is said; a recipe without reading steps says nothing.
func TestRunReadingsEmptyAndAbsent(t *testing.T) {
	s := newSession(t)
	s.write("a.png", []byte("the image"))
	s.write("r.toml", []byte("[output]\n"))
	headers := map[string]string{"X-Barcodes": "[]", "X-Text": "[]"}
	s.api.handle("POST /v1/run", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		_, _ = w.Write([]byte("png"))
	})

	res := s.ok("run", "--recipe", "r.toml", "-o", "o1.png", "a.png")
	if !strings.Contains(res.stdout, "no barcode found") || !strings.Contains(res.stdout, "no text found") {
		t.Errorf("found nothing:\n%s", res.stdout)
	}
	res = s.ok("--json", "run", "--recipe", "r.toml", "-o", "o2.png", "a.png")
	if !strings.Contains(res.stdout, `"barcodes":[]`) || !strings.Contains(res.stdout, `"text":[]`) {
		t.Errorf("json, found nothing: %s", res.stdout)
	}

	headers = map[string]string{"X-Text": `["a"]`, "X-Text-Truncated": "true"}
	res = s.ok("run", "--recipe", "r.toml", "-o", "o3.png", "a.png")
	if !strings.Contains(res.stdout, "the first 1 lines (more were left out)") {
		t.Errorf("truncated:\n%s", res.stdout)
	}

	headers = nil
	res = s.ok("--json", "run", "--recipe", "r.toml", "-o", "o4.png", "a.png")
	if strings.Contains(res.stdout, "barcodes") || strings.Contains(res.stdout, `"text"`) {
		t.Errorf("without reading steps: %s", res.stdout)
	}
}

func TestPresetsPrintsOnesRecipe(t *testing.T) {
	s := newSession(t)
	const toml = "description = \"Web\"\n[output]\nformat = \"webp\"\n"
	s.api.handle("GET /v1/presets/web", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/toml; charset=utf-8")
		_, _ = w.Write([]byte(toml))
	})
	s.api.handle("GET /v1/presets/nope", func(w http.ResponseWriter, r *http.Request) {
		apiError(w, 404, "not_found", `no preset named "nope"`)
	})

	if res := s.ok("presets", "web"); res.stdout != toml {
		t.Errorf("stdout = %q", res.stdout)
	}
	res := s.ok("--json", "presets", "web")
	var got map[string]string
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil || got["name"] != "web" || got["recipe"] != toml {
		t.Errorf("json = %q (%v)", res.stdout, err)
	}
	res = s.run("presets", "nope")
	if res.code == 0 || !strings.Contains(res.stderr, "photos presets` lists the presets") {
		t.Errorf("unknown preset: exit %d\n%s", res.code, res.stderr)
	}
}
