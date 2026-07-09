#!/usr/bin/env bash
# Sync release version from tag into build assets (Info.plist, versioninfo.json, nsis tools.nsh).
#
# Usage:
#   VERSION=1.2.3 bash scripts/sync-release-version.sh
#   bash scripts/sync-release-version.sh   # uses GITHUB_REF_NAME or 0.0.0-dev
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

raw="${VERSION:-${GITHUB_REF_NAME:-}}"
raw="${raw#v}"

if [[ -z "$raw" || "$raw" == "main" || "$raw" == "master" ]]; then
  raw="0.0.0-dev"
fi

echo "Syncing version: ${raw}"

# ── macOS Info.plist ──────────────────────────────────────────────────────────
for plist in build/darwin/Info.plist build/darwin/Info.dev.plist; do
  if [[ -f "$plist" ]]; then
    if [[ "$(uname -s)" == "Darwin" ]]; then
      sed -i '' \
        -e "s|<string>[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*</string>|<string>${raw}</string>|g" \
        "$plist"
    else
      sed -i \
        -e "s|<string>[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*</string>|<string>${raw}</string>|g" \
        "$plist"
    fi
    echo "Updated ${plist}"
  fi
done

# ── Windows versioninfo.json ──────────────────────────────────────────────────
VINFO="build/windows/versioninfo.json"
if [[ -f "$VINFO" ]]; then
  IFS='.' read -r major minor patch _ <<< "${raw//-*/}"  # strip pre-release suffix
  major="${major:-0}"; minor="${minor:-0}"; patch="${patch:-0}"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$VINFO" "$major" "$minor" "$patch" "$raw" <<'PYEOF'
import json, sys

path, major, minor, patch, raw = sys.argv[1:]
major, minor, patch = int(major), int(minor), int(patch)

with open(path) as f:
    v = json.load(f)

for key in ("FileVersion", "ProductVersion"):
    if key in v.get("FixedFileInfo", {}):
        v["FixedFileInfo"][key] = {"Major": major, "Minor": minor, "Patch": patch, "Build": 0}

for key in ("FileVersion", "ProductVersion"):
    if key in v.get("StringFileInfo", {}):
        v["StringFileInfo"][key] = f"{raw}.0"

with open(path, "w") as f:
    json.dump(v, f, indent="\t")
    f.write("\n")

print(f"Updated {path}")
PYEOF
  else
    echo "warning: python3 not found; skipping versioninfo.json update" >&2
  fi
fi

# ── NSIS tools.nsh ────────────────────────────────────────────────────────────
NSH="build/windows/nsis/tools.nsh"
if [[ -f "$NSH" ]]; then
  if [[ "$(uname -s)" == "Darwin" ]]; then
    sed -i '' "s/!define INFO_PRODUCTVERSION \"[^\"]*\"/!define INFO_PRODUCTVERSION \"${raw}\"/" "$NSH"
  else
    sed -i "s/!define INFO_PRODUCTVERSION \"[^\"]*\"/!define INFO_PRODUCTVERSION \"${raw}\"/" "$NSH"
  fi
  echo "Updated ${NSH}"
fi

echo "Version sync complete: ${raw}"
