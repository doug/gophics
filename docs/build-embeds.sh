#!/usr/bin/env bash
# Refresh the code listings embedded in the docs pages from their real source.
#
# A listing pasted into HTML drifts from the code it claims to show. This repo
# has done it twice: once shipping a StateBase method that does not exist, and
# again the day after the sample was "generated from source" — because it was
# generated once and pasted, not regenerated on build.
#
# Each embed is delimited by <!-- generated:<path> --> … <!-- /generated -->.
# This rewrites the span between them from the file the marker names.
#
#   ./docs/build-embeds.sh          # rewrite the embeds
#   ./docs/build-embeds.sh -check   # exit non-zero if any is stale (CI)
set -euo pipefail
cd "$(dirname "$0")/.."

check=0
[ "${1:-}" = "-check" ] && check=1

for page in docs/*.html; do
	grep -q "<!-- generated:" "$page" 2>/dev/null || continue
	python3 docs/tools/embed.py "$page" "$check"
done
