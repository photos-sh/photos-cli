package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	photos "github.com/photos-sh/photos-go"
)

// The commands that take images and give images back.

// imagesArgs is their usage line.
const imagesArgs = "[flags] <image|directory|->..."

// filesLong is the part of their help that is the same for all of them.
const filesLong = `Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
<name>%s.<ext>, the extension being the result's format; --out-dir writes
<name>.<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.`

// fileFlags are the flags about where results go, shared by those commands.
type fileFlags struct {
	output    string
	outDir    string
	suffix    optionalString
	force     bool
	recursive bool
	parallel  int
}

func (f *fileFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.output, "output", "", "write the result to this `file`, or - for standard output (one input only)")
	short(fs, "o", "output")
	fs.StringVar(&f.outDir, "out-dir", "", "write the results into this `directory`, created if missing, as <name>.<ext>")
	fs.Var(&f.suffix, "suffix", "the `text` added to each name instead of the command's own; may be empty")
	fs.BoolVar(&f.force, "force", false, "replace files that exist")
	fs.BoolVar(&f.recursive, "recursive", false, "search directories below the ones named, keeping the layout in --out-dir")
	fs.IntVar(&f.parallel, "parallel", 4, "how many `files` to send at once")
}

// formatFlags registers --format and --quality, which most of them take.
func formatFlags(fs *flag.FlagSet, format *string, quality *int, formatDefault string) {
	fs.StringVar(format, "format", "", "output `format`: png, jpg, webp or avif (default: "+formatDefault+")")
	fs.IntVar(quality, "quality", 0, "JPEG, WebP or AVIF quality, `1-100`")
}

// imageOp is one of those commands, reduced to what differs.
type imageOp struct {
	// suffix is added to a result's name when it is written beside its
	// source: "-resized".
	suffix string
	// ext predicts the result's extension from the source's, so a file that
	// exists is refused before the request is paid for; "" when unknown.
	ext func(sourceExt string) string
	// call does the request. extra adds what the operation reports beside
	// the image; it may be nil.
	call func(ctx context.Context, c *photos.Client, in photos.Input) (img *photos.Image, extra func(*fileResult), err error)
}

// runImages is the whole of an image command after its flags.
func runImages(ctx context.Context, env *Env, op imageOp, f *fileFlags, args []string) error {
	if len(args) == 0 {
		return usagef("no input: name a file, a directory, or - for standard input")
	}
	if f.parallel < 1 {
		return usagef("--parallel must be at least 1")
	}
	if f.output != "" && f.outDir != "" {
		return usagef("-o names the file and --out-dir a directory: give one of them")
	}
	suffix := op.suffix
	if f.outDir != "" {
		suffix = ""
	}
	if f.suffix.set {
		suffix = f.suffix.value
	}
	if strings.ContainsAny(suffix, `/\`) {
		return usagef("--suffix is part of a file name and cannot contain a slash")
	}

	opts := inputOptions{recursive: f.recursive, skipDir: f.outDir}
	if f.output == "" && f.outDir == "" {
		opts.skipSuffix = suffix
	}
	sources, _, err := expandInputs(env, args, opts)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return fmt.Errorf("no images to process")
	}

	if f.output != "" && len(sources) != 1 {
		return usagef("-o names one file and there are %d inputs: use --out-dir", len(sources))
	}
	toStdout := f.output == "-"
	if f.output == "" && f.outDir == "" {
		for _, s := range sources {
			if !s.stdin {
				continue
			}
			if len(sources) > 1 {
				return usagef("standard input has no directory to write beside: use --out-dir")
			}
			toStdout = true // a filter: in on standard input, out on standard output
		}
	}
	if toStdout && env.flagJSON {
		return usagef("standard output cannot carry both the image and --json: give -o a file, or --out-dir")
	}
	if toStdout && env.stdoutIsTerminal() {
		return usagef("not writing an image to the terminal: give -o a file, or pipe the output")
	}

	client, err := env.Client()
	if err != nil {
		return err
	}

	var names namer
	tasks := make([]task, 0, len(sources))
	for _, s := range sources {
		var dir, stem string
		if !toStdout && f.output == "" {
			dir = filepath.Dir(s.path)
			if f.outDir != "" {
				dir = filepath.Join(f.outDir, s.subdir)
			}
			stem = names.claim(dir, s.stem()+suffix)
		}
		tasks = append(tasks, task{display: s.display, do: func(ctx context.Context) (*fileResult, error) {
			if s.err != nil {
				return nil, s.err
			}
			// Refuse a file that is in the way before spending the request.
			if !f.force && !toStdout {
				if f.output != "" && exists(f.output) {
					return nil, fmt.Errorf("%s %w", f.output, errExists)
				}
				if ext := op.ext(s.ext()); f.output == "" && ext != "" && exists(filepath.Join(dir, stem+ext)) {
					return nil, fmt.Errorf("%s %w", filepath.Join(dir, stem+ext), errExists)
				}
			}

			img, extra, err := op.call(ctx, client, s.input())
			if err != nil {
				return nil, err
			}
			res := &fileResult{
				Input: s.display, ContentType: img.ContentType,
				Width: img.Width, Height: img.Height, Quality: img.Quality, Bytes: len(img.Bytes),
			}
			if extra != nil {
				extra(res)
			}

			switch {
			case toStdout:
				res.Output = "-"
				return res, env.Printer.Bytes(img.Bytes)
			case f.output != "":
				res.Output = f.output
			default:
				fallback := s.ext()
				if fallback == "" {
					fallback = ".bin"
				}
				if f.outDir != "" {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						return nil, pathError(err)
					}
				}
				res.Output = filepath.Join(dir, stem+extensionFor(img.ContentType, fallback))
			}
			if err := writeAtomic(res.Output, img.Bytes, f.force); err != nil {
				return nil, fmt.Errorf("%s %w", res.Output, err)
			}
			return res, nil
		}})
	}
	return runTasks(ctx, env, tasks, f.parallel, "files")
}

// unknownExt is for operations whose output format the server decides.
func unknownExt(string) string { return "" }

func removeBackgroundCommand() *Command {
	var o photos.RemoveBackgroundOptions
	var f fileFlags
	var backgroundImage string
	return &Command{
		Name:    "remove-background",
		Aliases: []string{"bg-remove"},
		Group:   groupEdit,
		Summary: "cut the subject out of its background",
		Args:    imagesArgs,
		Long: `The cutout is a transparent PNG unless --format or --background say
otherwise.

--studio is for studio shots: a product on a plain, light backdrop, as in a
light tent. It also keeps what clearly stands out from the backdrop and
touches the product, such as the gift box, case or lid shown with a bottle,
or a pale box the model left half transparent. Shadows and the floor below
the product are left out. A photo without a plain light backdrop is cut out
as without it.

The cutout can be staged in the same call: on a canvas (--width and
--height, with --fill and --align), with a --shadow or a --reflection under
it, and with something behind it: a colour (--background), an image
(--background-image) or the photo's own background blurred (--blur); one of
the three at most. --mask takes none of these, and --crop goes with neither
a canvas nor --blur. When the service finds no subject in a photo (a close-up
of a box, a plain surface), the result is the photo as it was, with or
without a canvas, and its line says "no subject found".

` + fmt.Sprintf(filesLong, "-nobg"),
		Examples: []string{
			"photos remove-background shoe.jpg",
			"photos remove-background --background white --crop --padding 40 --format jpg --out-dir cutouts photos/",
			"photos remove-background --studio perfume.jpg",
			"photos remove-background --width 2000 --height 2000 --fill 0.8 --shadow contact --background white --format jpg shoe.jpg",
		},
		Flags: func(fs *flag.FlagSet) {
			formatFlags(fs, &o.Format, &o.Quality, "png")
			fs.StringVar(&o.Background, "background", "", "composite onto a `colour` (white, #rrggbb) instead of transparency; JPEG defaults to white")
			fs.BoolVar(&o.Crop, "crop", false, "crop to the subject")
			fs.IntVar(&o.Padding, "padding", 0, "margin around the subject when cropping, in `pixels`")
			fs.BoolVar(&o.Mask, "mask", false, "write the greyscale alpha mask instead of the cutout")
			fs.BoolVar(&o.Studio, "studio", false, "for a product on a plain, light backdrop: also keep what stands out from it and touches the product (a gift box, case or lid); shadows and the floor are left out")
			fs.IntVar(&o.Width, "width", 0, "place the subject on a canvas this wide, in `pixels`; needs --height")
			fs.IntVar(&o.Height, "height", 0, "the canvas's height in `pixels`; needs --width")
			fs.Float64Var(&o.Fill, "fill", 0, "how much of the canvas the subject may take in its tighter dimension, as a `fraction` from 0.05 to 1 (default: 0.85)")
			fs.StringVar(&o.Align, "align", "", "`where` the subject sits on the canvas: center, top or bottom (default: center)")
			fs.StringVar(&o.Shadow, "shadow", "", "a shadow under the subject, by `kind`: contact (soft, at its base), drop (the silhouette, offset) or none (default: none)")
			fs.BoolVar(&o.Reflection, "reflection", false, "add a fading mirror image under the subject; leave room below it (--fill under 1)")
			fs.Float64Var(&o.Blur, "blur", 0, "keep the photo's own background behind the subject, blurred by this `amount`, 1 (slight) to 100")
			fs.StringVar(&backgroundImage, "background-image", "", "an image `file` to put behind the subject, scaled to cover the result")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if backgroundImage != "" {
				in, err := fileInput("background-image", backgroundImage)
				if err != nil {
					return err
				}
				o.BackgroundImage = in
			}
			suffix := "-nobg"
			if o.Mask {
				suffix = "-mask"
			}
			return runImages(ctx, env, imageOp{
				suffix: suffix,
				ext: func(string) string {
					if o.Format == "" {
						return ".png"
					}
					return formatExtension(o.Format)
				},
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					img, err := c.RemoveBackground(ctx, in, &o)
					if err != nil {
						return nil, nil, err
					}
					// The API says so only when the photo has no subject.
					return img, func(r *fileResult) { r.SubjectFound = img.SubjectFound }, nil
				},
			}, &f, args)
		},
	}
}

func resizeCommand() *Command {
	var o photos.ResizeOptions
	var f fileFlags
	return &Command{
		Name:    "resize",
		Group:   groupEdit,
		Summary: "resize, convert and compress",
		Args:    imagesArgs,
		Long: `Give --width or --height to keep the proportions, or both with --fit.
Without either, the photo is only converted (--format) or compressed
(--quality, --max-bytes). No model is involved: this is fast.

--trim cuts away an even border first: whatever matches the top-left pixel,
or is transparent like it. --dpi writes a print resolution into a JPEG or
PNG without changing a pixel.

` + fmt.Sprintf(filesLong, "-resized"),
		Examples: []string{
			"photos resize --width 1600 --format webp --quality 82 shoe.jpg",
			"photos resize --width 800 --height 800 --fit cover --out-dir thumbs --recursive photos/",
			"cat shoe.jpg | photos resize --width 400 -o - - > shoe-400.jpg",
			"photos resize --trim --trim-tolerance 20 --dpi 300 scan.png",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.IntVar(&o.Width, "width", 0, "target width in `pixels`")
			fs.IntVar(&o.Height, "height", 0, "target height in `pixels`")
			fs.StringVar(&o.Fit, "fit", "", "with both sides given, `how` to fit: contain, cover or fill (default: contain)")
			fs.BoolVar(&o.Enlarge, "enlarge", false, "allow scaling up with contain")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			fs.IntVar(&o.MaxBytes, "max-bytes", 0, "lower the quality until the file fits in this many `bytes`")
			fs.StringVar(&o.Background, "background", "", "flatten transparency onto this `colour`")
			fs.BoolVar(&o.Trim, "trim", false, "cut away an even border first: whatever matches the top-left pixel, or is transparent like it")
			fs.Var(optionalInt{&o.TrimTolerance}, "trim-tolerance", "how far a pixel may differ from the border's colour and still be border, per channel, `0-255` (default: 10); needs --trim")
			fs.IntVar(&o.DPI, "dpi", 0, "the print resolution to write into a JPEG or PNG, `1-2400`; no pixel changes")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			return runImages(ctx, env, imageOp{
				suffix: "-resized",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					img, err := c.Resize(ctx, in, &o)
					return img, nil, err
				},
			}, &f, args)
		},
	}
}

func upscaleCommand() *Command {
	var o photos.UpscaleOptions
	var f fileFlags
	return &Command{
		Name:    "upscale",
		Group:   groupEdit,
		Summary: "enlarge with an AI model",
		Args:    imagesArgs,
		Long:    fmt.Sprintf(filesLong, "-upscaled"),
		Examples: []string{
			"photos upscale --scale 2 small.jpg",
			"photos upscale --format webp -o big.webp small.png",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.Float64Var(&o.Scale, "scale", 0, "the `factor`, 1 to 4 (default: 4)")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			return runImages(ctx, env, imageOp{
				suffix: "-upscaled",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					img, err := c.Upscale(ctx, in, &o)
					return img, nil, err
				},
			}, &f, args)
		},
	}
}

// parseRect reads the "x,y,width,height" of the flag named.
func parseRect(flagName, s string) (photos.Rect, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return photos.Rect{}, usagef("--%s %q: want x,y,width,height", flagName, s)
	}
	var n [4]int
	for i, part := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || v < 0 {
			return photos.Rect{}, usagef("--%s %q: want four whole numbers, x,y,width,height", flagName, s)
		}
		n[i] = v
	}
	return photos.Rect{X: n[0], Y: n[1], Width: n[2], Height: n[3]}, nil
}

// fileInput is the file a flag names (a mask, a logo, a background), for the
// library. It is looked at here so that a wrong name fails once, before any
// request, and not once per image.
func fileInput(flagName, path string) (photos.Input, error) {
	info, err := os.Stat(path)
	if err != nil {
		return photos.Input{}, fmt.Errorf("--%s %s: %w", flagName, path, pathError(err))
	}
	if info.IsDir() {
		return photos.Input{}, usagef("--%s %s is a directory", flagName, path)
	}
	return photos.File(path), nil
}

func eraseCommand() *Command {
	var o photos.EraseOptions
	var f fileFlags
	var rects stringList
	var mask string
	return &Command{
		Name:    "erase",
		Group:   groupEdit,
		Summary: "remove objects by painting over an area",
		Args:    imagesArgs,
		Long: `Say what to remove with --rect (repeatable), with --mask (an image the
size of the photo, white where to remove), or with both. ` + "`photos select --mask`" + `
makes such a mask of one object.

` + fmt.Sprintf(filesLong, "-erased"),
		Examples: []string{
			"photos erase --rect 40,60,200,120 room.jpg",
			"photos erase --mask room-mask.png --rect 0,0,80,80 -o clean.jpg room.jpg",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.Var(&rects, "rect", "an area to remove, as `x,y,w,h` in pixels from the top-left corner; repeatable")
			fs.StringVar(&mask, "mask", "", "a mask `file`: the size of the photo, white where to remove")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(rects) == 0 && mask == "" {
				return usagef("say what to erase: --rect x,y,w,h, --mask file, or both")
			}
			for _, r := range rects {
				rect, err := parseRect("rect", r)
				if err != nil {
					return err
				}
				o.Rects = append(o.Rects, rect)
			}
			if mask != "" {
				in, err := fileInput("mask", mask)
				if err != nil {
					return err
				}
				o.Mask = in
			}
			return runImages(ctx, env, imageOp{
				suffix: "-erased",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					img, err := c.Erase(ctx, in, &o)
					return img, nil, err
				},
			}, &f, args)
		},
	}
}

func removeTextCommand() *Command {
	var o photos.RemoveTextOptions
	var f fileFlags
	return &Command{
		Name:    "remove-text",
		Group:   groupEdit,
		Summary: "paint out text, watermarks and captions",
		Args:    imagesArgs,
		Long: `By default only text off the product goes (watermarks, captions, prices);
labels printed on the product stay. Each line of output says how many lines
of text were removed and how many kept.

` + fmt.Sprintf(filesLong, "-notext"),
		Examples: []string{
			"photos remove-text listing.jpg",
			`photos remove-text --region all --matching 'sale|€|www\.' --out-dir clean listings/`,
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&o.Region, "region", "", "`where` to remove text: background (off the product) or all (default: background)")
			fs.StringVar(&o.Matching, "matching", "", "remove only lines matching this case-insensitive `regexp`")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			return runImages(ctx, env, imageOp{
				suffix: "-notext",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					res, err := c.RemoveText(ctx, in, &o)
					if err != nil {
						return nil, nil, err
					}
					return &res.Image, func(r *fileResult) { r.Removed, r.Kept = &res.Removed, &res.Kept }, nil
				},
			}, &f, args)
		},
	}
}

// safeNamePart makes a preset or marketplace name usable in a file name.
func safeNamePart(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		}
		return '-'
	}, s)
}

func runCommand() *Command {
	var preset, recipe string
	var f fileFlags
	return &Command{
		Name:    "run",
		Group:   groupEdit,
		Summary: "apply a preset or a recipe of your own",
		Args:    "[flags] (--preset <name> | --recipe <file.toml>) <image|directory|->...",
		Long: `A preset is a built-in chain of steps (` + "`photos presets`" + ` lists them); a
recipe is the same thing written by you, as TOML. Either is one request a
photo, charged as one, however many steps it has. ` + "`photos presets <name>`" + `
prints a preset's recipe to start from; the steps and their options are in
the API reference under POST /v1/run.

A recipe's [barcodes] and [text] steps read the photo before any step
changes it. What they read is on the file's line (the codes, and the lines
of text under it) and, with --json, in "barcodes" and "text".

` + fmt.Sprintf(filesLong, "-<preset>"),
		Examples: []string{
			"photos run --preset marketplace shoe.jpg",
			"photos run --recipe shop.toml --out-dir ready --recursive photos/",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&preset, "preset", "", "the `name` of a built-in preset, such as marketplace")
			fs.StringVar(&recipe, "recipe", "", "a recipe `file`, as TOML")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if (preset == "") == (recipe == "") {
				return usagef("give --preset <name> or --recipe <file.toml>, one of them")
			}
			o := photos.RunOptions{Preset: preset}
			suffix := "-" + safeNamePart(preset)
			if recipe != "" {
				text, err := os.ReadFile(recipe)
				if err != nil {
					return fmt.Errorf("--recipe %s: %w", recipe, pathError(err))
				}
				if strings.TrimSpace(string(text)) == "" {
					return fmt.Errorf("--recipe %s: the file is empty", recipe)
				}
				o.Recipe = string(text)
				suffix = "-" + safeNamePart(strings.TrimSuffix(filepath.Base(recipe), filepath.Ext(recipe)))
			}
			return runImages(ctx, env, imageOp{
				suffix: suffix,
				ext:    unknownExt,
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					img, err := c.Run(ctx, in, &o)
					if err != nil {
						return nil, nil, err
					}
					return img, func(r *fileResult) { r.read(img) }, nil
				},
			}, &f, args)
		},
	}
}

func fixCommand() *Command {
	var marketplace string
	var f fileFlags
	return &Command{
		Name:    "fix",
		Group:   groupEdit,
		Summary: "make a photo meet a marketplace's rules",
		Args:    "[flags] --marketplace <name> <image|directory|->...",
		Long: `Marketplaces: amazon, ebay, etsy, shopify, google, allegro, zalando
(` + "`photos marketplaces`" + ` shows their rules). Each line of output says whether
the fixed photo passes, and the findings still present if it does not; that
is information, not a failure, and the exit code stays 0.

` + fmt.Sprintf(filesLong, "-<marketplace>"),
		Examples: []string{
			"photos fix --marketplace amazon shoe.jpg",
			"photos fix --marketplace etsy --out-dir etsy --json photos/ | jq -r 'select(.passed == false) | .input'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&marketplace, "marketplace", "", "the marketplace's `name`: amazon, ebay, etsy, shopify, google, allegro or zalando")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if marketplace == "" {
				return usagef("say which marketplace: --marketplace amazon")
			}
			return runImages(ctx, env, imageOp{
				suffix: "-" + safeNamePart(marketplace),
				ext:    unknownExt,
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					res, err := c.Fix(ctx, in, marketplace)
					if err != nil {
						return nil, nil, err
					}
					return &res.Image, func(r *fileResult) { r.Passed, r.Findings = &res.Passed, &res.Findings }, nil
				},
			}, &f, args)
		},
	}
}
