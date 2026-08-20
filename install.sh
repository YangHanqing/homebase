#!/bin/sh
# Download a Homebase release and place the binary on PATH.
# Does not compile, does not install launchd/systemd — that is `homebase start`.
#
# tmux search order must stay in sync with internal/tmux.LocalBinary.

set -eu

REPO="yanghanqing/homebase"
DEST="${HOMEBASE_INSTALL_DIR:-${HOME}/.local/bin}"

workdir=""
tmpbin=""
cleanup() {
  if [ -n "$workdir" ]; then
    rm -rf "$workdir"
  fi
  if [ -n "$tmpbin" ] && [ -e "$tmpbin" ]; then
    rm -f "$tmpbin"
  fi
}
trap cleanup EXIT

die() {
  echo "$*" >&2
  exit 1
}

need_curl() {
  command -v curl >/dev/null 2>&1 || die "curl is required to download Homebase"
}

# Same order as internal/tmux.LocalBinary — keep both in sync.
find_tmux() {
  if command -v tmux >/dev/null 2>&1; then
    command -v tmux
    return 0
  fi
  for p in /opt/homebrew/bin/tmux /usr/local/bin/tmux /usr/bin/tmux /opt/local/bin/tmux; do
    if [ -f "$p" ]; then
      echo "$p"
      return 0
    fi
  done
  return 1
}

print_tmux_hint() {
  os=$1
  echo "homebase is installed, but tmux was not found." >&2
  echo "Homebase is a tmux frontend — install tmux, then start it." >&2
  echo >&2
  if [ "$os" = "darwin" ]; then
    if command -v brew >/dev/null 2>&1; then
      echo "  macOS (Homebrew):     brew install tmux" >&2
    else
      echo "  macOS: install Homebrew (https://brew.sh), then:  brew install tmux" >&2
      echo "  or:    https://github.com/tmux/tmux/wiki/Installing" >&2
    fi
    echo "  other systems: Debian/Ubuntu apt, Fedora dnf, Arch pacman" >&2
  else
    id=""
    if [ -f /etc/os-release ]; then
      # shellcheck disable=SC2002
      id=$(cat /etc/os-release | grep '^ID=' | head -n 1 | cut -d= -f2 | tr -d '"')
    fi
    case $id in
      debian|ubuntu)
        echo "  Debian / Ubuntu:      sudo apt install tmux" >&2
        echo "  other systems: macOS brew, Fedora dnf, Arch pacman" >&2
        ;;
      fedora)
        echo "  Fedora:               sudo dnf install tmux" >&2
        echo "  other systems: macOS brew, Debian/Ubuntu apt, Arch pacman" >&2
        ;;
      arch|manjaro)
        echo "  Arch:                 sudo pacman -S tmux" >&2
        echo "  other systems: macOS brew, Debian/Ubuntu apt, Fedora dnf" >&2
        ;;
      *)
        echo "  Debian / Ubuntu:      sudo apt install tmux" >&2
        echo "  Fedora:               sudo dnf install tmux" >&2
        echo "  Arch:                 sudo pacman -S tmux" >&2
        echo "  other systems: macOS brew install tmux" >&2
        ;;
    esac
  fi
  echo >&2
  echo "Then:  homebase start" >&2
}

sha256_of() {
  file=$1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    die "need sha256sum or shasum to verify the download (will not skip checksums)"
  fi
}

detect_os() {
  sys=$(uname -s | tr '[:upper:]' '[:lower:]')
  case $sys in
    darwin) echo darwin ;;
    linux) echo linux ;;
    mingw*|msys*|cygwin*|windows*)
      echo "Homebase cannot host on Windows: it is a tmux frontend, and Windows has no tmux." >&2
      echo "Open the web UI from a Windows browser against a macOS or Linux host." >&2
      exit 1
      ;;
    *)
      echo "unsupported OS: $(uname -s) (v1 hosts are macOS and Linux)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  m=$(uname -m)
  case $m in
    x86_64|amd64) echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *)
      echo "unsupported architecture: $m (v1 supports amd64 and arm64)" >&2
      exit 1
      ;;
  esac
}

os=$(detect_os)
arch=$(detect_arch)
need_curl

workdir=$(mktemp -d)

version="${HOMEBASE_VERSION:-}"
if [ "$version" = "latest" ]; then
  version=""
fi

if [ -n "$version" ]; then
  version=${version#v}
  base="https://github.com/${REPO}/releases/download/v${version}"
  tarball="homebase_${version}_${os}_${arch}.tar.gz"
else
  base="https://github.com/${REPO}/releases/latest/download"
  tarball=""
fi

echo "downloading checksums.txt"
curl -fsSL "$base/checksums.txt" -o "$workdir/checksums.txt"

if [ -z "$tarball" ]; then
  # latest: the version lives in the checksums filenames.
  tarball=$(awk -v os="$os" -v arch="$arch" '
    $2 ~ ("^homebase_.+_" os "_" arch "\\.tar\\.gz$") { print $2; exit }
  ' "$workdir/checksums.txt")
  [ -n "$tarball" ] || die "checksums.txt has no homebase_*_${os}_${arch}.tar.gz entry"
fi

expect=$(awk -v f="$tarball" '$2 == f { print $1; exit }' "$workdir/checksums.txt")
[ -n "$expect" ] || die "checksums.txt has no entry for $tarball"

echo "downloading $tarball"
curl -fsSL "$base/$tarball" -o "$workdir/$tarball"

got=$(sha256_of "$workdir/$tarball")
if [ "$got" != "$expect" ]; then
  die "checksum mismatch for $tarball (got $got, want $expect)"
fi

tar -xzf "$workdir/$tarball" -C "$workdir"
[ -f "$workdir/homebase" ] || die "tarball did not contain a homebase binary"

chmod +x "$workdir/homebase"

if [ "$os" = "darwin" ]; then
  xattr -d com.apple.quarantine "$workdir/homebase" 2>/dev/null || true
fi

if ! mkdir -p "$DEST" 2>/dev/null; then
  die "cannot create $DEST — create it, or set HOMEBASE_INSTALL_DIR to a directory you can write"
fi
if [ ! -w "$DEST" ]; then
  die "cannot write to $DEST — create ~/.local/bin, or set HOMEBASE_INSTALL_DIR to a directory you can write"
fi

# Same filesystem as DEST so mv is a rename (running processes keep the old inode).
tmpbin=$(mktemp "$DEST/.homebase.XXXXXX")
cp "$workdir/homebase" "$tmpbin"
chmod +x "$tmpbin"
if [ "$os" = "darwin" ]; then
  xattr -d com.apple.quarantine "$tmpbin" 2>/dev/null || true
fi
mv "$tmpbin" "$DEST/homebase"
tmpbin=""

echo "installed $DEST/homebase"

case ":$PATH:" in
  *":$DEST:"*) ;;
  *)
    echo
    echo "$DEST is not on PATH. Add this to ~/.zshrc or ~/.bashrc:"
    echo "  export PATH=\"$DEST:\$PATH\""
    echo "then open a new terminal."
    ;;
esac

if find_tmux >/dev/null; then
  echo
  echo "If a Homebase service is already running:  homebase restart"
  echo "Next:  homebase start"
  exit 0
fi

echo >&2
print_tmux_hint "$os"
exit 2
