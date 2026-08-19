#!/usr/bin/env bash
# Provisions the Stage 20D2A headless secret-store master key
# (docs/linux-headless-server.md §9) - a single random 32-byte (256-bit)
# file, written atomically with restrictive permissions, referenced from
# the shipped systemd unit's own LoadCredential= directive.
#
# This script does NOT run automatically on package install - the
# operator runs it explicitly, once, before first enabling the headless
# service (docs/linux-headless-server.md §15). It never prints the key
# material, never accepts key material on the command line, and never
# overwrites an existing key file without an explicit --force.
#
# Usage:
#   sudo scripts/provision-headless-master-key.sh /etc/streaming-tree/master.key
#   sudo scripts/provision-headless-master-key.sh --force /etc/streaming-tree/master.key

set -euo pipefail

log() { printf '==> %s\n' "$1"; }
fail() { printf 'FAILED: %s\n' "$1" >&2; exit 1; }

FORCE=0
TARGET=""

while [ $# -gt 0 ]; do
  case "$1" in
    --force)
      FORCE=1; shift ;;
    -*)
      fail "unknown argument: $1" ;;
    *)
      TARGET="$1"; shift ;;
  esac
done

[ -n "$TARGET" ] || fail "usage: $0 [--force] <path-to-key-file>"

if [ -e "$TARGET" ] && [ "$FORCE" -ne 1 ]; then
  fail "$TARGET already exists - pass --force to deliberately overwrite it (this permanently invalidates every secret already encrypted under the existing key, see docs/linux-headless-server.md §9)"
fi

DIR="$(dirname "$TARGET")"
mkdir -p "$DIR"
chmod 0700 "$DIR"

# Write to a temp file in the same directory first, then rename - a
# partially-written key file is never visible under the real path.
TMP="$(mktemp "$DIR/.master-key-XXXXXX")"
trap 'rm -f "$TMP"' EXIT

# 32 raw random bytes, straight from the kernel CSPRNG - no shell
# interpolation of key material at any point.
head -c 32 /dev/urandom > "$TMP"

ACTUAL_SIZE="$(wc -c < "$TMP" | tr -d '[:space:]')"
[ "$ACTUAL_SIZE" = "32" ] || fail "expected exactly 32 bytes, got $ACTUAL_SIZE - refusing to provision a short key"

chmod 0600 "$TMP"
mv -f "$TMP" "$TARGET"
trap - EXIT

log "Provisioned a new 32-byte master key at $TARGET (mode 0600)."
log "Next steps (docs/linux-headless-server.md §15):"
log "  1. Point the unit's LoadCredential= at this path if it differs from the default (systemctl edit streaming-tree.service)."
log "  2. sudo systemctl daemon-reload"
log "  3. sudo systemctl enable --now streaming-tree.service"
log ""
log "Back up this file together with the service's StateDirectory - losing either one alone makes existing encrypted provider secrets unrecoverable."
