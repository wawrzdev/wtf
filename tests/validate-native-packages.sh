#!/usr/bin/env bash
set -euo pipefail

dist=${1:-dist}
expected_bash=etc/bash_completion.d/wawrzdev-wtf
old_bash=usr/share/bash-completion/completions/wtf
expected_zsh=usr/share/zsh/site-functions/_wtf
expected_fish=usr/share/fish/vendor_completions.d/wtf.fish

shopt -s nullglob
debs=("$dist"/wtf_*_linux_*.deb)
arch_packages=("$dist"/wtf_*_linux_*.pkg.tar.zst)
if (( ${#debs[@]} != 2 || ${#arch_packages[@]} != 2 )); then
  echo "expected two Debian and two Arch Linux packages" >&2
  exit 1
fi

normalize_paths() {
  sed -e 's#^\./##' -e 's#/$##'
}

assert_paths() {
  local listing=$1
  for expected in "$expected_bash" "$expected_zsh" "$expected_fish"; do
    grep -Fxq "$expected" "$listing"
  done
  if grep -Fxq "$old_bash" "$listing"; then
    echo "package still owns the distribution bash-completion path" >&2
    exit 1
  fi
}

for package in "${debs[@]}"; do
  listing=$(mktemp)
  dpkg-deb --fsys-tarfile "$package" | tar -tf - | normalize_paths > "$listing"
  assert_paths "$listing"
  dpkg-deb --fsys-tarfile "$package" | tar -xOf - "./$expected_bash" | cmp - completions/wtf.bash
  rm -f "$listing"
done

for package in "${arch_packages[@]}"; do
  listing=$(mktemp)
  bsdtar -tf "$package" | normalize_paths > "$listing"
  assert_paths "$listing"
  bsdtar -xOf "$package" "$expected_bash" | cmp - completions/wtf.bash
  rm -f "$listing"
done
