#!/usr/bin/env bash
# Builds a local Stage 20D1 Linux release: the production frontend
# embedded into a statically-linked (CGO_ENABLED=0) Go executable,
# staged with the four mandatory legal documents, assembled into a
# .deb package with a SHA-256 digest. When VERSION is a strict
# major.minor.patch release version, also generates a release-manifest
# fragment (docs/linux-desktop-packaging.md §21) from the real .deb -
# use --in on a subsequent invocation (or the Windows/macOS build's own
# manifest) to fold it into one canonical multi-platform manifest.
#
# See docs/linux-desktop-packaging.md for the full architecture this
# script implements. This produces LOCAL build artifacts only:
#
#   - it does not publish anything to GitHub;
#   - it does not create a Git tag;
#   - it does not install anything via apt/dnf/pacman;
#   - it does not sign anything (no Linux release signing is
#     implemented at any stage yet).
#
# Runs only on a real Linux host - this is not a cross-compilation
# script, the same way build-release-macos.sh is not.
#
# Usage:
#   scripts/build-release-linux.sh --version 0.1.0-dev+local
#   scripts/build-release-linux.sh --version 0.2.0 --in /path/to/existing-manifest.json
#
# Flags:
#   --version VERSION   application version to inject (required)
#   --in PATH           existing manifest JSON to add this artifact to
#                        (optional - see cmd/releasemanifest's own -in)

set -euo pipefail

log() { printf '==> %s\n' "$1"; }
fail() { printf 'FAILED: %s\n' "$1" >&2; exit 1; }

# --- 0. Parse arguments -----------------------------------------------------
VERSION=""
IN_MANIFEST=""

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="${2:-}"; shift 2 ;;
    --in)
      IN_MANIFEST="${2:-}"; shift 2 ;;
    *)
      fail "unknown argument: $1" ;;
  esac
done

[ -n "$VERSION" ] || fail "--version is required"

# --- 1. Verify a real Linux host ---------------------------------------------
log "Verifying host platform"
HOST_OS="$(uname -s)"
[ "$HOST_OS" = "Linux" ] || fail "this script only runs on Linux (uname -s reported '$HOST_OS')"

# --- 2. Verify a supported architecture --------------------------------------
log "Verifying host architecture"
HOST_ARCH_RAW="$(uname -m)"
case "$HOST_ARCH_RAW" in
  x86_64) GOARCH_VALUE="amd64"; DEB_ARCH="amd64" ;;
  aarch64) GOARCH_VALUE="arm64"; DEB_ARCH="arm64" ;;
  *) fail "unsupported Linux architecture '$HOST_ARCH_RAW' (only x86_64 and aarch64 are packaged - docs/linux-desktop-packaging.md §21)" ;;
esac
log "Building for linux/$GOARCH_VALUE"

# --- 3. Validate the requested version ---------------------------------------
log "Validating requested version '$VERSION'"
case "$VERSION" in
  [A-Za-z0-9]*) : ;;
  *) fail "version '$VERSION' must start with a letter or digit" ;;
esac
if printf '%s' "$VERSION" | LC_ALL=C grep -qv '^[A-Za-z0-9][A-Za-z0-9.+-]*$'; then
  fail "version '$VERSION' contains characters unsafe for a package version field. Allowed: letters, digits, '.', '-', '+'."
fi
if [ "$VERSION" = "1.0.0" ]; then
  fail "refusing to build version 1.0.0: Stage 20D1 is packaging infrastructure, not the public 1.0 release. Use an explicit development/test version such as '0.1.0-dev'."
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
STAGING_DIR="$REPO_ROOT/build/release-linux/staging"
OUTPUT_DIR="$REPO_ROOT/build/release-linux/output"

PACKAGE_NAME="streaming-tree-for-obs"
EXE_NAME="streaming-tree-server"

# --- 5. Verify required build tools ------------------------------------------
log "Verifying required build tools"
command -v go >/dev/null 2>&1 || fail "required tool 'go' was not found on PATH"
command -v npm >/dev/null 2>&1 || fail "required tool 'npm' was not found on PATH"
command -v dpkg-deb >/dev/null 2>&1 || fail "required tool 'dpkg-deb' was not found on PATH"
log "  go:       $(command -v go)"
log "  npm:      $(command -v npm)"
log "  dpkg-deb: $(command -v dpkg-deb)"

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

# --- 9. Build the Linux release executable (CGO disabled) -------------------
log "Building the Linux release executable (version $VERSION)"

COMMIT_HASH="$(git -C "$REPO_ROOT" rev-parse HEAD)"
[ -n "$COMMIT_HASH" ] || fail "could not resolve the current Git commit"

rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR"

EXE_PATH="$STAGING_DIR/$EXE_NAME"
LDFLAGS_PKG="github.com/streaming-tree/server/internal/buildinfo"
LDFLAGS="-X $LDFLAGS_PKG.releaseVersion=$VERSION -X $LDFLAGS_PKG.releaseCommit=$COMMIT_HASH -X $LDFLAGS_PKG.packagedFlag=true"

(
  cd "$SERVER_DIR"
  GOOS=linux GOARCH="$GOARCH_VALUE" CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o "$EXE_PATH" ./cmd/server
)
[ -f "$EXE_PATH" ] || fail "expected release executable was not produced at $EXE_PATH"

# --- 10. Assemble the .deb staging layout ------------------------------------
log "Assembling the .deb package layout"
rm -rf "$OUTPUT_DIR"
DEB_ROOT="$OUTPUT_DIR/deb-root"
mkdir -p "$DEB_ROOT/DEBIAN" \
         "$DEB_ROOT/usr/bin" \
         "$DEB_ROOT/usr/share/applications" \
         "$DEB_ROOT/usr/share/doc/$PACKAGE_NAME"

cp "$EXE_PATH" "$DEB_ROOT/usr/bin/$EXE_NAME"
chmod 0755 "$DEB_ROOT/usr/bin/$EXE_NAME"

cp "$REPO_ROOT/LICENSE" "$DEB_ROOT/usr/share/doc/$PACKAGE_NAME/copyright"
for doc in LEGAL.md PRIVACY.md THIRD_PARTY_NOTICES.md; do
  cp "$REPO_ROOT/$doc" "$DEB_ROOT/usr/share/doc/$PACKAGE_NAME/$doc"
done
chmod 0644 "$DEB_ROOT/usr/share/doc/$PACKAGE_NAME/"*

DESKTOP_FILE="$DEB_ROOT/usr/share/applications/$PACKAGE_NAME.desktop"
cat > "$DESKTOP_FILE" <<DESKTOP
[Desktop Entry]
Type=Application
Name=Streaming Tree for OBS
Exec=/usr/bin/$EXE_NAME
Terminal=false
Categories=AudioVideo;
DESKTOP
chmod 0644 "$DESKTOP_FILE"

if command -v desktop-file-validate >/dev/null 2>&1; then
  desktop-file-validate "$DESKTOP_FILE" || fail "generated .desktop file failed desktop-file-validate"
else
  log "desktop-file-validate not found on PATH - skipping (not installed on every host, non-fatal)"
fi

# Debian package versions must not contain characters outside the accepted
# set (roughly alphanumerics, '.', '+', '-', '~', ':') - the same character
# class this script's own --version validation above already enforces is a
# strict subset of what dpkg-deb accepts, so no further translation is
# needed here.
cat > "$DEB_ROOT/DEBIAN/control" <<CONTROL
Package: $PACKAGE_NAME
Version: $VERSION
Section: video
Priority: optional
Architecture: $DEB_ARCH
Maintainer: Czekosabe <Czekosabe@users.noreply.github.com>
Homepage: https://github.com/Czekosabe/streaming-tree-for-obs
Description: Local-first multistreaming control plane for OBS
 Streaming Tree for OBS runs alongside OBS Studio and MediaMTX on the
 same machine, giving one local web UI to manage multistreaming to
 Twitch, YouTube, Kick, and TikTok. Loopback-only; no data leaves this
 machine except direct connections to the platforms you configure.
CONTROL
chmod 0644 "$DEB_ROOT/DEBIAN/control"

log "Staged .deb layout at: $DEB_ROOT"

# --- 11. Build the .deb -------------------------------------------------------
log "Building the .deb package (dpkg-deb)"
DEB_NAME="StreamingTreeForOBS-$VERSION-linux-$GOARCH_VALUE.deb"
DEB_PATH="$OUTPUT_DIR/$DEB_NAME"

dpkg-deb --build --root-owner-group "$DEB_ROOT" "$DEB_PATH" >/dev/null

[ -f "$DEB_PATH" ] || fail "dpkg-deb reported success but no .deb was found at $DEB_PATH"

# --- 12. Generate a SHA-256 digest -------------------------------------------
log "Generating the SHA-256 digest"
DEB_SHA256="$(sha256sum "$DEB_PATH" | awk '{print $1}')"
printf '%s  %s' "$DEB_SHA256" "$DEB_NAME" > "$DEB_PATH.sha256"

# --- 13. Generate the release manifest fragment ------------------------------
# Only attempted for a strict "major.minor.patch" version - see
# docs/updater.md §4/§5's own strict version format, which a "-dev+..."
# local/test build (this script's own everyday use, per its own doc comment
# above) never matches. Mirrors build-release-macos.sh §13 exactly, with
# --in reused verbatim from the same already-shipped mechanism
# (docs/linux-desktop-packaging.md §21).
MANIFEST_PATH="$OUTPUT_DIR/streaming-tree-release.json"
if printf '%s' "$VERSION" | LC_ALL=C grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  log "Generating the release manifest fragment"
  MANIFEST_ARGS=(
    -version "$VERSION"
    -artifact "$DEB_PATH"
    -artifact-name "$DEB_NAME"
    -os linux -arch "$GOARCH_VALUE" -kind deb
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
echo "Linux release build complete."
echo "  Executable: $EXE_PATH"
echo "  Package:    $DEB_PATH"
echo "  SHA-256:    $DEB_PATH.sha256"
if [ -n "$MANIFEST_PATH" ]; then
  echo "  Manifest:   $MANIFEST_PATH"
fi
