#!/usr/bin/env bash
# test-js.sh — run the js/wasm tests under node.
#
# This lane exists because the js halves of the tree used to execute for the
# first time in a user's browser: fetch_js.go, folder_web.go and textinput_web.go
# shipped with zero executed tests, and verifying the folder capability meant
# hand-driving Chrome. Node supplies the same whatwg fetch(), AbortController
# and data: URLs the browser does, so everything short of a DOM runs for real.
#
# The package list is curated, not ./... — two kinds of package cannot run here
# and excluding them is correct rather than lazy:
#   - tests that need host fonts or spin httptest listeners (text, widget):
#     a wasm process cannot bind a socket, so those tests are native-only by
#     nature, not by neglect;
#   - shell/web itself: it needs a DOM, which is the browser-verification tier,
#     not this one.
#
# Usage: ./scripts/test-js.sh          (node must be on PATH; nvm counts)
#
# Under ci-local.sh the ubuntu jobs run in a golang container that has no node
# and skips the setup-node `uses:` step, so this job reports its clear "node
# not found" error there. Run it directly on the host instead — the nvm
# fallback below finds the usual local install. On GitHub, setup-node feeds it.
set -euo pipefail
cd "$(dirname "$0")/.."

if ! command -v node >/dev/null; then
	# nvm installs are the common local case and are not on cron/CI PATHs.
	latest="$(ls "$HOME/.nvm/versions/node" 2>/dev/null | sort -V | tail -1 || true)"
	if [ -n "$latest" ]; then
		export PATH="$HOME/.nvm/versions/node/$latest/bin:$PATH"
	fi
fi
command -v node >/dev/null || { echo "test-js: node not found (install node, or nvm install --lts)"; exit 1; }

pkgs=(
	./fetch/
	./geom/
	./intl/
	./anim/
	./input/
	./chart/
	./examples/todo/
	./examples/solitaire/klondike/
)

exec_wrapper="$(go env GOROOT)/lib/wasm/go_js_wasm_exec"
GOOS=js GOARCH=wasm go test -exec="$exec_wrapper" "${pkgs[@]}"
