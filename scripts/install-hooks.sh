#!/bin/sh
# Install only in this clone; preserve existing custom hook configuration.
set -eu
root=$(git rev-parse --show-toplevel)
cd "$root"
configured=$(git config --get core.hooksPath || true)
if [ -n "$configured" ] && [ "$configured" != ".githooks" ]; then
    echo "Existing core.hooksPath found; review integration before installing." >&2
    exit 1
fi
if [ -z "$configured" ]; then
    hooks_dir=$(git rev-parse --git-path hooks)
    for hook in "$hooks_dir"/*; do
        case "$hook" in *.sample) continue ;; esac
        if [ -f "$hook" ] && [ -x "$hook" ]; then
            echo "Existing active Git hook found; review integration before installing." >&2
            exit 1
        fi
    done
fi
command -v python3 >/dev/null
python3 -c 'import sys; sys.exit(0 if sys.version_info >= (3, 9) else 1)'
for hook in pre-commit commit-msg pre-push; do
    test -f ".githooks/$hook"
    chmod +x ".githooks/$hook"
done
git config --local core.hooksPath .githooks
echo "Installed clone-local Git hooks: .githooks"
