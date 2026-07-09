#!/usr/bin/env bash
# Generates darwin/icons.icns and windows/icon.ico from build/appicon.png.
# Run from the project root (called via build/Taskfile.yml generate:icons task).
set -euo pipefail

SRC="build/appicon.png"

if [[ ! -f "$SRC" ]]; then
  echo "error: $SRC not found; run 'task common:prepare:appicon' first" >&2
  exit 1
fi

# ── macOS .icns ───────────────────────────────────────────────────────────────
if command -v iconutil >/dev/null 2>&1 && command -v sips >/dev/null 2>&1; then
  ICONSET="build/darwin/icons.iconset"
  mkdir -p "$ICONSET"

  for size in 16 32 64 128 256 512; do
    sips -z "$size" "$size" "$SRC" --out "${ICONSET}/icon_${size}x${size}.png"   >/dev/null
    sips -z "$((size*2))" "$((size*2))" "$SRC" --out "${ICONSET}/icon_${size}x${size}@2x.png" >/dev/null
  done

  iconutil -c icns -o build/darwin/icons.icns "$ICONSET"
  rm -rf "$ICONSET"
  echo "Generated build/darwin/icons.icns"
elif command -v convert >/dev/null 2>&1; then
  mkdir -p build/darwin
  convert -resize 512x512 "$SRC" build/darwin/icons.icns
  echo "Generated build/darwin/icons.icns (via ImageMagick — not a proper .icns)"
else
  echo "warning: iconutil/sips or convert not found; skipping .icns generation" >&2
fi

# ── Windows .ico ──────────────────────────────────────────────────────────────
if command -v convert >/dev/null 2>&1; then
  mkdir -p build/windows
  convert "$SRC" \
    \( -clone 0 -resize 16x16   \) \
    \( -clone 0 -resize 32x32   \) \
    \( -clone 0 -resize 48x48   \) \
    \( -clone 0 -resize 64x64   \) \
    \( -clone 0 -resize 128x128 \) \
    \( -clone 0 -resize 256x256 \) \
    -delete 0 build/windows/icon.ico
  echo "Generated build/windows/icon.ico"
elif command -v magick >/dev/null 2>&1; then
  mkdir -p build/windows
  magick "$SRC" \
    \( -clone 0 -resize 16x16   \) \
    \( -clone 0 -resize 32x32   \) \
    \( -clone 0 -resize 48x48   \) \
    \( -clone 0 -resize 64x64   \) \
    \( -clone 0 -resize 128x128 \) \
    \( -clone 0 -resize 256x256 \) \
    -delete 0 build/windows/icon.ico
  echo "Generated build/windows/icon.ico"
else
  echo "warning: ImageMagick (convert/magick) not found; skipping .ico generation" >&2
fi
