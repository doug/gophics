#!/usr/bin/env bash
# Build the live-demo examples to WebAssembly for the docs site (docs/demos/).
# Run locally to preview, or let the Pages workflow run it on deploy.
#
#   ./docs/build-demos.sh
#   go run docs/serve.go        # then open http://localhost:8099
#
# To feature another example: add its name here and a card in docs/gallery.html
# linking to demo.html?sketch=<name>. Only examples that compile to js/wasm and
# render via WebGPU belong here (desktop/mobile-only examples are omitted).
set -euo pipefail
cd "$(dirname "$0")/.."

# The featured set (kept lean — each Go/WASM binary is multiple MB even stripped;
# GitHub Pages serves them gzipped so a click costs ~1/3 of the on-disk size).
demos=(hn gallery canvas flowfield flocking particles match3 solitaire roguelike todo notes health 2048 sudoku epub)

mkdir -p docs/demos
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" docs/demos/wasm_exec.js
for d in "${demos[@]}"; do
	GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -ldflags="-s -w" -o "docs/demos/$d.wasm" "./examples/$d"
	echo "built docs/demos/$d.wasm ($(du -h "docs/demos/$d.wasm" | cut -f1))"
done
echo "done — ${#demos[@]} demo(s) in docs/demos/"
