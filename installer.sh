#!/usr/bin/env bash
#
# Installs the photos CLI, the command-line client for Photos.sh.
#
#   curl -fsSL https://raw.githubusercontent.com/photos-sh/photos-cli/main/installer.sh | bash
#
# What it needs: curl, and either sha256sum or shasum. Nothing else: no jq,
# no tar, no unzip. The release asset is the binary itself.
#
# What it does: finds the build for this machine in the latest GitHub release
# of photos-sh/photos-cli, downloads it and the release's checksums.txt,
# and installs the binary only if its SHA-256 is the one checksums.txt lists.
#
# Settings, all optional:
#
#   PHOTOS_INSTALL_DIR   where to put the binary. Default: /usr/local/bin if
#                        you can write there, else ~/.local/bin.
#   PHOTOS_VERSION       a release to install instead of the latest: v1.0.0.
#   PHOTOS_RELEASES_URL  where the releases are, for a mirror. Default:
#                        https://github.com/photos-sh/photos-cli/releases
set -euo pipefail

REPO="photos-sh/photos-cli"
RELEASES="${PHOTOS_RELEASES_URL:-https://github.com/$REPO/releases}"
BINARY=photos

# Said on stderr, so that standard output stays empty for whoever captures it.
say() { printf '%s\n' "$*" >&2; }
die() { printf 'installer: %s\n' "$*" >&2; exit 1; }

# The temporary files, removed however this ends. Not local to main: the
# trap runs after main has returned.
tmp=""
sums=""
cleanup() {
  [ -z "$tmp" ] || rm -f "$tmp"
  [ -z "$sums" ] || rm -f "$sums"
}
trap cleanup EXIT

need() { command -v "$1" >/dev/null 2>&1 || die "this needs $1 on PATH."; }

# The release asset for this machine, in Go's spelling of both halves, which
# is how the assets are named: photos-linux-amd64, photos-windows-arm64.exe.
asset() {
  local os arch ext=""
  os="$(uname -s)"
  arch="$(uname -m)"

  case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    MINGW* | MSYS* | CYGWIN*) os=windows; ext=.exe ;;
    *) die "no build for $os. There are builds for Linux, macOS and Windows." ;;
  esac

  case "$arch" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) die "no build for $arch. There are builds for amd64 and arm64." ;;
  esac

  printf '%s-%s-%s%s' "$BINARY" "$os" "$arch" "$ext"
}

# Where a file of the release is. "latest/download" is GitHub's own redirect
# to the newest release that is not a pre-release, so no API call and no JSON
# to parse.
url() {
  if [ -n "${PHOTOS_VERSION:-}" ]; then
    printf '%s/download/%s/%s' "$RELEASES" "$PHOTOS_VERSION" "$1"
  else
    printf '%s/latest/download/%s' "$RELEASES" "$1"
  fi
}

# Where to put it: asked for, or the first writable of the usual two.
install_dir() {
  if [ -n "${PHOTOS_INSTALL_DIR:-}" ]; then
    printf '%s' "$PHOTOS_INSTALL_DIR"
  elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    printf '/usr/local/bin'
  else
    printf '%s/.local/bin' "$HOME"
  fi
}

main() {
  need curl
  need uname

  local sha_tool
  if command -v sha256sum >/dev/null 2>&1; then
    sha_tool="sha256sum"
  elif command -v shasum >/dev/null 2>&1; then
    sha_tool="shasum -a 256"
  else
    die "this needs sha256sum or shasum to verify the download."
  fi

  local name dir target
  name="$(asset)"
  dir="$(install_dir)"
  target="$dir/$BINARY"
  case "$name" in *.exe) target="$target.exe" ;; esac

  mkdir -p "$dir" || die "cannot create $dir."
  [ -w "$dir" ] || die "cannot write to $dir. Set PHOTOS_INSTALL_DIR to a directory you can write to."

  # Downloaded into the target directory, so that the move at the end is a
  # rename within one filesystem: the binary is there whole or not at all.
  tmp="$(mktemp "$dir/.$BINARY.XXXXXX")" || die "cannot write a temporary file in $dir."
  sums="$(mktemp "$dir/.$BINARY-checksums.XXXXXX")" || die "cannot write a temporary file in $dir."

  say "Downloading $name (${PHOTOS_VERSION:-latest release})."
  curl -fsSL --max-time 300 -o "$tmp" "$(url "$name")" \
    || die "could not download $(url "$name"). Is there a release with a build for this machine?"
  curl -fsSL --max-time 60 -o "$sums" "$(url checksums.txt)" \
    || die "could not download $(url checksums.txt). Nothing was installed."

  # The line of checksums.txt for this asset: "<sha256>  <name>" (sha256sum
  # marks binary mode with a star before the name).
  local want got
  want="$(awk -v name="$name" '{ file = $2; sub(/^\*/, "", file); if (file == name) { print $1; exit } }' "$sums")"
  [ -n "$want" ] || die "checksums.txt does not list $name. Nothing was installed."
  got="$($sha_tool "$tmp" | cut -d' ' -f1)"
  if [ "$got" != "$want" ]; then
    die "checksum mismatch for $name: checksums.txt says $want, the download is $got. Nothing was installed."
  fi

  chmod 0755 "$tmp"
  mv -f "$tmp" "$target"
  say "Installed $target ($("$target" --version 2>/dev/null || echo "version unknown"))."

  case ":${PATH}:" in
    *":$dir:"*)
      local first
      first="$(command -v "$BINARY" 2>/dev/null || true)"
      if [ -n "$first" ] && [ "$first" != "$target" ]; then
        say ""
        say "Note: $first comes first on your PATH, so that is what runs."
        say "Remove it, or put $dir earlier."
      else
        say "Sign in with: $BINARY login"
      fi
      ;;
    *)
      say ""
      say "$dir is not on your PATH. Add it:"
      say "    export PATH=\"$dir:\$PATH\""
      ;;
  esac
}

main "$@"
