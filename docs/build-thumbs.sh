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

# name  settle_seconds  realtime(1|"")  render_size(WxH|"")
#   settle = max animation seconds to step before capture (one-shot intros like
#   the solitaire deal stop early; continuous scenes run the full budget).
#   render_size overrides the logical size an example lays out at. It exists for
#   apps that switch layout on width: telemetry drops to its phone columns below
#   900pt, so rendering it at the card's own 760 would thumbnail the wrong UI.
#   Keep the aspect ratio equal to GOPHICS_THUMB_OUT or the downscale distorts.
specs=(
  "canvas     2 "
  "gallery    2 "
  "ledger     2 "
  "telemetry  2 '' 1500x1115"
  "mirror     5 "
  "todo       2 "
  "notes      2 "
  "epub       1 "
  "match3     6 "
  "roguelike  4 "
  "flowfield  6 "
  "flocking   6 "
  "particles  6 "
  "2048        2 "
  "sudoku      1 "
  "drummachine 1 "
  "luminaria   3 "
  "whiteboard  1 "
  "solitaire  8 "
  "hn        10 1"   # networked — realtime so the fetch completes
)

want=("$@")
for spec in "${specs[@]}"; do
  read -r name settle realtime size <<<"$spec"
  [ "$realtime" = "''" ] && realtime=""
  if [ ${#want[@]} -gt 0 ] && ! printf '%s\n' "${want[@]}" | grep -qx "$name"; then
    continue
  fi
  echo "== $name =="
  env GOPHICS_THUMB="$OUT/$name.png" \
      GOPHICS_THUMB_SETTLE="$settle" \
      ${size:+GOPHICS_THUMB_SIZE=$size} \
      ${realtime:+GOPHICS_THUMB_REALTIME=1} \
      go run "./examples/$name"
done

# health is a phone-portrait app; frame its dashboard as a device shot centered
# on a soft backdrop (needs ImageMagick `magick`). Skipped if magick is absent.
if { [ ${#want[@]} -eq 0 ] || printf '%s\n' "${want[@]}" | grep -qx health; }; then
  if command -v magick >/dev/null 2>&1; then
    echo "== health (framed portrait) =="
    tmp=$(mktemp -d)
    # GOPHICS_THUMB_OUT= disables the shared 760x565 downscale so the raw stays a
    # clean portrait (680x1200); magick frames it below.
    env HEALTH_VIEW=dashboard GOPHICS_THUMB="$tmp/raw.png" GOPHICS_THUMB_OUT= \
        GOPHICS_THUMB_SIZE=340x600 GOPHICS_THUMB_SCALE=2 GOPHICS_THUMB_SETTLE=20 \
        go run ./examples/health
    read -r w h < <(magick identify -format '%w %h\n' "$tmp/raw.png")
    magick -size "${w}x${h}" xc:black -fill white \
      -draw "roundrectangle 0,0,$((w - 1)),$((h - 1)),40,40" "$tmp/mask.png"
    magick "$tmp/raw.png" "$tmp/mask.png" -alpha off -compose CopyOpacity -composite "$tmp/round.png"
    magick "$tmp/round.png" -resize x500 \
      \( +clone -background black -shadow 60x18+0+12 \) +swap -background none -layers merge +repage "$tmp/shadow.png"
    magick -size 760x565 xc:'#f5f1ea' "$tmp/shadow.png" -gravity center -composite "$OUT/health.png"
    rm -rf "$tmp"
  else
    echo "== health: skipped (needs ImageMagick 'magick') =="
  fi
fi

# The home page hero: the counter example at the exact logical size the live
# frame is pinned to (see .stage in style.css). The two have to be the same
# render or the screenshot-to-live swap jumps — gophics lays out responsively,
# so a smaller render scaled up is a different picture, not a blurrier one.
if [ ${#want[@]} -eq 0 ] || printf '%s\n' "${want[@]}" | grep -qx counter; then
  echo "== counter (home hero) =="
  # One still: the example pins itself to the light theme precisely so that a
  # screenshot can stand in for it.
  env GOPHICS_THUMB=docs/hero-counter.png \
      GOPHICS_THUMB_SIZE=512x352 GOPHICS_THUMB_OUT=1024x704 \
      GOPHICS_THUMB_SCALE=2 GOPHICS_THUMB_SETTLE=1 \
      go run ./examples/counter
fi

echo "thumbnails written to $OUT/"
