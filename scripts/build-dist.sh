#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="${ROOT}/dist"

rm -rf "${DIST}"
mkdir -p "${DIST}"

cd "${ROOT}"
go test ./...

cd "${ROOT}/ide/goland"
gradle test buildPlugin
GOLAND_VERSION="$(gradle -q properties | awk -F': ' '/^version:/ {print $2; exit}')"
cp "build/distributions/go-clue-goland-plugin-${GOLAND_VERSION}.zip" "${DIST}/"

bash "${ROOT}/scripts/build-vscode.sh"

bash "${ROOT}/scripts/build-sublime.sh"
bash "${ROOT}/scripts/build-vim.sh"
bash "${ROOT}/scripts/build-neovim.sh"

cd "${DIST}"
find . -maxdepth 1 -type f ! -name SHA256SUMS.txt -print0 |
  sort -z |
  xargs -0 sha256sum > SHA256SUMS.txt
