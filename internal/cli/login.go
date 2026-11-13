package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	photos "github.com/photos-sh/photos-go"

	"github.com/photos-sh/photos-cli/internal/config"
	"github.com/photos-sh/photos-cli/internal/terminal"
)

// readKey reads the API key: typed at a prompt that does not show it, or as
// the first line of standard input when that is a pipe or a file.
func readKey(ctx context.Context, env *Env) (string, error) {
	var (
		key string
		err error
	)
	if f, ok := env.In.(*os.File); ok && terminal.IsTerminal(f) {
		fmt.Fprint(env.Err, "API key (it is not shown): ")
		key, err = terminal.ReadSecret(ctx, f)
		if errors.Is(err, terminal.ErrNoEcho) {
			// Said, because a prompt that silently shows a pasted key is
			// worse than one that admits it.
			fmt.Fprint(env.Err, "\nThis terminal cannot hide what you type; the key will be visible.\nAPI key: ")
			key, err = terminal.ReadLine(f)
		} else {
			fmt.Fprintln(env.Err) // the Enter was not shown either
		}
	} else {
		key, err = terminal.ReadLine(env.In)
	}
	if errors.Is(err, context.Canceled) {
		return "", err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", usagef("no key given: paste it at the prompt, or pipe it in (photos login < key.txt)")
	}
	if strings.ContainsAny(key, " \t\r\n") {
		return "", usagef("that does not look like an API key: it contains a space")
	}
	return key, nil
}

func loginCommand() *Command {
	return &Command{
		Name:    "login",
		Group:   groupOther,
		Summary: "save your API key on this machine",
		Args:    "[flags]",
		Long: `Asks for your API key (from your organisation's API keys page in the Photos.sh
console) without showing it, checks it with the API, and saves it in a file
only you can read:

    Linux    ~/.config/photos-sh/credentials
    macOS    ~/Library/Application Support/photos-sh/credentials
    Windows  %AppData%\photos-sh\credentials

PHOTOS_CONFIG_DIR names another directory. When standard input is not a
terminal, the key is read from it, so a script can pipe it in. The key is
never taken as an argument: it would land in your shell's history and in
the process list.

Where PHOTOS_API_KEY is set, it is used instead of the saved key, and
there is no need to log in. With --base-url (or PHOTOS_BASE_URL) the key is
checked against that API and saved for it.`,
		Examples: []string{
			"photos login",
			"photos login < ~/secrets/photos-key.txt",
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) > 0 {
				// The argument is not repeated: it may be the key.
				return usagef("the key is not taken as an argument (it would be kept in your shell's history): run `photos login` and paste it, or pipe it in")
			}
			key, err := readKey(ctx, env)
			if err != nil {
				return err
			}
			baseURL := env.baseURL(nil)
			client, err := env.newClient(key, baseURL)
			if err != nil {
				return err
			}
			acc, err := client.Account(ctx)
			if err != nil {
				if ExitCode(err) == ExitAuth {
					env.Printer.Error("%s", describe(err))
					fmt.Fprintln(env.Err, "The key was refused; nothing was saved.")
					return &exitError{code: ExitAuth}
				}
				return err
			}
			creds := &config.Credentials{APIKey: key, Account: acc.Account}
			if strings.TrimRight(baseURL, "/") != photos.DefaultBaseURL {
				creds.BaseURL = baseURL
			}
			path, err := config.Save(creds)
			if err != nil {
				return err
			}

			p := env.Printer
			if p.JSON {
				_ = p.JSONLine(map[string]any{"account": acc.Account, "key": maskKey(key), "saved": path})
			} else {
				who := ""
				if acc.Account != "" {
					who = " as " + acc.Account
				}
				p.Info("Signed in%s (key %s). Saved in %s", who, maskKey(key), path)
			}
			if env := strings.TrimSpace(os.Getenv(EnvAPIKey)); env != "" && env != key {
				p.Warn("%s is set and is used instead of the saved key", EnvAPIKey)
			}
			return nil
		},
	}
}

func logoutCommand() *Command {
	return &Command{
		Name:     "logout",
		Group:    groupOther,
		Summary:  "remove the saved API key",
		Args:     "[flags]",
		Long:     "Deletes the file `photos login` wrote. The key itself stays valid: revoke it in the Photos.sh console if it should stop working.",
		Examples: []string{"photos logout"},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) > 0 {
				return usagef("logout takes no arguments")
			}
			removed, err := config.Remove()
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				_ = p.JSONLine(map[string]any{"removed": removed})
			} else if removed {
				p.Info("Signed out: the saved key is removed.")
			} else {
				p.Info("No key was saved.")
			}
			if os.Getenv(EnvAPIKey) != "" {
				p.Warn("%s is still set, and commands will use it", EnvAPIKey)
			}
			return nil
		},
	}
}
