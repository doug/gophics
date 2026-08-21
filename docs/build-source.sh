#!/usr/bin/env bash
# Generate a static, syntax-highlighted source page for each featured demo,
# into docs/source/<name>.html. Highlighting is baked at build time by the
# stdlib-only tool in docs/tools/highlight — pure HTML + CSS, zero client JS.
# Re-run when an example changes.
#
#   ./docs/build-source.sh
set -euo pipefail
cd "$(dirname "$0")/.."

# Keep in sync with build-demos.sh and the cards in gallery.html — every demo
# with a source link must appear here or its "source" page 404s.
demos=(counter hn gallery canvas flowfield flocking particles match3 solitaire roguelike todo notes health ledger 2048 sudoku drummachine luminaria whiteboard epub capabilities)

nav='<nav class="top"><div class="wrap">
  <a class="brand" href="../index.html">gophics<span class="dot">.</span></a>
  <a class="link" href="../gallery.html">Gallery</a>
  <a class="link" href="../get-started.html">Get started</a>
  <span class="spacer"></span>
  <a class="gh" href="https://github.com/doug/gophics">GitHub ↗</a>
</div></nav>'

footer='<footer><div class="wrap">
  <span>Gophics — cross-platform native UI in pure Go.</span>
  <span class="spacer"></span>
  <a href="https://github.com/doug/gophics">GitHub</a>
  <a href="../gallery.html">Gallery</a>
</div></footer>'

mkdir -p docs/source
for name in "${demos[@]}"; do
	out="docs/source/$name.html"
	{
		printf '<!doctype html>\n<html lang="en">\n<head>\n'
		printf '<meta charset="utf-8">\n<meta name="viewport" content="width=device-width, initial-scale=1">\n'
		printf '<title>gophics — %s source</title>\n' "$name"
		printf '<link rel="stylesheet" href="../style.css">\n</head>\n<body>\n'
		printf '%s\n' "$nav"
		printf '<section><div class="wrap">\n'
		printf '  <div class="srchead"><h2>%s</h2><span class="sp"></span>\n' "$name"
		printf '    <div class="links"><a href="../demo.html?demo=%s">▶ run live</a>\n' "$name"
		printf '      <a href="https://github.com/doug/gophics/tree/main/examples/%s">on GitHub ↗</a></div>\n' "$name"
		printf '  </div>\n'
		# All Go source under the example, recursively — so multi-package examples
		# (hn's ui/, model, etc.) are meaningful. Skip tests and the non-Go host
		# templates (android/ios dirs hold Kotlin/Swift/gradle, no .go anyway).
		root_main="examples/$name/main.go"
		rest=$(find examples/"$name" -name '*.go' ! -name '*_test.go' \
			! -path '*/android/*' ! -path '*/ios/*' ! -path "$root_main" | sort)
		ordered=""
		[[ -f "$root_main" ]] && ordered="$root_main"
		ordered="$ordered $rest"
		multi=$(echo $ordered | wc -w)
		for f in $ordered; do
			[[ -f "$f" ]] || continue
			[[ $multi -gt 1 ]] && printf '  <p class="filelabel">%s</p>\n' "${f#examples/$name/}"
			go run ./docs/tools/highlight "$f"
		done
		printf '</div></section>\n'
		printf '%s\n' "$footer"
		printf '</body>\n</html>\n'
	} > "$out"
	echo "wrote $out"
done
