package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	photos "github.com/photos-sh/photos-go"
)

// The editing tools: more commands that take images and give images back.
// They are built like the ones in images.go, on runImages.

// simpleImage is the call of a command that reports nothing beside the
// image.
func simpleImage(do func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, error)) func(context.Context, *photos.Client, photos.Input) (*photos.Image, func(*fileResult), error) {
	return func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
		img, err := do(ctx, c, in)
		return img, nil, err
	}
}

func enhanceCommand() *Command {
	var o photos.EnhanceOptions
	var f fileFlags
	return &Command{
		Name:    "enhance",
		Group:   groupEdit,
		Summary: "fix light and colour",
		Args:    imagesArgs,
		Long: `Without settings the tones are stretched to the full range and a colour
cast is taken out. With any setting, only what is given is applied; add
--auto to keep the automatic part beside it. Transparency is kept. No model
is involved: it does not denoise, deblur or recover blown highlights.

` + fmt.Sprintf(filesLong, "-enhanced"),
		Examples: []string{
			"photos enhance shoe.jpg",
			"photos enhance --auto --saturation 15 --sharpen 30 --out-dir better photos/",
			"photos enhance --saturation -100 -o bw.jpg shoe.jpg",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.Var(optionalBool{&o.Auto}, "auto", "automatic levels and white balance; on when no other setting is given")
			fs.Var(optionalBool{&o.Levels}, "levels", "stretch the tones so the darkest pixels are black and the brightest white; --levels=false leaves it out of --auto")
			fs.Var(optionalBool{&o.WhiteBalance}, "white-balance", "take out a colour cast; --white-balance=false leaves it out of --auto")
			fs.Float64Var(&o.Brightness, "brightness", 0, "brightness, as an `amount` from -100 to 100")
			fs.Float64Var(&o.Contrast, "contrast", 0, "contrast, as an `amount` from -100 to 100")
			fs.Float64Var(&o.Saturation, "saturation", 0, "saturation, as an `amount` from -100 (black and white) to 100")
			fs.Float64Var(&o.Temperature, "temperature", 0, "colour temperature, as an `amount` from -100 (cooler, bluer) to 100 (warmer)")
			fs.Float64Var(&o.Sharpen, "sharpen", 0, "sharpening, as an `amount` from 0 to 100")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			return runImages(ctx, env, imageOp{
				suffix: "-enhanced",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: simpleImage(func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, error) {
					return c.Enhance(ctx, in, &o)
				}),
			}, &f, args)
		},
	}
}

func cropCommand() *Command {
	var o photos.CropOptions
	var f fileFlags
	return &Command{
		Name:    "crop",
		Group:   groupEdit,
		Summary: "crop to a shape around the subject",
		Args:    imagesArgs,
		Long: `Finds the subject and crops the photo to a shape around it, keeping the
background. Give the shape with --aspect, or with --width and --height,
which also scale the crop to exactly that size. Without --zoom the crop is
the largest window of the shape, moved to hold the subject; with it, the
smallest that holds the subject and its --padding.

With --focus face the faces are found instead, all of them as one area: a
portrait or an avatar is --focus face --zoom --padding 0.6. Each line of
output says whether a subject was found; when none is, the crop is centred.

` + fmt.Sprintf(filesLong, "-cropped"),
		Examples: []string{
			"photos crop --aspect 4:5 shoe.jpg",
			"photos crop --focus face --zoom --padding 0.6 --width 400 --height 400 --out-dir avatars team/",
			"photos crop --aspect 1:1 --json photos/ | jq -r 'select(.subject_found == false) | .input'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&o.Aspect, "aspect", "", "the `shape`, as width:height (4:5, 16:9, 1:1) or a number (1.5)")
			fs.IntVar(&o.Width, "width", 0, "scale the crop to this width in `pixels`; needs --height")
			fs.IntVar(&o.Height, "height", 0, "scale the crop to this height in `pixels`; needs --width")
			fs.BoolVar(&o.Zoom, "zoom", false, "crop tightly around the subject")
			fs.Var(optionalFloat{&o.Padding}, "padding", "room around the subject with --zoom, as a `fraction` of its longest side, 0 to 2 (default: 0.1)")
			fs.StringVar(&o.Focus, "focus", "", "`what` the crop is built around: subject or face (default: subject)")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			return runImages(ctx, env, imageOp{
				suffix: "-cropped",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					img, err := c.Crop(ctx, in, &o)
					if err != nil {
						return nil, nil, err
					}
					return img, func(r *fileResult) { r.SubjectFound = img.SubjectFound }, nil
				},
			}, &f, args)
		},
	}
}

func watermarkCommand() *Command {
	var o photos.WatermarkOptions
	var f fileFlags
	var logo string
	return &Command{
		Name:    "watermark",
		Group:   groupEdit,
		Summary: "add a logo or text",
		Args:    "[flags] (--text <text> | --logo <file>) <image|directory|->...",
		Long: `Puts --text or a --logo image on each photo: at one of nine positions, or
tiled across it with --tile. --scale is the mark's width as a percentage of
the photo's width, so photos of any size are marked alike. Text is drawn in
a bold sans-serif with Latin, Greek and Cyrillic letters; other scripts need
a logo. No model is involved.

` + fmt.Sprintf(filesLong, "-watermarked"),
		Examples: []string{
			`photos watermark --text "© Acme 2026" shoe.jpg`,
			"photos watermark --logo logo.png --position top-left --scale 15 --opacity 80 --out-dir marked photos/",
			`photos watermark --text PROOF --tile --opacity 20 -o proof.jpg shoe.jpg`,
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&o.Text, "text", "", "the `text` to draw, up to 200 characters")
			fs.StringVar(&logo, "logo", "", "an image `file` to put on the photo instead of text; a PNG with transparency works best")
			fs.StringVar(&o.Color, "color", "", "the text's `colour`: white, black or #rrggbb (default: white)")
			fs.StringVar(&o.Position, "position", "", "`where` the mark goes: top-left, top, top-right, left, center, right, bottom-left, bottom or bottom-right (default: bottom-right)")
			fs.Float64Var(&o.Scale, "scale", 0, "the mark's width as a `percentage` of the photo's width, 1 to 100 (default: 25)")
			fs.Float64Var(&o.Opacity, "opacity", 0, "how solid the mark is, as a `percentage` from 1 to 100 (default: 50)")
			fs.Var(optionalFloat{&o.Margin}, "margin", "distance from the edges as a `percentage` of the photo's shorter side, 0 to 50 (default: 3)")
			fs.BoolVar(&o.Tile, "tile", false, "repeat the mark across the whole photo; --position is then unused")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if (o.Text == "") == (logo == "") {
				return usagef("give --text <text> or --logo <file>, one of them")
			}
			if logo != "" {
				in, err := fileInput("logo", logo)
				if err != nil {
					return err
				}
				o.Watermark = in
			}
			return runImages(ctx, env, imageOp{
				suffix: "-watermarked",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: simpleImage(func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, error) {
					return c.Watermark(ctx, in, &o)
				}),
			}, &f, args)
		},
	}
}

func expandCommand() *Command {
	var o photos.ExpandOptions
	var f fileFlags
	return &Command{
		Name:    "expand",
		Group:   groupEdit,
		Summary: "add room around a photo",
		Args:    imagesArgs,
		Long: `Grows the canvas to a shape (--aspect, the photo staying in the centre), or
by pixels on each side. The new area continues the photo by inpainting,
which suits plain and softly textured backgrounds: a studio backdrop, a
table, a wall, sky. It does not invent content: a subject cut off at the
edge stays cut off, and a busy scene continues as a smear. --fill with a
colour makes a flat border instead, and needs no model. Each side can grow
by at most the photo's own width or height. Transparency is flattened onto
white.

` + fmt.Sprintf(filesLong, "-expanded"),
		Examples: []string{
			"photos expand --aspect 1:1 shoe.jpg",
			"photos expand --left 200 --right 200 --fill white --out-dir wide photos/",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&o.Aspect, "aspect", "", "the `shape` to reach, as width:height (1:1, 16:9) or a number; not with the sides")
			fs.IntVar(&o.Left, "left", 0, "`pixels` to add on the left")
			fs.IntVar(&o.Right, "right", 0, "`pixels` to add on the right")
			fs.IntVar(&o.Top, "top", 0, "`pixels` to add at the top")
			fs.IntVar(&o.Bottom, "bottom", 0, "`pixels` to add at the bottom")
			fs.StringVar(&o.Fill, "fill", "", "`what` fills the new area: inpaint, or a colour (white, black, #rrggbb) (default: inpaint)")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			sides := o.Left != 0 || o.Right != 0 || o.Top != 0 || o.Bottom != 0
			switch {
			case o.Aspect == "" && !sides:
				return usagef("say how far to expand: --aspect 1:1, or --left, --right, --top and --bottom in pixels")
			case o.Aspect != "" && sides:
				return usagef("--aspect names a shape and --left, --right, --top and --bottom add pixels: give one or the other")
			case o.Left < 0 || o.Right < 0 || o.Top < 0 || o.Bottom < 0:
				return usagef("--left, --right, --top and --bottom add pixels and cannot be negative")
			}
			return runImages(ctx, env, imageOp{
				suffix: "-expanded",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: simpleImage(func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, error) {
					return c.Expand(ctx, in, &o)
				}),
			}, &f, args)
		},
	}
}

func vectorizeCommand() *Command {
	var o photos.VectorizeOptions
	var f fileFlags
	return &Command{
		Name:    "vectorize",
		Group:   groupEdit,
		Summary: "trace a logo or a drawing into SVG",
		Args:    imagesArgs,
		Long: `Each area of one colour becomes a filled path, and the result is an .svg
file. It is for logos, icons and flat drawings: a photograph traces into
thousands of paths and a large file. Images over 2000 pixels on a side are
traced from a smaller copy. Each line of output says how many paths the SVG
has. No model is involved.

` + fmt.Sprintf(filesLong, "-vector"),
		Examples: []string{
			"photos vectorize logo.png",
			"photos vectorize --mode mono --curves polygon -o stamp.svg stamp.png",
			"photos vectorize --detail 5 --speckle 8 --out-dir svg icons/",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&o.Mode, "mode", "", "`how` to trace: color, or mono for one black shape of whatever is darker than mid grey (default: color)")
			fs.StringVar(&o.Curves, "curves", "", "the `kind` of outline: spline (smooth), polygon (straight segments) or pixel (every pixel step, for pixel art) (default: spline)")
			fs.IntVar(&o.Detail, "detail", 0, "bits kept per colour channel when grouping pixels, `1-8`; lower merges similar shades into fewer shapes (default: 8)")
			fs.Var(optionalInt{&o.Speckle}, "speckle", "drop areas smaller than this many `pixels` across, 0 to 100 (default: 4)")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			return runImages(ctx, env, imageOp{
				suffix: "-vector",
				ext:    func(string) string { return ".svg" },
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					res, err := c.Vectorize(ctx, in, &o)
					if err != nil {
						return nil, nil, err
					}
					return &res.Image, func(r *fileResult) { r.Paths = &res.Paths }, nil
				},
			}, &f, args)
		},
	}
}

func blurFacesCommand() *Command {
	var o photos.BlurFacesOptions
	var f fileFlags
	return &Command{
		Name:    "blur-faces",
		Group:   groupEdit,
		Summary: "hide the faces in a photo",
		Args:    imagesArgs,
		Long: `Finds every face and hides it inside an ellipse: blurred, pixelated
(--mode pixelate) or covered with a colour (--mode fill). Blur and
pixelation stop a person from recognising the face; only fill leaves nothing
of it.

Each line of output says how many faces were hidden. Check it before relying
on the result: a small face, a profile, or a face seen from behind or mostly
covered may not be found, and a face that was not found still shows. No face
found is information, not a failure: nothing is hidden, the line says so,
and the exit code stays 0.

` + fmt.Sprintf(filesLong, "-nofaces"),
		Examples: []string{
			"photos blur-faces street.jpg",
			"photos blur-faces --mode fill --color black --padding 0.3 --out-dir public photos/",
			"photos blur-faces --json --out-dir public photos/ | jq -r 'select(.faces == 0) | .input'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&o.Mode, "mode", "", "`how` to hide a face: blur, pixelate or fill (default: blur)")
			fs.Float64Var(&o.Strength, "strength", 0, "how strong the blur or how coarse the mosaic is, as an `amount` from 1 to 100 (default: 50)")
			fs.Var(optionalFloat{&o.Padding}, "padding", "how far past the face's box the hidden area reaches, as a `fraction` of the box, 0 to 1 (default: 0.15)")
			fs.StringVar(&o.Color, "color", "", "the `colour` for --mode fill: black, white or #rrggbb (default: black)")
			fs.Float64Var(&o.MinScore, "min-score", 0, "how sure the model must be that something is a face, as a `score` from 0.1 to 1; lower finds more faces and more things that are not (default: 0.6)")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			return runImages(ctx, env, imageOp{
				suffix: "-nofaces",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					res, err := c.BlurFaces(ctx, in, &o)
					if err != nil {
						return nil, nil, err
					}
					return &res.Image, func(r *fileResult) { r.Faces = &res.Faces }, nil
				},
			}, &f, args)
		},
	}
}

func idPhotoCommand() *Command {
	var o photos.IDPhotoOptions
	var f fileFlags
	return &Command{
		Name:    "id-photo",
		Group:   groupEdit,
		Summary: "frame a portrait for an identity document",
		Args:    imagesArgs,
		Long: `Levels the head, scales it to the document's head size and places it on a
photo of the document's size, as a JPEG at --dpi. Name a built-in --document
(` + "`photos id-photos`" + ` lists them), or give the four sizes yourself:
--width-mm, --height-mm, --head-min-mm and --head-max-mm; the result's name
then ends in -id.

The photo's own background is kept unless --background is given, because
passport authorities do not accept edited photos: the photo is only turned,
scaled and cropped. --background white (or document, the colour the document
asks for) cuts the person out and replaces the background, for uses where
that is allowed, such as a staff badge.

Each line of output gives the head's height and the findings: head_tilted,
not_facing_camera, head_cut_off, not_enough_room, low_resolution,
background_not_plain. The tool cannot judge expression, glasses, shadows,
focus or likeness, so "no findings" is not an approval. Findings are
information, not a failure, and the exit code stays 0; a photo with no face,
or more than one, is refused and fails.

` + fmt.Sprintf(filesLong, "-<document>"),
		Examples: []string{
			"photos id-photo --document us-passport portrait.jpg",
			"photos id-photo --width-mm 35 --height-mm 45 --head-min-mm 32 --head-max-mm 36 --dpi 600 -o visa.jpg portrait.jpg",
			"photos id-photo --document uk-passport --json portrait.jpg | jq '.findings'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&o.Document, "document", "", "the `name` of a built-in document, such as us-passport; not with the sizes")
			fs.Float64Var(&o.WidthMm, "width-mm", 0, "the photo's width in `mm`, 10 to 200")
			fs.Float64Var(&o.HeightMm, "height-mm", 0, "the photo's height in `mm`, 10 to 200")
			fs.Float64Var(&o.HeadMinMm, "head-min-mm", 0, "the smallest allowed head, chin to crown, in `mm`")
			fs.Float64Var(&o.HeadMaxMm, "head-max-mm", 0, "the largest allowed head, chin to crown, in `mm`")
			fs.Float64Var(&o.EyesMinMm, "eyes-min-mm", 0, "the eyes' smallest distance from the bottom edge in `mm`, for a document with such a rule; needs --eyes-max-mm")
			fs.Float64Var(&o.EyesMaxMm, "eyes-max-mm", 0, "the eyes' largest distance from the bottom edge in `mm`; needs --eyes-min-mm")
			fs.IntVar(&o.DPI, "dpi", 0, "pixels per inch of the result, `100-1200`, also written into the file (default: 300)")
			fs.StringVar(&o.Background, "background", "", "`what` is behind the person: keep (the photo's own), document (the colour the document asks for) or a colour (white, #rrggbb); anything but keep edits the photo (default: keep)")
			formatFlags(fs, &o.Format, &o.Quality, "jpg")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			sizes := o.WidthMm != 0 || o.HeightMm != 0 || o.HeadMinMm != 0 || o.HeadMaxMm != 0
			switch {
			case o.Document == "" && !sizes:
				return usagef("say which document: --document us-passport (`photos id-photos` lists them), or --width-mm, --height-mm, --head-min-mm and --head-max-mm")
			case o.Document != "" && (sizes || o.EyesMinMm != 0 || o.EyesMaxMm != 0):
				return usagef("--document brings its own sizes: give it, or the sizes in millimetres, not both")
			case o.Document == "" && (o.WidthMm == 0 || o.HeightMm == 0 || o.HeadMinMm == 0 || o.HeadMaxMm == 0):
				return usagef("without --document all four sizes are needed: --width-mm, --height-mm, --head-min-mm and --head-max-mm")
			}
			suffix := "-id"
			if o.Document != "" {
				suffix = "-" + safeNamePart(o.Document)
			}
			return runImages(ctx, env, imageOp{
				suffix: suffix,
				ext: func(string) string {
					if o.Format == "" {
						return ".jpg"
					}
					return formatExtension(o.Format)
				},
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					res, err := c.IDPhoto(ctx, in, &o)
					if err != nil {
						return nil, nil, err
					}
					return &res.Image, func(r *fileResult) {
						r.Passed, r.Findings, r.HeadMm = &res.Passed, &res.Findings, &res.HeadMm
					}, nil
				},
			}, &f, args)
		},
	}
}

func denoiseCommand() *Command {
	var o photos.DenoiseOptions
	var f fileFlags
	return &Command{
		Name:    "denoise",
		Group:   groupEdit,
		Summary: "remove noise and JPEG artefacts",
		Args:    imagesArgs,
		Long: `Removes sensor noise and the blocks and ringing of heavy JPEG compression.
It does not sharpen a blurred photo or add detail, and on a photo that is
already clean it takes away a little fine texture: lower --strength there,
or do not use it. Transparency is kept.

It is slow without a GPU (about half a minute a megapixel) and takes photos
up to 12 megapixels; give a large photo a longer --timeout.

` + fmt.Sprintf(filesLong, "-denoised"),
		Examples: []string{
			"photos denoise night.jpg",
			"photos denoise --strength 60 --timeout 10m --parallel 1 --out-dir clean photos/",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.Float64Var(&o.Strength, "strength", 0, "how much of the cleaned result replaces the photo, as a `percentage` from 1 to 100; 50 is half-way back to the original (default: 100)")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			return runImages(ctx, env, imageOp{
				suffix: "-denoised",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: simpleImage(func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, error) {
					return c.Denoise(ctx, in, &o)
				}),
			}, &f, args)
		},
	}
}

func colorizeCommand() *Command {
	var o photos.ColorizeOptions
	var f fileFlags
	return &Command{
		Name:    "colorize",
		Group:   groupEdit,
		Summary: "add colour to a black-and-white photo",
		Args:    imagesArgs,
		Long: `The colours are guesses from what such things usually look like: skin, sky,
grass and wood come out believable, while the colour of clothes, cars or
walls cannot be known and is invented. A photo that already has colour is
treated as black and white: its colours are replaced. The photo keeps its
own light and detail, and its transparency.

` + fmt.Sprintf(filesLong, "-colorized"),
		Examples: []string{
			"photos colorize grandmother.jpg",
			"photos colorize --strength 70 --out-dir colour album/",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.Float64Var(&o.Strength, "strength", 0, "how colourful the result is, as a `percentage` from 1 to 100; lower is paler (default: 100)")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			return runImages(ctx, env, imageOp{
				suffix: "-colorized",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: simpleImage(func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, error) {
					return c.Colorize(ctx, in, &o)
				}),
			}, &f, args)
		},
	}
}

// parsePoint reads the "x,y" of the flag named: pixels from the top-left
// corner, which need not be whole.
func parsePoint(flagName, s string) (photos.Point, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return photos.Point{}, usagef("--%s %q: want x,y", flagName, s)
	}
	var n [2]float64
	for i, part := range parts {
		v, err := parseNumber(part)
		if err != nil || v < 0 {
			return photos.Point{}, usagef("--%s %q: want two numbers, x,y, in pixels from the top-left corner", flagName, s)
		}
		n[i] = v
	}
	return photos.Point{X: n[0], Y: n[1]}, nil
}

func selectCommand() *Command {
	var o photos.SelectOptions
	var f fileFlags
	var points, excludes stringList
	var box string
	return &Command{
		Name:    "select",
		Group:   groupEdit,
		Summary: "cut out the object at a point or in a box",
		Args:    "[flags] (--point <x,y>... | --box <x,y,w,h>) <image|directory|->...",
		Long: `Where remove-background decides for itself what the subject is, here you
say: --point on the object (repeat it to add to the selection), --exclude
for what must stay out, or a --box around it. Coordinates are in the photo's
pixels from its top-left corner, after its EXIF rotation.

A point is ambiguous (the face, the head or the person), so the model has
three answers; the surest one that agrees with every point is used, and
--pick asks for the smallest or the largest instead. The result is the
object cut out, a transparent PNG unless --format or --background say
otherwise, or with --mask its mask, which ` + "`photos erase --mask`" + ` takes to
remove the object. Each line of output gives the model's score, 0 to 1: how
well it thinks the mask fits.

When nothing is found at the points the file fails. Edges are softer than
remove-background's on large photos, and hair is not followed.

` + fmt.Sprintf(filesLong, "-selected"),
		Examples: []string{
			"photos select --point 420,310 desk.jpg",
			"photos select --point 420,310 --point 440,300 --exclude 600,120 --pick largest --crop -o lamp.png desk.jpg",
			"photos select --box 100,80,400,300 --mask -o lamp-mask.png desk.jpg && photos erase --mask lamp-mask.png desk.jpg",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.Var(&points, "point", "a point on the object, as `x,y` in pixels from the top-left corner; repeatable")
			fs.Var(&excludes, "exclude", "a point that must stay outside the selection, as `x,y`; repeatable")
			fs.StringVar(&box, "box", "", "a box around the object, as `x,y,w,h` in pixels from the top-left corner")
			fs.StringVar(&o.Pick, "pick", "", "`which` of the model's answers: auto (the surest), smallest (the part pointed at) or largest (the whole it belongs to) (default: auto)")
			fs.BoolVar(&o.Mask, "mask", false, "write the greyscale mask instead of the cutout")
			fs.BoolVar(&o.Crop, "crop", false, "crop to the object")
			fs.IntVar(&o.Padding, "padding", 0, "margin around the object when cropping, in `pixels`")
			fs.StringVar(&o.Background, "background", "", "composite onto a `colour` (white, #rrggbb) instead of transparency; JPEG defaults to white")
			formatFlags(fs, &o.Format, &o.Quality, "png")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(points) == 0 && box == "" {
				return usagef("say which object: --point x,y on it, or --box x,y,w,h around it")
			}
			for _, s := range points {
				point, err := parsePoint("point", s)
				if err != nil {
					return err
				}
				o.Points = append(o.Points, point)
			}
			for _, s := range excludes {
				point, err := parsePoint("exclude", s)
				if err != nil {
					return err
				}
				o.Exclude = append(o.Exclude, point)
			}
			if box != "" {
				rect, err := parseRect("box", box)
				if err != nil {
					return err
				}
				if rect.Width == 0 || rect.Height == 0 {
					return usagef("--box %q: the width and the height must be more than zero", box)
				}
				o.Box = rect
			}
			suffix := "-selected"
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
					res, err := c.Select(ctx, in, &o)
					if err != nil {
						return nil, nil, err
					}
					return &res.Image, func(r *fileResult) { r.Score = &res.Score }, nil
				},
			}, &f, args)
		},
	}
}

func restoreFacesCommand() *Command {
	var o photos.RestoreFacesOptions
	var f fileFlags
	return &Command{
		Name:    "restore-faces",
		Group:   groupEdit,
		Summary: "redraw the faces of an old or poor photo sharp",
		Args:    imagesArgs,
		Long: `Finds the faces in an old, blurred, small or noisy photo and redraws each
one sharp; the rest of the photo is not touched.

It invents the detail: the result resembles the person, but the colour of
the eyes, the teeth and the skin's texture are the model's, so it is not for
identity photos or evidence.

Each line of output says how many faces were restored and how many were
skipped because they are large and detailed enough already. With no face
found nothing is restored, and the exit code stays 0.

` + fmt.Sprintf(filesLong, "-restored"),
		Examples: []string{
			"photos restore-faces grandmother.jpg",
			"photos restore-faces --strength 70 --out-dir restored album/",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.Float64Var(&o.Strength, "strength", 0, "how much of the restored face replaces the original, as a `percentage` from 1 to 100 (default: 100)")
			fs.Float64Var(&o.MinScore, "min-score", 0, "how sure the detector must be that something is a face, as a `score` from 0.1 to 1 (default: 0.6)")
			formatFlags(fs, &o.Format, &o.Quality, "the photo's own")
			f.register(fs)
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			return runImages(ctx, env, imageOp{
				suffix: "-restored",
				ext:    func(src string) string { return sameFormatExtension(o.Format, src) },
				call: func(ctx context.Context, c *photos.Client, in photos.Input) (*photos.Image, func(*fileResult), error) {
					res, err := c.RestoreFaces(ctx, in, &o)
					if err != nil {
						return nil, nil, err
					}
					return &res.Image, func(r *fileResult) { r.Faces, r.FacesSkipped = &res.Faces, &res.FacesSkipped }, nil
				},
			}, &f, args)
		},
	}
}
