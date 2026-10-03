#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="${ROOT}/dist"

mkdir -p "${DIST}"
cd "${ROOT}/ide/vscode"
npm ci
node --check extension.js
VERSION="$(node -p "require('./package.json').version")"
npx --yes @vscode/vsce package --out "${DIST}/go-clue-vscode-${VERSION}.vsix"
