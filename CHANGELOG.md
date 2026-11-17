# Changelog

All notable changes to the `photos` command-line client are recorded here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and the versions follow [semantic versioning](https://semver.org/). The exit
codes and the shape of `--json` output are part of what a version promises.

## Unreleased

## 1.3.0 (2026-11-17)

### Added

- `photos jobs create` sends more than one job may hold as several jobs, one
  after another, whole images in the order given: at most `--max-upload-mb`
  (100 MB, the service's limit for a job upload) and 1000 images each. Each
  id is printed on its own line as its job starts. One image over the limit
  stops it before anything is sent.
- `photos jobs wait` and `photos jobs download` take several job ids, waited
  for one after another within one `--wait-timeout`; `download` saves the
  results of all of them into one directory, named as one.

### Changed

- `photos jobs download` waits for each job to complete before saving
  (`--interval`, `--wait-timeout`); `--no-wait` saves only what is finished,
  as it did before.
- `--wait-timeout` is an hour by default (it was 10 minutes, which a job of a
  few hundred photos queued behind others outlasted).
- Splitting at 100 MB needs the engine release that raised the job upload
  limit; against an older one use `--max-upload-mb 24`.

## 1.2.0 (2026-11-16)

### Added

- `photos run` shows what a recipe's `[barcodes]` and `[text]` steps read off
  the photo: the codes on the file's line and the lines of text under it, and
  with `--json` the `barcodes` and `text` fields (left out without the step,
  `[]` when it found nothing; `text_truncated` when the service left lines
  out). `photos jobs download` shows them for `run` jobs too. Reading needs
  the engine release that added it; an older one refuses those steps.
- `photos presets <name>` prints that preset's recipe as TOML, to start a
  recipe of your own from (`--json`: `{"name", "recipe"}`).

### Changed

- Built with photos-go 1.2.1 (for `Image.Barcodes`, `Image.Text` and
  `Client.Preset`; its User-Agent now says its own version).

## 1.1.0 (2026-11-16)

### Added

- `photos update`: replaces this binary with the latest release, downloaded
  for this machine and checked against the release's `checksums.txt`, as the
  installer does. `--check` only says whether there is a newer one, `--to`
  installs a particular release, and `--force` replaces a development build.
- `remove-background --studio`, for studio shots: a product on a plain, light
  backdrop. It also keeps what clearly stands out from the backdrop and
  touches the product, such as the gift box, case or lid shown with a bottle.
  Shadows and the floor are left out, and a photo without a plain light
  backdrop is cut out as without it.

### Changed

- Built with photos-go 1.1.0 (for `RemoveBackgroundOptions.Studio`).
- `remove-background` with no subject in the photo: the service now returns
  the photo as it was, with or without a canvas, instead of an empty canvas.
  The help, the README and USAGE.md say so; the line still says
  `no subject found`, and `--json` has `"subject_found": false`.

## 1.0.0 (2026-11-13)

First version.

- `identify` matches against the organisation's own catalogues (uploaded in
  the console; turned on per organisation). Without it the API answers
  `404 not_found`, and `photos identify` says that catalogue
  matching is not on or no catalogue is uploaded. A candidate's evidence can be `sku`.
- Image commands that write one result per input: `remove-background` (alias
  `bg-remove`), `resize`, `upscale`, `erase`, `remove-text`, `run` (presets
  and recipes) and `fix`. Inputs are files, directories (`--recursive`) and
  standard input; outputs go beside their sources, into `--out-dir`, to `-o
  file`, or to standard output with `-o -`. Existing files are not replaced
  without `--force`; results are written through a temporary file. Several
  files are sent at once (`--parallel`).
- Commands that read a photo: `check`, `barcodes`, `ocr`, `label`, `identify`.
- `presets`, `marketplaces`, `account`, `health`.
- The editing tools the API added on 2026-11-12, one command each, with a
  flag for every option: `enhance`, `crop`, `expand`, `watermark` (`--text`
  or `--logo file`), `denoise`, `colorize`, `select` (`--point x,y` and
  `--exclude x,y`, repeatable, and `--box x,y,w,h`), `blur-faces`,
  `restore-faces`, `id-photo` and `vectorize` (writes `.svg`) write one
  result per input; `palette`, `faces` and `caption` read a photo;
  `id-photos` lists the documents `id-photo` knows.
- `remove-background` stages the cutout: `--width`, `--height`, `--fill`,
  `--align`, `--shadow`, `--reflection`, `--blur` and `--background-image`.
- `resize` takes `--trim`, `--trim-tolerance` and `--dpi`.
- What a tool found is on its line and in `--json`: `subject_found` (`crop`,
  and `remove-background` when a canvas came out empty), `paths`
  (`vectorize`), `faces` (`blur-faces`), `faces` and `faces_skipped`
  (`restore-faces`), `score` (`select`), and `passed`, `findings` and
  `head_mm` (`id-photo`). `jobs download` reports the same for a job's items.
  None of it changes the exit code, as with `check` and `fix`: findings on an
  id photo, no face found and no subject found exit 0.
- `jobs create` takes the new operations; a parameter that repeats is given
  more than once (`--param point=1,2 --param point=3,4`).
- Batch jobs: `jobs create` (files and URLs, `--param`, `--recipe`,
  `--webhook-url`, `--wait`), `jobs list`, `jobs get`, `jobs wait`,
  `jobs download`, `jobs delete`.
- `webhook verify` and `webhook secret`.
- `login` and `logout`: the key is read without echo or from standard input,
  checked with the API and saved with mode 0600. `PHOTOS_API_KEY` wins over a
  saved key.
- `--json` on every command, `--quiet`, `--base-url`, `--timeout`,
  `--retries`.
- Exit codes: 0 done, 1 failed, 2 usage, 3 not signed in or key refused,
  4 out of credits or suspended, 5 busy or unreachable, 130 interrupted.
- `completion bash|zsh|fish`, `help`, `version`.
- Builds for Linux, macOS and Windows on amd64 and arm64; `installer.sh`.
