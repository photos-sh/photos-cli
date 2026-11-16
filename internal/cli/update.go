package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// EnvReleasesURL names where the releases are, for a mirror; the installer
// reads the same variable.
const EnvReleasesURL = "PHOTOS_RELEASES_URL"

// defaultReleases is where the releases are.
const defaultReleases = "https://github.com/photos-sh/photos-cli/releases"

// maxBinary is the most a release binary may be. They are about 10 MB; the
// limit only stops a wrong answer from filling the disk.
const maxBinary = 200 << 20

// executable is the binary that is running, the one an update replaces.
// A variable so the tests can point it at a file of their own.
var executable = os.Executable

// updateCommand replaces this binary with the latest release, as the
// installer would: the same release asset, checked against the same
// checksums.txt.
func updateCommand() *Command {
	var check, force bool
	var to string
	return &Command{
		Name:    "update",
		Group:   groupOther,
		Summary: "update this tool to the latest release",
		Args:    "[flags]",
		Long: `Downloads the build for this machine from the latest release on GitHub,
checks it against the release's checksums.txt and replaces the binary that
is running. Nothing is replaced if the download or the check fails.

When the binary is in a directory you cannot write to (/usr/local/bin, say),
run it with sudo, or run the installer again.

PHOTOS_RELEASES_URL names a mirror of the releases, as for the installer.`,
		Examples: []string{
			"photos update",
			"photos update --check",
			"photos update --to v1.0.0",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&check, "check", false, "only say whether there is a newer release; change nothing")
			fs.StringVar(&to, "to", "", "install this `release` (v1.0.0) instead of the latest, older ones included")
			fs.BoolVar(&force, "force", false, "install even when this is the same version, or a development build")
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) > 0 {
				return usagef("update takes no arguments")
			}
			if check && to != "" {
				return usagef("--check and --to cannot be used together")
			}
			u := &updater{env: env, releases: releasesURL()}
			return u.run(ctx, check, force, to)
		},
	}
}

// releasesURL is where the releases are, without a slash at the end.
func releasesURL() string {
	if v := strings.TrimSpace(os.Getenv(EnvReleasesURL)); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultReleases
}

// updater does one `photos update`.
type updater struct {
	env      *Env
	releases string
}

// updateResult is what --json says.
type updateResult struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	// Newer is whether Latest is newer than Current; with --check only.
	Newer *bool `json:"newer,omitempty"`
	// Updated is whether the binary was replaced; not with --check.
	Updated *bool  `json:"updated,omitempty"`
	Path    string `json:"path,omitempty"`
}

func (u *updater) run(ctx context.Context, check, force bool, to string) error {
	env := u.env
	current := env.Version

	var tag string
	if to != "" {
		if _, ok := parseVersion(to); !ok {
			return usagef("--to %q is not a release: give it as v1.2.3", to)
		}
		tag = "v" + strings.TrimPrefix(to, "v")
	} else {
		latest, err := u.latest(ctx)
		if err != nil {
			return err
		}
		tag = latest
	}
	target := strings.TrimPrefix(tag, "v")
	cmp, comparable := compareVersions(current, target)

	if check {
		newer := comparable && cmp < 0
		if env.Printer.JSON {
			return env.Printer.JSONLine(updateResult{Current: current, Latest: target, Newer: &newer})
		}
		switch {
		case !comparable:
			env.Printer.Data("%s is the latest release; this is a development build (%s).", target, current)
		case newer:
			env.Printer.Data("%s is available; this is %s. Run `photos update`.", target, current)
		default:
			env.Printer.Data("%s is the latest release, and this is it.", current)
		}
		return nil
	}

	exe, err := u.executable()
	if err != nil {
		return err
	}
	switch {
	case force:
	case !comparable:
		return withHint(fmt.Errorf("this is a development build (%s), not a release", current),
			"Run `photos update --force` to replace it with "+tag+" all the same.")
	case cmp == 0:
		return u.done(current, target, false, exe, current+" is the latest release, and this is it.")
	case cmp > 0 && to == "":
		return u.done(current, target, false, exe, fmt.Sprintf("This is %s, newer than the latest release (%s): nothing to do.", current, target))
	}

	asset, err := assetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	env.Printer.Note("Downloading %s (%s).", asset, tag)
	if err := u.install(ctx, tag, asset, exe); err != nil {
		return err
	}
	return u.done(current, target, true, exe, fmt.Sprintf("Updated %s from %s to %s.", exe, current, target))
}

// done reports the outcome.
func (u *updater) done(current, target string, updated bool, exe, message string) error {
	if u.env.Printer.JSON {
		return u.env.Printer.JSONLine(updateResult{Current: current, Latest: target, Updated: &updated, Path: exe})
	}
	u.env.Printer.Info("%s", message)
	return nil
}

// latest is the tag of the latest release: GitHub's releases/latest
// redirects to releases/tag/<tag>. The redirect is read, not followed, so
// there is no API call and no rate limit.
func (u *updater) latest(ctx context.Context) (string, error) {
	where := u.releases + "/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, where, nil)
	if err != nil {
		return "", err
	}
	client := u.client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach %s: %w", where, err)
	}
	_ = resp.Body.Close()
	location := resp.Header.Get("Location")
	if resp.StatusCode < 300 || resp.StatusCode > 399 || location == "" {
		return "", fmt.Errorf("could not find the latest release: %s answered %s", where, resp.Status)
	}
	tag := path.Base(strings.TrimRight(location, "/"))
	if _, ok := parseVersion(tag); !ok {
		return "", fmt.Errorf("could not find the latest release: %s points at %s", where, location)
	}
	return tag, nil
}

// executable is the file to replace, with symbolic links followed so the
// file itself is replaced rather than the link.
func (u *updater) executable() (string, error) {
	exe, err := executable()
	if err != nil {
		return "", fmt.Errorf("cannot tell where this binary is: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// install downloads the asset of the release and its checksums.txt, checks
// the one against the other and puts the binary in place of exe.
func (u *updater) install(ctx context.Context, tag, asset, exe string) error {
	dir := filepath.Dir(exe)
	// In the binary's own directory, so the swap at the end is a rename
	// within one filesystem: the binary is there whole or not at all.
	tmp, err := os.CreateTemp(dir, ".photos-update-*")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return withHint(fmt.Errorf("cannot write to %s", dir),
				"Run `sudo photos update`, or the installer again:\n  curl -fsSL https://raw.githubusercontent.com/photos-sh/photos-cli/main/installer.sh | bash")
		}
		return fmt.Errorf("cannot write a temporary file in %s: %w", dir, err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	download := u.releases + "/download/" + tag + "/"
	hash := sha256.New()
	if err := u.fetch(ctx, download+asset, io.MultiWriter(tmp, hash), maxBinary); err != nil {
		return fmt.Errorf("could not download %s: %w. Nothing was changed", download+asset, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not write %s: %w", tmp.Name(), err)
	}

	var sums strings.Builder
	if err := u.fetch(ctx, download+"checksums.txt", &sums, 1<<20); err != nil {
		return fmt.Errorf("could not download %s: %w. Nothing was changed", download+"checksums.txt", err)
	}
	want, ok := checksumFor(sums.String(), asset)
	if !ok {
		return fmt.Errorf("checksums.txt of %s does not list %s. Nothing was changed", tag, asset)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s: checksums.txt says %s, the download is %s. Nothing was changed", asset, want, got)
	}

	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if err := replaceFile(tmp.Name(), exe); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return withHint(fmt.Errorf("cannot replace %s", exe), "Run `sudo photos update`.")
		}
		return fmt.Errorf("cannot replace %s: %w", exe, err)
	}
	keep = true
	return nil
}

// fetch writes the body of a GET to w, failing on anything but 200 or on
// more than limit bytes.
func (u *updater) fetch(ctx context.Context, url string, w io.Writer, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := u.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the answer was %s", resp.Status)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("it is larger than %d bytes", limit)
	}
	return nil
}

// client is a plain HTTP client with the --timeout of the command line.
func (u *updater) client() *http.Client {
	return &http.Client{Timeout: u.env.flagTimeout}
}

// replaceFile puts src in place of dst. Windows does not let a running
// binary be overwritten, but does let it be renamed: it is moved aside to
// <name>.old first, and that is removed when it can be (on the next update).
func replaceFile(src, dst string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(src, dst)
	}
	old := dst + ".old"
	_ = os.Remove(old)
	if err := os.Rename(dst, old); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		_ = os.Rename(old, dst)
		return err
	}
	_ = os.Remove(old)
	return nil
}

// assetName is the release asset for a platform, as the installer and the
// Makefile name them: photos-linux-amd64, photos-windows-arm64.exe.
func assetName(goos, goarch string) (string, error) {
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return "", fmt.Errorf("no release is built for %s: there are builds for Linux, macOS and Windows", goos)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("no release is built for %s: there are builds for amd64 and arm64", goarch)
	}
	name := "photos-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name, nil
}

// checksumFor finds the SHA-256 of name in a checksums.txt: lines of
// "<sha256>  <name>", where sha256sum marks binary mode with a star before
// the name.
func checksumFor(sums, name string) (string, bool) {
	scanner := bufio.NewScanner(strings.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return fields[0], true
		}
	}
	return "", false
}

// parseVersion reads "1.2.3" or "v1.2.3"; anything after a "-" or "+" (a
// pre-release, build metadata) is ignored.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// compareVersions is -1, 0 or 1 as a is older than, the same as or newer
// than b; false when either is not a release version ("dev", or what `git
// describe` makes of a commit after a tag).
func compareVersions(a, b string) (int, bool) {
	if strings.Contains(a, "-") || strings.Contains(a, "+") {
		return 0, false
	}
	va, ok := parseVersion(a)
	if !ok {
		return 0, false
	}
	vb, ok := parseVersion(b)
	if !ok {
		return 0, false
	}
	for i := range va {
		switch {
		case va[i] < vb[i]:
			return -1, true
		case va[i] > vb[i]:
			return 1, true
		}
	}
	return 0, true
}
