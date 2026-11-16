package cli

import (
	"context"

	photos "github.com/photos-sh/photos-go"
)

// libraryVersion is the version of the Go library this is built on.
const libraryVersion = photos.Version

// The headings of the root's help, in order of appearance in Root.
const (
	groupEdit    = "Edit a photo (image in, image out)"
	groupRead    = "Read a photo (image in, data out)"
	groupJobs    = "Batch jobs"
	groupAccount = "Lists and account"
	groupOther   = "This tool"
)

// Root is the whole command tree. Every call builds a new one.
func Root() *Command {
	root := &Command{
		Name:    "photos",
		Summary: "use the Photos.sh image tools from a terminal and from scripts",
		Args:    "<command> [flags] [arguments]",
		Long: `Remove backgrounds and stage the cutout, resize and compress, upscale,
enhance, crop, expand, watermark, denoise, colorize, erase objects and text,
select an object, hide or restore faces, frame identity photos, trace
logos into SVG, check photos against marketplace rules and fix them, and
read barcodes, text, labels, colours, faces and captions.

Sign in once with ` + "`photos login`" + `, or set PHOTOS_API_KEY. Every command takes
--json for output a script can read; progress and errors go to standard
error. Exit codes: 0 done, 1 a file or the request failed, 2 wrong usage,
3 not signed in or key refused, 4 out of credits or account suspended,
5 service busy or unreachable.`,
		Examples: []string{
			"photos remove-background shoe.jpg",
			"photos resize --width 1600 --format webp --out-dir web photos/",
			"photos check --marketplace amazon --json shoe.jpg | jq '.marketplace.passed'",
		},
	}
	root.Children = []*Command{
		removeBackgroundCommand(),
		resizeCommand(),
		upscaleCommand(),
		eraseCommand(),
		removeTextCommand(),
		enhanceCommand(),
		cropCommand(),
		expandCommand(),
		watermarkCommand(),
		denoiseCommand(),
		colorizeCommand(),
		selectCommand(),
		blurFacesCommand(),
		restoreFacesCommand(),
		idPhotoCommand(),
		vectorizeCommand(),
		runCommand(),
		fixCommand(),

		checkCommand(),
		barcodesCommand(),
		ocrCommand(),
		labelCommand(),
		identifyCommand(),
		paletteCommand(),
		facesCommand(),
		captionCommand(),

		jobsCommand(),
		webhookCommand(),

		presetsCommand(),
		marketplacesCommand(),
		idPhotosCommand(),
		accountCommand(),
		healthCommand(),

		loginCommand(),
		logoutCommand(),
		versionCommand(),
		updateCommand(),
		completionCommand(),
		helpCommand(root),
	}
	return root
}

func versionCommand() *Command {
	return &Command{
		Name:     "version",
		Group:    groupOther,
		Summary:  "print the version of this tool",
		Args:     "[flags]",
		Long:     "The same as `photos --version`. With --json, also the version of the Go library it is built on.",
		Examples: []string{"photos version"},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if env.Printer.JSON {
				return env.Printer.JSONLine(map[string]string{"version": env.Version, "library": libraryVersion})
			}
			env.Printer.Data("%s", env.Version)
			return nil
		},
	}
}

func helpCommand(root *Command) *Command {
	return &Command{
		Name:     "help",
		Group:    groupOther,
		Summary:  "show help for a command",
		Args:     "[<command>...]",
		Long:     "The same as the command with --help.",
		Examples: []string{"photos help resize", "photos help jobs create"},
		Run: func(ctx context.Context, env *Env, args []string) error {
			cmd, path, err := find(root, args)
			if err != nil {
				return &UsageError{Path: path, Err: err}
			}
			printHelp(env.Out, root, cmd, path)
			return nil
		},
	}
}
