package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveLoadRemove(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "photos-sh")
	t.Setenv(EnvDir, dir)

	if c, err := Load(); c != nil || err != nil {
		t.Fatalf("nothing saved: %v, %v", c, err)
	}
	if removed, err := Remove(); removed || err != nil {
		t.Fatalf("removing nothing: %v, %v", removed, err)
	}

	path, err := Save(&Credentials{APIKey: "pk_1", BaseURL: "http://localhost:8080", Account: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "credentials") {
		t.Errorf("saved in %s", path)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Errorf("file mode %o, want 600", info.Mode().Perm())
		}
		info, _ = os.Stat(dir)
		if info.Mode().Perm() != 0o700 {
			t.Errorf("directory mode %o, want 700", info.Mode().Perm())
		}
	}

	c, err := Load()
	if err != nil || c == nil || c.APIKey != "pk_1" || c.BaseURL != "http://localhost:8080" || c.Account != "acme" {
		t.Fatalf("loaded %+v, %v", c, err)
	}

	// Saving again replaces, keeps the mode, and leaves no temporary file.
	if _, err := Save(&Credentials{APIKey: "pk_2"}); err != nil {
		t.Fatal(err)
	}
	if c, _ := Load(); c.APIKey != "pk_2" || c.BaseURL != "" {
		t.Errorf("after the second save: %+v", c)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left in the directory: %v", entries)
	}

	if removed, err := Remove(); !removed || err != nil {
		t.Fatalf("remove: %v, %v", removed, err)
	}
	if c, _ := Load(); c != nil {
		t.Error("still there after Remove")
	}
}

// A file that is not valid is an error that does not quote the file.
func TestLoadBrokenFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDir, dir)
	if err := os.WriteFile(filepath.Join(dir, "credentials"), []byte(`{"api_key": "pk_secret_value`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load()
	if err == nil {
		t.Fatal("no error for a broken file")
	}
	if strings.Contains(err.Error(), "pk_secret_value") {
		t.Errorf("the error quotes the key: %v", err)
	}
}

// Without the override the file is in the user's configuration directory.
func TestDefaultDir(t *testing.T) {
	t.Setenv(EnvDir, "")
	base, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no user configuration directory here")
	}
	dir, err := Dir()
	if err != nil || dir != filepath.Join(base, "photos-sh") {
		t.Errorf("Dir() = %s, %v", dir, err)
	}
}
