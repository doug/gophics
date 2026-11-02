# Fonts served to web demos

Fonts the docs site serves to its own WebAssembly demos, fetched at runtime by
`text.FetchFontSource` through `app.Config.WebFonts`.

They are here because a browser will not hand a WebAssembly page the bytes of an
installed font, so `Config.SystemFonts` — which covers the desktop builds — has
no implementation on the web at all. Fetching is the route that works in every
browser, and `FetchFontSource` deliberately takes URLs from the app rather than
shipping a built-in CDN, so these come from this site's own origin.

## NotoColorEmoji-gallery-subset.ttf — 27KB

A subset of Noto Color Emoji holding the eight emoji the gallery's colour-emoji
section shows (🚀 🎉 🌍 🐙 🍜 🎸 🧭 🐢) plus `.notdef`: 9 glyphs out of 4,046,
27KB out of 10.7MB.

Built from the upstream **CBDT/CBLC bitmap** build:

```
# source: googlefonts/noto-emoji, tag v2026-09-24-unicode18_0
#   2D/fonts/NotoColorEmoji.ttf
#   sha256 15671215ab769fdc7162a045d56fd7d7e477c51b04e6b3c761d914d8fdd6cc44
#   10,730,124 bytes
uv run --with fonttools python -m fontTools.subset NotoColorEmoji.ttf \
  --unicodes=U+1F680,U+1F389,U+1F30D,U+1F419,U+1F35C,U+1F3B8,U+1F9ED,U+1F422 \
  --output-file=NotoColorEmoji-gallery-subset.ttf \
  --no-hinting --drop-tables+=GSUB
```

### It must stay a bitmap build

**Do not replace this with the Noto Color Emoji from fonts.google.com.** That
file has the same name and only `COLR`/`CPAL` and OT-SVG tables — no
`CBDT`/`CBLC`. gophics rasterizes colour *bitmap* glyphs (sbix and CBDT, PNG
strikes) and does not implement COLR paint graphs, so the COLRv1 face would
fetch successfully, report no error, and draw nothing. The same gap is why the
section is blank on Windows, whose Segoe UI Emoji is COLR.

The upstream bitmap build is only in the GitHub repo, under `2D/fonts/`
(it was `fonts/` before the repo was restructured) — not on Google Fonts.

`TestGalleryEmojiSubsetIsAColourBitmapFace` in `examples/gallery/ui` loads this
file and asserts a colour glyph comes back, so a re-subset that drops the bitmap
tables fails the build rather than silently blanking the demo.

### Licence

SIL Open Font License 1.1, in `LICENSE-NotoColorEmoji.txt` as OFL §2 requires.
Copyright 2013 Google LLC. The copyright statement names no Reserved Font Name,
so the subset keeps the original name.
