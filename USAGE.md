# Using photos

<!-- Generated from the command tree by `make usage`. Do not edit by hand: a test fails when this file and the binary disagree. -->

Every command and every flag of the `photos` command-line client. The
[README](README.md) has what it is, how to install it and a tour;
`photos help <command>` prints the same as the sections below.

## Commands

- [`photos remove-background`](#photos-remove-background): cut the subject out of its background
- [`photos resize`](#photos-resize): resize, convert and compress
- [`photos upscale`](#photos-upscale): enlarge with an AI model
- [`photos erase`](#photos-erase): remove objects by painting over an area
- [`photos remove-text`](#photos-remove-text): paint out text, watermarks and captions
- [`photos enhance`](#photos-enhance): fix light and colour
- [`photos crop`](#photos-crop): crop to a shape around the subject
- [`photos expand`](#photos-expand): add room around a photo
- [`photos watermark`](#photos-watermark): add a logo or text
- [`photos denoise`](#photos-denoise): remove noise and JPEG artefacts
- [`photos colorize`](#photos-colorize): add colour to a black-and-white photo
- [`photos select`](#photos-select): cut out the object at a point or in a box
- [`photos blur-faces`](#photos-blur-faces): hide the faces in a photo
- [`photos restore-faces`](#photos-restore-faces): redraw the faces of an old or poor photo sharp
- [`photos id-photo`](#photos-id-photo): frame a portrait for an identity document
- [`photos vectorize`](#photos-vectorize): trace a logo or a drawing into SVG
- [`photos run`](#photos-run): apply a preset or a recipe of your own
- [`photos fix`](#photos-fix): make a photo meet a marketplace's rules
- [`photos check`](#photos-check): measure a photo's quality and check it against a marketplace's rules
- [`photos barcodes`](#photos-barcodes): read the barcodes in a photo
- [`photos ocr`](#photos-ocr): read the text in a photo
- [`photos label`](#photos-label): read product details off a label
- [`photos identify`](#photos-identify): match photos of a product against your catalogue
- [`photos palette`](#photos-palette): list the main colours of a photo
- [`photos faces`](#photos-faces): find the faces in a photo
- [`photos caption`](#photos-caption): describe a photo in words
- [`photos jobs`](#photos-jobs): run one operation on many images in the background
  - [`photos jobs create`](#photos-jobs-create): start a job
  - [`photos jobs list`](#photos-jobs-list): list your recent jobs, newest first
  - [`photos jobs get`](#photos-jobs-get): show a job: its status, progress and items
  - [`photos jobs wait`](#photos-jobs-wait): wait for jobs to complete, then show them
  - [`photos jobs download`](#photos-jobs-download): wait for jobs, then save the result of every item
  - [`photos jobs delete`](#photos-jobs-delete): cancel a job if it is unfinished, and delete it and its results
- [`photos webhook`](#photos-webhook): verify a job's webhook delivery, and show the signing key
  - [`photos webhook verify`](#photos-webhook-verify): check a webhook delivery's signature
  - [`photos webhook secret`](#photos-webhook-secret): print the key your job webhooks are signed with
- [`photos presets`](#photos-presets): list the built-in presets `photos run` takes
- [`photos marketplaces`](#photos-marketplaces): list the marketplaces `photos check` and `photos fix` know
- [`photos id-photos`](#photos-id-photos): list the documents `photos id-photo` knows
- [`photos account`](#photos-account): show the account, its credits and prices
- [`photos health`](#photos-health): say whether the service is ready
- [`photos login`](#photos-login): save your API key on this machine
- [`photos logout`](#photos-logout): remove the saved API key
- [`photos version`](#photos-version): print the version of this tool
- [`photos update`](#photos-update): update this tool to the latest release
- [`photos completion`](#photos-completion): write a shell completion script
- [`photos help`](#photos-help): show help for a command

## Global flags

Every command takes these, before or after its own arguments.

| Flag | |
|---|---|
| `--base-url url` | the API to talk to, as a url (default https://api.photos.sh, or $PHOTOS_BASE_URL) |
| `--json` | machine-readable output on standard output: JSON, and nothing else |
| `-q, --quiet` | print nothing but errors and the data asked for |
| `--retries times` | how many times to try again when the service is busy or unreachable (default 2) |
| `--timeout duration` | how long one request may take, as a duration like 30s or 5m; 0 for no limit (default 2m0s) |
| `-h, --help` | show help for the command |

## Exit codes

Stable: a script may rely on them. With several files, the code is that of the most severe failure (the highest of 1 to 5).

| Code | Meaning |
|---|---|
| `0` | Everything asked for was done. |
| `1` | A file or the request failed: a bad parameter, a broken image, a file in the way, a job item that failed, a webhook that does not verify. |
| `2` | The command was wrong: an unknown command or flag, a missing argument. Nothing was sent. |
| `3` | Not signed in, or the API refused the key. |
| `4` | Out of credits, or the account is suspended. |
| `5` | The service is busy or could not be reached, after the retries; or a request timed out. |
| `130` | Interrupted (Ctrl-C). |

## Environment

| Variable | Meaning |
|---|---|
| `PHOTOS_API_KEY` | The API key. Wins over the key saved by `photos login`. |
| `PHOTOS_BASE_URL` | The API to talk to; `--base-url` wins over it. Default `https://api.photos.sh`. |
| `PHOTOS_CONFIG_DIR` | The directory `photos login` saves the key in, instead of the user's configuration directory. |
| `NO_COLOR` | Set to anything: no colour, also on a terminal. |

## photos remove-background

Cut the subject out of its background.

```
photos remove-background [flags] <image|directory|->...
```

Alias: `bg-remove`.

The cutout is a transparent PNG unless --format or --background say
otherwise.

--model picks the model that finds the subject. Each costs the same: try
another when one drops part of what you sell.

    atlas  the default: everyday product photos, a product alone or with
           its box, on any backdrop
    lumen  pale packaging on a light backdrop, such as a cream or wooden
           box beside a bottle, and close-ups where the product fills the
           photo; on those, atlas can keep the bottle and drop the box.
           It may keep a dark backdrop at the photo's edge, and can cut
           away part of an open frame or wire stand
    muse   people: portraits, and models wearing the product

--studio is for studio shots: a product on a plain, light backdrop, as in a
light tent. It also keeps what clearly stands out from the backdrop and
touches the product, such as the gift box, case or lid shown with a bottle,
or a pale box the model left half transparent. Shadows and the floor below
the product are left out. A photo without a plain light backdrop is cut out
as without it. It works with every --model.

The cutout's edge can be tuned. --shift moves it, in pixels: inward when
negative, which trims the light rim a dark product shows on a dark page,
outward when positive. --hardness makes it crisper, down to about a pixel
wide at 100; it sharpens a light rim too, so on a dark page give it with a
small negative --shift. --smooth irons out ripples in the outline; from
about 4 it rounds small corners and breaks up thin parts such as a wire
stand. --feather is how soft the edge is to begin with: left out, it grows
with the photo, and 0 is the model's hard edge.

The cutout can be staged in the same call: on a canvas (--width and
--height, with --fill and --align), with a --shadow or a --reflection under
it, and with something behind it: a colour (--background), an image
(--background-image) or the photo's own background blurred (--blur); one of
the three at most. --mask takes none of these, and --crop goes with neither
a canvas nor --blur. When the service finds no subject in a photo (a close-up
of a box, a plain surface), the result is the photo as it was, with or
without a canvas, and its line says "no subject found".

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-nobg.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--align where` | where the subject sits on the canvas: center, top or bottom (default: center) |
| `--background colour` | composite onto a colour (white, #rrggbb) instead of transparency; JPEG defaults to white |
| `--background-image file` | an image file to put behind the subject, scaled to cover the result |
| `--blur amount` | keep the photo's own background behind the subject, blurred by this amount, 1 (slight) to 100 |
| `--crop` | crop to the subject |
| `--decontaminate` | clean the old background's colour out of the soft edge; --decontaminate=false keeps the photo's own colours there (default: true) |
| `--feather pixels` | how soft the cutout's edge is: a blur of this many pixels, 0 to 10; 0 is the model's hard edge (default: grows with the photo, 1.4 at 2000 px) |
| `--fill fraction` | how much of the canvas the subject may take in its tighter dimension, as a fraction from 0.05 to 1 (default: 0.85) |
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: png) |
| `--hardness amount` | make the cutout's edge crisper by this amount, 0 to 100 (about a pixel wide at 100); on a dark page, pair it with a small negative --shift |
| `--height pixels` | the canvas's height in pixels; needs --width |
| `--mask` | write the greyscale alpha mask instead of the cutout |
| `--model name` | the model that finds the subject, by name: atlas for everyday product photos, lumen for pale packaging on a light backdrop and close-ups, muse for people (default: atlas) |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--padding pixels` | margin around the subject when cropping, in pixels |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--reflection` | add a fading mirror image under the subject; leave room below it (--fill under 1) |
| `--shadow kind` | a shadow under the subject, by kind: contact (soft, at its base), drop (the silhouette, offset) or none (default: none) |
| `--shift pixels` | move the cutout's edge by this many pixels, -10 to 10: inward when negative (trims a light rim), outward when positive |
| `--smooth amount` | iron out ripples in the cutout's outline by this amount, 0 to 10; from about 4 it rounds small corners and breaks up thin parts |
| `--solidify` | make semi-transparent areas inside the subject opaque, for an opaque product; wrong for glass, smoke or hair |
| `--studio` | for a product on a plain, light backdrop: also keep what stands out from it and touches the product (a gift box, case or lid); shadows and the floor are left out |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |
| `--width pixels` | place the subject on a canvas this wide, in pixels; needs --height |

```sh
photos remove-background shoe.jpg
photos remove-background --background white --crop --padding 40 --format jpg --out-dir cutouts photos/
photos remove-background --studio perfume.jpg
photos remove-background --model lumen --studio perfume-boxed.jpg
photos remove-background --model muse portrait.jpg
photos remove-background --shift -1.5 --hardness 50 bottle.jpg
photos remove-background --width 2000 --height 2000 --fill 0.8 --shadow contact --background white --format jpg shoe.jpg
```

## photos resize

Resize, convert and compress.

```
photos resize [flags] <image|directory|->...
```

Give --width or --height to keep the proportions, or both with --fit.
Without either, the photo is only converted (--format) or compressed
(--quality, --max-bytes). No model is involved: this is fast.

--trim cuts away an even border first: whatever matches the top-left pixel,
or is transparent like it. --dpi writes a print resolution into a JPEG or
PNG without changing a pixel.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-resized.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--background colour` | flatten transparency onto this colour |
| `--dpi 1-2400` | the print resolution to write into a JPEG or PNG, 1-2400; no pixel changes |
| `--enlarge` | allow scaling up with contain |
| `--fit how` | with both sides given, how to fit: contain, cover or fill (default: contain) |
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--height pixels` | target height in pixels |
| `--max-bytes bytes` | lower the quality until the file fits in this many bytes |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |
| `--trim` | cut away an even border first: whatever matches the top-left pixel, or is transparent like it |
| `--trim-tolerance 0-255` | how far a pixel may differ from the border's colour and still be border, per channel, 0-255 (default: 10); needs --trim |
| `--width pixels` | target width in pixels |

```sh
photos resize --width 1600 --format webp --quality 82 shoe.jpg
photos resize --width 800 --height 800 --fit cover --out-dir thumbs --recursive photos/
cat shoe.jpg | photos resize --width 400 -o - - > shoe-400.jpg
photos resize --trim --trim-tolerance 20 --dpi 300 scan.png
```

## photos upscale

Enlarge with an AI model.

```
photos upscale [flags] <image|directory|->...
```

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-upscaled.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--scale factor` | the factor, 1 to 4 (default: 4) |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos upscale --scale 2 small.jpg
photos upscale --format webp -o big.webp small.png
```

## photos erase

Remove objects by painting over an area.

```
photos erase [flags] <image|directory|->...
```

Say what to remove with --rect (repeatable), with --mask (an image the
size of the photo, white where to remove), or with both. `photos select --mask`
makes such a mask of one object.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-erased.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--mask file` | a mask file: the size of the photo, white where to remove |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--rect x,y,w,h` | an area to remove, as x,y,w,h in pixels from the top-left corner; repeatable |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos erase --rect 40,60,200,120 room.jpg
photos erase --mask room-mask.png --rect 0,0,80,80 -o clean.jpg room.jpg
```

## photos remove-text

Paint out text, watermarks and captions.

```
photos remove-text [flags] <image|directory|->...
```

By default only text off the product goes (watermarks, captions, prices);
labels printed on the product stay. Each line of output says how many lines
of text were removed and how many kept.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-notext.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--matching regexp` | remove only lines matching this case-insensitive regexp |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--region where` | where to remove text: background (off the product) or all (default: background) |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos remove-text listing.jpg
photos remove-text --region all --matching 'sale|€|www\.' --out-dir clean listings/
```

## photos enhance

Fix light and colour.

```
photos enhance [flags] <image|directory|->...
```

Without settings the tones are stretched to the full range and a colour
cast is taken out. With any setting, only what is given is applied; add
--auto to keep the automatic part beside it. Transparency is kept. No model
is involved: it does not denoise, deblur or recover blown highlights.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-enhanced.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--auto` | automatic levels and white balance; on when no other setting is given |
| `--brightness amount` | brightness, as an amount from -100 to 100 |
| `--contrast amount` | contrast, as an amount from -100 to 100 |
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--levels` | stretch the tones so the darkest pixels are black and the brightest white; --levels=false leaves it out of --auto |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--saturation amount` | saturation, as an amount from -100 (black and white) to 100 |
| `--sharpen amount` | sharpening, as an amount from 0 to 100 |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |
| `--temperature amount` | colour temperature, as an amount from -100 (cooler, bluer) to 100 (warmer) |
| `--white-balance` | take out a colour cast; --white-balance=false leaves it out of --auto |

```sh
photos enhance shoe.jpg
photos enhance --auto --saturation 15 --sharpen 30 --out-dir better photos/
photos enhance --saturation -100 -o bw.jpg shoe.jpg
```

## photos crop

Crop to a shape around the subject.

```
photos crop [flags] <image|directory|->...
```

Finds the subject and crops the photo to a shape around it, keeping the
background. Give the shape with --aspect, or with --width and --height,
which also scale the crop to exactly that size. Without --zoom the crop is
the largest window of the shape, moved to hold the subject; with it, the
smallest that holds the subject and its --padding.

With --focus face the faces are found instead, all of them as one area: a
portrait or an avatar is --focus face --zoom --padding 0.6. Each line of
output says whether a subject was found; when none is, the crop is centred.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-cropped.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--aspect shape` | the shape, as width:height (4:5, 16:9, 1:1) or a number (1.5) |
| `--focus what` | what the crop is built around: subject or face (default: subject) |
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--height pixels` | scale the crop to this height in pixels; needs --width |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--padding fraction` | room around the subject with --zoom, as a fraction of its longest side, 0 to 2 (default: 0.1) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |
| `--width pixels` | scale the crop to this width in pixels; needs --height |
| `--zoom` | crop tightly around the subject |

```sh
photos crop --aspect 4:5 shoe.jpg
photos crop --focus face --zoom --padding 0.6 --width 400 --height 400 --out-dir avatars team/
photos crop --aspect 1:1 --json photos/ | jq -r 'select(.subject_found == false) | .input'
```

## photos expand

Add room around a photo.

```
photos expand [flags] <image|directory|->...
```

Grows the canvas to a shape (--aspect, the photo staying in the centre), or
by pixels on each side. The new area continues the photo by inpainting,
which suits plain and softly textured backgrounds: a studio backdrop, a
table, a wall, sky. It does not invent content: a subject cut off at the
edge stays cut off, and a busy scene continues as a smear. --fill with a
colour makes a flat border instead, and needs no model. Each side can grow
by at most the photo's own width or height. Transparency is flattened onto
white.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-expanded.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--aspect shape` | the shape to reach, as width:height (1:1, 16:9) or a number; not with the sides |
| `--bottom pixels` | pixels to add at the bottom |
| `--fill what` | what fills the new area: inpaint, or a colour (white, black, #rrggbb) (default: inpaint) |
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--left pixels` | pixels to add on the left |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--right pixels` | pixels to add on the right |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |
| `--top pixels` | pixels to add at the top |

```sh
photos expand --aspect 1:1 shoe.jpg
photos expand --left 200 --right 200 --fill white --out-dir wide photos/
```

## photos watermark

Add a logo or text.

```
photos watermark [flags] (--text <text> | --logo <file>) <image|directory|->...
```

Puts --text or a --logo image on each photo: at one of nine positions, or
tiled across it with --tile. --scale is the mark's width as a percentage of
the photo's width, so photos of any size are marked alike. Text is drawn in
a bold sans-serif with Latin, Greek and Cyrillic letters; other scripts need
a logo. No model is involved.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-watermarked.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--color colour` | the text's colour: white, black or #rrggbb (default: white) |
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--logo file` | an image file to put on the photo instead of text; a PNG with transparency works best |
| `--margin percentage` | distance from the edges as a percentage of the photo's shorter side, 0 to 50 (default: 3) |
| `--opacity percentage` | how solid the mark is, as a percentage from 1 to 100 (default: 50) |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--position where` | where the mark goes: top-left, top, top-right, left, center, right, bottom-left, bottom or bottom-right (default: bottom-right) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--scale percentage` | the mark's width as a percentage of the photo's width, 1 to 100 (default: 25) |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |
| `--text text` | the text to draw, up to 200 characters |
| `--tile` | repeat the mark across the whole photo; --position is then unused |

```sh
photos watermark --text "© Acme 2026" shoe.jpg
photos watermark --logo logo.png --position top-left --scale 15 --opacity 80 --out-dir marked photos/
photos watermark --text PROOF --tile --opacity 20 -o proof.jpg shoe.jpg
```

## photos denoise

Remove noise and JPEG artefacts.

```
photos denoise [flags] <image|directory|->...
```

Removes sensor noise and the blocks and ringing of heavy JPEG compression.
It does not sharpen a blurred photo or add detail, and on a photo that is
already clean it takes away a little fine texture: lower --strength there,
or do not use it. Transparency is kept.

It is slow without a GPU (about half a minute a megapixel) and takes photos
up to 12 megapixels; give a large photo a longer --timeout.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-denoised.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--strength percentage` | how much of the cleaned result replaces the photo, as a percentage from 1 to 100; 50 is half-way back to the original (default: 100) |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos denoise night.jpg
photos denoise --strength 60 --timeout 10m --parallel 1 --out-dir clean photos/
```

## photos colorize

Add colour to a black-and-white photo.

```
photos colorize [flags] <image|directory|->...
```

The colours are guesses from what such things usually look like: skin, sky,
grass and wood come out believable, while the colour of clothes, cars or
walls cannot be known and is invented. A photo that already has colour is
treated as black and white: its colours are replaced. The photo keeps its
own light and detail, and its transparency.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-colorized.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--strength percentage` | how colourful the result is, as a percentage from 1 to 100; lower is paler (default: 100) |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos colorize grandmother.jpg
photos colorize --strength 70 --out-dir colour album/
```

## photos select

Cut out the object at a point or in a box.

```
photos select [flags] (--point <x,y>... | --box <x,y,w,h>) <image|directory|->...
```

Where remove-background decides for itself what the subject is, here you
say: --point on the object (repeat it to add to the selection), --exclude
for what must stay out, or a --box around it. Coordinates are in the photo's
pixels from its top-left corner, after its EXIF rotation.

A point is ambiguous (the face, the head or the person), so the model has
three answers; the surest one that agrees with every point is used, and
--pick asks for the smallest or the largest instead. The result is the
object cut out, a transparent PNG unless --format or --background say
otherwise, or with --mask its mask, which `photos erase --mask` takes to
remove the object. Each line of output gives the model's score, 0 to 1: how
well it thinks the mask fits.

When nothing is found at the points the file fails. Edges are softer than
remove-background's on large photos, and hair is not followed.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-selected.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--background colour` | composite onto a colour (white, #rrggbb) instead of transparency; JPEG defaults to white |
| `--box x,y,w,h` | a box around the object, as x,y,w,h in pixels from the top-left corner |
| `--crop` | crop to the object |
| `--exclude x,y` | a point that must stay outside the selection, as x,y; repeatable |
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: png) |
| `--mask` | write the greyscale mask instead of the cutout |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--padding pixels` | margin around the object when cropping, in pixels |
| `--parallel files` | how many files to send at once (default 4) |
| `--pick which` | which of the model's answers: auto (the surest), smallest (the part pointed at) or largest (the whole it belongs to) (default: auto) |
| `--point x,y` | a point on the object, as x,y in pixels from the top-left corner; repeatable |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos select --point 420,310 desk.jpg
photos select --point 420,310 --point 440,300 --exclude 600,120 --pick largest --crop -o lamp.png desk.jpg
photos select --box 100,80,400,300 --mask -o lamp-mask.png desk.jpg && photos erase --mask lamp-mask.png desk.jpg
```

## photos blur-faces

Hide the faces in a photo.

```
photos blur-faces [flags] <image|directory|->...
```

Finds every face and hides it inside an ellipse: blurred, pixelated
(--mode pixelate) or covered with a colour (--mode fill). Blur and
pixelation stop a person from recognising the face; only fill leaves nothing
of it.

Each line of output says how many faces were hidden. Check it before relying
on the result: a small face, a profile, or a face seen from behind or mostly
covered may not be found, and a face that was not found still shows. No face
found is information, not a failure: nothing is hidden, the line says so,
and the exit code stays 0.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-nofaces.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--color colour` | the colour for --mode fill: black, white or #rrggbb (default: black) |
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--min-score score` | how sure the model must be that something is a face, as a score from 0.1 to 1; lower finds more faces and more things that are not (default: 0.6) |
| `--mode how` | how to hide a face: blur, pixelate or fill (default: blur) |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--padding fraction` | how far past the face's box the hidden area reaches, as a fraction of the box, 0 to 1 (default: 0.15) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--strength amount` | how strong the blur or how coarse the mosaic is, as an amount from 1 to 100 (default: 50) |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos blur-faces street.jpg
photos blur-faces --mode fill --color black --padding 0.3 --out-dir public photos/
photos blur-faces --json --out-dir public photos/ | jq -r 'select(.faces == 0) | .input'
```

## photos restore-faces

Redraw the faces of an old or poor photo sharp.

```
photos restore-faces [flags] <image|directory|->...
```

Finds the faces in an old, blurred, small or noisy photo and redraws each
one sharp; the rest of the photo is not touched.

It invents the detail: the result resembles the person, but the colour of
the eyes, the teeth and the skin's texture are the model's, so it is not for
identity photos or evidence.

Each line of output says how many faces were restored and how many were
skipped because they are large and detailed enough already. With no face
found nothing is restored, and the exit code stays 0.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-restored.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: the photo's own) |
| `--min-score score` | how sure the detector must be that something is a face, as a score from 0.1 to 1 (default: 0.6) |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--strength percentage` | how much of the restored face replaces the original, as a percentage from 1 to 100 (default: 100) |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos restore-faces grandmother.jpg
photos restore-faces --strength 70 --out-dir restored album/
```

## photos id-photo

Frame a portrait for an identity document.

```
photos id-photo [flags] <image|directory|->...
```

Levels the head, scales it to the document's head size and places it on a
photo of the document's size, as a JPEG at --dpi. Name a built-in --document
(`photos id-photos` lists them), or give the four sizes yourself:
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

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-\<document>.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--background what` | what is behind the person: keep (the photo's own), document (the colour the document asks for) or a colour (white, #rrggbb); anything but keep edits the photo (default: keep) |
| `--document name` | the name of a built-in document, such as us-passport; not with the sizes |
| `--dpi 100-1200` | pixels per inch of the result, 100-1200, also written into the file (default: 300) |
| `--eyes-max-mm mm` | the eyes' largest distance from the bottom edge in mm; needs --eyes-min-mm |
| `--eyes-min-mm mm` | the eyes' smallest distance from the bottom edge in mm, for a document with such a rule; needs --eyes-max-mm |
| `--force` | replace files that exist |
| `--format format` | output format: png, jpg, webp or avif (default: jpg) |
| `--head-max-mm mm` | the largest allowed head, chin to crown, in mm |
| `--head-min-mm mm` | the smallest allowed head, chin to crown, in mm |
| `--height-mm mm` | the photo's height in mm, 10 to 200 |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--quality 1-100` | JPEG, WebP or AVIF quality, 1-100 |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |
| `--width-mm mm` | the photo's width in mm, 10 to 200 |

```sh
photos id-photo --document us-passport portrait.jpg
photos id-photo --width-mm 35 --height-mm 45 --head-min-mm 32 --head-max-mm 36 --dpi 600 -o visa.jpg portrait.jpg
photos id-photo --document uk-passport --json portrait.jpg | jq '.findings'
```

## photos vectorize

Trace a logo or a drawing into SVG.

```
photos vectorize [flags] <image|directory|->...
```

Each area of one colour becomes a filled path, and the result is an .svg
file. It is for logos, icons and flat drawings: a photograph traces into
thousands of paths and a large file. Images over 2000 pixels on a side are
traced from a smaller copy. Each line of output says how many paths the SVG
has. No model is involved.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-vector.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--curves kind` | the kind of outline: spline (smooth), polygon (straight segments) or pixel (every pixel step, for pixel art) (default: spline) |
| `--detail 1-8` | bits kept per colour channel when grouping pixels, 1-8; lower merges similar shades into fewer shapes (default: 8) |
| `--force` | replace files that exist |
| `--mode how` | how to trace: color, or mono for one black shape of whatever is darker than mid grey (default: color) |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--speckle pixels` | drop areas smaller than this many pixels across, 0 to 100 (default: 4) |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos vectorize logo.png
photos vectorize --mode mono --curves polygon -o stamp.svg stamp.png
photos vectorize --detail 5 --speckle 8 --out-dir svg icons/
```

## photos run

Apply a preset or a recipe of your own.

```
photos run [flags] (--preset <name> | --recipe <file.toml>) <image|directory|->...
```

A preset is a built-in chain of steps (`photos presets` lists them); a
recipe is the same thing written by you, as TOML. Either is one request a
photo, charged as one, however many steps it has. `photos presets <name>`
prints a preset's recipe to start from; the steps and their options are in
the API reference under POST /v1/run.

A recipe's [barcodes] and [text] steps read the photo before any step
changes it. What they read is on the file's line (the codes, and the lines
of text under it) and, with --json, in "barcodes" and "text".

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-\<preset>.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--force` | replace files that exist |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--preset name` | the name of a built-in preset, such as marketplace |
| `--recipe file` | a recipe file, as TOML |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos run --preset marketplace shoe.jpg
photos run --recipe shop.toml --out-dir ready --recursive photos/
```

## photos fix

Make a photo meet a marketplace's rules.

```
photos fix [flags] --marketplace <name> <image|directory|->...
```

Marketplaces: amazon, ebay, etsy, shopify, google, allegro, zalando
(`photos marketplaces` shows their rules). Each line of output says whether
the fixed photo passes, and the findings still present if it does not; that
is information, not a failure, and the exit code stays 0.

Inputs are files, directories (searched for images; --recursive goes below
them) and - for standard input. Each result is written beside its source as
\<name>-\<marketplace>.\<ext>, the extension being the result's format; --out-dir writes
\<name>.\<ext> into one directory instead, and for a single input -o names the
file, or - for standard output. A file that exists is never replaced unless
--force is given, and a file is never left half-written. Several files are
sent a few at once (--parallel); one failing does not stop the others.

| Flag | |
|---|---|
| `--force` | replace files that exist |
| `--marketplace name` | the marketplace's name: amazon, ebay, etsy, shopify, google, allegro or zalando |
| `--out-dir directory` | write the results into this directory, created if missing, as \<name>.\<ext> |
| `-o, --output file` | write the result to this file, or - for standard output (one input only) |
| `--parallel files` | how many files to send at once (default 4) |
| `--recursive` | search directories below the ones named, keeping the layout in --out-dir |
| `--suffix text` | the text added to each name instead of the command's own; may be empty |

```sh
photos fix --marketplace amazon shoe.jpg
photos fix --marketplace etsy --out-dir etsy --json photos/ | jq -r 'select(.passed == false) | .input'
```

## photos check

Measure a photo's quality and check it against a marketplace's rules.

```
photos check [flags] <image|->
```

Measures sharpness, noise, exposure and colour cast. With --marketplace
the photo is also checked against that marketplace's rules. The exit code
says whether the check ran, not what it found: read the verdict from the
output (with --json, .marketplace.passed).

| Flag | |
|---|---|
| `--marketplace name` | check against this marketplace's rules: its name, such as amazon |
| `--subject` | find the product, for framing and background checks |
| `--text` | read the text in the photo |

```sh
photos check shoe.jpg
photos check --marketplace amazon --json shoe.jpg | jq '.marketplace.passed'
```

## photos barcodes

Read the barcodes in a photo.

```
photos barcodes [flags] <image|->
```

Prints one code per line: its format and its value. Retail codes (EAN-13,
UPC-A, ITF) are read by default; --all-formats adds the others.

| Flag | |
|---|---|
| `--all-formats` | also read EAN-8, UPC-E, Code 128/39/93, QR, Data Matrix and PDF417 |
| `--engine name` | the decoder's name: auto, zbar or rxing (default: auto) |

```sh
photos barcodes box.jpg
photos barcodes --all-formats --json box.jpg | jq -r '.barcodes[].value'
```

## photos ocr

Read the text in a photo.

```
photos ocr [flags] <image|->
```

Prints the text line by line, top to bottom. With --json each line comes
with its confidence and its corners in pixels.

```sh
photos ocr label.jpg
photos ocr --json label.jpg | jq -r '.lines[] | select(.score > 0.9) | .text'
```

## photos label

Read product details off a label.

```
photos label [flags] <image|directory|->...
```

Takes one to eight photos of ONE product (front, back, side…): text and
codes from all of them are pooled into one answer. Prints what was
understood: the headline, GTIN, net quantity, ingredients, allergens, dates
and so on. A directory stands for the images in it.

```sh
photos label front.jpg back.jpg
photos label --json front.jpg back.jpg | jq -r '.label.gtin'
```

## photos identify

Match photos of a product against your catalogue.

```
photos identify [flags] <image|directory|->...
```

Takes one to eight photos of ONE product and prints the catalogue products
that match, best first, with what matched (barcode, sku, ...). The catalogue
is one of your organisation's own, uploaded on the console's Catalogues page.
Catalogue matching is turned on per organisation: until it is on for yours
and a catalogue is uploaded, the API answers "not found".

| Flag | |
|---|---|
| `--catalogue name` | the catalogue's name; not needed when your organisation has exactly one |
| `--top 1-50` | how many candidates to print, 1-50 (default: 5) |

```sh
photos identify --catalogue shoes front.jpg back.jpg
photos identify --top 1 --json front.jpg | jq -r '.candidates[0].product_id'
```

## photos palette

List the main colours of a photo.

```
photos palette [flags] <image|->
```

Prints one colour per line, the most common first: its hex value, the
share of the image it stands for, and its red, green and blue. Transparent
pixels do not count, so a cutout's palette is the subject's. No model is
involved, and the same image always gives the same colours.

| Flag | |
|---|---|
| `--colors 1-16` | how many colours to print at most, 1-16 (default: 5) |

```sh
photos palette shoe.jpg
photos palette --colors 3 --json shoe.jpg | jq -r '.colors[].hex'
```

## photos faces

Find the faces in a photo.

```
photos faces [flags] <image|->
```

Prints one face per line, the surest first: where its box is (x,y of the
top-left corner, then width x height, in the photo's pixels after its EXIF
rotation), how sure the model is, and the tilt of the eyes in degrees. With
--json each face also has five landmarks: the eyes, the nose and the corners
of the mouth.

It finds faces; it does not recognise who anyone is. A face must be about
1/60 of the photo's longest side to be found, a profile is found less
surely, and a head seen from behind or mostly covered is not found. No face
found is an answer, not a failure: the exit code stays 0.

| Flag | |
|---|---|
| `--min-score score` | how sure the model must be that something is a face, as a score from 0.1 to 1 (default: 0.6) |

```sh
photos faces team.jpg
photos faces --min-score 0.4 --json team.jpg | jq '.faces | length'
```

## photos caption

Describe a photo in words.

```
photos caption [flags] <image|->
```

Prints a description of the photo: one sentence, or more with --detail.
The same photo always gets the same words.

The description can be wrong (a colour, a count, what an object is) and is
in English only; it does not know names, brands or places unless they are
written in the photo. Read it before publishing it as alt text or a product
description. The short form is the dependable one: the longer forms say more
and are wrong more often.

| Flag | |
|---|---|
| `--detail length` | the length of the description: short (one sentence), detailed (two or three) or paragraph (default: short) |

```sh
photos caption shoe.jpg
photos caption --detail paragraph --json shoe.jpg | jq -r '.caption'
```

## photos jobs

Run one operation on many images in the background.

```
photos jobs <command> [flags]
```

A job applies one operation to many images on the server and can outlive
this terminal: create it, wait for it (or give it a webhook), then download
the results.

| Command | |
|---|---|
| [`photos jobs create`](#photos-jobs-create) | start a job |
| [`photos jobs list`](#photos-jobs-list) | list your recent jobs, newest first |
| [`photos jobs get`](#photos-jobs-get) | show a job: its status, progress and items |
| [`photos jobs wait`](#photos-jobs-wait) | wait for jobs to complete, then show them |
| [`photos jobs download`](#photos-jobs-download) | wait for jobs, then save the result of every item |
| [`photos jobs delete`](#photos-jobs-delete) | cancel a job if it is unfinished, and delete it and its results |

```sh
id=$(photos jobs create remove-background --param format=png photos/)
photos jobs wait "$id" && photos jobs download "$id" --out-dir cutouts
```

## photos jobs create

Start a job.

```
photos jobs create [flags] <operation> <image|directory|url|->...
```

Operations: run, remove-background, resize, upscale, ocr, barcodes, check,
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

One job uploads at most --max-upload-mb (100 MB, the most the service takes)
and 1000 images. More is sent as several jobs, one after another, each
holding whole images in the order given.

Prints each job's id on its own line of standard output, as it starts, so
that a script can keep them. With --wait it waits for the jobs and prints
them, and the exit code is 1 if an item failed.

| Flag | |
|---|---|
| `--interval time` | the time between two looks at the job (default 2s) |
| `--max-upload-mb MB` | the most one job uploads, in MB; more is sent as several jobs (default 100) |
| `--param name=value` | a parameter of the operation, as name=value; repeatable |
| `--recipe file` | a recipe file (TOML), for the operation run |
| `--recursive` | search directories below the ones named |
| `--wait` | wait for the jobs to complete, then print them |
| `--wait-timeout time` | the longest time to wait, for all the jobs named (default 1h0m0s) |
| `--webhook-url url` | a url to send a signed job.completed event to when the job is done |

```sh
photos jobs create resize --param width=1600 --param format=webp photos/
photos jobs create run --param preset=marketplace --webhook-url https://example.com/hooks/photos https://example.com/1.jpg https://example.com/2.jpg
photos jobs create remove-background --wait --json photos/ | jq '.counts'
photos jobs create select --param point=420,310 --param point=440,300 --param crop=true photos/
photos jobs download --out-dir results $(photos jobs create run --recipe lot.toml photos/)
```

## photos jobs list

List your recent jobs, newest first.

```
photos jobs list [flags]
```

```sh
photos jobs list
photos jobs list --json | jq -r '.[] | select(.status != "completed") | .id'
```

## photos jobs get

Show a job: its status, progress and items.

```
photos jobs get [flags] <id>
```

```sh
photos jobs get job_5f0c2a
photos jobs get job_5f0c2a --json | jq -r '.items[] | select(.status == "failed") | .input'
```

## photos jobs wait

Wait for jobs to complete, then show them.

```
photos jobs wait [flags] <id>...
```

Several jobs are waited for one after another, within one --wait-timeout,
and each is shown as it completes. Progress goes to standard error. The exit
code is 0 when every item is done, and 1 when an item failed or a job is
still running after --wait-timeout (an hour unless set).

| Flag | |
|---|---|
| `--interval time` | the time between two looks at the job (default 2s) |
| `--wait-timeout time` | the longest time to wait, for all the jobs named (default 1h0m0s) |

```sh
photos jobs wait job_5f0c2a
photos jobs wait job_5f0c2a job_7d1e9b --wait-timeout 3h --quiet && photos jobs download job_5f0c2a job_7d1e9b
```

## photos jobs download

Wait for jobs, then save the result of every item.

```
photos jobs download [flags] <id>...
```

Each result is named after its input: shoe.jpg gives shoe.png for an image
result, shoe.svg for a vectorize job, shoe.json for a data result (ocr,
barcodes, check, label, palette, faces, caption). Two inputs with one name
give shoe.png and shoe-2.png. A file that exists is not replaced unless
--force is given. Each line says what the matching command would: the faces
hidden or restored, whether a subject was found, an id photo's findings.

Each job is waited for first, as photos jobs wait does, and its results are
saved as soon as it completes; several jobs go into one directory, named as
one. With --no-wait, or once --wait-timeout (an hour) has passed, only the
items finished by then are saved.

Items that failed, or are not finished, are reported and make the exit code
1; the others are still saved.

| Flag | |
|---|---|
| `--force` | replace files that exist |
| `--interval time` | the time between two looks at the job (default 2s) |
| `--no-wait` | save the items finished now, without waiting for the rest |
| `--out-dir directory` | the directory to save into, created if missing (default .) |
| `--parallel files` | how many files to fetch at once (default 4) |
| `--wait-timeout time` | the longest time to wait, for all the jobs named (default 1h0m0s) |

```sh
photos jobs download job_5f0c2a --out-dir results
photos jobs download job_5f0c2a job_7d1e9b --out-dir results --wait-timeout 3h
photos jobs download job_5f0c2a --json | jq -r 'select(.error) | .input'
```

## photos jobs delete

Cancel a job if it is unfinished, and delete it and its results.

```
photos jobs delete [flags] <id>...
```

```sh
photos jobs delete job_5f0c2a
photos jobs list --json | jq -r '.[] | select(.status == "completed") | .id' | xargs photos jobs delete
```

## photos webhook

Verify a job's webhook delivery, and show the signing key.

```
photos webhook <command> [flags]
```

A job created with --webhook-url sends a signed job.completed event there
when it is done. `photos webhook secret` prints the key the signature is made
with; `photos webhook verify` checks a delivery against it.

| Command | |
|---|---|
| [`photos webhook verify`](#photos-webhook-verify) | check a webhook delivery's signature |
| [`photos webhook secret`](#photos-webhook-secret) | print the key your job webhooks are signed with |

## photos webhook verify

Check a webhook delivery's signature.

```
photos webhook verify [flags] --secret-env <NAME> --signature <header> [<file>|-]
```

Reads the delivery's body, exactly as it was received, from a file or from
standard input, and checks it against the Photos-Signature header and your
signing key. The key is read from the environment variable --secret-env
names, never from the command line. Needs no API key and no network.

Exit code 0: the delivery is genuine (the event is printed). 1: it is not,
or it is older than --tolerance.

| Flag | |
|---|---|
| `--secret-env NAME` | the NAME of the environment variable that holds your signing key |
| `--signature header` | the delivery's Photos-Signature header: t=\<unix seconds>,v1=\<hex> |
| `--tolerance duration` | how old a delivery may be, as a duration (default 5m0s) |

```sh
photos webhook verify --secret-env PHOTOS_WEBHOOK_SECRET --signature "$HTTP_PHOTOS_SIGNATURE" body.json
photos webhook verify --secret-env PHOTOS_WEBHOOK_SECRET --signature "t=1767225600,v1=5f0c…" --json < body.json | jq '.job.id'
```

## photos webhook secret

Print the key your job webhooks are signed with.

```
photos webhook secret [flags]
```

Prints the signing key, alone on standard output. It is a secret: keep it
where your webhook handler reads it, not in a script or a log.

```sh
export PHOTOS_WEBHOOK_SECRET=$(photos webhook secret)
photos webhook secret --json | jq -r '.scheme'
```

## photos presets

List the built-in presets `photos run` takes.

```
photos presets [flags] [name]
```

With a name, prints that preset's recipe as TOML: a recipe of your own can
start from it (`photos run --recipe`).

```sh
photos presets
photos presets --json | jq -r '.[].name'
photos presets studio > mine.toml
```

## photos marketplaces

List the marketplaces `photos check` and `photos fix` know.

```
photos marketplaces [flags]
```

With --json, each marketplace comes with its rules and what `photos fix` produces for it.

```sh
photos marketplaces
photos marketplaces --json | jq '.[] | select(.name == "amazon") | .rules'
```

## photos id-photos

List the documents `photos id-photo` knows.

```
photos id-photos [flags]
```

Prints each document's name (what --document takes), its photo and head
sizes in millimetres, the background its rules ask for, and the date the
rules were last compared with the authority's own pages. With --json, each
document also comes with those pages, the rules the tool cannot check, and
what the authority says about photos changed by software.

```sh
photos id-photos
photos id-photos --json | jq -r '.[] | select(.name == "us-passport") | .alteration'
```

## photos account

Show the account, its credits and prices.

```
photos account [flags]
```

Shows which account the key belongs to, the credits left and what one
image costs for each operation. The key itself is never printed: only its
last four characters, and where it came from.

```sh
photos account
photos account --json | jq '.credits'
```

## photos health

Say whether the service is ready.

```
photos health [flags]
```

Prints ok, loading or failed. Needs no key. The exit code is 0 for ok and
5 otherwise, so a script can wait for the service:

    until photos health --quiet; do sleep 2; done

```sh
photos health
photos health --base-url http://localhost:8080
```

## photos login

Save your API key on this machine.

```
photos login [flags]
```

Asks for your API key (from your organisation's API keys page in the Photos.sh
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
checked against that API and saved for it.

```sh
photos login
photos login < ~/secrets/photos-key.txt
```

## photos logout

Remove the saved API key.

```
photos logout [flags]
```

Deletes the file `photos login` wrote. The key itself stays valid: revoke it in the Photos.sh console if it should stop working.

```sh
photos logout
```

## photos version

Print the version of this tool.

```
photos version [flags]
```

The same as `photos --version`. With --json, also the version of the Go library it is built on.

```sh
photos version
```

## photos update

Update this tool to the latest release.

```
photos update [flags]
```

Downloads the build for this machine from the latest release on GitHub,
checks it against the release's checksums.txt and replaces the binary that
is running. Nothing is replaced if the download or the check fails.

When the binary is in a directory you cannot write to (/usr/local/bin, say),
run it with sudo, or run the installer again.

PHOTOS_RELEASES_URL names a mirror of the releases, as for the installer.

| Flag | |
|---|---|
| `--check` | only say whether there is a newer release; change nothing |
| `--force` | install even when this is the same version, or a development build |
| `--to release` | install this release (v1.0.0) instead of the latest, older ones included |

```sh
photos update
photos update --check
photos update --to v1.0.0
```

## photos completion

Write a shell completion script.

```
photos completion bash|zsh|fish
```

Completes command names, subcommands and flags, and file names after
them. The script is written to standard output:

    # bash, this session
    source <(photos completion bash)
    # bash, every session
    photos completion bash > ~/.local/share/bash-completion/completions/photos

    # zsh, somewhere on $fpath
    photos completion zsh > "${fpath[1]}/_photos"

    # fish
    photos completion fish > ~/.config/fish/completions/photos.fish

```sh
source <(photos completion bash)
photos completion fish > ~/.config/fish/completions/photos.fish
```

## photos help

Show help for a command.

```
photos help [<command>...]
```

The same as the command with --help.

```sh
photos help resize
photos help jobs create
```
