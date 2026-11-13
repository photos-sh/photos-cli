package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	photos "github.com/photos-sh/photos-go"

	"github.com/photos-sh/photos-cli/internal/output"
)

// maxWebhookBody is more than any delivery is: a job with thousands of items.
const maxWebhookBody = 64 << 20

func webhookCommand() *Command {
	return &Command{
		Name:    "webhook",
		Group:   groupJobs,
		Summary: "verify a job's webhook delivery, and show the signing key",
		Long: `A job created with --webhook-url sends a signed job.completed event there
when it is done. ` + "`photos webhook secret`" + ` prints the key the signature is made
with; ` + "`photos webhook verify`" + ` checks a delivery against it.`,
		Children: []*Command{webhookVerifyCommand(), webhookSecretCommand()},
	}
}

func webhookVerifyCommand() *Command {
	var (
		secretEnv string
		signature string
		tolerance time.Duration
	)
	return &Command{
		Name:    "verify",
		Summary: "check a webhook delivery's signature",
		Args:    "[flags] --secret-env <NAME> --signature <header> [<file>|-]",
		Long: `Reads the delivery's body, exactly as it was received, from a file or from
standard input, and checks it against the Photos-Signature header and your
signing key. The key is read from the environment variable --secret-env
names, never from the command line. Needs no API key and no network.

Exit code 0: the delivery is genuine (the event is printed). 1: it is not,
or it is older than --tolerance.`,
		Examples: []string{
			`photos webhook verify --secret-env PHOTOS_WEBHOOK_SECRET --signature "$HTTP_PHOTOS_SIGNATURE" body.json`,
			`photos webhook verify --secret-env PHOTOS_WEBHOOK_SECRET --signature "t=1767225600,v1=5f0c…" --json < body.json | jq '.job.id'`,
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&secretEnv, "secret-env", "", "the `NAME` of the environment variable that holds your signing key")
			fs.StringVar(&signature, "signature", "", "the delivery's Photos-Signature `header`: t=<unix seconds>,v1=<hex>")
			fs.DurationVar(&tolerance, "tolerance", photos.DefaultWebhookTolerance, "how old a delivery may be, as a `duration`")
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			switch {
			case secretEnv == "":
				return usagef("name the environment variable that holds the signing key: --secret-env PHOTOS_WEBHOOK_SECRET")
			case signature == "":
				return usagef("give the delivery's Photos-Signature header: --signature 't=…,v1=…'")
			case len(args) > 1:
				return usagef("give one file with the body, or - for standard input")
			case tolerance <= 0:
				return usagef("--tolerance must be more than zero")
			}
			secret := os.Getenv(secretEnv)
			if secret == "" {
				return usagef("the environment variable %s is not set (it holds the signing key: `photos webhook secret`)", secretEnv)
			}

			var body []byte
			var err error
			if len(args) == 0 || args[0] == "-" {
				if env.stdinIsTerminal() {
					return usagef("the body is read from standard input, which is the keyboard: name a file, or pipe it in")
				}
				body, err = io.ReadAll(io.LimitReader(env.In, maxWebhookBody))
			} else {
				body, err = os.ReadFile(args[0])
			}
			if err != nil {
				return fmt.Errorf("reading the body: %w", pathError(err))
			}

			event, err := photos.VerifyWebhook(body, signature, secret, tolerance)
			if err != nil {
				var werr *photos.WebhookError
				if errors.As(err, &werr) {
					return fmt.Errorf("not verified: %s", werr.Reason)
				}
				return err
			}
			p := env.Printer
			if p.JSON {
				return p.Raw(event.Raw)
			}
			line := "verified: " + event.Type
			if job := event.Job; job != nil {
				line += fmt.Sprintf("  %s  %s: %s", job.ID, job.Status, countsText(job.Counts))
			}
			p.Info("%s", output.OneLine(line))
			return nil
		},
	}
}

func webhookSecretCommand() *Command {
	return &Command{
		Name:    "secret",
		Summary: "print the key your job webhooks are signed with",
		Args:    "[flags]",
		Long: `Prints the signing key, alone on standard output. It is a secret: keep it
where your webhook handler reads it, not in a script or a log.`,
		Examples: []string{
			"export PHOTOS_WEBHOOK_SECRET=$(photos webhook secret)",
			"photos webhook secret --json | jq -r '.scheme'",
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) > 0 {
				return usagef("webhook secret takes no arguments")
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			s, err := client.WebhookSecret(ctx)
			if err != nil {
				return err
			}
			if env.Printer.JSON {
				return env.Printer.JSONLine(s)
			}
			env.Printer.Data("%s", output.OneLine(s.Secret))
			return nil
		},
	}
}
