#!/usr/bin/env bash
# Provisions the Stage 20D2B single-administrator password
# (docs/remote-management.md §9.2). This does NOT run the real
# streaming-tree-server service - it runs the same binary's own
# `--provision-admin-password` mode once, through `systemd-run`,
# reproducing the shipped unit's own LoadCredential=/DynamicUser=yes/
# StateDirectory=/Environment=STREAMING_TREE_DATA_DIR properties
# exactly, so the provisioned verifier lands in the *same* encrypted
# secrets.json the real service reads, under the *same* identity - not
# a second, parallel state path.
#
# This requires the Stage 20D2A headless master key to already be
# provisioned (scripts/provision-headless-master-key.sh) at the path
# the unit's own LoadCredential= directive names.
#
# The new password is read interactively (hidden input, with
# confirmation) - never accepted on the command line, never read from
# an environment variable.
#
# Usage:
#   sudo scripts/provision-admin-password.sh
#   sudo scripts/provision-admin-password.sh --force
#
# --force overwrites an existing administrator password. Every active
# remote-management session is invalidated by this (the running
# service, if any, must be restarted to pick up a session store that
# has already been cleared here - a fresh SessionStore only exists
# from process start, so a reset performed while the service is
# stopped already achieves this; docs/remote-management.md §10/§33).

set -euo pipefail

log() { printf '==> %s\n' "$1"; }
fail() { printf 'FAILED: %s\n' "$1" >&2; exit 1; }

FORCE=0
BINARY="/usr/bin/streaming-tree-server"
MASTER_KEY_PATH="/etc/streaming-tree/master.key"

while [ $# -gt 0 ]; do
  case "$1" in
    --force)
      FORCE=1; shift ;;
    --binary)
      BINARY="$2"; shift 2 ;;
    --master-key-file)
      MASTER_KEY_PATH="$2"; shift 2 ;;
    -*)
      fail "unknown argument: $1" ;;
    *)
      fail "unexpected positional argument: $1" ;;
  esac
done

[ -e "$BINARY" ] || fail "$BINARY does not exist - pass --binary to override"
[ -e "$MASTER_KEY_PATH" ] || fail "$MASTER_KEY_PATH does not exist - provision it first with scripts/provision-headless-master-key.sh"

ARGS=("$BINARY" "--provision-admin-password")
if [ "$FORCE" -eq 1 ]; then
  ARGS+=("--force")
fi

log "Provisioning the administrator password under the real service identity (systemd-run)..."

# The exact same properties scripts/systemd/streaming-tree.service
# itself declares - reused, not duplicated by value, so this
# provisioning run and the real service always agree on identity and
# state path. --pty keeps stdin/stdout attached for the interactive
# hidden-input prompt; --collect removes the transient unit once it
# exits, success or failure, so no leftover unit accumulates.
#
# The literal /var/lib/streaming-tree path is used instead of the
# %S specifier the real unit *file* uses: %S/%d-style specifiers are
# only expanded when systemd parses a property from a real unit
# file's own [Service] section, never when the identical text is set
# via `systemd-run --property=` on a transient unit - confirmed the
# hard way (docs/progress.md, PRE-20E.1) via a real `mkdir /%S:
# read-only file system` failure, the literal, unexpanded specifier
# string passed straight through as a path. StateDirectory=
# streaming-tree deterministically resolves under /var/lib for a
# system unit (systemd.exec(5)) - not something that varies per host
# or per run, so a literal path here is exactly as correct as the
# specifier would have been, had it expanded.
exec systemd-run \
  --pty \
  --collect \
  --wait \
  --property="LoadCredential=streaming-tree-master-key:$MASTER_KEY_PATH" \
  --property="DynamicUser=yes" \
  --property="StateDirectory=streaming-tree" \
  --property="Environment=STREAMING_TREE_DATA_DIR=/var/lib/streaming-tree" \
  -- "${ARGS[@]}"
