package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	photos "github.com/photos-sh/photos-go"

	"github.com/photos-sh/photos-cli/internal/config"
	"github.com/photos-sh/photos-cli/internal/output"
	"github.com/photos-sh/photos-cli/internal/terminal"
)

// Exit codes. Documented in README.md and USAGE.md, and stable: a script
// may rely on them.
const (
	// ExitOK: everything asked for was done.
	ExitOK = 0
	// ExitFailed: a file or the request failed.
	ExitFailed = 1
	// ExitUsage: the command was wrong: an unknown flag, a missing argument.
	ExitUsage = 2
	// ExitAuth: not signed in, or the API refused the key.
	ExitAuth = 3
	// ExitPayment: out of credits, or the account is suspended.
	ExitPayment = 4
	// ExitUnavailable: the service is busy or cannot be reached, after the
	// retries.
	ExitUnavailable = 5
	// ExitInterrupted: stopped by Ctrl-C (128 + SIGINT, as shells report it).
	ExitInterrupted = 130
)

// The environment variables the CLI reads.
const (
	EnvAPIKey  = "PHOTOS_API_KEY"
	EnvBaseURL = "PHOTOS_BASE_URL"
)

// ErrNotSignedIn means there is no key: none in PHOTOS_API_KEY and none
// saved by `photos login`.
var ErrNotSignedIn = errors.New("not signed in")

// exitError carries an exit code for failures that were already reported,
// one line per file.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// Env is what every command is handed: where to write, and how to reach the
// API.
type Env struct {
	// Version is this binary's version.
	Version string

	Out io.Writer
	Err io.Writer
	In  io.Reader

	// Printer writes results; set once the flags are parsed.
	Printer *output.Printer

	// Global flag values.
	flagJSON    bool
	flagQuiet   bool
	flagBaseURL string
	flagTimeout time.Duration
	flagRetries int
	registered  bool

	// path is the command being run, as words.
	path []string

	// client is built on the first use.
	client *photos.Client
}

// NewEnv builds an Env around the real process.
func NewEnv(version string) *Env {
	return &Env{Version: version, Out: os.Stdout, Err: os.Stderr, In: os.Stdin}
}

// registerGlobals adds the flags every command accepts. They are on every
// command's own FlagSet, so they work before the command and after it; a
// value given before it is kept as the default when registered again.
func (e *Env) registerGlobals(fs *flag.FlagSet) {
	if !e.registered {
		e.registered = true
		e.flagTimeout = photos.DefaultTimeout
		e.flagRetries = photos.DefaultRetries
	}
	fs.BoolVar(&e.flagJSON, "json", e.flagJSON, "machine-readable output on standard output: JSON, and nothing else")
	fs.BoolVar(&e.flagQuiet, "quiet", e.flagQuiet, "print nothing but errors and the data asked for")
	short(fs, "q", "quiet")
	fs.StringVar(&e.flagBaseURL, "base-url", e.flagBaseURL, "the API to talk to, as a `url` (default https://api.photos.sh, or $PHOTOS_BASE_URL)")
	fs.DurationVar(&e.flagTimeout, "timeout", e.flagTimeout, "how long one request may take, as a `duration` like 30s or 5m; 0 for no limit")
	fs.IntVar(&e.flagRetries, "retries", e.flagRetries, "how many `times` to try again when the service is busy or unreachable")
}

// setup settles the output once the flags are parsed.
func (e *Env) setup() error {
	if e.flagTimeout < 0 {
		return usagef("--timeout cannot be negative")
	}
	if e.flagRetries < 0 {
		return usagef("--retries cannot be negative")
	}
	e.Printer = output.New(e.Out, e.Err)
	e.Printer.JSON = e.flagJSON
	e.Printer.Quiet = e.flagQuiet
	return nil
}

// credential is the API key in use and where it came from.
type credential struct {
	key string
	// source is a few words for a person: where the key came from.
	source string
	// baseURL is the API the key was saved for, when it was saved and that
	// was not the default.
	baseURL string
}

// credential finds the key: PHOTOS_API_KEY wins, then the one `photos
// login` saved. It returns ErrNotSignedIn when there is neither.
func (e *Env) credential() (*credential, error) {
	if key := strings.TrimSpace(os.Getenv(EnvAPIKey)); key != "" {
		return &credential{key: key, source: "from " + EnvAPIKey}, nil
	}
	saved, err := config.Load()
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, ErrNotSignedIn
	}
	return &credential{key: saved.APIKey, source: "saved by photos login", baseURL: saved.BaseURL}, nil
}

// baseURL is the API in use: --base-url, then PHOTOS_BASE_URL, then the one
// the saved key belongs to, then the default.
func (e *Env) baseURL(cred *credential) string {
	if e.flagBaseURL != "" {
		return e.flagBaseURL
	}
	if v := os.Getenv(EnvBaseURL); v != "" {
		return v
	}
	if cred != nil && cred.baseURL != "" {
		return cred.baseURL
	}
	return photos.DefaultBaseURL
}

// newClient builds a client of the library for a key.
func (e *Env) newClient(key, baseURL string) (*photos.Client, error) {
	client, err := photos.New(key,
		photos.WithBaseURL(baseURL),
		photos.WithTimeout(e.flagTimeout),
		photos.WithRetries(e.flagRetries),
		photos.WithUserAgent("photos-cli/"+e.Version),
	)
	if err != nil {
		// What is left to go wrong is the base URL.
		return nil, usagef("%s", strings.TrimPrefix(err.Error(), "photos: "))
	}
	return client, nil
}

// Client returns the client for the key in use.
func (e *Env) Client() (*photos.Client, error) {
	if e.client != nil {
		return e.client, nil
	}
	cred, err := e.credential()
	if err != nil {
		return nil, err
	}
	client, err := e.newClient(cred.key, e.baseURL(cred))
	if err != nil {
		return nil, err
	}
	e.client = client
	return client, nil
}

// stdinIsTerminal reports whether a person is at the keyboard.
func (e *Env) stdinIsTerminal() bool {
	f, ok := e.In.(*os.File)
	return ok && terminal.IsTerminal(f)
}

// stdoutIsTerminal reports whether standard output is a screen.
func (e *Env) stdoutIsTerminal() bool {
	f, ok := e.Out.(*os.File)
	return ok && terminal.IsTerminal(f)
}

// ExitCode maps an error onto the code a script reads. One place, so the
// mapping is a table.
func ExitCode(err error) int {
	var (
		exit   *exitError
		usage  *UsageError
		apiErr *photos.Error
		conn   *photos.ConnectionError
	)
	switch {
	case err == nil:
		return ExitOK
	case errors.As(err, &exit):
		return exit.code
	case errors.As(err, &usage):
		return ExitUsage
	case errors.Is(err, context.Canceled):
		return ExitInterrupted
	case errors.Is(err, ErrNotSignedIn):
		return ExitAuth
	case errors.As(err, &apiErr):
		switch {
		case apiErr.Status == 401 || apiErr.Code == photos.CodeUnauthorized:
			return ExitAuth
		case apiErr.Status == 402 || apiErr.Code == photos.CodePaymentRequired,
			apiErr.Code == photos.CodeAccountSuspended:
			return ExitPayment
		case apiErr.Code == photos.CodeModelUnavailable:
			// A 503, but a permanent one: the tool is not on that server.
			return ExitFailed
		case apiErr.Code == photos.CodeBusy, apiErr.Status == 502, apiErr.Status == 503, apiErr.Status == 504:
			return ExitUnavailable
		}
		return ExitFailed
	case errors.As(err, &conn), errors.Is(err, context.DeadlineExceeded):
		return ExitUnavailable
	}
	return ExitFailed
}

// errorCode is a short stable word for an error, for --json: the API's own
// code for an API error, one of ours otherwise.
func errorCode(err error) string {
	var (
		apiErr *photos.Error
		conn   *photos.ConnectionError
		item   *photos.ItemError
	)
	switch {
	case errors.As(err, &apiErr):
		if apiErr.Code != "" {
			return apiErr.Code
		}
		return fmt.Sprintf("http_%d", apiErr.Status)
	case errors.As(err, &item) && item.Code != "":
		return item.Code
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.As(err, &conn):
		if conn.Timeout {
			return "timeout"
		}
		return "unreachable"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrNotSignedIn):
		return "not_signed_in"
	case errors.Is(err, errExists):
		return "file_exists"
	}
	return "failed"
}

// httpStatus is the HTTP status of an error the API answered with, else 0.
func httpStatus(err error) int {
	var apiErr *photos.Error
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}

// describe is an error in words for a person, without the library's prefix.
func describe(err error) string {
	var (
		apiErr *photos.Error
		conn   *photos.ConnectionError
		item   *photos.ItemError
	)
	switch {
	case errors.As(err, &item):
		// A job item that failed: the error the endpoint would have given.
		if item.Code != "" {
			return fmt.Sprintf("%s (%s)", output.Clean(item.Message), item.Code)
		}
		return output.Clean(item.Message)
	case errors.As(err, &apiErr):
		msg := apiErr.Message
		if msg == "" {
			msg = "the API answered with an error"
		}
		if apiErr.Code != "" {
			return fmt.Sprintf("%s (%s, HTTP %d)", msg, apiErr.Code, apiErr.Status)
		}
		return fmt.Sprintf("%s (HTTP %d)", msg, apiErr.Status)
	case errors.As(err, &conn):
		if conn.Timeout {
			return fmt.Sprintf("the request timed out: %v", conn.Err)
		}
		return fmt.Sprintf("cannot reach the API: %v", conn.Err)
	case errors.Is(err, context.Canceled):
		return "interrupted"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, ErrNotSignedIn):
		return "not signed in: no key in " + EnvAPIKey + " and none saved"
	}
	return output.Clean(strings.TrimPrefix(err.Error(), "photos: "))
}

// hintedError is an error with what to do about it, said by the command
// that knows (the API's "not found" means different things per endpoint).
type hintedError struct {
	err  error
	hint string
}

func (e *hintedError) Error() string { return e.err.Error() }
func (e *hintedError) Unwrap() error { return e.err }

// withHint attaches a hint to an error; the error itself is unchanged.
func withHint(err error, hint string) error {
	return &hintedError{err: err, hint: hint}
}

// hint is what to do about an error, or "".
func hint(err error) string {
	var hinted *hintedError
	if errors.As(err, &hinted) {
		return hinted.hint
	}
	switch ExitCode(err) {
	case ExitAuth:
		if errors.Is(err, ErrNotSignedIn) {
			return "Sign in with `photos login`, or set " + EnvAPIKey + "."
		}
		return "The key was refused. Sign in again with `photos login`, or check " + EnvAPIKey + "."
	case ExitPayment:
		if errors.Is(err, photos.ErrAccountSuspended) {
			return "The account is suspended: see the Photos.sh console."
		}
		return "Out of credits: add some in the Photos.sh console. `photos account` shows the balance."
	}
	return ""
}

// Report writes an error the way the CLI reports errors, and returns its
// exit code. The message goes to standard error whatever --json says: a
// script reading standard output finds valid JSON or nothing.
func Report(env *Env, root *Command, err error) int {
	code := ExitCode(err)
	if err == nil {
		return code
	}
	var exit *exitError
	if errors.As(err, &exit) {
		return code // already reported, file by file
	}

	printer := env.Printer
	if printer == nil {
		printer = output.New(env.Out, env.Err)
	}
	printer.Error("%s", output.Clean(describe(err)))

	var usage *UsageError
	if errors.As(err, &usage) {
		path := usage.Path
		if path == nil {
			path = []string{root.Name}
		}
		if cmd, _, ferr := find(root, path[1:]); ferr == nil {
			fmt.Fprintf(env.Err, "\nUsage:\n  %s\n", usageLine(cmd, path))
		}
		fmt.Fprintf(env.Err, "\nRun `%s` for more.\n", strings.Join(append([]string{root.Name, "help"}, path[1:]...), " "))
		return code
	}
	if h := hint(err); h != "" {
		fmt.Fprintln(env.Err, h)
	}
	return code
}

// Main runs the CLI and returns the exit code.
func Main(ctx context.Context, env *Env, args []string) int {
	root := Root()
	return Report(env, root, Execute(ctx, root, env, args))
}

// maskKey is a key as it may be shown: its last four characters.
func maskKey(key string) string {
	if len(key) < 12 {
		return "…"
	}
	return "…" + key[len(key)-4:]
}
