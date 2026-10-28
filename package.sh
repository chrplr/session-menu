#!/usr/bin/env bash
# Copyright (2026) Christophe Pallier <christophe@pallier.org>
# Licensed under the Apache License, Version 2.0 (see LICENSE).

# Build session-menu for Linux, Windows and macOS, and pack each build with
# README.md, LICENSE and NOTICE, for the GitHub releases (see
# .github/workflows/release.yml). From anywhere:
#
#   ./package.sh
#
# Outputs, in dist/:
#   session-menu_<version>_linux_amd64.tar.gz
#   session-menu_<version>_windows_amd64.zip
#   session-menu_<version>_macos_arm64.tar.gz    (Apple silicon)
#   session-menu_<version>_macos_amd64.tar.gz    (Intel)
#   SHA256SUMS
#
# <version> is the tag of the commit (e.g. v0.2.0), which go build also
# writes into the program (session-menu -version); without a tag, the short
# commit hash.

set -euo pipefail
cd "$(dirname "$0")"

VERSION="$(git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD)"
export CGO_ENABLED=0

for tool in zip tar sha256sum; do
  command -v "${tool}" >/dev/null 2>&1 || { echo "error: '${tool}' is not installed" >&2; exit 1; }
done

DIST="$(pwd)/dist"
STAGE="$(mktemp -d)"
trap 'rm -rf "${STAGE}"' EXIT
rm -rf "${DIST}"
mkdir -p "${DIST}"

# goos goarch name: name is the platform in the archive's name
for target in "linux amd64 linux" "windows amd64 windows" "darwin arm64 macos" "darwin amd64 macos"; do
  read -r goos goarch name <<<"${target}"
  base="session-menu_${VERSION}_${name}_${goarch}"
  dir="${STAGE}/${base}"
  mkdir -p "${dir}"
  exe="session-menu"
  [[ "${goos}" == windows ]] && exe="session-menu.exe"
  echo "${base}"
  GOOS="${goos}" GOARCH="${goarch}" go build -trimpath -ldflags="-s -w" -o "${dir}/${exe}" .
  cp README.md LICENSE NOTICE "${dir}/"
  if [[ "${goos}" == windows ]]; then
    (cd "${STAGE}" && zip -qr "${DIST}/${base}.zip" "${base}")
  else
    tar -C "${STAGE}" -czf "${DIST}/${base}.tar.gz" "${base}"
  fi
done

(cd "${DIST}" && sha256sum session-menu_* > SHA256SUMS)
ls -l "${DIST}"
