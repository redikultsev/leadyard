#!/bin/sh
# Managed by leadyard init. Blocks the tool call (exit 2) when leadyard is missing.
# Agents started from a desktop app may not inherit the shell PATH, so look in the
# usual install places too.
LY=$(command -v leadyard 2>/dev/null)
for c in "$LEADYARD_BIN" "$HOME/go/bin/leadyard" /opt/homebrew/bin/leadyard /usr/local/bin/leadyard; do
  [ -n "$LY" ] && break
  [ -n "$c" ] && [ -x "$c" ] && LY=$c
done
if [ -z "$LY" ]; then
  echo "leadyard is not installed or not found; guarded actions are blocked. Install leadyard or set LEADYARD_BIN." >&2
  exit 2
fi
"$LY" gate
code=$?
[ "$code" -eq 0 ] || exit 2
