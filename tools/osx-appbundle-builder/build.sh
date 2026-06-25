#!/usr/bin/env bash
set -euo pipefail

# ── Platform guard ─────────────────────────────────────────────────────────
if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "error: this script only runs on macOS (Darwin)" >&2
    exit 1
fi

# ── Paths ──────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
GUI_ASSETS="$PROJECT_ROOT/tools/neoviolet-gui/assets"

APP_NAME="NeoViolet GUI"
BUNDLE_ID="org.aurorast.neovioletgui"

VERSION="${VERSION:-$(git -C "$PROJECT_ROOT" describe --tags --always --dirty 2>/dev/null || echo "0.1.0")}"
BUILD="${BUILD:-$(git -C "$PROJECT_ROOT" rev-list --count HEAD 2>/dev/null || echo "1")}"
APP_DIR="$PROJECT_ROOT/dist/${APP_NAME}.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"
SIGN_IDENTITY="${SIGN_IDENTITY:--}"

ICON_SRC="$GUI_ASSETS/NeoViolet.icon"
ICON_NAME="NeoViolet"
PRE_COMPILED="$GUI_ASSETS/pre-compiled"

# ── Build ──────────────────────────────────────────────────────────────────
echo "Building (make build) …"
cd "$PROJECT_ROOT"
make build

# ── Bundle skeleton ────────────────────────────────────────────────────────
rm -rf "$APP_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

for bin in neoviolet-gui neoviolet apecli; do
    cp "$PROJECT_ROOT/$bin" "$MACOS_DIR/"
done

cp "$PROJECT_ROOT/tools/neoviolet-gui/LICENSE" "$RESOURCES_DIR/LICENSE-GPL"
cp "$PROJECT_ROOT/tools/apecli/LICENSE"       "$RESOURCES_DIR/LICENSE-MIT"
cp "$PROJECT_ROOT/docs/ACKNOWLEDGEMENTS.md"       "$RESOURCES_DIR/ACKNOWLEDGEMENTS.md"

# ── Icon: compile (Xcode ≥ 26) else pre-compiled ──────────────────────────
XCODE_MAJOR=0
if xcrun xcodebuild -version &>/dev/null; then
    XCODE_MAJOR=$(xcrun xcodebuild -version | head -1 | awk '{print $2}' | cut -d. -f1)
fi

if [[ "$XCODE_MAJOR" -ge 26 ]] && xcrun actool "$ICON_SRC" \
    --compile "$RESOURCES_DIR" \
    --platform macosx \
    --target-device mac \
    --minimum-deployment-target 26.0 \
    --app-icon "$ICON_NAME" \
    --output-partial-info-plist "/dev/null" \
    --include-all-app-icons \
    --enable-on-demand-resources NO \
    --development-region en &>/dev/null; then
    echo "Icon: compiled (Xcode $XCODE_MAJOR)"
    [[ -f "$RESOURCES_DIR/$ICON_NAME.icns" ]] || cp "$PRE_COMPILED/NeoViolet.icns" "$RESOURCES_DIR/"
else
    echo "Icon: pre-compiled"
    cp "$PRE_COMPILED/Assets.car"    "$RESOURCES_DIR/"
    cp "$PRE_COMPILED/NeoViolet.icns" "$RESOURCES_DIR/"
fi

# ── Info.plist ─────────────────────────────────────────────────────────────
cat > "$CONTENTS_DIR/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleExecutable</key>
  <string>neoviolet-gui</string>
  <key>CFBundleGetInfoString</key>
  <string>$APP_NAME $VERSION</string>
  <key>CFBundleIconFile</key>
  <string>NeoViolet.icns</string>
  <key>CFBundleIconName</key>
  <string>$ICON_NAME</string>
  <key>CFBundleIdentifier</key>
  <string>$BUNDLE_ID</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>$APP_NAME</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$VERSION</string>
  <key>CFBundleVersion</key>
  <string>$BUILD</string>
  <key>LSMinimumSystemVersion</key>
  <string>12.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
  <key>CFBundleDocumentTypes</key>
  <array>
    <dict>
      <key>CFBundleTypeName</key>
      <string>Audio File</string>
      <key>CFBundleTypeRole</key>
      <string>Viewer</string>
      <key>LSHandlerRank</key>
      <string>Alternate</string>
      <key>LSItemContentTypes</key>
      <array>
        <string>public.mp3</string>
        <string>org.xiph.flac</string>
        <string>com.microsoft.waveform-audio</string>
        <string>org.xiph.ogg-audio</string>
        <string>org.xiph.opus-audio</string>
        <string>public.mpeg-4-audio</string>
        <string>public.midi-audio</string>
        <string>com.apple.m4a-audio</string>
        <string>public.audio</string>
      </array>
    </dict>
    <dict>
      <key>CFBundleTypeName</key>
      <string>MPEG Audio Layer II</string>
      <key>CFBundleTypeRole</key>
      <string>Viewer</string>
      <key>LSHandlerRank</key>
      <string>Alternate</string>
      <key>CFBundleTypeExtensions</key>
      <array>
        <string>mp2</string>
      </array>
    </dict>
    <dict>
      <key>CFBundleTypeName</key>
      <string>Monkey's Audio</string>
      <key>CFBundleTypeRole</key>
      <string>Viewer</string>
      <key>LSHandlerRank</key>
      <string>Alternate</string>
      <key>CFBundleTypeExtensions</key>
      <array>
        <string>ape</string>
      </array>
    </dict>
    <dict>
      <key>CFBundleTypeName</key>
      <string>Tracker Module</string>
      <key>CFBundleTypeRole</key>
      <string>Viewer</string>
      <key>LSHandlerRank</key>
      <string>Alternate</string>
      <key>CFBundleTypeExtensions</key>
      <array>
        <string>mod</string>
        <string>xm</string>
        <string>it</string>
        <string>s3m</string>
      </array>
    </dict>
  </array>
</dict>
</plist>
EOF

# ── PkgInfo ────────────────────────────────────────────────────────────────
printf 'APPL????' > "$CONTENTS_DIR/PkgInfo"

# ── Code-sign ──────────────────────────────────────────────────────────────
if command -v codesign >/dev/null 2>&1; then
  codesign --force --deep --sign "$SIGN_IDENTITY" "$APP_DIR" >/dev/null

  if codesign -d --entitlements :- "$APP_DIR" 2>/dev/null | grep -q "com.apple.security.app-sandbox"; then
    echo "error: app bundle is sandboxed; remove app sandbox entitlements before packaging" >&2
    exit 1
  fi
fi

echo "$APP_DIR"
