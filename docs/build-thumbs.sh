#!/usr/bin/env bash
#
# build-thumbs.sh — regenerate the gallery thumbnails in docs/thumbs/.
#
# Each example is rendered headless (app.Run's GOPHICS_THUMB hook, see
# app/thumb.go): no browser, no GPU, no display. The CPU rasterizer is
# pixel-identical to every backend, so a thumbnail matches what ships. The one
# networked example (hn) renders in realtime so its fetch can land.
#
#   ./docs/build-thumbs.sh              # all
#   ./docs/build-thumbs.sh solitaire hn # a subset
#
set -euo pipefail
cd "$(dirname "$0")/.."

OUT=docs/thumbs
mkdir -p "$OUT"
export GOPHICS_THUMB_SIZE=760x565   # logical render size (gallery card aspect)
export GOPHICS_THUMB_OUT=760x565    # downscaled from SCALE× for crisp AA
export GOPHICS_THUMB_SCALE=2

# name  settle_seconds  realtime(1|"")
#   settle = max animation seconds to step before capture (one-shot intros like
#   the solitaire deal stop early; continuous scenes run the full budget).
specs=(
  "canvas     2 "
  "gallery    2 "
  "todo       2 "
  "notes      2 "
  "match3     6 "
  "roguelike  4 "
  "flowfield  6 "
  "flocking   6 "
  "particles  6 "
  "solitaire  8 "
  "hn        10 1"   # networked — realtime so the fetch completes
)

want=("$@")
for spec in "${specs[@]}"; do
  read -r name settle realtime <<<"$spec"
  if [ ${#want[@]} -gt 0 ] && ! printf '%s\n' "${want[@]}" | grep -qx "$name"; then
    continue
  fi
  echo "== $name =="
  env GOPHICS_THUMB="$OUT/$name.png" \
      GOPHICS_THUMB_SETTLE="$settle" \
      ${realtime:+GOPHICS_THUMB_REALTIME=1} \
      go run "./examples/$name"
done

echo "thumbnails written to $OUT/"
