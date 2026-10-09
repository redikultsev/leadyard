#!/bin/sh
# Managed by leadyard init. Prints the current task state into the session.
LY=$(command -v leadyard 2>/dev/null)
for c in "$LEADYARD_BIN" "$HOME/go/bin/leadyard" /opt/homebrew/bin/leadyard /usr/local/bin/leadyard; do
  [ -n "$LY" ] && break
  [ -n "$c" ] && [ -x "$c" ] && LY=$c
done
[ -n "$LY" ] || { echo "leadyard is not installed; task state is not loaded."; exit 0; }
"$LY" resume --quiet || true
