package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// fakeReleases stands in for GitHub's releases of photos-cli.
type fakeReleases struct {
	srv *httptest.Server
	// latest is the tag releases/latest redirects to.
	latest string
	// binaries are the asset of this machine per tag.
	binaries map[string][]byte
	// badSum makes checksums.txt list a wrong SHA-256.
	badSum bool

	mu   sync.Mutex
	hits []string
}

func newFakeReleases(t *testing.T, latest string, tags ...string) *fakeReleases {
	t.Helper()
	f := &fakeReleases{latest: latest, binaries: map[string][]byte{}}
	for _, tag := range tags {
		f.binaries[tag] = []byte("#!/bin/sh\necho " + tag + "\n")
	}
	asset, err := assetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits = append(f.hits, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/"+f.latest, http.StatusFound)
			return
		}
		for tag, bin := range f.binaries {
			switch r.URL.Path {
			case "/releases/download/" + tag + "/" + asset:
				_, _ = w.Write(bin)
				return
			case "/releases/download/" + tag + "/checksums.txt":
				sum := sha256.Sum256(bin)
				if f.badSum {
					sum = sha256.Sum256([]byte("something else"))
				}
				// As sha256sum writes it, with another asset around it.
				_, _ = w.Write([]byte("0000  photos-plan9-mips\n" + hex.EncodeToString(sum[:]) + " *" + asset + "\n"))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(f.srv.Close)
	t.Setenv(EnvReleasesURL, f.srv.URL+"/releases/")
	return f
}

func (f *fakeReleases) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hits...)
}

// installed is a stand-in for the running binary, in a directory of its
// own; executable points at it for the test.
func installed(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "photos")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := executable
	executable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { executable = old })
	return exe
}

// runAs runs the CLI as the given version.
func runAs(t *testing.T, version string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	env := &Env{Version: version, Out: &out, Err: &errOut, In: strings.NewReader("")}
	code := Main(context.Background(), env, args)
	return result{stdout: out.String(), stderr: errOut.String(), code: code}
}

func content(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// leftovers are files in dir other than the binary: a temporary download
// that was not cleaned up.
func leftovers(t *testing.T, exe string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(exe))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.Name() != filepath.Base(exe) {
			names = append(names, e.Name())
		}
	}
	return names
}

func TestUpdateReplacesTheBinary(t *testing.T) {
	rel := newFakeReleases(t, "v1.2.0", "v1.2.0")
	exe := installed(t)

	res := runAs(t, "1.1.0", "update")
	if res.code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	if got := content(t, exe); got != string(rel.binaries["v1.2.0"]) {
		t.Errorf("the binary is %q", got)
	}
	if info, err := os.Stat(exe); err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("mode: %v %v", info.Mode(), err)
	}
	if !strings.Contains(res.stdout, "from 1.1.0 to 1.2.0") {
		t.Errorf("stdout: %q", res.stdout)
	}
	if !strings.Contains(res.stderr, "Downloading photos-") {
		t.Errorf("stderr: %q", res.stderr)
	}
	if left := leftovers(t, exe); len(left) > 0 {
		t.Errorf("left behind: %v", left)
	}
}

func TestUpdateJSON(t *testing.T) {
	newFakeReleases(t, "v1.2.0", "v1.2.0")
	exe := installed(t)

	res := runAs(t, "1.1.0", "update", "--json")
	if res.code != 0 {
		t.Fatalf("exit %d: %s", res.code, res.stderr)
	}
	line := jsonLines(t, res.stdout)[0]
	if line["current"] != "1.1.0" || line["latest"] != "1.2.0" || line["updated"] != true || line["path"] != exe {
		t.Errorf("json: %v", line)
	}
}

func TestUpdateChecksumMismatchChangesNothing(t *testing.T) {
	rel := newFakeReleases(t, "v1.2.0", "v1.2.0")
	rel.badSum = true
	exe := installed(t)

	res := runAs(t, "1.1.0", "update")
	if res.code != ExitFailed {
		t.Fatalf("exit %d", res.code)
	}
	if !strings.Contains(res.stderr, "checksum mismatch") || !strings.Contains(res.stderr, "Nothing was changed") {
		t.Errorf("stderr: %q", res.stderr)
	}
	if got := content(t, exe); got != "old binary" {
		t.Errorf("the binary was changed: %q", got)
	}
	if left := leftovers(t, exe); len(left) > 0 {
		t.Errorf("left behind: %v", left)
	}
}

func TestUpdateWithoutTheAssetChangesNothing(t *testing.T) {
	// releases/latest names a tag that has no binaries.
	newFakeReleases(t, "v1.3.0", "v1.2.0")
	exe := installed(t)

	res := runAs(t, "1.1.0", "update")
	if res.code != ExitFailed || !strings.Contains(res.stderr, "could not download") {
		t.Fatalf("exit %d: %s", res.code, res.stderr)
	}
	if got := content(t, exe); got != "old binary" {
		t.Errorf("the binary was changed: %q", got)
	}
	if left := leftovers(t, exe); len(left) > 0 {
		t.Errorf("left behind: %v", left)
	}
}

func TestUpdateWhenUpToDate(t *testing.T) {
	rel := newFakeReleases(t, "v1.2.0", "v1.2.0")
	exe := installed(t)

	for _, version := range []string{"1.2.0", "1.3.0"} {
		res := runAs(t, version, "update")
		if res.code != 0 {
			t.Fatalf("%s: exit %d: %s", version, res.code, res.stderr)
		}
		if got := content(t, exe); got != "old binary" {
			t.Errorf("%s: the binary was changed", version)
		}
	}
	for _, hit := range rel.requests() {
		if strings.Contains(hit, "/download/") {
			t.Errorf("downloaded while up to date: %s", hit)
		}
	}
}

func TestUpdateCheck(t *testing.T) {
	rel := newFakeReleases(t, "v1.2.0", "v1.2.0")
	exe := installed(t)

	res := runAs(t, "1.1.0", "update", "--check")
	if res.code != 0 || !strings.Contains(res.stdout, "1.2.0 is available; this is 1.1.0") {
		t.Errorf("exit %d: %q", res.code, res.stdout)
	}
	res = runAs(t, "1.2.0", "update", "--check", "--quiet")
	if res.code != 0 || !strings.Contains(res.stdout, "1.2.0 is the latest release, and this is it") {
		t.Errorf("exit %d: %q", res.code, res.stdout)
	}
	res = runAs(t, "1.1.0", "update", "--check", "--json")
	line := jsonLines(t, res.stdout)[0]
	if line["current"] != "1.1.0" || line["latest"] != "1.2.0" || line["newer"] != true {
		t.Errorf("json: %v", line)
	}
	if got := content(t, exe); got != "old binary" {
		t.Errorf("--check changed the binary")
	}
	for _, hit := range rel.requests() {
		if hit != "HEAD /releases/latest" {
			t.Errorf("--check asked for more than the latest tag: %s", hit)
		}
	}
}

func TestUpdateDevelopmentBuild(t *testing.T) {
	rel := newFakeReleases(t, "v1.2.0", "v1.2.0")
	exe := installed(t)

	for _, version := range []string{"dev", "1.1.0-3-gabc1234-dirty"} {
		res := runAs(t, version, "update")
		if res.code != ExitFailed || !strings.Contains(res.stderr, "development build") || !strings.Contains(res.stderr, "--force") {
			t.Errorf("%s: exit %d: %s", version, res.code, res.stderr)
		}
	}
	if got := content(t, exe); got != "old binary" {
		t.Fatalf("replaced a development build without --force")
	}
	if res := runAs(t, "dev", "update", "--force"); res.code != 0 {
		t.Fatalf("--force: exit %d: %s", res.code, res.stderr)
	}
	if got := content(t, exe); got != string(rel.binaries["v1.2.0"]) {
		t.Errorf("--force did not replace it: %q", got)
	}
}

func TestUpdateTo(t *testing.T) {
	rel := newFakeReleases(t, "v1.2.0", "v1.0.0", "v1.2.0")
	exe := installed(t)

	res := runAs(t, "1.2.0", "update", "--to", "1.0.0")
	if res.code != 0 {
		t.Fatalf("exit %d: %s", res.code, res.stderr)
	}
	if got := content(t, exe); got != string(rel.binaries["v1.0.0"]) {
		t.Errorf("the binary is %q", got)
	}
	for _, hit := range rel.requests() {
		if strings.Contains(hit, "latest") {
			t.Errorf("--to asked for the latest release: %s", hit)
		}
	}

	if res := runAs(t, "1.2.0", "update", "--to", "next"); res.code != ExitUsage {
		t.Errorf("--to next: exit %d", res.code)
	}
	if res := runAs(t, "1.2.0", "update", "--to", "v1.0.0", "--check"); res.code != ExitUsage {
		t.Errorf("--to with --check: exit %d", res.code)
	}
}

func TestUpdateFollowsASymlink(t *testing.T) {
	rel := newFakeReleases(t, "v1.2.0", "v1.2.0")
	exe := installed(t)
	link := filepath.Join(t.TempDir(), "photos")
	if err := os.Symlink(exe, link); err != nil {
		t.Skip(err)
	}
	executable = func() (string, error) { return link, nil }

	if res := runAs(t, "1.1.0", "update"); res.code != 0 {
		t.Fatalf("exit %d: %s", res.code, res.stderr)
	}
	if got := content(t, exe); got != string(rel.binaries["v1.2.0"]) {
		t.Errorf("the file behind the link is %q", got)
	}
	if target, err := os.Readlink(link); err != nil || target != exe {
		t.Errorf("the link is now %q, %v", target, err)
	}
}

func TestUpdateInADirectoryNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions do not stop this user")
	}
	newFakeReleases(t, "v1.2.0", "v1.2.0")
	exe := installed(t)
	dir := filepath.Dir(exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	res := runAs(t, "1.1.0", "update")
	if res.code != ExitFailed || !strings.Contains(res.stderr, "cannot write to "+dir) || !strings.Contains(res.stderr, "sudo photos update") {
		t.Errorf("exit %d: %s", res.code, res.stderr)
	}
	if got := content(t, exe); got != "old binary" {
		t.Errorf("the binary was changed")
	}
}

func TestVersionsAndChecksums(t *testing.T) {
	for _, c := range []struct {
		a, b string
		cmp  int
		ok   bool
	}{
		{"1.0.0", "1.1.0", -1, true},
		{"1.10.0", "1.9.9", 1, true},
		{"v2.0.0", "2.0.0", 0, true},
		{"dev", "1.0.0", 0, false},
		{"1.0.0-3-gabc", "1.0.0", 0, false},
		{"1.0", "1.0.0", 0, false},
	} {
		if cmp, ok := compareVersions(c.a, c.b); cmp != c.cmp || ok != c.ok {
			t.Errorf("compareVersions(%q, %q) = %d, %v", c.a, c.b, cmp, ok)
		}
	}

	for goos, want := range map[string]string{"linux": "photos-linux-arm64", "windows": "photos-windows-arm64.exe"} {
		if got, err := assetName(goos, "arm64"); err != nil || got != want {
			t.Errorf("assetName(%s) = %q, %v", goos, got, err)
		}
	}
	if _, err := assetName("freebsd", "amd64"); err == nil {
		t.Error("an asset for freebsd")
	}

	sums := "aaa  photos-linux-amd64\nbbb *photos-darwin-arm64\n"
	if sum, ok := checksumFor(sums, "photos-darwin-arm64"); !ok || sum != "bbb" {
		t.Errorf("checksumFor: %q %v", sum, ok)
	}
	if _, ok := checksumFor(sums, "photos-linux"); ok {
		t.Error("checksumFor matched a prefix")
	}
}
