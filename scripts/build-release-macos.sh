#!/usr/bin/env bash
# Builds a local Stage 20C1 macOS release: the production frontend
# embedded into a CGO-enabled Go executable, staged with the four
# mandatory legal documents, assembled into a real .app bundle, and
# packaged into a DMG with a SHA-256 digest. When VERSION is a strict
# major.minor.patch release version, also generates a Stage 20B/20C1
# release manifest fragment (docs/macos-packaging.md §22) from the real
# DMG - use -in on a subsequent invocation (or the Windows build's own
# manifest) to fold it into one canonical multi-platform manifest.
#
# See docs/macos-packaging.md for the full architecture this script
# implements. This produces LOCAL build artifacts only:
#
#   - it does not publish anything to GitHub;
#   - it does not create a Git tag;
#   - it does not create a GitHub Release;
#   - it does not sign or notarize anything (Stage 20C1 packages are
#     UNSIGNED and NOT NOTARIZED - see docs/macos-packaging.md §25/§32,
#     Stage 20C2's own scope).
#
# Runs only on a real Darwin host - this is not a cross-compilation
# script, the same way build-release.ps1 is not.
#
# Usage:
#   scripts/build-release-macos.sh --version 0.1.0-dev+local
#   scripts/build-release-macos.sh --version 0.2.0 --in /path/to/existing-manifest.json
#   scripts/build-release-macos.sh --version 0.1.0-dev+ci --skip-dmg
#
# Flags:
#   --version VERSION   application version to inject (required)
#   --in PATH           existing manifest JSON to add this artifact to
#                        (optional - see cmd/releasemanifest's own -in)
#   --skip-dmg           build and stage the .app bundle but do not
#                        invoke hdiutil - useful when only the .app
#                        itself needs verifying

set -euo pipefail

log() { printf '==> %s\n' "$1"; }
fail() { printf 'FAILED: %s\n' "$1" >&2; exit 1; }

# --- 0. Parse arguments -----------------------------------------------------
VERSION=""
IN_MANIFEST=""
SKIP_DMG=0

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="${2:-}"; shift 2 ;;
    --in)
      IN_MANIFEST="${2:-}"; shift 2 ;;
    --skip-dmg)
      SKIP_DMG=1; shift ;;
    *)
      fail "unknown argument: $1" ;;
  esac
done

[ -n "$VERSION" ] || fail "--version is required"

# --- 1. Verify a real Darwin host -------------------------------------------
log "Verifying host platform"
HOST_OS="$(uname -s)"
[ "$HOST_OS" = "Darwin" ] || fail "this script only runs on macOS (uname -s reported '$HOST_OS')"

# --- 2. Verify a supported architecture -------------------------------------
log "Verifying host architecture"
HOST_ARCH_RAW="$(uname -m)"
case "$HOST_ARCH_RAW" in
  arm64) GOARCH_VALUE="arm64" ;;
  x86_64) GOARCH_VALUE="amd64" ;;
  *) fail "unsupported macOS architecture '$HOST_ARCH_RAW' (only arm64 and x86_64/amd64 are packaged - docs/macos-packaging.md §4)" ;;
esac
log "Building for darwin/$GOARCH_VALUE"

# --- 3. Validate the requested version ---------------------------------------
log "Validating requested version '$VERSION'"
case "$VERSION" in
  [A-Za-z0-9]*) : ;;
  *) fail "version '$VERSION' must start with a letter or digit" ;;
esac
if printf '%s' "$VERSION" | LC_ALL=C grep -qv '^[A-Za-z0-9][A-Za-z0-9.+-]*$'; then
  fail "version '$VERSION' contains characters unsafe for a bundle version field. Allowed: letters, digits, '.', '-', '+'."
fi
if [ "$VERSION" = "1.0.0" ]; then
  fail "refusing to build version 1.0.0: Stage 20C1 is unsigned packaging infrastructure, not the public 1.0 release. Use an explicit development/test version such as '0.1.0-dev'."
fi

# --- 4. Resolve fixed, repository-relative paths ----------------------------
# Never a caller-supplied arbitrary path: every path below is derived from
# this script's own known location, so nothing outside the repository is
# ever touched or deleted.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WEB_DIR="$REPO_ROOT/apps/web"
SERVER_DIR="$REPO_ROOT/apps/server"
EMBEDDED_FRONTEND_DIR="$SERVER_DIR/internal/webassets/embedded"
EMBEDDED_LEGAL_DIR="$SERVER_DIR/internal/webassets/legal"
STAGING_DIR="$REPO_ROOT/build/release-macos/staging"
OUTPUT_DIR="$REPO_ROOT/build/release-macos/output"

APP_NAME="Streaming Tree for OBS"
APP_BUNDLE_DIR="$OUTPUT_DIR/$APP_NAME.app"
EXE_NAME="streaming-tree-server"
BUNDLE_ID="io.github.czekosabe.streaming-tree-for-obs"

# --- 5. Verify required build tools ------------------------------------------
log "Verifying required build tools"
command -v go >/dev/null 2>&1 || fail "required tool 'go' was not found on PATH"
command -v npm >/dev/null 2>&1 || fail "required tool 'npm' was not found on PATH"
command -v plutil >/dev/null 2>&1 || fail "required tool 'plutil' was not found on PATH"
if [ "$SKIP_DMG" -eq 0 ]; then
  command -v hdiutil >/dev/null 2>&1 || fail "required tool 'hdiutil' was not found on PATH"
fi
log "  go:      $(command -v go)"
log "  npm:     $(command -v npm)"
log "  plutil:  $(command -v plutil)"

# --- 6. Ensure frontend dependencies match the lockfile ----------------------
log "Installing frontend dependencies (npm ci)"
(cd "$WEB_DIR" && npm ci)

# --- 7. Run the frontend production build ------------------------------------
log "Building the production frontend (npm run build)"
(cd "$WEB_DIR" && npm run build)

DIST_DIR="$WEB_DIR/dist"
[ -f "$DIST_DIR/index.html" ] || fail "frontend build did not produce $DIST_DIR/index.html"

# --- 8. Stage the embedded frontend/legal directories ------------------------
# Bounded, safe cleanup: only ever removes files *inside* these two fixed,
# known subdirectories - never a caller-supplied path, never anything above
# the repository. The tracked .gitkeep placeholder is intentionally
# overwritten along with everything else; it returns on the next `git
# checkout`/clean clone, which is exactly the point of it being a
# placeholder rather than real content.
log "Staging the embedded production frontend"
find "$EMBEDDED_FRONTEND_DIR" -mindepth 1 -delete
cp -R "$DIST_DIR/." "$EMBEDDED_FRONTEND_DIR/"

log "Staging the embedded legal documents"
find "$EMBEDDED_LEGAL_DIR" -mindepth 1 -delete
for doc in LICENSE THIRD_PARTY_NOTICES.md LEGAL.md PRIVACY.md; do
  [ -f "$REPO_ROOT/$doc" ] || fail "required legal document '$doc' is missing from the repository root"
  cp "$REPO_ROOT/$doc" "$EMBEDDED_LEGAL_DIR/"
done

# --- 9. Build the macOS release executable (CGO enabled) ---------------------
log "Building the macOS release executable (version $VERSION)"

COMMIT_HASH="$(git -C "$REPO_ROOT" rev-parse HEAD)"
[ -n "$COMMIT_HASH" ] || fail "could not resolve the current Git commit"

rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR"

EXE_PATH="$STAGING_DIR/$EXE_NAME"
LDFLAGS_PKG="github.com/streaming-tree/server/internal/buildinfo"
LDFLAGS="-X $LDFLAGS_PKG.releaseVersion=$VERSION -X $LDFLAGS_PKG.releaseCommit=$COMMIT_HASH -X $LDFLAGS_PKG.packagedFlag=true"

(
  cd "$SERVER_DIR"
  GOOS=darwin GOARCH="$GOARCH_VALUE" CGO_ENABLED=1 go build -ldflags "$LDFLAGS" -o "$EXE_PATH" ./cmd/server
)
[ -f "$EXE_PATH" ] || fail "expected release executable was not produced at $EXE_PATH"

# --- 10. Assemble the .app bundle --------------------------------------------
log "Assembling the .app bundle"
rm -rf "$OUTPUT_DIR"
mkdir -p "$APP_BUNDLE_DIR/Contents/MacOS" "$APP_BUNDLE_DIR/Contents/Resources"

cp "$EXE_PATH" "$APP_BUNDLE_DIR/Contents/MacOS/$EXE_NAME"
chmod +x "$APP_BUNDLE_DIR/Contents/MacOS/$EXE_NAME"

for doc in LICENSE THIRD_PARTY_NOTICES.md LEGAL.md PRIVACY.md; do
  cp "$REPO_ROOT/$doc" "$APP_BUNDLE_DIR/Contents/Resources/$doc"
done

INFO_PLIST="$APP_BUNDLE_DIR/Contents/Info.plist"
cat > "$INFO_PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>$BUNDLE_ID</string>
	<key>CFBundleExecutable</key>
	<string>$EXE_NAME</string>
	<key>CFBundleName</key>
	<string>Streaming Tree</string>
	<key>CFBundleDisplayName</key>
	<string>$APP_NAME</string>
	<key>CFBundleShortVersionString</key>
	<string>$VERSION</string>
	<key>CFBundleVersion</key>
	<string>$VERSION</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>LSMinimumSystemVersion</key>
	<string>12.0</string>
	<key>LSUIElement</key>
	<true/>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
PLIST

# Self-validate the plist before trusting it further (docs/macos-packaging.md
# §7/§13/§38) - a malformed Info.plist must fail this script loudly, not
# silently produce a bundle macOS itself would refuse to launch.
plutil -lint "$INFO_PLIST" >/dev/null || fail "generated Info.plist failed plutil -lint"

log "Staged .app bundle at: $APP_BUNDLE_DIR"

# --- 11. Build the DMG --------------------------------------------------------
if [ "$SKIP_DMG" -eq 1 ]; then
  log "--skip-dmg set - not invoking hdiutil"
  echo ""
  echo "macOS app bundle build complete (UNSIGNED, NOT NOTARIZED - see docs/macos-packaging.md §25/§32)."
  echo "  Executable: $EXE_PATH"
  echo "  App bundle: $APP_BUNDLE_DIR"
  exit 0
fi

log "Building the DMG (hdiutil)"
DMG_STAGING_DIR="$(mktemp -d)"
trap 'rm -rf "$DMG_STAGING_DIR"' EXIT

cp -R "$APP_BUNDLE_DIR" "$DMG_STAGING_DIR/"
ln -s /Applications "$DMG_STAGING_DIR/Applications"

DMG_NAME="StreamingTreeForOBS-$VERSION-darwin-$GOARCH_VALUE.dmg"
DMG_PATH="$OUTPUT_DIR/$DMG_NAME"

hdiutil create \
  -volname "$APP_NAME" \
  -srcfolder "$DMG_STAGING_DIR" \
  -fs HFS+ \
  -format UDZO \
  -ov \
  "$DMG_PATH" >/dev/null

[ -f "$DMG_PATH" ] || fail "hdiutil reported success but no DMG was found at $DMG_PATH"

# --- 12. Generate a SHA-256 digest -------------------------------------------
log "Generating the SHA-256 digest"
DMG_SHA256="$(shasum -a 256 "$DMG_PATH" | awk '{print $1}')"
printf '%s  %s' "$DMG_SHA256" "$DMG_NAME" > "$DMG_PATH.sha256"

# --- 13. Generate the release manifest fragment ------------------------------
# Only attempted for a strict "major.minor.patch" version - see
# docs/updater.md §4/§5's own strict version format, which a "-dev+..."
# local/test build (this script's own everyday use, per its own doc comment
# above) never matches. Mirrors build-release.ps1 §10 exactly, with -in
# added so this can fold into an existing Windows-built manifest
# (docs/macos-packaging.md §22).
MANIFEST_PATH="$OUTPUT_DIR/streaming-tree-release.json"
if printf '%s' "$VERSION" | LC_ALL=C grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  log "Generating the release manifest fragment"
  MANIFEST_ARGS=(
    -version "$VERSION"
    -artifact "$DMG_PATH"
    -artifact-name "$DMG_NAME"
    -os darwin -arch "$GOARCH_VALUE" -kind dmg
    -out "$MANIFEST_PATH"
  )
  if [ -n "$IN_MANIFEST" ]; then
    MANIFEST_ARGS+=(-in "$IN_MANIFEST")
  fi
  (cd "$SERVER_DIR" && go run ./cmd/releasemanifest "${MANIFEST_ARGS[@]}")
else
  log "Skipping release manifest: '$VERSION' is not a strict major.minor.patch release version"
  MANIFEST_PATH=""
fi

echo ""
echo "macOS release build complete (UNSIGNED, NOT NOTARIZED - see docs/macos-packaging.md §25/§32)."
echo "  Executable: $EXE_PATH"
echo "  App bundle: $APP_BUNDLE_DIR"
echo "  DMG:        $DMG_PATH"
echo "  SHA-256:    $DMG_PATH.sha256"
if [ -n "$MANIFEST_PATH" ]; then
  echo "  Manifest:   $MANIFEST_PATH"
fi
