#!/bin/sh
# Records the demo GIFs into docs/demo/*.gif with VHS against the pretend gh (no network, no real account).
# Usage: docs/demo/record.sh [tape ...]     e.g. docs/demo/record.sh tapes/board.tape
set -e
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
(cd "$root" && mise exec -- go build -o "$work/gh-9s" .)
"$here/setup.sh" "$work/env" > "$work/env.sh"
export DEMO="$work"
cd "$here"
[ $# -gt 0 ] || set -- tapes/*.tape
for t in "$@"; do
  echo "recording $t"
  vhs "$t"
done
