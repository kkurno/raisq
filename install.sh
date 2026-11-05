#!/bin/sh
# Installs or updates RaisQ: curl -fsSL https://raw.githubusercontent.com/kkurno/raisq/main/install.sh | sh
# RAISQ_VERSION=v0.4.0 picks a version; RAISQ_DIR=~/bin picks the folder.
set -eu

repo=kkurno/raisq

fail() { echo "raisq install: $*" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "this script is for macOS and Linux. On Windows, download raisq-windows-*.exe from https://github.com/$repo/releases" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "no RaisQ build for $(uname -m). Build it with Go: go install github.com/$repo/cmd/raisq@latest" ;;
esac

if command -v curl >/dev/null 2>&1; then
  get() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  get() { wget -q "$1" -O "$2"; }
else
  fail "needs curl or wget"
fi

version="${RAISQ_VERSION:-latest}"
if [ "$version" = latest ]; then
  base="https://github.com/$repo/releases/latest/download"
else
  base="https://github.com/$repo/releases/download/$version"
fi
file="raisq-$os-$arch"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
get "$base/$file" "$tmp/$file" || fail "couldn't download $base/$file"
get "$base/checksums.txt" "$tmp/checksums.txt" || fail "couldn't download $base/checksums.txt"

# A file that doesn't match the release's checksum is never installed.
want="$(awk -v f="$file" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")"
[ -n "$want" ] || fail "$file isn't listed in the release's checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  got="$(sha256sum "$tmp/$file" | awk '{ print $1 }')"
else
  got="$(shasum -a 256 "$tmp/$file" | awk '{ print $1 }')"
fi
[ "$got" = "$want" ] || fail "the download doesn't match its checksum, so it wasn't installed. Try again"

on_path() { case ":$PATH:" in *":$1:"*) return 0 ;; *) return 1 ;; esac; }

# A folder already on PATH, so raisq runs straight away.
dir="${RAISQ_DIR:-}"
if [ -z "$dir" ]; then
  for d in "$HOME/.local/bin" "$HOME/bin" /usr/local/bin /opt/homebrew/bin; do
    if on_path "$d" && { [ -w "$d" ] || { [ ! -e "$d" ] && [ -w "$(dirname "$d")" ]; }; }; then
      dir="$d"
      break
    fi
  done
fi
[ -n "$dir" ] || dir="$HOME/.local/bin"

mkdir -p "$dir"
chmod +x "$tmp/$file"
mv "$tmp/$file" "$dir/raisq"

echo "Installed $("$dir/raisq" version) to $dir/raisq"
if ! on_path "$dir"; then
  case "${SHELL:-}" in
    */zsh) rc="~/.zshrc" ;;
    */bash) rc="~/.bashrc" ;;
    *) rc="your shell's startup file" ;;
  esac
  echo
  echo "$dir isn't on your PATH yet, so the raisq command won't be found. Add it once:"
  echo "  echo 'export PATH=\"$dir:\$PATH\"' >> $rc"
  echo "Then open a new terminal."
fi
