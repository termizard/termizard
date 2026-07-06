#!/usr/bin/env bash
# Sync release version from tag into build/config.yml and regenerate Wails build assets.
#
# Usage:
#   VERSION=0.5.3 bash scripts/sync-release-version.sh
#   bash scripts/sync-release-version.sh   # uses GITHUB_REF_NAME or 0.0.0-dev
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

raw="${VERSION:-${GITHUB_REF_NAME:-}}"
raw="${raw#v}"

if [[ -z "$raw" || "$raw" == "main" || "$raw" == "master" ]]; then
  raw="0.0.0-dev"
fi

CONFIG="build/config.yml"
if [[ ! -f "$CONFIG" ]]; then
  echo "error: $CONFIG not found" >&2
  exit 1
fi

if [[ "$(uname -s)" == "Darwin" ]]; then
  sed -i '' "s/^  version: .*/  version: \"${raw}\"/" "$CONFIG"
else
  sed -i "s/^  version: .*/  version: \"${raw}\"/" "$CONFIG"
fi

echo "Set version to ${raw} in ${CONFIG}"

if ! command -v wails3 >/dev/null 2>&1; then
  echo "error: wails3 not found in PATH" >&2
  exit 1
fi

cd build
wails3 update build-assets -name termizard -binaryname termizard -config config.yml -dir .

echo "Build assets updated for version ${raw}"
