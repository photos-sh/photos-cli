package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	photos "github.com/photos-sh/photos-go"

	"github.com/photos-sh/photos-cli/internal/output"
)

// Batch jobs: one operation on many images, in the background.

func jobsCommand() *Command {
	return &Command{
		Name:    "jobs",
		Group:   groupJobs,
		Summary: "run one operation on many images in the background",
		Long: `A job applies one operation to many images on the server and can outlive
this terminal: create it, wait for it (or give it a webhook), then download
the results.`,
		Examples: []string{
			"id=$(photos jobs create remove-background --param format=png photos/)",
			`photos jobs wait "$id" && photos jobs download "$id" --out-dir cutouts`,
		},
		Children: []*Command{
			jobsCreateCommand(),
			jobsListCommand(),
			jobsGetCommand(),
			jobsWaitCommand(),
			jobsDownloadCommand(),
			jobsDeleteCommand(),
		},
	}
}

// waitFlags say how a job is waited for.
type waitFlags struct {
	interval time.Duration
	timeout  time.Duration
}

func (w *waitFlags) register(fs *flag.FlagSet) {
	fs.DurationVar(&w.interval, "interval", photos.DefaultWaitInterval, "the `time` between two looks at the job")
	fs.DurationVar(&w.timeout, "wait-timeout", photos.DefaultWaitTimeout, "the longest `time` to wait for the job")
}

func (w *waitFlags) check() error {
	if w.interval <= 0 || w.timeout <= 0 {
		return usagef("--interval and --wait-timeout must be more than zero")
	}
	return nil
}

// errStillRunning is a job that did not complete within --wait-timeout.
var errStillRunning = errors.New("the job is still running")

// waitJob polls a job until every item was attempted, saying on standard
// error how far it is whenever that changes.
//
// The library's Jobs.Wait does the same but cannot report progress, so the
// loop is here, on the library's Jobs.Get.
func waitJob(ctx context.Context, env *Env, client *photos.Client, id string, w *waitFlags) (*photos.Job, error) {
	deadline := time.Now().Add(w.timeout)
	last := ""
	for {
		job, err := client.Jobs.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		progress := fmt.Sprintf("%s: %s, %d of %d done", job.ID, job.Status, job.Counts.Done+job.Counts.Failed, job.Counts.Total)
		if job.Counts.Failed > 0 {
			progress += fmt.Sprintf(" (%d failed)", job.Counts.Failed)
		}
		if progress != last {
			env.Printer.Note("%s", output.OneLine(progress))
			last = progress
		}
		if job.Completed() {
			return job, nil
		}
		if !time.Now().Add(w.interval).Before(deadline) {
			return job, fmt.Errorf("%w after %s (%s); `photos jobs wait %s` waits again", errStillRunning, w.timeout, job.Status, id)
		}
		timer := time.NewTimer(w.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return job, ctx.Err()
		case <-timer.C:
		}
	}
}

// jobFailures is the exit for a job whose items did not all succeed.
func jobFailures(env *Env, job *photos.Job) error {
	if job.Counts.Failed == 0 {
		return nil
	}
	env.Printer.Error("%d of %d items failed", job.Counts.Failed, job.Counts.Total)
	return &exitError{code: ExitFailed}
}

func when(unix int64) string {
	if unix == 0 {
		return ""
	}
	return time.Unix(unix, 0).Format("2006-01-02 15:04:05")
}

func countsText(c photos.JobCounts) string {
	return fmt.Sprintf("%d done, %d failed, %d pending", c.Done, c.Failed, c.Pending)
}

// printJob shows a job and its items to a person, or its JSON as it came.
func printJob(env *Env, job *photos.Job) error {
	p := env.Printer
	if p.JSON {
		return p.Raw(job.Raw)
	}
	p.Table(nil, [][]string{
		{"Job", job.ID},
		{"Operation", strings.TrimSpace(job.Operation + "  " + job.Params)},
		{"Status", job.Status + ": " + countsText(job.Counts)},
		{"Created", when(job.CreatedAt)},
		{"Finished", when(job.FinishedAt)},
	})
	if wh := job.Webhook; wh != nil {
		state := fmt.Sprintf("not delivered yet (%d attempts)", wh.Attempts)
		if wh.Delivered {
			state = "delivered"
		} else if wh.LastError != "" {
			state += ": " + wh.LastError
		}
		p.Table(nil, [][]string{{"Webhook", wh.URL + "  " + state}})
	}
	rows := make([][]string, 0, len(job.Items))
	for n, item := range job.Items {
		result := ""
		switch {
		case item.Error != nil:
			result = item.Error.Error()
		case item.Status == photos.ItemDone:
			result = item.ContentType + "  " + output.Size(item.Bytes)
			if w, h := item.Headers["x-image-width"], item.Headers["x-image-height"]; w != "" && h != "" {
				result += "  " + w + "x" + h
			}
		}
		rows = append(rows, []string{strconv.Itoa(n), item.Status, item.Input, result})
	}
	if len(rows) > 0 {
		p.Table([]string{"#", "STATUS", "INPUT", "RESULT"}, rows)
	}
	return nil
}

// parseParams reads --param name=value pairs. A name given twice becomes a
// list.
func parseParams(pairs []string) (map[string]any, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	params := map[string]any{}
	for _, pair := range pairs {
		name, value, ok := strings.Cut(pair, "=")
		if !ok || name == "" {
			return nil, usagef("--param %q: want name=value, such as --param format=png", pair)
		}
		// The API's names have underscores; the CLI's flags have dashes.
		name = strings.ReplaceAll(name, "-", "_")
		switch old := params[name].(type) {
		case nil:
			params[name] = value
		case string:
			params[name] = []string{old, value}
		case []string:
			params[name] = append(old, value)
		}
	}
	return params, nil
}

func jobsCreateCommand() *Command {
	var (
		params    stringList
		recipe    string
		webhook   string
		wait      bool
		recursive bool
		w         waitFlags
	)
	return &Command{
		Name:    "create",
		Summary: "start a job",
		Args:    "[flags] <operation> <image|directory|url|->...",
		Long: `Operations: run, remove-background, resize, upscale, ocr, barcodes, check,
fix, remove-text, label, enhance, crop, palette, watermark, expand,
vectorize, faces, blur-faces, id-photo, denoise, colorize, select,
restore-faces, caption. Inputs are files and directories to upload, and
http(s) URLs the service fetches itself. The operation's parameters are
given with --param, named as the matching command's flags (--param
width=800, --param max-bytes=300000). A parameter that repeats is given
more than once (--param point=420,310 --param point=440,300), and a switch
as true (--param zoom=true).

An item is one image, so a watermark job takes text only (no logo) and a
remove-background job no background image.

Prints the job's id, alone on standard output so that a script can keep it.
With --wait it waits for the job and prints it, and the exit code is 1 if an
item failed.`,
		Examples: []string{
			"photos jobs create resize --param width=1600 --param format=webp photos/",
			"photos jobs create run --param preset=marketplace --webhook-url https://example.com/hooks/photos https://example.com/1.jpg https://example.com/2.jpg",
			"photos jobs create remove-background --wait --json photos/ | jq '.counts'",
			"photos jobs create select --param point=420,310 --param point=440,300 --param crop=true photos/",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.Var(&params, "param", "a parameter of the operation, as `name=value`; repeatable")
			fs.StringVar(&recipe, "recipe", "", "a recipe `file` (TOML), for the operation run")
			fs.StringVar(&webhook, "webhook-url", "", "a `url` to send a signed job.completed event to when the job is done")
			fs.BoolVar(&wait, "wait", false, "wait for the job to complete, then print it")
			fs.BoolVar(&recursive, "recursive", false, "search directories below the ones named")
			w.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) == 0 {
				return usagef("name the operation, then the images: photos jobs create resize --param width=800 photos/")
			}
			if len(args) == 1 {
				return usagef("no inputs: name files, directories or URLs after the operation")
			}
			if err := w.check(); err != nil {
				return err
			}
			req := &photos.JobRequest{Operation: args[0], WebhookURL: webhook}
			var err error
			if req.Params, err = parseParams(params); err != nil {
				return err
			}
			if recipe != "" {
				text, err := os.ReadFile(recipe)
				if err != nil {
					return fmt.Errorf("--recipe %s: %w", recipe, pathError(err))
				}
				req.Recipe = string(text)
			}
			sources, urls, err := expandInputs(env, args[1:], inputOptions{recursive: recursive, urls: true})
			if err != nil {
				return err
			}
			req.URLs = urls
			for _, s := range sources {
				if s.err != nil {
					return fmt.Errorf("%s: %w", s.display, s.err)
				}
				req.Images = append(req.Images, s.input())
			}
			if len(req.URLs)+len(req.Images) == 0 {
				return errors.New("no images to send")
			}

			client, err := env.Client()
			if err != nil {
				return err
			}
			job, err := client.Jobs.Create(ctx, req)
			if err != nil {
				return err
			}
			p := env.Printer
			if !wait {
				if p.JSON {
					return p.Raw(job.Raw)
				}
				p.Data("%s", output.OneLine(job.ID))
				p.Note("Started a %s job with %d items. Follow it with: photos jobs wait %s", job.Operation, job.Counts.Total, job.ID)
				return nil
			}
			job, err = waitJob(ctx, env, client, job.ID, &w)
			if err != nil {
				return err
			}
			if err := printJob(env, job); err != nil {
				return err
			}
			return jobFailures(env, job)
		},
	}
}

func jobsListCommand() *Command {
	return &Command{
		Name:    "list",
		Summary: "list your recent jobs, newest first",
		Args:    "[flags]",
		Examples: []string{
			"photos jobs list",
			`photos jobs list --json | jq -r '.[] | select(.status != "completed") | .id'`,
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) > 0 {
				return usagef("jobs list takes no arguments")
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			jobs, err := client.Jobs.List(ctx)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				if jobs == nil {
					jobs = []photos.JobSummary{}
				}
				return p.JSONLine(jobs)
			}
			if len(jobs) == 0 {
				p.Note("No jobs.")
				return nil
			}
			rows := make([][]string, 0, len(jobs))
			for _, j := range jobs {
				rows = append(rows, []string{j.ID, j.Operation, j.Status, countsText(j.Counts), when(j.CreatedAt)})
			}
			p.Table([]string{"ID", "OPERATION", "STATUS", "ITEMS", "CREATED"}, rows)
			return nil
		},
	}
}

// oneJobID is the single job id of a command that takes one.
func oneJobID(args []string) (string, error) {
	if len(args) != 1 || args[0] == "" {
		return "", usagef("give one job id, as `photos jobs list` shows them")
	}
	return args[0], nil
}

func jobsGetCommand() *Command {
	return &Command{
		Name:    "get",
		Summary: "show a job: its status, progress and items",
		Args:    "[flags] <id>",
		Examples: []string{
			"photos jobs get job_5f0c2a",
			"photos jobs get job_5f0c2a --json | jq -r '.items[] | select(.status == \"failed\") | .input'",
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			id, err := oneJobID(args)
			if err != nil {
				return err
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			job, err := client.Jobs.Get(ctx, id)
			if err != nil {
				return err
			}
			return printJob(env, job)
		},
	}
}

func jobsWaitCommand() *Command {
	var w waitFlags
	return &Command{
		Name:    "wait",
		Summary: "wait for a job to complete, then show it",
		Args:    "[flags] <id>",
		Long: `Progress goes to standard error. The exit code is 0 when every item is
done, and 1 when an item failed or the job is still running after
--wait-timeout.`,
		Examples: []string{
			"photos jobs wait job_5f0c2a",
			"photos jobs wait job_5f0c2a --wait-timeout 30m --quiet && photos jobs download job_5f0c2a",
		},
		Flags: w.register,
		Run: func(ctx context.Context, env *Env, args []string) error {
			id, err := oneJobID(args)
			if err != nil {
				return err
			}
			if err := w.check(); err != nil {
				return err
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			job, err := waitJob(ctx, env, client, id, &w)
			if err != nil {
				return err
			}
			if !env.Printer.Quiet || env.Printer.JSON {
				if err := printJob(env, job); err != nil {
					return err
				}
			}
			return jobFailures(env, job)
		},
	}
}

// itemStem is a file name (without extension) for a job item's result, from
// the item's input: an upload's file name, or the last part of a URL.
func itemStem(input string, n int) string {
	name := input
	if isURL(input) {
		if u, err := url.Parse(input); err == nil {
			name = path.Base(u.Path)
		}
	}
	name = filepath.Base(filepath.FromSlash(strings.ReplaceAll(name, `\`, "/")))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, name)
	if strings.Trim(name, ". -") == "" {
		return fmt.Sprintf("item-%d", n)
	}
	return name
}

func jobsDownloadCommand() *Command {
	var (
		outDir   string
		force    bool
		parallel int
	)
	return &Command{
		Name:    "download",
		Summary: "save the result of every finished item of a job",
		Args:    "[flags] <id>",
		Long: `Each result is named after its input: shoe.jpg gives shoe.png for an image
result, shoe.svg for a vectorize job, shoe.json for a data result (ocr,
barcodes, check, label, palette, faces, caption). Two inputs with one name
give shoe.png and shoe-2.png. A file that exists is not replaced unless
--force is given. Each line says what the matching command would: the faces
hidden or restored, whether a subject was found, an id photo's findings.

Items that failed, or are not finished, are reported and make the exit code
1; the others are still saved.`,
		Examples: []string{
			"photos jobs download job_5f0c2a --out-dir results",
			"photos jobs download job_5f0c2a --json | jq -r 'select(.error) | .input'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&outDir, "out-dir", ".", "the `directory` to save into, created if missing")
			fs.BoolVar(&force, "force", false, "replace files that exist")
			fs.IntVar(&parallel, "parallel", 4, "how many `files` to fetch at once")
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			id, err := oneJobID(args)
			if err != nil {
				return err
			}
			if parallel < 1 {
				return usagef("--parallel must be at least 1")
			}
			if outDir == "" {
				return usagef("--out-dir cannot be empty")
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			job, err := client.Jobs.Get(ctx, id)
			if err != nil {
				return err
			}
			if len(job.Items) == 0 {
				return errors.New("the job has no items")
			}
			if !job.Completed() {
				env.Printer.Warn("%s is %s: only the finished items are saved", job.ID, job.Status)
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return pathError(err)
			}

			var names namer
			tasks := make([]task, 0, len(job.Items))
			for n, item := range job.Items {
				stem := names.claim(outDir, itemStem(item.Input, n))
				tasks = append(tasks, task{display: item.Input, item: &n, do: func(ctx context.Context) (*fileResult, error) {
					switch {
					case item.Error != nil:
						return nil, item.Error
					case item.Status != photos.ItemDone:
						return nil, fmt.Errorf("not finished (%s)", item.Status)
					}
					if ext := extensionFor(item.ContentType, ""); !force && ext != "" && exists(filepath.Join(outDir, stem+ext)) {
						return nil, fmt.Errorf("%s %w", filepath.Join(outDir, stem+ext), errExists)
					}
					got, err := client.Jobs.Item(ctx, job.ID, n)
					if err != nil {
						return nil, err
					}
					res := &fileResult{
						Item: &n, Input: item.Input, ContentType: got.ContentType, Bytes: len(got.Body),
						Output: filepath.Join(outDir, stem+extensionFor(got.ContentType, ".bin")),
					}
					if img := got.Image(); img != nil {
						res.Width, res.Height, res.Quality = img.Width, img.Height, img.Quality
						// crop always says; remove-background when a canvas
						// came out empty; the others never.
						res.SubjectFound = img.SubjectFound
					}
					// What the operation reports beside the image, when the
					// engine keeps it on job items.
					switch job.Operation {
					case photos.OperationFix:
						if fix, err := got.Fix(); err == nil {
							res.Passed, res.Findings = &fix.Passed, &fix.Findings
						}
					case photos.OperationRemoveText:
						if rt, err := got.RemoveText(); err == nil {
							res.Removed, res.Kept = &rt.Removed, &rt.Kept
						}
					case photos.OperationVectorize:
						if v, err := got.Vectorize(); err == nil {
							res.Paths = &v.Paths
						}
					case photos.OperationBlurFaces:
						if bf, err := got.BlurFaces(); err == nil {
							res.Faces = &bf.Faces
						}
					case photos.OperationRestoreFaces:
						if rf, err := got.RestoreFaces(); err == nil {
							res.Faces, res.FacesSkipped = &rf.Faces, &rf.FacesSkipped
						}
					case photos.OperationSelect:
						if sel, err := got.Select(); err == nil {
							res.Score = &sel.Score
						}
					case photos.OperationIDPhoto:
						if id, err := got.IDPhoto(); err == nil {
							res.Passed, res.Findings, res.HeadMm = &id.Passed, &id.Findings, &id.HeadMm
						}
					}
					if err := writeAtomic(res.Output, got.Body, force); err != nil {
						return nil, fmt.Errorf("%s %w", res.Output, err)
					}
					return res, nil
				}})
			}
			return runTasks(ctx, env, tasks, parallel, "items")
		},
	}
}

func jobsDeleteCommand() *Command {
	return &Command{
		Name:    "delete",
		Summary: "cancel a job if it is unfinished, and delete it and its results",
		Args:    "[flags] <id>...",
		Examples: []string{
			"photos jobs delete job_5f0c2a",
			`photos jobs list --json | jq -r '.[] | select(.status == "completed") | .id' | xargs photos jobs delete`,
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) == 0 {
				return usagef("give the id of the job to delete")
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			p := env.Printer
			var worst error
			for _, id := range args {
				if err := client.Jobs.Delete(ctx, id); err != nil {
					if errors.Is(err, context.Canceled) {
						return err
					}
					if worst == nil || ExitCode(err) > ExitCode(worst) {
						worst = err
					}
					p.Error("%s: %s", output.OneLine(id), describe(err))
					continue
				}
				if p.JSON {
					_ = p.JSONLine(map[string]any{"id": id, "deleted": true})
				} else {
					p.Info("Deleted %s", output.OneLine(id))
				}
			}
			if worst != nil {
				return &exitError{code: ExitCode(worst)}
			}
			return nil
		},
	}
}
