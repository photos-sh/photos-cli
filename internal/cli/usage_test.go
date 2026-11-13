package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite USAGE.md from the command tree")

// USAGE.md is generated from the command tree, and this fails when the file
// in the repository is not what the tree says: a command or a flag changed
// without `make usage`.
func TestUsageIsInSync(t *testing.T) {
	path := filepath.Join("..", "..", "USAGE.md")
	want := UsageMarkdown()
	if *update {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run `make usage`)", err)
	}
	// A checkout on Windows may have turned the line ends.
	got = bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))
	if string(got) != want {
		t.Fatal("USAGE.md is out of date with the command tree: run `make usage` and commit the result")
	}
}

// The generated document has every command, every flag, the global flags
// and the exit codes.
func TestUsageCoversTheTree(t *testing.T) {
	doc := UsageMarkdown()
	var walk func(cmd *Command, path []string)
	walk = func(cmd *Command, path []string) {
		for _, child := range cmd.Children {
			childPath := append(append([]string{}, path...), child.Name)
			title := strings.Join(childPath, " ")
			section := "\n## " + title + "\n"
			i := strings.Index(doc, section)
			if i < 0 {
				t.Errorf("no section for %s", title)
				continue
			}
			body := doc[i+len(section):]
			if end := strings.Index(body, "\n## "); end >= 0 {
				body = body[:end]
			}
			for _, f := range commandFlags(child) {
				if !strings.Contains(body, "`"+strings.ReplaceAll(f.label(), "|", `\|`)+"`") {
					t.Errorf("%s: --%s is not in its section", title, f.Name)
				}
			}
			for _, example := range child.Examples {
				if !strings.Contains(body, example) {
					t.Errorf("%s: an example is missing: %s", title, example)
				}
			}
			for _, alias := range child.Aliases {
				if !strings.Contains(body, alias) {
					t.Errorf("%s: the alias %s is missing", title, alias)
				}
			}
			walk(child, childPath)
		}
	}
	walk(Root(), []string{"photos"})

	for _, f := range globalFlags() {
		if !strings.Contains(doc, "--"+f.Name) {
			t.Errorf("global flag --%s is missing", f.Name)
		}
	}
	for _, want := range []string{"| `0` |", "| `1` |", "| `2` |", "| `3` |", "| `4` |", "| `5` |", "PHOTOS_API_KEY", "PHOTOS_BASE_URL", "PHOTOS_CONFIG_DIR", "NO_COLOR"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the document lacks %q", want)
		}
	}
}
