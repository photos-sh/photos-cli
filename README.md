# photos

The command line for [Photos.sh](https://photos.sh): image tools for product
photos. Remove backgrounds and stage the cutout, resize and compress, upscale,
enhance, crop, expand, watermark, denoise and colorize, erase objects and
text, select an object, hide or restore faces, frame identity photos, trace
logos into SVG, check photos against marketplace rules and fix them, and read
barcodes, text, labels, colours, faces and captions, from a terminal and from
scripts.

One static binary, no runtime to install. Its only dependency is the Photos.sh
Go library, [photos-go](https://github.com/photos-sh/photos-go), which itself
needs nothing beyond the Go standard library.

```console
$ photos remove-background shoe.jpg
shoe.jpg -> shoe-nobg.png  1600x1200  image/png  812.4 kB

$ photos resize --width 1600 --format webp --out-dir web photos/
photos/boot.jpg   -> web/boot.webp  1600x1067  image/webp  148.2 kB
photos/shoe.jpg   -> web/shoe.webp  1600x1200  image/webp  131.9 kB
photos/sandal.png -> web/sandal.webp  1600x1600  image/webp  96.3 kB

$ photos check --marketplace amazon shoe.jpg
Size        1600x1200
Sharpness   0.82
Noise       1.50
Brightness  0.71
Findings: none
Marketplace amazon: does not pass
  error  background_not_white  the background must be pure white

$ photos fix --marketplace amazon shoe.jpg
shoe.jpg -> shoe-amazon.jpg  2000x2000  image/jpeg  412.7 kB  passes
```

## Installing

```sh
curl -fsSL https://raw.githubusercontent.com/photos-sh/photos-cli/main/installer.sh | bash
```

Linux, macOS and Windows (in Git Bash), amd64 and arm64. The installer needs
`curl` and one of `sha256sum` or `shasum`, and nothing else. It downloads the
build for your machine from the latest
[GitHub release](https://github.com/photos-sh/photos-cli/releases) and
installs it only if its SHA-256 is the one the release's `checksums.txt`
lists. The binary lands in `/usr/local/bin` where you can write there and in
`~/.local/bin` otherwise; `PHOTOS_INSTALL_DIR` names another directory, and
`PHOTOS_VERSION=v1.0.0` a release other than the latest.

Or download the file for your platform from the releases page, make it
executable and put it on your `PATH` as `photos`.

### Updating

```sh
photos update           # to the latest release
photos update --check   # only say whether there is one
```

It downloads the same build the installer would, checks it against the
release's `checksums.txt` and replaces the binary that is running; nothing
is replaced if either fails. `--to v1.0.0` installs a particular release,
older ones included. Where the binary is in a directory you cannot write to,
run `sudo photos update`. Version 1.0.0 has no `update`: run the installer
again once to get past it.

Or build it from a checkout (Go 1.23 or later):

```sh
make build      # dist/photos
make install    # into $GOBIN
make dist       # all six platforms, with dist/checksums.txt
```

Tab completion for bash, zsh and fish:

```sh
photos completion bash > ~/.local/share/bash-completion/completions/photos
```

## Signing in

Create an API key on your organisation's API keys page in the Photos.sh console,
then:

```console
$ photos login
API key (it is not shown):
Signed in as acme (key …9f3c). Saved in /home/ana/.config/photos-sh/credentials

$ photos account
Account  acme
Key      …9f3c (saved by photos login)
API      https://api.photos.sh
Credits  4200
Credits per image:
  remove-background  10
  resize             1
```

`photos login` checks the key with the API and saves it in a file only you
can read (mode 0600). `photos logout` removes it.

In CI and in containers, set `PHOTOS_API_KEY` instead; it wins over a saved
key and nothing needs to be written to disk. A script can also pipe the key
in: `photos login < key.txt`.

The key is never printed (only its last four characters) and is never taken
as a command-line argument, where it would be kept in your shell's history
and shown in the process list.

## A tour

Every command has `--help`, and [USAGE.md](USAGE.md) lists every command and
flag.

### Images in, images out

`remove-background`, `resize`, `upscale`, `erase`, `remove-text`, `enhance`,
`crop`, `expand`, `watermark`, `denoise`, `colorize`, `select`, `blur-faces`,
`restore-faces`, `id-photo`, `vectorize`, `run` and `fix` take one or many
images and write one result for each.

```sh
# One file: the result is written beside it, named for the operation
photos remove-background shoe.jpg                 # shoe-nobg.png
photos upscale --scale 2 small.jpg                # small-upscaled.jpg
photos erase --rect 40,60,200,120 room.jpg        # room-erased.jpg
photos remove-text --matching 'sale|www\.' listing.jpg
photos run --preset marketplace shoe.jpg          # shoe-marketplace.jpg
photos presets studio > shop.toml                 # a preset's recipe, to edit
photos run --recipe shop.toml shoe.jpg            # several steps, one request

# The cutout staged in the same call: canvas, shadow, background
photos remove-background --width 2000 --height 2000 --fill 0.8 \
  --shadow contact --background white --format jpg shoe.jpg

# A studio shot: keep the gift box beside the bottle, leave out the shadow
photos remove-background --studio perfume.jpg     # perfume-nobg.png

# Another model, at the same price: lumen for a pale box on a light backdrop,
# muse for people (atlas, the default, suits everyday product photos)
photos remove-background --model lumen --studio perfume-boxed.jpg
photos remove-background --model muse portrait.jpg

# The edge: trim the light rim a dark bottle shows on a dark page, and make
# the edge crisp
photos remove-background --shift -1.5 --hardness 50 bottle.jpg

# Light and colour, shape, room, a mark
photos enhance shoe.jpg                           # shoe-enhanced.jpg
photos crop --aspect 4:5 shoe.jpg                 # shoe-cropped.jpg
photos expand --aspect 1:1 shoe.jpg               # shoe-expanded.jpg
photos watermark --text "© Acme 2026" shoe.jpg    # shoe-watermarked.jpg
photos watermark --logo logo.png --position top-left shoe.jpg
photos resize --trim --dpi 300 scan.png           # border cut away, 300 dpi

# Old and poor photos
photos denoise night.jpg                          # night-denoised.jpg
photos colorize grandmother.jpg                   # grandmother-colorized.jpg
photos restore-faces grandmother.jpg              # grandmother-restored.jpg

# One object, by pointing at it; its mask is what erase takes
photos select --point 420,310 desk.jpg            # desk-selected.png
photos select --point 420,310 --mask -o lamp.png desk.jpg
photos erase --mask lamp.png desk.jpg

# Faces and identity photos
photos blur-faces street.jpg                      # street-nofaces.jpg
photos id-photo --document us-passport me.jpg     # me-us-passport.jpg

# A logo as SVG
photos vectorize logo.png                         # logo-vector.svg

# Name the output yourself
photos resize --width 800 -o thumb.webp --format webp shoe.jpg

# Many files and whole directories, into one directory
photos remove-background --crop --out-dir cutouts *.jpg
photos resize --width 1600 --format webp --out-dir web --recursive photos/
```

Inputs are files, directories (searched for images; `--recursive` goes below
them and `--out-dir` keeps the layout) and `-` for standard input. The
extension of a result is its format, whatever the source was called.

A file that exists is **never replaced** unless you pass `--force`, and a
result is written to a temporary file and moved into place, so a file is
never left half-written, also when you press Ctrl-C. Several files are sent a
few at once (`--parallel`, default 4); one failing does not stop the others,
and each gets its line, in the order you named them.

### What a tool found, and what it cannot know

Some tools say on their line what they found:

```console
$ photos blur-faces --out-dir public team.jpg street.jpg
team.jpg   -> public/team.jpg  1600x1067  image/jpeg  301.2 kB  faces hidden: 5
street.jpg -> public/street.jpg  1600x1200  image/jpeg  288.0 kB  no faces found

$ photos id-photo --document us-passport me.jpg
me.jpg -> me-us-passport.jpg  602x602  image/jpeg  48.3 kB  head: 30.1 mm  findings: background_not_plain
```

`crop` says whether a subject was found (the crop is centred when not),
`remove-background` says `no subject found` when it found nothing to cut out
(the result is then the photo as it was),
`restore-faces` how many faces it restored and skipped, `select` the model's
score, `vectorize` how many paths the SVG has, and `id-photo` the head's
height and its findings. None of that changes the exit code; see
[Exit codes](#exit-codes).

Read these before relying on a result:

- `blur-faces` hides the faces it finds. A small face, a profile, or a face
  seen from behind may not be found, and still shows: check the count. Only
  `--mode fill` leaves nothing of a face.
- `id-photo` keeps the photo's own background unless you give `--background`,
  because passport authorities do not accept edited photos. Its findings are
  what it could see; it cannot judge expression, glasses, shadows or
  likeness, so `no findings` is not an approval. `photos id-photos` lists the
  built-in documents.
- `colorize` guesses the colours: skin and sky come out believable, the
  colour of clothes or cars is invented.
- `restore-faces` invents the detail it draws. It is not for identity photos
  or evidence.
- `caption` can be wrong, and writes English only. Read it before you publish
  it.
- `denoise` is slow without a GPU (about half a minute a megapixel) and takes
  photos up to 12 megapixels; give it a longer `--timeout`.
- `expand` continues plain backgrounds. It does not invent content: a
  subject cut off at the edge stays cut off.

### In a pipeline

With `-` for standard input and `-o -` for standard output, a command is a
filter: nothing but the image is written to standard output.

```sh
cat shoe.jpg | photos resize --width 100 -o - - | file -
# /dev/stdin: JPEG image data, JFIF standard 1.01, ..., 100x75, components 3

curl -fsS https://example.com/shoe.jpg \
  | photos remove-background --background white --format jpg - \
  | photos resize --width 1000 - > shoe-1000.jpg
```

### Output for scripts

`--json` on any command makes standard output machine-readable, with nothing
else on it. Image commands print one JSON object per file (JSON Lines):

```console
$ photos resize --width 800 --json a.jpg broken.jpg c.jpg
{"input":"a.jpg","output":"a-resized.jpg","content_type":"image/jpeg","width":800,"height":600,"quality":85,"bytes":48211}
{"input":"broken.jpg","error":{"code":"bad_request","message":"cannot decode the image (bad_request, HTTP 400)","status":400}}
{"input":"c.jpg","output":"c-resized.jpg","content_type":"image/jpeg","width":800,"height":533,"quality":85,"bytes":51730}
photos: broken.jpg: cannot decode the image (bad_request, HTTP 400)
3 files: 2 done, 1 failed
$ echo $?
1
```

(The last two lines are on standard error.) So:

```sh
# The files that failed, and why
photos resize --width 800 --json photos/ 2>/dev/null \
  | jq -r 'select(.error) | "\(.input)\t\(.error.code)"'

# Which photos would Amazon reject after fixing?
photos fix --marketplace amazon --out-dir amazon --json photos/ \
  | jq -r 'select(.passed == false) | "\(.input): \(.findings | join(", "))"'
```

`remove-text` adds `removed` and `kept` to each object, `fix` adds `passed`
and `findings`. Likewise `crop` adds `subject_found` (and `remove-background`
when a canvas came out empty), `vectorize` adds `paths`, `blur-faces` adds
`faces`, `restore-faces` adds `faces` and `faces_skipped`, `select` adds
`score`, and `id-photo` adds `passed`, `findings` and `head_mm`:

```sh
# Photos in which no face was found, so nothing was hidden
photos blur-faces --out-dir public --json photos/ \
  | jq -r 'select(.faces == 0) | .input'
```

An error has the API's code (`bad_request`,
`unsupported_format`, `payment_required`…) and HTTP status, or one of the
CLI's own codes for what never reached the API: `file_exists`, `unreachable`,
`timeout`, `failed`.

Commands that read a photo print the API's JSON as it came:

```sh
photos check --marketplace amazon --json shoe.jpg | jq '.marketplace.passed'
photos barcodes --json box.jpg | jq -r '.barcodes[].value'
photos ocr label.jpg                      # the text, line by line
photos label front.jpg back.jpg           # one product, up to 8 photos
photos palette --json shoe.jpg | jq -r '.colors[].hex'
photos faces --json team.jpg | jq '.faces | length'
photos caption shoe.jpg                   # one sentence, in English
photos account --json | jq '.credits'
```

Without `--json`, output is for a person: short, aligned, in colour only when
standard output is a terminal and `NO_COLOR` is not set. Progress, warnings
and errors go to standard error. `--quiet` silences everything but errors
(and the data a command was asked for).

### Batch jobs

A job runs one operation on many images on the server; it can outlive your
terminal, and takes URLs as well as files.

```console
$ id=$(photos jobs create remove-background --param format=png photos/)
Started a remove-background job with 240 items. Follow it with: photos jobs wait job_5f0c2a

$ photos jobs download "$id" --out-dir cutouts
job_5f0c2a: running, 96 of 240 done
job_5f0c2a: completed, 240 of 240 done
boot.jpg -> cutouts/boot.png  1600x1067  image/png  701.3 kB
shoe.jpg -> cutouts/shoe.png  1600x1200  image/png  812.4 kB
```

`jobs download` waits for the job first (up to an hour; `--wait-timeout`,
or `--no-wait` for what is finished now). One job uploads at most 100 MB and
1000 images; `jobs create` sends more as several jobs and prints each id on
its own line, and `jobs wait` and `jobs download` take them all:

```sh
photos jobs download --out-dir results $(photos jobs create run --recipe lot.toml photos/)
```

`photos jobs create … --wait` waits and shows the jobs in one step. `jobs list`,
`jobs get <id>` and `jobs delete <id>` do what they say. Every tool is an
operation: its parameters are given with `--param`, named as the command's
flags, and one that repeats is given more than once:

```sh
photos jobs create select --param point=420,310 --param point=440,300 photos/
photos jobs create id-photo --param document=us-passport portraits/
```

A job item is one image, so a `watermark` job takes `text` only, and a
`remove-background` job no background image. `jobs download` says for each
item what the matching command would, and names a `vectorize` result `.svg`. With
`--webhook-url` the job sends a signed event when it is done; verify a
delivery in a handler script with:

```sh
export PHOTOS_WEBHOOK_SECRET=$(photos webhook secret)
photos webhook verify --secret-env PHOTOS_WEBHOOK_SECRET \
  --signature "$HTTP_PHOTOS_SIGNATURE" body.json && echo genuine
```

## Exit codes

Stable, so a script can branch on them:

| Code | Meaning |
|---|---|
| `0` | Everything asked for was done. |
| `1` | A file or the request failed: a bad parameter, a broken image, a file in the way, a job item that failed, a webhook that does not verify. |
| `2` | The command was wrong: an unknown command or flag, a missing argument. Nothing was sent. |
| `3` | Not signed in, or the API refused the key. |
| `4` | Out of credits, or the account is suspended. |
| `5` | The service is busy or could not be reached, after the retries; or a request timed out. |
| `130` | Interrupted (Ctrl-C). |

With several files, the code is that of the most severe failure (the highest
of 1 to 5). When the key is refused or the credits run out, the files not yet
sent are not tried.

```sh
photos resize --width 800 --out-dir web photos/
case $? in
  0) echo "all done" ;;
  1) echo "some files failed; see above" ;;
  3) echo "set PHOTOS_API_KEY" ;;
  4) echo "out of credits" ;;
  5) echo "try again later" ;;
esac
```

A marketplace verdict is not an exit code: `photos check` and `photos fix`
exit 0 when they ran, whatever they found. Read `.marketplace.passed` or
`.passed` from `--json`.

The same holds for what the other tools find. `id-photo` with findings,
`blur-faces`, `restore-faces` and `faces` with no face found, and `crop`
with no subject found all exit 0: read `.findings`, `.faces` or
`.subject_found`. What the API refuses is a failure (exit 1): `select` when
nothing is found at the points, `id-photo` when the photo has no face or
more than one, `denoise` on a photo over 12 megapixels.

## Configuration

| Setting | Flag | Environment | Default |
|---|---|---|---|
| API key | (never a flag) | `PHOTOS_API_KEY` | the key saved by `photos login` |
| API address | `--base-url` | `PHOTOS_BASE_URL` | `https://api.photos.sh` |
| Time for one request | `--timeout` | | `2m` |
| Retries | `--retries` | | `2` |
| Where the key is saved | | `PHOTOS_CONFIG_DIR` | `~/.config/photos-sh` (Linux), `~/Library/Application Support/photos-sh` (macOS), `%AppData%\photos-sh` (Windows) |
| Colour | | `NO_COLOR` | on a terminal only |

A flag wins over the environment, which wins over what is saved. A key saved
while `--base-url` or `PHOTOS_BASE_URL` pointed somewhere else is saved with
that address and used with it, so it is not sent to another API by accident.

Retries are the library's: a request is sent again only when the API answers
`503 busy` (after the `Retry-After` it gives) or could not be reached at all.
Nothing else is retried. `--timeout` bounds one attempt.

## Development

```sh
make check      # gofmt, go vet, go test -race
make usage      # regenerate USAGE.md after changing a command or a flag
make dist       # check, then all six platforms and checksums.txt
```

The tests need no network: they run the commands against an in-process
stand-in for the API. `USAGE.md` is generated from the command tree, and a
test fails when it is stale.

The only dependency is the Go library,
[photos-go](https://github.com/photos-sh/photos-go). To work on both together,
add a `replace` line to `go.mod` that points at a checkout of the library
(`replace github.com/photos-sh/photos-go => ../photos-go`), and leave it out
of what you commit.

## Licence

MIT. See [LICENSE](LICENSE).
