#!/usr/bin/env bash
# Each driver owns a fresh, separate Docker network and its own fixtures.
set -euo pipefail
cd "$(dirname "$0")/.."
logs=$(mktemp -d)
trap 'rm -f "$logs/ordinary" "$logs/email-lifecycle" "$logs/email-archive-recovery"; rmdir "$logs"' EXIT

python3 -B scripts/test-session-protocol.py "$@" --suite ordinary >"$logs/ordinary" 2>&1 &
ordinary=$!
python3 -B scripts/test-session-protocol.py "$@" --suite email-lifecycle >"$logs/email-lifecycle" 2>&1 &
email=$!

result=0
report() {
    local status=0
    wait "$2" || status=$?
    printf '\n=== %s (exit %s) ===\n' "$1" "$status"
    cat "$logs/$1"
    if ((status != 0)); then
        result=1
    fi
}
report email-lifecycle "$email"
python3 -B scripts/test-session-protocol.py "$@" --suite email-archive-recovery >"$logs/email-archive-recovery" 2>&1 &
archive=$!
report ordinary "$ordinary"
report email-archive-recovery "$archive"
exit "$result"
