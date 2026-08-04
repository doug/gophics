#!/usr/bin/env bash
# Generate a static, syntax-highlighted source page for each featured demo,
# into docs/source/<name>.html. Highlighting is baked at build time by the
# stdlib-only tool in docs/tools/highlight — pure HTML + CSS, zero client JS.
# Re-run when an example changes.
#
#   ./docs/build-source.sh
set -euo pipefail
cd "$(dirname "$0")/.."

demos=(hn gallery canvas match3 solitaire roguelike todo notes)

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
		multi=0; for f in examples/"$name"/*.go; do [[ "$f" == *_test.go ]] && continue; multi=$((multi+1)); done
		for f in examples/"$name"/*.go; do
			[[ "$f" == *_test.go ]] && continue
			[[ $multi -gt 1 ]] && printf '  <p class="filelabel">%s</p>\n' "$(basename "$f")"
			go run ./docs/tools/highlight "$f"
		done
		printf '</div></section>\n'
		printf '%s\n' "$footer"
		printf '</body>\n</html>\n'
	} > "$out"
	echo "wrote $out"
done
