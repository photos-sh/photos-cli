package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// script runs `photos completion <shell>` and writes the script to a file.
func script(t *testing.T, shell string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	env := &Env{Version: "test", Out: &out, Err: &errOut, In: strings.NewReader("")}
	if code := Main(context.Background(), env, []string{"completion", shell}); code != 0 {
		t.Fatalf("photos completion %s: exit %d: %s", shell, code, errOut.String())
	}
	path := filepath.Join(t.TempDir(), "photos."+shell)
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// findShell finds a shell to check a script with, or skips the test.
func findShell(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shells are not checked on Windows")
	}
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("no %s to check against", name)
	}
	return path
}

// A completion script is read by a shell and never by a person: one that
// does not parse fails silently, TAB just stops working. So each is given
// to its shell.
func TestCompletionScriptsParse(t *testing.T) {
	for shell, check := range map[string][]string{
		"bash": {"-n"},
		"zsh":  {"-n"},
		"fish": {"--no-execute"},
	} {
		t.Run(shell, func(t *testing.T) {
			bin := findShell(t, shell)
			out, err := exec.Command(bin, append(check, script(t, shell))...).CombinedOutput()
			if err != nil {
				t.Fatalf("%s rejected the script: %v\n%s", shell, err, out)
			}
		})
	}
}

// And the bash script completes, driven the way bash drives it.
func TestBashCompletes(t *testing.T) {
	bash := findShell(t, "bash")
	path := script(t, "bash")

	tests := []struct {
		words string // the command line so far; the last word is being typed
		want  []string
		not   []string
	}{
		{"photos re", []string{"remove-background", "remove-text", "resize"}, []string{"run", "--json"}},
		{"photos bg", []string{"bg-remove"}, nil},
		{"photos jobs ", []string{"create", "delete", "download", "get", "list", "wait"}, []string{"resize"}},
		{"photos jobs cr", []string{"create"}, nil},
		{"photos resize --w", []string{"--width"}, []string{"--json"}},
		{"photos resize --", []string{"--width", "--out-dir", "--json", "--help", "--force"}, []string{"--marketplace"}},
		{"photos bg-remove --cr", []string{"--crop"}, nil},
		// A flag's value, and a boolean flag, do not hide the command.
		{"photos --base-url http://x jobs ", []string{"create"}, nil},
		{"photos --json jobs ", []string{"create"}, nil},
		{"photos jobs create --pa", []string{"--param"}, nil},
		// After a file name, flags are still the command's.
		{"photos resize shoe.jpg --h", []string{"--height", "--help"}, nil},
		{"photos webhook ", []string{"secret", "verify"}, nil},
		{"photos id", []string{"id-photo", "id-photos"}, nil},
		{"photos select --p", []string{"--point", "--pick", "--padding", "--parallel"}, []string{"--position"}},
		// A switch that may be turned off takes no value: the file after it
		// does not hide the command.
		{"photos enhance --levels shoe.jpg --sh", []string{"--sharpen"}, nil},
		{"photos completion ", nil, []string{"--json"}},
	}
	for _, tc := range tests {
		t.Run(tc.words, func(t *testing.T) {
			words := strings.Split(tc.words, " ")
			var cmd strings.Builder
			cmd.WriteString("source " + path + "\nCOMP_WORDS=(")
			for _, w := range words {
				cmd.WriteString("'" + w + "' ")
			}
			cmd.WriteString(")\n")
			cmd.WriteString("COMP_CWORD=$(( ${#COMP_WORDS[@]} - 1 ))\n_photos\nprintf '%s\\n' \"${COMPREPLY[@]}\"\n")
			out, err := exec.Command(bash, "-c", cmd.String()).CombinedOutput()
			if err != nil {
				t.Fatalf("bash: %v\n%s", err, out)
			}
			got := map[string]bool{}
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				got[line] = true
			}
			for _, w := range tc.want {
				if !got[w] {
					t.Errorf("does not offer %s; offers %q", w, strings.TrimSpace(string(out)))
				}
			}
			for _, w := range tc.not {
				if got[w] {
					t.Errorf("offers %s", w)
				}
			}
		})
	}
}

// Every command and every flag is in every script.
func TestCompletionCoversTheTree(t *testing.T) {
	root := Root()
	for _, shell := range []string{"bash", "zsh", "fish"} {
		data, err := os.ReadFile(script(t, shell))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		var walk func(cmd *Command)
		walk = func(cmd *Command) {
			for _, child := range cmd.Children {
				if !strings.Contains(text, child.Name) {
					t.Errorf("%s: no %s", shell, child.Name)
				}
				for _, f := range commandFlags(child) {
					if !strings.Contains(text, f.Name) {
						t.Errorf("%s: no --%s of %s", shell, f.Name, child.Name)
					}
				}
				walk(child)
			}
		}
		walk(root)
	}
}
