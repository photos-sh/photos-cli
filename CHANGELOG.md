# Changelog

All notable changes to the `photos` command-line client are recorded here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and the versions follow [semantic versioning](https://semver.org/). The exit
codes and the shape of `--json` output are part of what a version promises.

## 1.0.0 (unreleased)

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
